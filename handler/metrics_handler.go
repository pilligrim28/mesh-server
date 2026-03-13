package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

type MetricsHandler struct {
	metricsRepo *repository.MetricsRepository
}

func NewMetricsHandler(metricsRepo *repository.MetricsRepository) *MetricsHandler {
	return &MetricsHandler{metricsRepo: metricsRepo}
}

func (h *MetricsHandler) GetByDeviceID(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	if deviceIDStr == "" {
		http.Error(w, "device_id parameter required", http.StatusBadRequest)
		return
	}

	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid device_id", http.StatusBadRequest)
		return
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	metrics, err := h.metricsRepo.GetByDeviceID(deviceID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *MetricsHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	if deviceIDStr == "" {
		// Get all latest metrics
		metrics, err := h.metricsRepo.GetAllLatest()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
		return
	}

	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid device_id", http.StatusBadRequest)
		return
	}

	metrics, err := h.metricsRepo.GetLatest(deviceID)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			http.Error(w, "no metrics found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *MetricsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var metrics models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if metrics.DeviceID == 0 {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	if metrics.Timestamp.IsZero() {
		metrics.Timestamp = time.Now()
	}

	if err := h.metricsRepo.Create(&metrics); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(metrics)
}
