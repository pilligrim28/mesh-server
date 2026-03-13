package mqtt

import "context"

// Service интерфейс для MQTT сервиса
type Service interface {
	Start(ctx context.Context) error
	Stop()
	IsConnected() bool
	GetStatus() map[string]interface{}
	Reconnect() error
	SendMessage(fromNode, toNode, text string, channel int) error
}
