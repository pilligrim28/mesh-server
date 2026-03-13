package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"mesh-server/client"
	"mesh-server/discovery"
	"mesh-server/models"
	"mesh-server/repository"
)

// ESP32Handler обрабатывает HTTP запросы для ESP32 API
type ESP32Handler struct {
	bleScanner    *discovery.BLEScanner
	esp32Client   *client.ESP32BluetoothClient
	messageRepo   *repository.MessageRepository
	wsHandler     *WebSocketHandler
	currentESP32  string
}

// NewESP32Handler создает новый handler для ESP32
func NewESP32Handler(
	bleScanner *discovery.BLEScanner,
	esp32Client *client.ESP32BluetoothClient,
	messageRepo *repository.MessageRepository,
	wsHandler *WebSocketHandler,
) *ESP32Handler {
	return &ESP32Handler{
		bleScanner:   bleScanner,
		esp32Client:  esp32Client,
		messageRepo:  messageRepo,
		wsHandler:    wsHandler,
	}
}

// ScanForESP32 сканирует ESP32 устройства
// GET /api/esp32/scan
func (h *ESP32Handler) ScanForESP32(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Сканируем сеть
	esp32Devices, err := client.ScanForESP32(ctx)
	if err != nil {
		// Возвращаем JSON с ошибкой вместо текста
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"wifi_devices": []string{},
			"ble_devices":  []string{},
			"count":        0,
			"error":        err.Error(),
		})
		return
	}

	// Получаем все BLE устройства
	bleDevices := h.bleScanner.GetAllBLEDevices()
	
	// Преобразуем BLE устройства в простой формат
	bleDeviceList := make([]map[string]interface{}, 0)
	var esp32MAC string
	
	for _, device := range bleDevices {
		deviceMap := map[string]interface{}{
			"address": device.Address,
			"name":    device.Name,
			"rssi":    device.RSSI,
			"isESP32": device.IsESP32,
		}
		bleDeviceList = append(bleDeviceList, deviceMap)
		
		if device.IsESP32 && esp32MAC == "" {
			esp32MAC = device.Address
		}
	}

	// Формируем подробный ответ
	response := map[string]interface{}{
		"wifi_devices": esp32Devices,
		"ble_devices":  bleDeviceList,
		"count":        len(esp32Devices),
		"message":      fmt.Sprintf("Найдено WiFi устройств: %d, Bluetooth: %d", len(esp32Devices), len(bleDeviceList)),
	}

	// Если найдено устройств, предлагаем подключиться
	if len(esp32Devices) > 0 {
		response["auto_connect"] = esp32Devices[0]
		response["message"] = fmt.Sprintf("Meshtastic найден! Рекомендуется подключиться к %s", esp32Devices[0])
	} else if esp32MAC != "" {
		response["auto_connect"] = esp32MAC
		response["message"] = fmt.Sprintf("Meshtastic найден по Bluetooth! MAC: %s", esp32MAC)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ConnectToESP32 подключается к ESP32
// POST /api/esp32/connect
// Body: {"ip": "192.168.4.1"} или {"mac": "AA:BB:CC:DD:EE:FF"}
func (h *ESP32Handler) ConnectToESP32(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP  string `json:"ip"`
		MAC string `json:"mac"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.IP != "" {
		h.currentESP32 = req.IP
		h.esp32Client.SetBaseURL("http://" + req.IP)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "connected",
		"ip":        h.currentESP32,
		"message":   "Connected to ESP32",
	})
}

// Disconnect отключается от ESP32
// POST /api/esp32/disconnect
func (h *ESP32Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.currentESP32 = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "disconnected",
		"message": "Disconnected from ESP32",
	})
}

// GetStatus возвращает статус подключения к ESP32
// GET /api/esp32/status
func (h *ESP32Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := map[string]interface{}{
		"connected": h.currentESP32 != "",
		"ip":        h.currentESP32,
	}

	// Проверяем доступность
	if h.currentESP32 != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		_, err := h.esp32Client.GetStatus(ctx)
		status["reachable"] = err == nil
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// ReceiveMessage принимает входящее сообщение от ESP32
// POST /api/esp32/message
func (h *ESP32Handler) ReceiveMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg models.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Устанавливаем направление
	msg.Direction = "inbound"
	msg.SentAt = time.Now()

	// Находим или создаем устройство
	device, err := h.messageRepo.GetDeviceByNodeID(msg.FromNode)
	if err != nil {
		// Создаем новое устройство
		device = &models.Device{
			NodeID:   msg.FromNode,
			Name:     "ESP32 Device",
			LastSeen: time.Now(),
		}
	}

	msg.DeviceID = device.ID

	// Сохраняем сообщение
	if err := h.messageRepo.Create(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Отправляем уведомление через WebSocket
	if h.wsHandler != nil {
		h.wsHandler.Broadcast(map[string]interface{}{
			"type":    "new_message",
			"message": msg,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

// GetBluetoothDevices возвращает BLE устройства
// GET /api/esp32/bluetooth
func (h *ESP32Handler) GetBluetoothDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем все BLE устройства
	bleDevices := h.bleScanner.GetAllBLEDevices()

	// Формируем ответ
	response := map[string]interface{}{
		"devices": bleDevices,
		"count":   len(bleDevices),
	}

	// Ищем ESP32
	for _, device := range bleDevices {
		if device.IsESP32 {
			response["esp32_found"] = true
			response["esp32_mac"] = device.Address
			response["esp32_name"] = device.Name
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
