package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mesh-server/ml"
	"mesh-server/models"
	"mesh-server/repository"
)

// RouteHandler HTTP-обработчик для маршрутов
type RouteHandler struct {
	routeRepo *repository.RouteRepository
	deviceRepo *repository.DeviceRepository
	simService interface{ SetRoute(routeName string) }
}

func NewRouteHandler(routeRepo *repository.RouteRepository, deviceRepo *repository.DeviceRepository) *RouteHandler {
	return &RouteHandler{
		routeRepo:  routeRepo,
		deviceRepo: deviceRepo,
	}
}

// GetAll возвращает список всех маршрутов
func (h *RouteHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	routes, err := h.routeRepo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if routes == nil {
		routes = []models.Route{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

// Create создаёт новый маршрут
func (h *RouteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string           `json:"name"`
		Description string           `json:"description"`
		DeviceID    int64            `json:"device_id"`
		Waypoints   []ml.Point2D     `json:"waypoints"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Waypoints) < 2 {
		http.Error(w, "need at least 2 waypoints", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = "Маршрут #" + strconv.FormatInt(time.Now().Unix(), 10)
	}

	route := &models.Route{
		Name:        req.Name,
		Description: req.Description,
		DeviceID:    req.DeviceID,
		Waypoints:   req.Waypoints,
	}
	if err := h.routeRepo.Create(route); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(route)
}

// Delete удаляет маршрут
func (h *RouteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/routes/")
	// Убираем остаток пути если есть
	if idx := strings.Index(idStr, "/"); idx >= 0 {
		idStr = idStr[:idx]
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid route id", http.StatusBadRequest)
		return
	}

	if err := h.routeRepo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPoints возвращает историю точек маршрута за период
func (h *RouteHandler) GetPoints(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "device_id required", http.StatusBadRequest)
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	points, err := h.routeRepo.GetPointsByPeriod(deviceID, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []models.RoutePoint{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id": deviceID,
		"from":      from.Format(time.RFC3339),
		"to":        to.Format(time.RFC3339),
		"points":    points,
		"count":     len(points),
		"total_distance_km": calculateTotalDistance(points),
	})
}

// RecordPoint записывает текущую точку устройства
func (h *RouteHandler) RecordPoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID int64   `json:"device_id"`
		Lat      float64 `json:"lat"`
		Lon      float64 `json:"lon"`
		RouteID  *int64  `json:"route_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := h.routeRepo.RecordPoint(req.DeviceID, req.RouteID, req.Lat, req.Lon); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func calculateTotalDistance(points []models.RoutePoint) float64 {
	if len(points) < 2 {
		return 0
	}
	var total float64
	for i := 1; i < len(points); i++ {
		dLat := (points[i].Latitude - points[i-1].Latitude) * 111320
		dLon := (points[i].Longitude - points[i-1].Longitude) * 111320 * 0.5
		total += dLat*dLat + dLon*dLon
	}
	return total / 1000.0
}

// MLHandler HTTP-обработчик для ML-функций
type MLHandler struct {
	detector  *ml.AnomalyDetector
	predictor *ml.Predictor
	routeRepo *repository.RouteRepository
}

func NewMLHandler(detector *ml.AnomalyDetector, predictor *ml.Predictor, routeRepo *repository.RouteRepository) *MLHandler {
	return &MLHandler{
		detector:  detector,
		predictor: predictor,
		routeRepo: routeRepo,
	}
}

// GetAnomalies возвращает список аномалий
func (h *MLHandler) GetAnomalies(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	var deviceID *int64
	if deviceIDStr != "" {
		if id, err := strconv.ParseInt(deviceIDStr, 10, 64); err == nil {
			deviceID = &id
		}
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	anomalies, err := h.routeRepo.GetAnomalies(deviceID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if anomalies == nil {
		anomalies = []models.Anomaly{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anomalies)
}

// PredictPosition предсказывает следующую позицию устройства
func (h *MLHandler) PredictPosition(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "device_id required", http.StatusBadRequest)
		return
	}

	points, err := h.routeRepo.GetRecentPoints(deviceID, 10)
	if err != nil || len(points) < 2 {
		http.Error(w, "not enough points for prediction", http.StatusBadRequest)
		return
	}

	mlPoints := make([]ml.Point2D, len(points))
	for i, p := range points {
		mlPoints[i] = ml.Point2D{Lat: p.Latitude, Lon: p.Longitude}
	}

	result := h.predictor.NextPosition(mlPoints)
	if result == nil {
		http.Error(w, "prediction failed", http.StatusInternalServerError)
		return
	}

	// Добавляем интерполированные точки для анимации на карте
	interpolated := h.predictor.InterpolatePoints(result.Current, result.Predicted, 20)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id":    deviceID,
		"current":      result.Current,
		"predicted":    result.Predicted,
		"confidence":   result.Confidence,
		"heading":      result.DirectionHeading,
		"smooth_path":  interpolated,
	})
}

// AnalyzeDevice анализирует показатели устройства
func (h *MLHandler) AnalyzeDevice(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "device_id required", http.StatusBadRequest)
		return
	}

	anomalies, err := h.routeRepo.GetAnomalies(&deviceID, 20)
	if err != nil {
		anomalies = []models.Anomaly{}
	}

	points, _ := h.routeRepo.GetRecentPoints(deviceID, 10)

	var currentPos *ml.Point2D
	if len(points) > 0 {
		last := points[len(points)-1]
		currentPos = &ml.Point2D{Lat: last.Latitude, Lon: last.Longitude}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id":       deviceID,
		"anomalies":       anomalies,
		"anomaly_count":   len(anomalies),
		"route_points":    len(points),
		"current_position": currentPos,
	})
}

// log — стubb для совместимости
func (h *RouteHandler) log(format string, args ...interface{}) {
	log.Printf(format, args...)
}
