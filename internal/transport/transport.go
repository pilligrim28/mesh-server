// Package transport — каналы связи сервера со шлюзом Meshtastic: BLE, USB (serial), симулятор.
package transport

// Sink — то, куда транспорт отдаёт данные (реализуется core.Service).
type Sink interface {
	HandleFrame(gatewayID string, frame []byte)
	SetGateway(id, transport, state, name, errText string)
	DropGateway(id string)
}
