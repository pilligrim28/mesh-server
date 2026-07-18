package handler

import (
	"mesh-server/client"
	"mesh-server/discovery"
	"mesh-server/ml"
	"mesh-server/repository"
)

// Handlers groups HTTP handlers created at application startup.
type Handlers struct {
	Device    *DeviceHandler
	Metrics   *MetricsHandler
	Alert     *AlertHandler
	Message   *MessageHandler
	Map       *MapHandler
	WS        *WebSocketHandler
	Discovery *DiscoveryHandler
	Route     *RouteHandler
	ML        *MLHandler
}

// NewHandlers wires HTTP handlers and shared dependencies.
func NewHandlers(
	deviceRepo *repository.DeviceRepository,
	metricsRepo *repository.MetricsRepository,
	alertRepo *repository.AlertRepository,
	messageRepo *repository.MessageRepository,
	discoveryService *discovery.DiscoveryService,
	esp32URL string,
	routeRepo *repository.RouteRepository,
	detector *ml.AnomalyDetector,
	predictor *ml.Predictor,
) *Handlers {
	wsHandler := NewWebSocketHandler()
	esp32Client := client.NewESP32Client(esp32URL)

	return &Handlers{
		Device:    NewDeviceHandler(deviceRepo),
		Metrics:   NewMetricsHandler(metricsRepo),
		Alert:     NewAlertHandler(alertRepo),
		Message:   NewMessageHandler(messageRepo, esp32Client, wsHandler),
		Map:       NewMapHandler(deviceRepo),
		WS:        wsHandler,
		Discovery: NewDiscoveryHandler(discoveryService),
		Route:     NewRouteHandler(routeRepo, deviceRepo),
		ML:        NewMLHandler(detector, predictor, routeRepo),
	}
}

// Close releases handler-owned resources.
func (h *Handlers) Close() {
	if h.WS != nil {
		h.WS.Close()
	}
}
