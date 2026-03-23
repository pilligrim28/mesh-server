package main

import (
	"context"
	"log"
)

// App struct
type App struct {
	ctx     context.Context
	service *MeshService
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		service: NewMeshService(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Инициализация сервиса
	if err := a.service.Init(ctx); err != nil {
		log.Printf("Error initializing MeshService: %v", err)
	}

	log.Printf("Desktop app connected to server: %s", a.service.GetServerURL())
}

// shutdown вызывается при закрытии приложения
func (a *App) shutdown(ctx context.Context) {
	if a.service != nil {
		a.service.Shutdown()
	}
}

// GetDevices возвращает все устройства
func (a *App) GetDevices() []MeshDevice {
	return a.service.GetDevices()
}

// GetMetrics возвращает последние метрики
func (a *App) GetMetrics() []MeshMetric {
	return a.service.GetMetrics()
}

// GetUnreadAlerts возвращает непрочитанные алерты
func (a *App) GetUnreadAlerts() []MeshAlert {
	return a.service.GetUnreadAlerts()
}

// MarkAlertAsRead помечает алерт как прочитанный
func (a *App) MarkAlertAsRead(alertID int64) bool {
	return a.service.MarkAlertAsRead(alertID)
}

// MarkAllAlertsAsRead помечает все алерты как прочитанные
func (a *App) MarkAllAlertsAsRead(deviceID int64) bool {
	return a.service.MarkAllAlertsAsRead(deviceID)
}

// GetMessages возвращает сообщения устройства
func (a *App) GetMessages(deviceID int64, limit int) []MeshMessage {
	return a.service.GetMessages(deviceID, limit)
}

// SendMessage отправляет сообщение
func (a *App) SendMessage(deviceID int64, fromNode, toNode, text string) bool {
	return a.service.SendMessage(deviceID, fromNode, toNode, text)
}

// GetDiscoveredDevices возвращает обнаруженные устройства
func (a *App) GetDiscoveredDevices() []DiscoveredDevice {
	return a.service.GetDiscoveredDevices()
}

// StartBluetoothScan запускает Bluetooth сканирование
func (a *App) StartBluetoothScan() bool {
	return a.service.StartBluetoothScan()
}

// StopBluetoothScan останавливает Bluetooth сканирование
func (a *App) StopBluetoothScan() bool {
	return a.service.StopBluetoothScan()
}

// StartWiFiScan запускает WiFi сканирование
func (a *App) StartWiFiScan() bool {
	return a.service.StartWiFiScan()
}

// StopWiFiScan останавливает WiFi сканирование
func (a *App) StopWiFiScan() bool {
	return a.service.StopWiFiScan()
}

// GetDiscoveryStatus возвращает статус сканирования
func (a *App) GetDiscoveryStatus() map[string]interface{} {
	return a.service.GetDiscoveryStatus()
}

// GetStats возвращает статистику
func (a *App) GetStats() map[string]interface{} {
	return a.service.GetStats()
}

// GetMapData возвращает данные для карты
func (a *App) GetMapData() []MeshDevice {
	return a.service.GetMapData()
}

// SetServerURL устанавливает URL сервера
func (a *App) SetServerURL(url string) bool {
	a.service.SetServerURL(url)
	log.Printf("Server URL changed to: %s", url)
	return true
}

// GetServerURL возвращает текущий URL сервера
func (a *App) GetServerURL() string {
	return a.service.GetServerURL()
}

// CheckServerConnection проверяет подключение к серверу
func (a *App) CheckServerConnection() bool {
	return a.service.CheckServerConnection()
}
