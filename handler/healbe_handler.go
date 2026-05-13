package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"mesh-server/client"
	"mesh-server/discovery"
	"mesh-server/models"
	"mesh-server/repository"
)

// HealbeHandler обрабатывает HTTP запросы для интеграции с часами Healbe
type HealbeHandler struct {
	healbeClient  *client.HealbeClient
	healbeRepo    *repository.HealbeRepository
	metricsRepo   *repository.MetricsRepository
	deviceRepo    *repository.DeviceRepository
	wsHandler     *WebSocketHandler
	bleScanner    *discovery.BLEScanner

	// Для передачи в Meshtastic
	meshtasticClient *client.ESP32Client
}

// NewHealbeHandler создает новый handler для Healbe
func NewHealbeHandler(
	healbeRepo *repository.HealbeRepository,
	metricsRepo *repository.MetricsRepository,
	deviceRepo *repository.DeviceRepository,
	wsHandler *WebSocketHandler,
	bleScanner *discovery.BLEScanner,
) *HealbeHandler {
	return &HealbeHandler{
		healbeRepo:  healbeRepo,
		metricsRepo: metricsRepo,
		deviceRepo:  deviceRepo,
		wsHandler:   wsHandler,
		bleScanner:  bleScanner,
	}
}

// SetMeshtasticClient устанавливает клиент для отправки в Meshtastic
func (h *HealbeHandler) SetMeshtasticClient(client *client.ESP32Client) {
	h.meshtasticClient = client
}

// ScanHealbeDevices сканирует устройства Healbe по Bluetooth
// GET /api/healbe/scan
func (h *HealbeHandler) ScanHealbeDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Выполняем реальное BLE сканирование
	var devices []map[string]interface{}

	if h.bleScanner != nil {
		// Сканируем BLE устройства
		bleDevices := h.bleScanner.GetAllBLEDevices()

		// Фильтруем только устройства Healbe
		for _, device := range bleDevices {
			if isHealbeDevice(device.Name) {
				model := detectHealbeModel(device.Name)
				devices = append(devices, map[string]interface{}{
					"address": device.Address,
					"name":    device.Name,
					"rssi":    device.RSSI,
					"model":   model,
				})
			}
		}
	}

	// Если устройства не найдены, возвращаем информативное сообщение
	message := fmt.Sprintf("Найдено устройств Healbe: %d", len(devices))
	if len(devices) == 0 {
		message = "Устройства Healbe не найдены. Убедитесь, что часы включены и находятся в режиме сопряжения."
	}

	response := map[string]interface{}{
		"devices": devices,
		"count":   len(devices),
		"message": message,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// isHealbeDevice проверяет является ли устройство Healbe
func isHealbeDevice(name string) bool {
	lowerName := strings.ToLower(name)
	return strings.Contains(lowerName, "healbe") ||
		strings.Contains(lowerName, "gobe") ||
		strings.Contains(lowerName, "gobe3") ||
		strings.Contains(lowerName, "gobe u")
}

// detectHealbeModel определяет модель устройства Healbe
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

// ConnectToHealbe подключается к часам Healbe
// POST /api/healbe/connect
// Body: {"mac": "AA:BB:CC:DD:EE:FF"}
func (h *HealbeHandler) ConnectToHealbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		MAC string `json:"mac"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.MAC == "" {
		http.Error(w, "MAC address required", http.StatusBadRequest)
		return
	}

	// Создаем клиент Healbe
	h.healbeClient = client.NewHealbeClient(req.MAC, h.healbeRepo, h.deviceRepo)

	// Устанавливаем callback для получения данных
	h.healbeClient.SetDataCallback(h.onHealbeData)

	// Подключаемся
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.healbeClient.Connect(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status":    "connected",
		"mac":       req.MAC,
		"message":   "Подключено к Healbe GoBe",
		"device_id": req.MAC,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DisconnectHealbe отключается от часов
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

	response := map[string]interface{}{
		"status":  "disconnected",
		"message": "Отключено от Healbe",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetHealbeStatus возвращает статус подключения
// GET /api/healbe/status
func (h *HealbeHandler) GetHealbeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := map[string]interface{}{
		"connected": h.healbeClient != nil && h.healbeClient.IsConnected(),
	}

	if h.healbeClient != nil {
		status["mac"] = h.healbeClient.GetDeviceAddress()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// GetHealbeData возвращает последние данные с часов
// GET /api/healbe/data
func (h *HealbeHandler) GetHealbeData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем последние метрики из базы
	metrics, err := h.metricsRepo.GetLatest(10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// onHealbeData callback при получении данных с часов
func (h *HealbeHandler) onHealbeData(data *client.HealbeData) {
	log.Printf("HealbeHandler: received data - HR: %d, Stress: %d", data.HeartRate, data.StressLevel)

	// Отправляем через WebSocket
	if h.wsHandler != nil {
		h.wsHandler.Broadcast(map[string]interface{}{
			"type":    "healbe_data",
			"payload": data,
		})
	}

	// Отправляем в Meshtastic сеть через ESP32
	if h.meshtasticClient != nil {
		h.sendToMeshtastic(data)
	}
}

// sendToMeshtastic отправляет данные в сеть Meshtastic
func (h *HealbeHandler) sendToMeshtastic(data *client.HealbeData) {
	// Формируем сообщение для Meshtastic
	stressText := client.ParseStressLevel(data.StressLevel)

	msg := &models.Message{
		FromNode:  "!HEALBE" + data.DeviceID[len(data.DeviceID)-4:],
		ToNode:    "^all",
		Text:      fmt.Sprintf("HR:%d Stress:%s", data.HeartRate, stressText),
		Direction: "outbound",
		SentAt:    time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := h.meshtasticClient.SendMessage(ctx, msg)
	if err != nil {
		log.Printf("HealbeHandler: failed to send to Meshtastic: %v", err)
		return
	}

	log.Printf("HealbeHandler: sent to Meshtastic - %s", msg.Text)
}

// ForwardToMeshtastic включает/выключает пересылку в Meshtastic
// POST /api/healbe/forward
// Body: {"enabled": true}
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

	// Логика включения/выключения пересылки

	response := map[string]interface{}{
		"forward_enabled": req.Enabled,
		"message":         "Пересылка в Meshtastic: " + string(rune('0'+map[bool]int{false: 0, true: 1}[req.Enabled])),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}