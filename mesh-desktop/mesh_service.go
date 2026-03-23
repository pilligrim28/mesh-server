package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// MeshService - сервис для работы с Meshtastic сервером через HTTP API
type MeshService struct {
	ctx        context.Context
	serverURL  string
	httpClient *http.Client
	mu         sync.RWMutex
}

// Конфигурация сервера
type ServerConfig struct {
	URL string `json:"url"`
}

// MeshDevice представляет устройство
type MeshDevice struct {
	ID        int64   `json:"id"`
	NodeID    string  `json:"node_id"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
	LastSeen  string  `json:"last_seen"`
	IsOnline  bool    `json:"is_online"`
}

// MeshMetric представляет метрики
type MeshMetric struct {
	ID        int64   `json:"id"`
	DeviceID  int64   `json:"device_id"`
	HeartRate int     `json:"heart_rate"`
	CO2       int     `json:"co2"`
	Temp      float64 `json:"temp"`
	Humidity  float64 `json:"humidity"`
	Battery   int     `json:"battery"`
	Timestamp string  `json:"timestamp"`
}

// MeshAlert представляет уведомление
type MeshAlert struct {
	ID        int64  `json:"id"`
	DeviceID  int64  `json:"device_id"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
	Read      bool   `json:"read"`
	Timestamp string `json:"timestamp"`
}

// MeshMessage представляет сообщение
type MeshMessage struct {
	ID        int64  `json:"id"`
	DeviceID  int64  `json:"device_id"`
	FromNode  string `json:"from_node"`
	ToNode    string `json:"to_node"`
	Text      string `json:"text"`
	Direction string `json:"direction"`
	Timestamp string `json:"timestamp"`
}

// DiscoveredDevice представляет обнаруженное устройство
type DiscoveredDevice struct {
	ID           int64  `json:"id"`
	Address      string `json:"address"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	RSSI         int    `json:"rssi,omitempty"`
	Meshtastic   bool   `json:"meshtastic"`
	LastSeen     string `json:"last_seen"`
	DiscoveredAt string `json:"discovered_at"`
}

// DiscoveryStatus статус сканирования
type DiscoveryStatus struct {
	BluetoothScanning bool `json:"bluetooth_scanning"`
	WiFiScanning      bool `json:"wifi_scanning"`
	DevicesFound      int  `json:"devices_found"`
}

// Stats статистика
type Stats struct {
	DeviceCount  int `json:"device_count"`
	AlertCount   int `json:"alert_count"`
	MessageCount int `json:"message_count"`
}

// NewMeshService создает новый сервис
func NewMeshService() *MeshService {
	return &MeshService{
		serverURL: "http://localhost:8080",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetServerURL устанавливает URL сервера
func (m *MeshService) SetServerURL(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serverURL = url
}

// Init инициализирует сервис
func (m *MeshService) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ctx = ctx
	log.Printf("MeshService initialized with server URL: %s", m.serverURL)
	return nil
}

// GetDevices возвращает все устройства
func (m *MeshService) GetDevices() []MeshDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Get(m.serverURL + "/api/devices")
	if err != nil {
		log.Printf("Error getting devices: %v", err)
		return []MeshDevice{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Error response status: %s", resp.Status)
		return []MeshDevice{}
	}

	var devices []MeshDevice
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		log.Printf("Error decoding devices: %v", err)
		return []MeshDevice{}
	}

	// Проверяем онлайн статус (last_seen менее 5 минут)
	for i := range devices {
		lastSeen, err := time.Parse("2006-01-02 15:04:05", devices[i].LastSeen)
		if err == nil {
			devices[i].IsOnline = time.Since(lastSeen) < 5*time.Minute
		}
	}

	return devices
}

// GetMetrics возвращает последние метрики
func (m *MeshService) GetMetrics() []MeshMetric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Get(m.serverURL + "/api/metrics")
	if err != nil {
		log.Printf("Error getting metrics: %v", err)
		return []MeshMetric{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []MeshMetric{}
	}

	var metrics []MeshMetric
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		log.Printf("Error decoding metrics: %v", err)
		return []MeshMetric{}
	}

	return metrics
}

// GetUnreadAlerts возвращает непрочитанные алерты
func (m *MeshService) GetUnreadAlerts() []MeshAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Get(m.serverURL + "/api/alerts")
	if err != nil {
		log.Printf("Error getting alerts: %v", err)
		return []MeshAlert{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []MeshAlert{}
	}

	var alerts []MeshAlert
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		log.Printf("Error decoding alerts: %v", err)
		return []MeshAlert{}
	}

	return alerts
}

// MarkAlertAsRead помечает алерт как прочитанный
func (m *MeshService) MarkAlertAsRead(alertID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/api/alerts/read?id=%d", m.serverURL, alertID), nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return false
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		log.Printf("Error marking alert as read: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// MarkAllAlertsAsRead помечает все алерты как прочитанные
func (m *MeshService) MarkAllAlertsAsRead(deviceID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	url := fmt.Sprintf("%s/api/alerts/read-all", m.serverURL)
	if deviceID > 0 {
		url += fmt.Sprintf("?device_id=%d", deviceID)
	}

	req, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return false
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		log.Printf("Error marking all alerts as read: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetMessages возвращает сообщения устройства
func (m *MeshService) GetMessages(deviceID int64, limit int) []MeshMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	url := fmt.Sprintf("%s/api/messages/device?device_id=%d&limit=%d", m.serverURL, deviceID, limit)
	resp, err := m.httpClient.Get(url)
	if err != nil {
		log.Printf("Error getting messages: %v", err)
		return []MeshMessage{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []MeshMessage{}
	}

	var messages []MeshMessage
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		log.Printf("Error decoding messages: %v", err)
		return []MeshMessage{}
	}

	return messages
}

// SendMessage отправляет сообщение
func (m *MeshService) SendMessage(deviceID int64, fromNode, toNode, text string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	payload := map[string]interface{}{
		"device_id": deviceID,
		"from_node": fromNode,
		"to_node":   toNode,
		"text":      text,
		"direction": "outbound",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return false
	}

	resp, err := m.httpClient.Post(m.serverURL+"/api/messages", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error sending message: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetDiscoveredDevices возвращает обнаруженные устройства
func (m *MeshService) GetDiscoveredDevices() []DiscoveredDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Get(m.serverURL + "/api/discovery")
	if err != nil {
		log.Printf("Error getting discovered devices: %v", err)
		return []DiscoveredDevice{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []DiscoveredDevice{}
	}

	var devices []DiscoveredDevice
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		log.Printf("Error decoding discovered devices: %v", err)
		return []DiscoveredDevice{}
	}

	return devices
}

// StartBluetoothScan запускает Bluetooth сканирование
func (m *MeshService) StartBluetoothScan() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Post(m.serverURL+"/api/discovery/bluetooth/start", "application/json", nil)
	if err != nil {
		log.Printf("Error starting Bluetooth scan: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// StopBluetoothScan останавливает Bluetooth сканирование
func (m *MeshService) StopBluetoothScan() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Post(m.serverURL+"/api/discovery/bluetooth/stop", "application/json", nil)
	if err != nil {
		log.Printf("Error stopping Bluetooth scan: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// StartWiFiScan запускает WiFi сканирование
func (m *MeshService) StartWiFiScan() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Post(m.serverURL+"/api/discovery/wifi/start", "application/json", nil)
	if err != nil {
		log.Printf("Error starting WiFi scan: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// StopWiFiScan останавливает WiFi сканирование
func (m *MeshService) StopWiFiScan() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Post(m.serverURL+"/api/discovery/wifi/stop", "application/json", nil)
	if err != nil {
		log.Printf("Error stopping WiFi scan: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetDiscoveryStatus возвращает статус сканирования
func (m *MeshService) GetDiscoveryStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Get(m.serverURL + "/api/discovery/status")
	if err != nil {
		log.Printf("Error getting discovery status: %v", err)
		return map[string]interface{}{
			"bluetooth_scanning": false,
			"wifi_scanning":      false,
			"devices_found":      0,
			"error":              err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return map[string]interface{}{
			"bluetooth_scanning": false,
			"wifi_scanning":      false,
			"devices_found":      0,
		}
	}

	var status DiscoveryStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return map[string]interface{}{
			"bluetooth_scanning": false,
			"wifi_scanning":      false,
			"devices_found":      0,
		}
	}

	return map[string]interface{}{
		"bluetooth_scanning": status.BluetoothScanning,
		"wifi_scanning":      status.WiFiScanning,
		"devices_found":      status.DevicesFound,
	}
}

// GetStats возвращает статистику
func (m *MeshService) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Получаем количество устройств
	devicesResp, err := m.httpClient.Get(m.serverURL + "/api/devices")
	deviceCount := 0
	if err == nil && devicesResp.StatusCode == http.StatusOK {
		var devices []MeshDevice
		if err := json.NewDecoder(devicesResp.Body).Decode(&devices); err == nil {
			deviceCount = len(devices)
		}
		devicesResp.Body.Close()
	}

	// Получаем количество алертов
	alertsResp, err := m.httpClient.Get(m.serverURL + "/api/alerts")
	alertCount := 0
	if err == nil && alertsResp.StatusCode == http.StatusOK {
		var alerts []MeshAlert
		if err := json.NewDecoder(alertsResp.Body).Decode(&alerts); err == nil {
			alertCount = len(alerts)
		}
		alertsResp.Body.Close()
	}

	return map[string]interface{}{
		"device_count":  deviceCount,
		"alert_count":   alertCount,
		"message_count": 0,
	}
}

// GetMapData возвращает данные для карты
func (m *MeshService) GetMapData() []MeshDevice {
	return m.GetDevices()
}

// CheckServerConnection проверяет подключение к серверу
func (m *MeshService) CheckServerConnection() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp, err := m.httpClient.Get(m.serverURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetServerURL возвращает текущий URL сервера
func (m *MeshService) GetServerURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serverURL
}

// Shutdown останавливает сервис
func (m *MeshService) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Println("MeshService shutdown complete")
}

// HTTP запросы
func (m *MeshService) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return m.httpClient.Do(req)
}
