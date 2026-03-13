package handler

import (
	"encoding/json"
	"net/http"

	"mesh-server/repository"
)

// MapPoint представляет точку на карте для устройства
type MapPoint struct {
	NodeID    string  `json:"node_id"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
	LastSeen  string  `json:"last_seen"`
}

type MapHandler struct {
	deviceRepo *repository.DeviceRepository
}

func NewMapHandler(deviceRepo *repository.DeviceRepository) *MapHandler {
	return &MapHandler{deviceRepo: deviceRepo}
}

// GetDevicesForMap возвращает все устройства с координатами для отображения на карте
func (h *MapHandler) GetDevicesForMap(w http.ResponseWriter, r *http.Request) {
	devices, err := h.deviceRepo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	points := make([]MapPoint, 0, len(devices))
	for _, device := range devices {
		if device.Latitude != 0 || device.Longitude != 0 {
			points = append(points, MapPoint{
				NodeID:    device.NodeID,
				Name:      device.Name,
				Latitude:  device.Latitude,
				Longitude: device.Longitude,
				Altitude:  device.Altitude,
				LastSeen:  device.LastSeen.Format("2006-01-02 15:04:05"),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}
