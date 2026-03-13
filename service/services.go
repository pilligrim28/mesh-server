package service

import (
	"mesh-server/client"
	"mesh-server/discovery"
	"mesh-server/handler"
	"mesh-server/pkg/mqtt"
	"mesh-server/repository"
)

type Services struct {
	DeviceHandler    *handler.DeviceHandler
	MetricsHandler   *handler.MetricsHandler
	AlertHandler     *handler.AlertHandler
	MessageHandler   *handler.MessageHandler
	MapHandler       *handler.MapHandler
	WSHandler        *handler.WebSocketHandler
	DiscoveryHandler *handler.DiscoveryHandler
	DiscoveryService *discovery.DiscoveryService
	ESP32Client      *client.ESP32Client
	MQTTService      mqtt.Service
}

func NewServices(
	deviceRepo *repository.DeviceRepository,
	metricsRepo *repository.MetricsRepository,
	alertRepo *repository.AlertRepository,
	messageRepo *repository.MessageRepository,
	discoveryRepo *repository.DiscoveryRepository,
	esp32URL string,
) *Services {
	wsHandler := handler.NewWebSocketHandler()
	discoveryService := discovery.NewDiscoveryService(discoveryRepo, deviceRepo)
	discoveryHandler := handler.NewDiscoveryHandler(discoveryService)
	esp32Client := client.NewESP32Client(esp32URL)

	return &Services{
		DeviceHandler:    handler.NewDeviceHandler(deviceRepo),
		MetricsHandler:   handler.NewMetricsHandler(metricsRepo),
		AlertHandler:     handler.NewAlertHandler(alertRepo),
		MessageHandler:   handler.NewMessageHandler(messageRepo, esp32Client, wsHandler),
		MapHandler:       handler.NewMapHandler(deviceRepo),
		WSHandler:        wsHandler,
		DiscoveryHandler: discoveryHandler,
		DiscoveryService: discoveryService,
		ESP32Client:      esp32Client,
	}
}

// Close закрывает все сервисы
func (s *Services) Close() {
	s.WSHandler.Close()
	if s.MQTTService != nil {
		s.MQTTService.Stop()
	}
}
