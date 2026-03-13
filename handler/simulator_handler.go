package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"mesh-server/simulator"
)

type SimulatorHandler struct {
	sim *simulator.Simulator
}

func NewSimulatorHandler(sim *simulator.Simulator) *SimulatorHandler {
	return &SimulatorHandler{sim: sim}
}

// Status возвращает статус симулятора
func (h *SimulatorHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"running": h.sim.IsRunning(),
	}
	
	if device := h.sim.GetDevice(); device != nil {
		status["device_id"] = device.ID
		status["node_id"] = device.NodeID
		status["name"] = device.Name
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Start запускает симулятор
func (h *SimulatorHandler) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	h.sim.Start(5 * time.Second)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// Stop останавливает симулятор
func (h *SimulatorHandler) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	h.sim.Stop()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// Event генерирует тестовое событие
func (h *SimulatorHandler) Event(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req struct {
		Type string `json:"type"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	
	if req.Type == "" {
		http.Error(w, "type is required", http.StatusBadRequest)
		return
	}
	
	h.sim.GenerateSingleEvent(req.Type)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "event generated"})
}
