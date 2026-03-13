package handler

import (
	"context"
	"encoding/json"
	"net/http"
)

// SerialAPI интерфейс для работы с COM-портом
type SerialAPI interface {
	ScanPorts() ([]string, error)
	Connect(ctx context.Context, port string) error
	Disconnect() error
	IsConnected() bool
	GetPort() string
	SendMessage(toNode, text string) error
}

// SerialHandler обрабатывает HTTP запросы для COM-порта (USB) API
type SerialHandler struct {
	serialAPI SerialAPI
}

// NewSerialHandler создает новый handler для COM-порта
func NewSerialHandler(serialAPI SerialAPI) *SerialHandler {
	return &SerialHandler{
		serialAPI: serialAPI,
	}
}

// ScanPorts сканирует доступные COM-порты
// GET /api/serial/scan
func (h *SerialHandler) ScanPorts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ports, err := h.serialAPI.ScanPorts()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"ports":   ports,
		"count":   len(ports),
	})
}

// Connect подключается к COM-порту
// POST /api/serial/connect
// Body: {"port": "COM3"}
func (h *SerialHandler) Connect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Port string `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Port == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "COM-порт не указан",
		})
		return
	}

	err := h.serialAPI.Connect(r.Context(), req.Port)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"port":    req.Port,
		"message": "Подключено к " + req.Port,
	})
}

// Disconnect отключается от COM-порта
// POST /api/serial/disconnect
func (h *SerialHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.serialAPI.Disconnect()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Отключено от COM-порта",
	})
}

// GetStatus возвращает статус подключения
// GET /api/serial/status
func (h *SerialHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := map[string]interface{}{
		"connected": h.serialAPI.IsConnected(),
		"port":      h.serialAPI.GetPort(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// SendMessage отправляет сообщение через COM-порт
// POST /api/serial/message
// Body: {"to_node": "!87654321", "text": "Привет!"}
func (h *SerialHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ToNode string `json:"to_node"`
		Text   string `json:"text"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.ToNode == "" || req.Text == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "to_node и text обязательны",
		})
		return
	}

	err := h.serialAPI.SendMessage(req.ToNode, req.Text)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"to_node":  req.ToNode,
		"text":     req.Text,
		"message":  "Сообщение отправлено",
	})
}
