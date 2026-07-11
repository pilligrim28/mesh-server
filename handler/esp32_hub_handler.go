package handler

import (
	"encoding/json"
	"net/http"
)

// ESP32HubStatusProvider возвращает статус ESP32-хаба.
type ESP32HubStatusProvider interface {
	GetStatus() map[string]interface{}
}

// ESP32HubHandler HTTP API для ESP32-хаба.
type ESP32HubHandler struct {
	hubService ESP32HubStatusProvider
}

// NewESP32HubHandler создаёт handler ESP32-хаба.
func NewESP32HubHandler(hubService ESP32HubStatusProvider) *ESP32HubHandler {
	return &ESP32HubHandler{hubService: hubService}
}

// GetStatus возвращает статус ESP32-хаба.
// GET /api/esp32/hub/status
func (h *ESP32HubHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.hubService.GetStatus())
}
