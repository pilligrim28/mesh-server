package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

type AlertHandler struct {
	alertRepo *repository.AlertRepository
}

func NewAlertHandler(alertRepo *repository.AlertRepository) *AlertHandler {
	return &AlertHandler{alertRepo: alertRepo}
}

func (h *AlertHandler) Create(w http.ResponseWriter, r *http.Request) {
	var alert models.Alert
	if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if alert.DeviceID == 0 {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	if alert.Type == "" {
		http.Error(w, "type is required", http.StatusBadRequest)
		return
	}

	if alert.Severity == "" {
		alert.Severity = "info"
	}

	if alert.CreatedAt.IsZero() {
		alert.CreatedAt = time.Now()
	}

	if err := h.alertRepo.Create(&alert); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(alert)
}

func (h *AlertHandler) GetByDeviceID(w http.ResponseWriter, r *http.Request) {
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

	alerts, err := h.alertRepo.GetByDeviceID(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

func (h *AlertHandler) GetUnread(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.alertRepo.GetUnread()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

func (h *AlertHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id parameter required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.alertRepo.MarkAsRead(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
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

	if err := h.alertRepo.MarkAllAsRead(deviceID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
