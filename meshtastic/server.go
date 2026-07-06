package meshtastic

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"mesh-server/repository"
)

// MeshtasticServer эмулирует HTTP API устройства Meshtastic
// чтобы мобильное приложение Meshtastic могло видеть сервер как устройство в сети
type MeshtasticServer struct {
	deviceRepo  *repository.DeviceRepository
	messageRepo *repository.MessageRepository
	mux         *http.ServeMux
	mu          sync.RWMutex
	deviceID    uint32
	longName    string
	shortName   string
	region      string
	firmware    string
}

// NodeInfo информация об узле для /json/nodes
type NodeInfo struct {
	ID         string   `json:"id"`
	SNR        float32  `json:"snr"`
	ViaMQTT    bool     `json:"via_mqtt"`
	LastHeard  int64    `json:"last_heard"`
	Position   *PosInfo `json:"position"`
	LongName   string   `json:"long_name"`
	ShortName  string   `json:"short_name"`
	MacAddress string   `json:"mac_address"`
	HWModel    string   `json:"hw_model"`
}

// PosInfo позиция узла
type PosInfo struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  int     `json:"altitude"`
}

// NodesResponse ответ /json/nodes
type NodesResponse struct {
	Data   NodesData `json:"data"`
	Status string    `json:"status"`
}

type NodesData struct {
	Nodes []NodeInfo `json:"nodes"`
}

// DeviceResponse ответ /json/device
type DeviceResponse struct {
	Data   DeviceData `json:"data"`
	Status string     `json:"status"`
}

type DeviceData struct {
	Version    string `json:"version"`
	Firmware   string `json:"firmware"`
	Hardware   string `json:"hardware"`
	Region     string `json:"region"`
	HasWiFi    bool   `json:"has_wifi"`
	HasBluetooth bool `json:"has_bluetooth"`
	HasEthernet bool  `json:"has_ethernet"`
}

// MessageRequest запрос на отправку сообщения
type MessageRequest struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Channel   int    `json:"channel"`
	Payload   string `json:"payload"`
	PortNum   string `json:"portnum"`
}

// MessageResponse ответ на сообщение
type MessageResponse struct {
	Status string `json:"status"`
}

// NewMeshtasticServer создаёт новый сервер
func NewMeshtasticServer(
	deviceRepo *repository.DeviceRepository,
	messageRepo *repository.MessageRepository,
) *MeshtasticServer {
	s := &MeshtasticServer{
		deviceRepo:  deviceRepo,
		messageRepo: messageRepo,
		mux:         http.NewServeMux(),
		deviceID:    0xdeadbeef,
		longName:    "MeshServer Hub",
		shortName:   "MSH",
		region:      "RU",
		firmware:    "2.5.0",
	}
	s.registerRoutes()
	return s
}

// GetMux возвращает ServeMux для подключения к основному серверу
func (s *MeshtasticServer) GetMux() *http.ServeMux {
	return s.mux
}

// GetPort возвращает порт Meshtastic API
func (s *MeshtasticServer) GetPort() string {
	return "4403"
}

func (s *MeshtasticServer) registerRoutes() {
	// Добавляем middleware для заголовков Meshtastic
	wrap := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", "Meshtastic/2.5.0")
			w.Header().Set("X-Meshtastic-Version", s.firmware)
			w.Header().Set("WWW-Authenticate", "Meshtastic realm=\"Meshtastic\"")
			handler(w, r)
		}
	}

	// JSON API (используется приложением Meshtastic)
	s.mux.HandleFunc("/json/nodes", wrap(s.handleNodes))
	s.mux.HandleFunc("/json/device", wrap(s.handleDevice))
	s.mux.HandleFunc("/json/report", wrap(s.handleReport))
	s.mux.HandleFunc("/json/scanNetworks", wrap(s.handleScanNetworks))

	// API v1 (protobuf, минимальная совместимость)
	s.mux.HandleFunc("/api/v1/fromradio", wrap(s.handleFromRadio))
	s.mux.HandleFunc("/api/v1/toradio", wrap(s.handleToRadio))

	// Captive portal detection (iOS/Android)
	s.mux.HandleFunc("/hotspot-detect.html", wrap(s.handleHotspot))
	s.mux.HandleFunc("/generate_204", wrap(s.handleHotspot))

	// Health & admin
	s.mux.HandleFunc("/admin", wrap(s.handleAdmin))
	s.mux.HandleFunc("/restart", wrap(s.handleRestart))

	// Root — редирект
	s.mux.HandleFunc("/", wrap(s.handleRoot))
}

// handleNodes возвращает список узлов в сети
func (s *MeshtasticServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")

	// Получаем все устройства из БД
	devices, err := s.deviceRepo.GetAll()
	if err != nil {
		log.Printf("MeshtasticServer: failed to get devices: %v", err)
		http.Error(w, `{"status":"error"}`, http.StatusInternalServerError)
		return
	}

	nodes := make([]NodeInfo, 0, len(devices))
	for _, dev := range devices {
		nodeID := dev.NodeID
		if nodeID == "" {
			continue
		}

		// Конвертируем node_id в uint32 формат
		var numID uint32
		fmt.Sscanf(nodeID, "!%x", &numID)
		if numID == 0 {
			numID = uint32(dev.ID)
		}

		// Статус online/offline
		isOnline := time.Since(dev.LastSeen) < 5*time.Minute
		snr := float32(-50)
		if isOnline {
			snr = float32(-30 + (dev.ID % 20))
		}

		var pos *PosInfo
		if dev.Latitude != 0 || dev.Longitude != 0 {
			pos = &PosInfo{
				Latitude:  dev.Latitude,
				Longitude: dev.Longitude,
				Altitude:  int(dev.Altitude),
			}
		}

		// Краткое имя (первые 2 символа или из имени)
		shortName := dev.Name
		if len(shortName) > 2 {
			shortName = shortName[:2]
		}

		nodes = append(nodes, NodeInfo{
			ID:         fmt.Sprintf("!%08x", numID),
			SNR:        snr,
			ViaMQTT:    false,
			LastHeard:  dev.LastSeen.Unix(),
			Position:   pos,
			LongName:   dev.Name,
			ShortName:  shortName,
			MacAddress: fmt.Sprintf("AA:BB:CC:%02X:%02X:%02X", byte(numID), byte(numID>>8), byte(numID>>16)),
			HWModel:    "UNSET",
		})
	}

	resp := NodesResponse{
		Data:   NodesData{Nodes: nodes},
		Status: "ok",
	}

	json.NewEncoder(w).Encode(resp)
}

// handleDevice возвращает информацию об устройстве
func (s *MeshtasticServer) handleDevice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	resp := DeviceResponse{
		Data: DeviceData{
			Version:      s.firmware,
			Firmware:     "Meshtastic",
			Hardware:     "MeshServer Hub",
			Region:       s.region,
			HasWiFi:      true,
			HasBluetooth: false,
			HasEthernet:  true,
		},
		Status: "ok",
	}

	json.NewEncoder(w).Encode(resp)
}

// handleReport возвращает отчёт об устройстве
func (s *MeshtasticServer) handleReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	report := map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"wifi": map[string]interface{}{
				"rssi": -45,
				"ip":   r.Host,
			},
			"memory": map[string]interface{}{
				"heap_total": 250000,
				"heap_free":  180000,
			},
			"power": map[string]interface{}{
				"battery_percent": 100,
				"has_battery":     false,
				"has_usb":         true,
				"is_charging":     false,
			},
			"device": map[string]interface{}{
				"reboot_counter": 0,
			},
			"radio": map[string]interface{}{
				"frequency":  868.0,
				"lora_channel": 1,
			},
		},
	}

	json.NewEncoder(w).Encode(report)
}

// handleScanNetworks возвращает список WiFi сетей
func (s *MeshtasticServer) handleScanNetworks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	resp := map[string]interface{}{
		"status": "ok",
		"data":   []interface{}{},
	}

	json.NewEncoder(w).Encode(resp)
}

// handleFromRadio — минимальная совместимость с protobuf API
func (s *MeshtasticServer) handleFromRadio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("X-Protobuf-Schema", "https://raw.githubusercontent.com/meshtastic/protobufs/master/meshtastic/mesh.proto")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Возвращаем пустой protobuf (0 bytes) —表示 нет данных
	w.WriteHeader(http.StatusOK)
}

// handleToRadio — принимает protobuf команды
func (s *MeshtasticServer) handleToRadio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Читаем тело (protobuf)
	buf := make([]byte, 4096)
	n, _ := r.Body.Read(buf)

	log.Printf("MeshtasticServer: received %d bytes on toradio", n)

	// Эхо — возвращаем те же данные
	w.Write(buf[:n])
}

// handleHotspot — captive portal detection
func (s *MeshtasticServer) handleHotspot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Connection", "close")

	// Редирект на Meshtastic
	w.Header().Set("X-Meshtastic-Version", s.firmware)
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleAdmin — страница администратора
func (s *MeshtasticServer) handleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Meshtastic Hub</title></head>
<body>
<h1>Meshtastic Hub</h1>
<p>Firmware: %s</p>
<p>Device: %s</p>
<p><a href="/json/nodes">Nodes</a></p>
<p><a href="/json/report">Report</a></p>
</body>
</html>`, s.firmware, s.longName)))
}

// handleRestart — перезагрузка (заглушка)
func (s *MeshtasticServer) handleRestart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte("<h1>Meshtastic</h1><p>Restart not available on hub</p>"))
}

// handleRoot — корневая страница
func (s *MeshtasticServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("X-Protobuf-Schema", "https://raw.githubusercontent.com/meshtastic/protobufs/master/meshtastic/mesh.proto")

	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Meshtastic Hub</title></head>
<body>
<h1>Meshtastic Hub</h1>
<p>Node ID: !%08x</p>
<p>Long Name: %s</p>
<p>Short Name: %s</p>
<p>Region: %s</p>
<p>Firmware: %s</p>
<p><a href="/json/nodes">Nodes</a></p>
<p><a href="/admin">Admin</a></p>
</body>
</html>`, s.deviceID, s.longName, s.shortName, s.region, s.firmware)))
}

// GetNodeID возвращает ID устройства в формате Meshtastic
func (s *MeshtasticServer) GetNodeID() string {
	return fmt.Sprintf("!%08x", s.deviceID)
}

// GetDeviceInfo возвращает информацию об устройстве
func (s *MeshtasticServer) GetDeviceInfo() map[string]interface{} {
	return map[string]interface{}{
		"node_id":   s.GetNodeID(),
		"long_name": s.longName,
		"short_name": s.shortName,
		"region":    s.region,
		"firmware":  s.firmware,
	}
}
