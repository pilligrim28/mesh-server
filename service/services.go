package service

import (
	"mesh-server/discovery"
	"mesh-server/repository"
)

type Services struct {
	DiscoveryService *discovery.DiscoveryService
	MQTTService      interface{}
}

func NewServices(
	discoveryRepo *repository.DiscoveryRepository,
	deviceRepo *repository.DeviceRepository,
) *Services {
	return &Services{
		DiscoveryService: discovery.NewDiscoveryService(discoveryRepo, deviceRepo),
	}
}

// Close закрывает фоновые сервисы.
func (s *Services) Close() {
	if mqttService, ok := s.MQTTService.(interface{ Stop() }); ok {
		mqttService.Stop()
	}
}
