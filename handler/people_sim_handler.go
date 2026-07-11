package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"mesh-server/simulator"
)

// PeopleSimAPI интерфейс симулятора передвижения людей.
type PeopleSimAPI interface {
	IsRunning() bool
	GetConfig() simulator.PeopleMovementConfig
	GetPeopleCount() int
	GetOutsideCount() int
	SetZone(centerLat, centerLon, radiusKm float64) error
	Start() error
	Stop()
}

// PeopleSimHandler HTTP API для симулятора людей.
type PeopleSimHandler struct {
	sim PeopleSimAPI
}

// NewPeopleSimHandler создаёт handler.
func NewPeopleSimHandler(sim PeopleSimAPI) *PeopleSimHandler {
	return &PeopleSimHandler{sim: sim}
}

func (h *PeopleSimHandler) zonePayload() map[string]interface{} {
	cfg := h.sim.GetConfig()
	return map[string]interface{}{
		"center": map[string]float64{
			"lat": cfg.CenterLat,
			"lon": cfg.CenterLon,
		},
		"radius_km":           cfg.RadiusKm,
		"radius_m":            cfg.RadiusKm * 1000,
		"alert_delay_minutes": cfg.OutsideAlertDelay.Minutes(),
		"outside_count":       h.sim.GetOutsideCount(),
		"people_count":        h.sim.GetPeopleCount(),
		"running":             h.sim.IsRunning(),
	}
}

// Status возвращает статус и параметры зоны.
// GET /api/people-sim/status
func (h *PeopleSimHandler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := h.sim.GetConfig()
	payload := h.zonePayload()
	payload["interval"] = cfg.Interval.Seconds()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

// Zone возвращает или задаёт геозону.
// GET /api/people-sim/zone
// POST /api/people-sim/zone  body: {"lat":59.93,"lon":30.33,"radius_km":2}
func (h *PeopleSimHandler) Zone(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(h.zonePayload())
	case http.MethodPost:
		var req struct {
			Lat      float64 `json:"lat"`
			Lon      float64 `json:"lon"`
			RadiusKm float64 `json:"radius_km"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.RadiusKm <= 0 {
			http.Error(w, "radius_km must be positive", http.StatusBadRequest)
			return
		}
		if err := h.sim.SetZone(req.Lat, req.Lon, req.RadiusKm); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Граница зоны обновлена",
			"zone":    h.zonePayload(),
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Start запускает симулятор.
// POST /api/people-sim/start
func (h *PeopleSimHandler) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.sim.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// Stop останавливает симулятор.
// POST /api/people-sim/stop
func (h *PeopleSimHandler) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.sim.Stop()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// DefaultPeopleSimInterval интервал по умолчанию.
const DefaultPeopleSimInterval = 5 * time.Second
