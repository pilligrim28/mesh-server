package handler

import (
	"encoding/json"
	"net/http"

	"mesh-server/discovery"
)

// DiscoveryHandler обрабатывает HTTP запросы для discovery API
type DiscoveryHandler struct {
	service *discovery.DiscoveryService
}

func NewDiscoveryHandler(service *discovery.DiscoveryService) *DiscoveryHandler {
	return &DiscoveryHandler{service: service}
}

// GetAllDevices возвращает все обнаруженные устройства
// GET /api/discovery
func (h *DiscoveryHandler) GetAllDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices, err := h.service.GetDiscoveredDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// GetBluetoothDevices возвращает устройства, обнаруженные через Bluetooth
// GET /api/discovery/bluetooth
func (h *DiscoveryHandler) GetBluetoothDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices, err := h.service.GetBluetoothDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// GetWiFiDevices возвращает устройства, обнаруженные через WiFi
// GET /api/discovery/wifi
func (h *DiscoveryHandler) GetWiFiDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices, err := h.service.GetWiFiDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// GetMeshtasticDevices возвращает только Meshtastic устройства
// GET /api/discovery/meshtastic
func (h *DiscoveryHandler) GetMeshtasticDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices, err := h.service.GetMeshtasticDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// GetStatus возвращает статус сканирования
// GET /api/discovery/status
func (h *DiscoveryHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := h.service.GetScanStatus()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// StartScan запускает сканирование
// POST /api/discovery/scan
// Body: {"type": "bluetooth|wifi|all"}
func (h *DiscoveryHandler) StartScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Type string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		req.Type = "all"
	}

	if err := h.service.TriggerScan(req.Type); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scanning_started"})
}

// StartBluetoothScan запускает сканирование Bluetooth
// POST /api/discovery/bluetooth/start
func (h *DiscoveryHandler) StartBluetoothScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.service.StartBluetoothScan(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "bluetooth_scanning_started"})
}

// StopBluetoothScan останавливает сканирование Bluetooth
// POST /api/discovery/bluetooth/stop
func (h *DiscoveryHandler) StopBluetoothScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.service.StopBluetoothScan(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "bluetooth_scanning_stopped"})
}

// StartWiFiScan запускает сканирование WiFi
// POST /api/discovery/wifi/start
func (h *DiscoveryHandler) StartWiFiScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.service.StartWiFiScan(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "wifi_scanning_started"})
}

// StopWiFiScan останавливает сканирование WiFi
// POST /api/discovery/wifi/stop
func (h *DiscoveryHandler) StopWiFiScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.service.StopWiFiScan(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "wifi_scanning_stopped"})
}

// ClearDevices очищает список обнаруженных устройств
// DELETE /api/discovery/clear
func (h *DiscoveryHandler) ClearDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.service.ClearDevices(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "devices_cleared"})
}

// GetNetworkInfo возвращает информацию о сетевых интерфейсах
// GET /api/discovery/network
func (h *DiscoveryHandler) GetNetworkInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	networks, err := h.service.GetNetworkInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networks)
}
