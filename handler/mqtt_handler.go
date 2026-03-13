package handler

import (
	"encoding/json"
	"net/http"

	"mesh-server/pkg/mqtt"
)

// MQTTHandler обрабатывает HTTP запросы для MQTT API
type MQTTHandler struct {
	mqttService mqtt.Service
}

// NewMQTTHandler создает новый handler для MQTT
func NewMQTTHandler(mqttService mqtt.Service) *MQTTHandler {
	return &MQTTHandler{mqttService: mqttService}
}

// GetStatus возвращает статус MQTT сервиса
// GET /api/mqtt/status
func (h *MQTTHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := h.mqttService.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Connect подключается к MQTT брокеру
// POST /api/mqtt/connect
func (h *MQTTHandler) Connect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.mqttService.Reconnect(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
}

// Disconnect отключается от MQTT брокера
// POST /api/mqtt/disconnect
func (h *MQTTHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.mqttService.Stop()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "disconnected"})
}

// SendMessage отправляет сообщение через MQTT
// POST /api/mqtt/message
func (h *MQTTHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FromNode string `json:"from_node"`
		ToNode   string `json:"to_node"`
		Text     string `json:"text"`
		Channel  int    `json:"channel"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.FromNode == "" || req.Text == "" {
		http.Error(w, "from_node and text are required", http.StatusBadRequest)
		return
	}

	if err := h.mqttService.SendMessage(req.FromNode, req.ToNode, req.Text, req.Channel); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}
