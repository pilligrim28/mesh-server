package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"mesh-server/simulator"
)

type SimulatorHandler struct {
	simulators []*simulator.Simulator
}

func NewSimulatorHandler(sims []*simulator.Simulator) *SimulatorHandler {
	return &SimulatorHandler{simulators: sims}
}

// getSim находит симулятор по ID из query ?id=N
func (h *SimulatorHandler) getSim(r *http.Request) *simulator.Simulator {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		if len(h.simulators) > 0 {
			return h.simulators[0]
		}
		return nil
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id < 0 || id >= len(h.simulators) {
		return nil
	}
	return h.simulators[id]
}

// Status возвращает статус всех симуляторов
func (h *SimulatorHandler) Status(w http.ResponseWriter, r *http.Request) {
	sim := h.getSim(r)
	if sim == nil {
		http.Error(w, "simulator not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sim.GetStatus())
}

// StatusAll возвращает статус всех симуляторов
func (h *SimulatorHandler) StatusAll(w http.ResponseWriter, r *http.Request) {
	statuses := make([]map[string]interface{}, len(h.simulators))
	for i, sim := range h.simulators {
		statuses[i] = sim.GetStatus()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statuses)
}

// Start запускает симулятор
func (h *SimulatorHandler) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sim := h.getSim(r)
	if sim == nil {
		http.Error(w, "simulator not found", http.StatusNotFound)
		return
	}

	sim.Start(5 * time.Second)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// StartAll запускает все симуляторы
func (h *SimulatorHandler) StartAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	for i, sim := range h.simulators {
		interval := time.Duration(4+i) * time.Second // 4-11 секунд между симуляторами
		sim.Start(interval)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "all started",
		"count":   len(h.simulators),
	})
}

// Stop останавливает симулятор
func (h *SimulatorHandler) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sim := h.getSim(r)
	if sim == nil {
		http.Error(w, "simulator not found", http.StatusNotFound)
		return
	}

	sim.Stop()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// StopAll останавливает все симуляторы
func (h *SimulatorHandler) StopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	for _, sim := range h.simulators {
		sim.Stop()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "all stopped"})
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

	sim := h.getSim(r)
	if sim == nil {
		http.Error(w, "simulator not found", http.StatusNotFound)
		return
	}

	sim.GenerateSingleEvent(req.Type)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "event generated"})
}

// Route устанавливает маршрут
func (h *SimulatorHandler) Route(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "route name is required", http.StatusBadRequest)
		return
	}

	sim := h.getSim(r)
	if sim == nil {
		http.Error(w, "simulator not found", http.StatusNotFound)
		return
	}

	sim.SetRoute(req.Name)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "route set",
		"route":  req.Name,
	})
}

// SetSpeed устанавливает скорость
func (h *SimulatorHandler) SetSpeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Speed float64 `json:"speed"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Speed < 0.1 || req.Speed > 5.0 {
		http.Error(w, "speed must be between 0.1 and 5.0", http.StatusBadRequest)
		return
	}

	sim := h.getSim(r)
	if sim == nil {
		http.Error(w, "simulator not found", http.StatusNotFound)
		return
	}

	sim.SetRoute("Случайный") // Устанавливаем маршрут для скорости

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "speed set",
		"speed":  req.Speed,
	})
}

// Locations возвращает список доступных точек
func (h *SimulatorHandler) Locations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(simulator.GetAllLocations())
}

// Routes возвращает список доступных маршрутов
func (h *SimulatorHandler) Routes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(simulator.GetAllRoutes())
}

// RouteHistory возвращает историю маршрута
func (h *SimulatorHandler) RouteHistory(w http.ResponseWriter, r *http.Request) {
	sim := h.getSim(r)
	if sim == nil {
		http.Error(w, "simulator not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sim.GetRouteHistory())
}

// RouteHistoryAll возвращает историю всех симуляторов
func (h *SimulatorHandler) RouteHistoryAll(w http.ResponseWriter, r *http.Request) {
	type simHistory struct {
		ID      int                     `json:"id"`
		Name    string                  `json:"name"`
		Color   string                  `json:"color"`
		History []simulator.RoutePoint  `json:"history"`
	}

	result := make([]simHistory, len(h.simulators))
	for i, sim := range h.simulators {
		result[i] = simHistory{
			ID:      sim.ID,
			Name:    sim.Name(),
			Color:   sim.Color(),
			History: sim.GetRouteHistory(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ClearHistory очищает историю
func (h *SimulatorHandler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sim := h.getSim(r)
	if sim != nil {
		sim.ClearRouteHistory()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "history cleared"})
}

// ClearHistoryAll очищает историю всех симуляторов
func (h *SimulatorHandler) ClearHistoryAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	for _, sim := range h.simulators {
		sim.ClearRouteHistory()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "all history cleared"})
}
