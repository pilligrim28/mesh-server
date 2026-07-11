package handler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"mesh-server/client"
	"mesh-server/discovery"
)

// HealbeRuntimeInfo описывает окружение для диагностики Healbe.
type HealbeRuntimeInfo struct {
	BridgeURL      string
	MeshSenderDesc string
}

// AutoConnect подключается к Healbe при старте сервера (из .env).
func (h *HealbeHandler) AutoConnect(ctx context.Context, mac, mode string, forward bool) error {
	mac = discovery.FormatBLEAddress(mac)
	if !isValidHealbeMAC(mac) {
		return fmt.Errorf("invalid HEALBE_MAC: %s", mac)
	}

	h.SetForwardEnabled(forward)
	result, err := h.connect(ctx, mac, normalizeHealbeMode(mode))
	if err != nil {
		return err
	}

	log.Printf("Healbe auto-connect: %s via %s (%s)", mac, result.Via, result.Message)
	return nil
}

// SetForwardEnabled включает пересылку метрик в Meshtastic.
func (h *HealbeHandler) SetForwardEnabled(enabled bool) {
	h.forwardEnabled = enabled
	if h.esp32Bridge != nil {
		h.esp32Bridge.SetForwardEnabled(enabled)
	}
}

// SetRuntimeInfo сохраняет параметры для диагностики.
func (h *HealbeHandler) SetRuntimeInfo(info HealbeRuntimeInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.healbeBridgeURL = info.BridgeURL
	h.meshSenderDesc = info.MeshSenderDesc
}

type healbeConnectResult struct {
	Via       string
	Message   string
	DeviceID  string
	Resolved  string
}

func (h *HealbeHandler) connect(ctx context.Context, mac, via string) (*healbeConnectResult, error) {
	via = normalizeHealbeMode(via)
	resolvedVia, viaLabel := h.resolveVia(ctx, via)

	h.mu.Lock()
	h.requestedMode = via
	h.activeVia = ""
	h.mu.Unlock()

	switch resolvedVia {
	case "demo":
		h.EnableDemo(mac, h.forwardEnabled)
		return &healbeConnectResult{
			Via:      "demo",
			Message:  "Демо-режим Healbe: данные генерируются автоматически",
			DeviceID: mac,
			Resolved: viaLabel,
		}, nil
	case "esp32":
		if h.esp32Bridge == nil {
			return nil, fmt.Errorf("ESP32 Healbe bridge is not configured")
		}
		if h.healbeClient != nil {
			h.healbeClient.Disconnect()
			h.healbeClient = nil
		}
		if err := h.esp32Bridge.Connect(ctx, mac); err != nil {
			return nil, err
		}
		h.esp32Bridge.SetForwardEnabled(h.forwardEnabled)
		h.mu.Lock()
		h.activeVia = "esp32"
		h.connectedMAC = mac
		h.mu.Unlock()

		msg := "Healbe подключён через ESP32. Ожидаем данные с часов."
		if !h.esp32Bridge.HasHTTPBridge() {
			msg = "Healbe зарегистрирован. Ожидаем данные от ESP32 bridge (ingest)."
		}
		return &healbeConnectResult{
			Via:      "esp32",
			Message:  msg,
			DeviceID: mac,
			Resolved: viaLabel,
		}, nil
	default:
		if h.esp32Bridge != nil && h.esp32Bridge.IsConnected() {
			h.esp32Bridge.Disconnect()
		}
		h.healbeClient = client.NewHealbeClient(mac, h.healbeRepo, h.deviceRepo)
		h.healbeClient.SetDataCallback(h.onHealbeData)
		if err := h.healbeClient.Connect(ctx); err != nil {
			h.healbeClient = nil
			return nil, err
		}
		h.mu.Lock()
		h.activeVia = "pc"
		h.connectedMAC = mac
		h.mu.Unlock()

		return &healbeConnectResult{
			Via:      "pc",
			Message:  "Подключено к Healbe GoBe через Bluetooth ПК",
			DeviceID: mac,
			Resolved: viaLabel,
		}, nil
	}
}

func (h *HealbeHandler) resolveVia(ctx context.Context, requested string) (string, string) {
	switch requested {
	case "demo":
		return "demo", "демо-симулятор"
	case "esp32":
		if h.esp32Bridge != nil && h.esp32Bridge.HasHTTPBridge() {
			return "esp32", "ESP32 HTTP bridge"
		}
		if h.esp32Bridge != nil {
			return "esp32", "ESP32 ingest"
		}
		return "pc", "ESP32 bridge недоступен → Bluetooth ПК"
	case "pc":
		return "pc", "Bluetooth ПК"
	default:
		if h.esp32Bridge != nil && h.esp32Bridge.IsHTTPBridgeReachable(ctx) {
			return "esp32", "авто: ESP32 HTTP bridge"
		}
		return "pc", "авто: Bluetooth ПК"
	}
}

func normalizeHealbeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "pc", "esp32", "auto", "demo":
		return mode
	default:
		return "auto"
	}
}

func (h *HealbeHandler) markHealbeDataReceived() {
	h.mu.Lock()
	h.lastDataAt = time.Now()
	h.mu.Unlock()
}

func (h *HealbeHandler) buildHealbeDiagnostics() map[string]interface{} {
	h.mu.RLock()
	requestedMode := h.requestedMode
	activeVia := h.activeVia
	connectedMAC := h.connectedMAC
	lastDataAt := h.lastDataAt
	bridgeURL := h.healbeBridgeURL
	meshSender := h.meshSenderDesc
	forwardEnabled := h.forwardEnabled
	h.mu.RUnlock()

	connected := false
	via := activeVia
	mac := connectedMAC

	if h.demoMode {
		connected = true
		via = "demo"
		mac = connectedMAC
	} else if h.esp32Bridge != nil && h.esp32Bridge.IsConnected() {
		connected = true
		via = "esp32"
		mac = h.esp32Bridge.GetMAC()
	} else if h.healbeClient != nil && h.healbeClient.IsConnected() {
		connected = true
		via = "pc"
		mac = h.healbeClient.GetDeviceAddress()
	}

	if requestedMode == "" {
		requestedMode = "auto"
	}
	if meshSender == "" {
		meshSender = "не настроен"
	}

	var lastDataAtStr interface{}
	dataStale := true
	transportReady := false
	reason := "not_connected"

	if !lastDataAt.IsZero() {
		lastDataAtStr = lastDataAt.Format(time.RFC3339)
		dataStale = time.Since(lastDataAt) > 60*time.Second
		transportReady = !dataStale
	}

	if connected {
		switch via {
		case "demo":
			reason = "demo_simulator"
			transportReady = !dataStale && !lastDataAt.IsZero()
			if !lastDataAt.IsZero() && !dataStale {
				reason = "receiving_data"
			} else if lastDataAt.IsZero() {
				reason = "waiting_for_data"
			}
		case "pc":
			reason = "pc_ble_active"
			if transportReady {
				reason = "receiving_data"
			} else if dataStale && !lastDataAt.IsZero() {
				reason = "data_stale"
			} else {
				reason = "waiting_for_data"
			}
		case "esp32":
			if h.esp32Bridge != nil && h.esp32Bridge.HasHTTPBridge() {
				reason = "esp32_http_bridge"
			} else {
				reason = "waiting_for_ingest"
			}
			if transportReady {
				reason = "receiving_data"
			}
		}
	}

	return map[string]interface{}{
		"connected":        connected,
		"forward_enabled":  forwardEnabled,
		"via":              via,
		"requested_mode":   requestedMode,
		"mac":              mac,
		"transport_ready":  transportReady,
		"data_stale":       connected && dataStale && !lastDataAt.IsZero(),
		"reason":           reason,
		"reason_text":      healbeReasonText(reason),
		"last_data_at":     lastDataAtStr,
		"bridge_url":       bridgeURL,
		"mesh_sender":      meshSender,
	}
}

func healbeReasonText(reason string) string {
	switch reason {
	case "not_connected":
		return "Не подключено к часам"
	case "demo_simulator":
		return "Демо-данные Healbe (симулятор)"
	case "waiting_for_ingest":
		return "Ожидание данных от ESP32 bridge (POST /api/healbe/ingest)"
	case "waiting_for_data":
		return "Подключено, ожидание первых данных с часов"
	case "esp32_http_bridge":
		return "ESP32 HTTP bridge активен, ожидание данных"
	case "pc_ble_active":
		return "Bluetooth ПК активен"
	case "receiving_data":
		return "Данные поступают"
	case "data_stale":
		return "Данные не обновлялись более 60 секунд"
	default:
		return reason
	}
}
