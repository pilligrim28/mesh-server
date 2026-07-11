package handler

import (
	"encoding/json"
	"net/http"
)

// SystemStatusProvider источник системного статуса для демо.
type SystemStatusProvider interface {
	IsConnected() bool
	GetPort() string
}

// SystemHandler отдаёт сводку для демонстрации заказчику.
type SystemHandler struct {
	demoMode      bool
	serverPort    string
	serialService SystemStatusProvider
}

// NewSystemHandler создаёт обработчик системного статуса.
func NewSystemHandler(demoMode bool, serverPort string, serialService SystemStatusProvider) *SystemHandler {
	return &SystemHandler{
		demoMode:      demoMode,
		serverPort:    serverPort,
		serialService: serialService,
	}
}

// Status возвращает сводку системы.
// GET /api/system/status
func (h *SystemHandler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serialConnected := false
	serialPort := ""
	if h.serialService != nil {
		serialConnected = h.serialService.IsConnected()
		serialPort = h.serialService.GetPort()
	}

	meshHub := "не подключён"
	if serialConnected {
		meshHub = "usb:" + serialPort
	}

	status := map[string]interface{}{
		"demo_mode":        h.demoMode,
		"server_port":      h.serverPort,
		"serial_connected": serialConnected,
		"serial_port":      serialPort,
		"mesh_hub":         meshHub,
		"healbe_source":    "live",
		"demo_message":     "",
	}

	if h.demoMode {
		status["healbe_source"] = "demo"
		status["demo_message"] = "Демо-режим: ESP32 по USB + симулированные данные Healbe"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
