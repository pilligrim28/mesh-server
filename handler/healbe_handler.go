package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"mesh-server/client"
	"mesh-server/discovery"
	"mesh-server/models"
	"mesh-server/repository"
)

// HealbeHandler обрабатывает HTTP запросы для интеграции с часами Healbe.
type HealbeHandler struct {
	healbeClient  *client.HealbeClient
	healbeRepo    *repository.HealbeRepository
	metricsRepo   *repository.MetricsRepository
	deviceRepo    *repository.DeviceRepository
	wsHandler     *WebSocketHandler
	bleScanner    *discovery.BLEScanner
	esp32Bridge   *HealbeESP32Bridge

	meshtasticClient *client.ESP32Client
	meshSender       MeshSender
	forwardEnabled   bool
	useESP32Bridge   bool
	demoMode         bool

	mu              sync.RWMutex
	requestedMode  string
	activeVia      string
	connectedMAC   string
	lastDataAt     time.Time
	healbeBridgeURL string
	meshSenderDesc  string
}

// NewHealbeHandler создает новый handler для Healbe.
func NewHealbeHandler(
	healbeRepo *repository.HealbeRepository,
	metricsRepo *repository.MetricsRepository,
	deviceRepo *repository.DeviceRepository,
	wsHandler *WebSocketHandler,
	bleScanner *discovery.BLEScanner,
) *HealbeHandler {
	h := &HealbeHandler{
		healbeRepo:  healbeRepo,
		metricsRepo: metricsRepo,
		deviceRepo:  deviceRepo,
		wsHandler:   wsHandler,
		bleScanner:  bleScanner,
	}
	return h
}

// InitESP32Bridge инициализирует мост Healbe через ESP32.
func (h *HealbeHandler) InitESP32Bridge(esp32URL string) {
	h.esp32Bridge = NewHealbeESP32Bridge(esp32URL, h.healbeRepo, h.deviceRepo, h.onHealbeData)
}

// SetUseESP32Bridge включает режим получения данных через ESP32 по умолчанию.
func (h *HealbeHandler) SetUseESP32Bridge(enabled bool) {
	h.useESP32Bridge = enabled
}

// SetMeshtasticClient устанавливает HTTP-клиент ESP32 для пересылки в mesh.
func (h *HealbeHandler) SetMeshtasticClient(client *client.ESP32Client) {
	h.meshtasticClient = client
}

// SetMeshSender устанавливает отправителя в mesh (USB hub).
func (h *HealbeHandler) SetMeshSender(sender MeshSender) {
	h.meshSender = sender
}

// EnableDemo включает демонстрационный режим Healbe без реального BLE.
func (h *HealbeHandler) EnableDemo(mac string, forward bool) {
	h.mu.Lock()
	h.demoMode = true
	h.requestedMode = "demo"
	h.activeVia = "demo"
	h.connectedMAC = discovery.FormatBLEAddress(mac)
	h.mu.Unlock()
	h.SetForwardEnabled(forward)
}

// PublishDemoData публикует демо-данные Healbe в UI и mesh.
func (h *HealbeHandler) PublishDemoData(data *client.HealbeData) {
	h.markHealbeDataReceived()
	if h.esp32Bridge != nil {
		_ = h.esp32Bridge.Ingest(*data)
		return
	}
	h.onHealbeData(data)
}

// IsDemoMode сообщает, активен ли демо-режим Healbe.
func (h *HealbeHandler) IsDemoMode() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.demoMode
}
// ScanHealbeDevices сканирует устройства Healbe по Bluetooth.
// GET /api/healbe/scan?mac=AA:BB:CC:DD:EE:FF
func (h *HealbeHandler) ScanHealbeDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestedMAC := discovery.FormatBLEAddress(r.URL.Query().Get("mac"))
	var devices []map[string]interface{}
	scanVia := "auto"

	if h.bleScanner != nil {
		bleDevices := h.bleScanner.ScanBLEDevices(10000)
		seen := make(map[string]bool)

		for _, device := range bleDevices {
			addr := discovery.FormatBLEAddress(device.Address)
			name := device.Name
			if name == "" {
				name = "BLE-" + addr
			}

			isHealbe := isHealbeDevice(name, addr)
			matchesRequested := requestedMAC != "" && addr == requestedMAC

			if !isHealbe && !matchesRequested {
				continue
			}
			if seen[addr] {
				continue
			}
			seen[addr] = true

			devices = append(devices, map[string]interface{}{
				"address": addr,
				"name":    name,
				"rssi":    device.RSSI,
				"model":   detectHealbeModel(name),
			})
		}
	}

	message := fmt.Sprintf("Найдено устройств Healbe: %d", len(devices))
	manualConnectOK := isValidHealbeMAC(requestedMAC)

	if len(devices) == 0 {
		message = "Устройства Healbe не найдены на ПК. В режиме «Авто» подключение пойдёт через Bluetooth ПК, если ESP32 bridge недоступен."
		if requestedMAC != "" && !manualConnectOK {
			message = "Некорректный MAC адрес. Формат: AA:BB:CC:DD:EE:FF"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices":           devices,
		"count":             len(devices),
		"message":           message,
		"scan_via":          scanVia,
		"manual_connect_ok": manualConnectOK || len(devices) > 0,
	})
}

func isHealbeDevice(name, address string) bool {
	lowerName := strings.ToLower(name)
	if strings.Contains(lowerName, "healbe") || strings.Contains(lowerName, "gobe") {
		return true
	}
	return isHealbeAddress(address)
}

func isHealbeAddress(address string) bool {
	addr := discovery.FormatBLEAddress(address)
	// Известные OUI Healbe GoBe (в т.ч. 8B:20:91).
	prefixes := []string{"8B:20:91", "A4:C1:38", "00:1A:7D", "C4:4F:33"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(addr, prefix) {
			return true
		}
	}
	return false
}

func isValidHealbeMAC(mac string) bool {
	mac = discovery.FormatBLEAddress(mac)
	if len(mac) != 17 {
		return false
	}
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	for _, p := range parts {
		if len(p) != 2 {
			return false
		}
		for _, c := range p {
			if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}

func detectHealbeModel(name string) string {
	lowerName := strings.ToLower(name)
	if strings.Contains(lowerName, "gobe3") || strings.Contains(lowerName, "gobe 3") {
		return "GoBe3"
	}
	if strings.Contains(lowerName, "gobe u") {
		return "GoBe U"
	}
	if strings.Contains(lowerName, "gobe2") || strings.Contains(lowerName, "gobe 2") {
		return "GoBe2"
	}
	return "GoBe"
}

// ConnectToHealbe подключается к часам Healbe.
// POST /api/healbe/connect
// Body: {"mac": "AA:BB:CC:DD:EE:FF", "via": "esp32"}
func (h *HealbeHandler) ConnectToHealbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		MAC string `json:"mac"`
		Via string `json:"via"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.MAC == "" {
		http.Error(w, "MAC address required", http.StatusBadRequest)
		return
	}

	via := normalizeHealbeMode(req.Via)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := h.connect(ctx, discovery.FormatBLEAddress(req.MAC), via)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "connected",
		"mac":       req.MAC,
		"via":       result.Via,
		"resolved":  result.Resolved,
		"message":   result.Message,
		"device_id": result.DeviceID,
	})
}

// DisconnectHealbe отключается от часов.
// POST /api/healbe/disconnect
func (h *HealbeHandler) DisconnectHealbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.healbeClient != nil {
		h.healbeClient.Disconnect()
		h.healbeClient = nil
	}
	if h.esp32Bridge != nil && h.esp32Bridge.IsConnected() {
		h.esp32Bridge.Disconnect()
	}

	h.mu.Lock()
	h.activeVia = ""
	h.connectedMAC = ""
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "disconnected",
		"message": "Отключено от Healbe",
	})
}

// GetHealbeStatus возвращает статус подключения.
// GET /api/healbe/status
func (h *HealbeHandler) GetHealbeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := h.buildHealbeDiagnostics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// GetHealbeData возвращает последние данные с часов.
// GET /api/healbe/data
func (h *HealbeHandler) GetHealbeData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics, err := h.healbeRepo.GetLatestForAll(10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// IngestHealbeData принимает данные от ESP32.
// POST /api/healbe/ingest
func (h *HealbeHandler) IngestHealbeData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data client.HealbeData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if data.DeviceID == "" {
		data.DeviceID = r.URL.Query().Get("mac")
	}
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	if h.esp32Bridge != nil {
		if err := h.esp32Bridge.Ingest(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		h.onHealbeData(&data)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// GetESP32HealbeConfig возвращает конфиг для ESP32-прошивки.
// GET /api/healbe/esp32/config
func (h *HealbeHandler) GetESP32HealbeConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := map[string]interface{}{
		"enabled": false,
		"mac":     "",
	}
	if h.esp32Bridge != nil {
		config = h.esp32Bridge.GetConfig()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func (h *HealbeHandler) onHealbeData(data *client.HealbeData) {
	h.markHealbeDataReceived()
	log.Printf("HealbeHandler: received data - HR: %d, Stress: %d", data.HeartRate, data.StressLevel)

	if h.wsHandler != nil {
		h.wsHandler.Broadcast(map[string]interface{}{
			"type":    "healbe_data",
			"payload": data,
		})
	}

	if h.forwardEnabled {
		h.sendToMeshtastic(data)
	}
}

func (h *HealbeHandler) sendToMeshtastic(data *client.HealbeData) {
	stressText := client.ParseStressLevel(data.StressLevel)
	text := fmt.Sprintf("HR:%d Stress:%s", data.HeartRate, stressText)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if h.meshSender != nil {
		if err := h.meshSender.SendMessage(ctx, "^all", text); err != nil {
			log.Printf("HealbeHandler: failed to send to mesh via ESP32 hub: %v", err)
		} else {
			log.Printf("HealbeHandler: sent to mesh via hub - %s", text)
		}
		return
	}

	if h.meshtasticClient == nil {
		return
	}

	suffix := data.DeviceID
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}

	msg := &models.Message{
		FromNode:  "!HEALBE" + suffix,
		ToNode:    "^all",
		Text:      text,
		Direction: "outbound",
		SentAt:    time.Now(),
	}

	_, err := h.meshtasticClient.SendMessage(ctx, msg)
	if err != nil {
		log.Printf("HealbeHandler: failed to send to Meshtastic: %v", err)
		return
	}

	log.Printf("HealbeHandler: sent to Meshtastic - %s", msg.Text)
}

// ForwardToMeshtastic включает/выключает пересылку в Meshtastic.
// POST /api/healbe/forward
func (h *HealbeHandler) ForwardToMeshtastic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.SetForwardEnabled(req.Enabled)

	state := "выключена"
	if req.Enabled {
		state = "включена"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"forward_enabled": req.Enabled,
		"message":         "Пересылка в Meshtastic " + state,
	})
}
