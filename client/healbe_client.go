package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

// HealbeClient клиент для подключения к часам Healbe GoBe через BLE
type HealbeClient struct {
	deviceAddress string
	connected     bool
	mu            sync.RWMutex
	stopChan      chan struct{}
	dataCallback  func(data *HealbeData)

	// BLE характеристики Healbe
	heartRateChar string
	stressChar    string

	// Репозитории для сохранения данных
	healbeRepo  *repository.HealbeRepository
	deviceRepo  *repository.DeviceRepository
}

// HealbeData структура данных с часов Healbe
type HealbeData struct {
	DeviceID   string    `json:"device_id"`
	HeartRate  int       `json:"heart_rate"`
	StressLevel int      `json:"stress_level"`
	Timestamp  time.Time `json:"timestamp"`
	Battery    int       `json:"battery,omitempty"`
}

// UUID сервисов и характеристик Healbe GoBe
const (
	// Healbe Service UUIDs
	HealbeServiceUUID        = "0000180d-0000-1000-8000-00805f9b34fb" // Heart Rate Service
	HealbeStressServiceUUID  = "0000ffe0-0000-1000-8000-00805f9b34fb" // Custom Service

	// Characteristic UUIDs
	HealbeHeartRateCharUUID  = "00002a37-0000-1000-8000-00805f9b34fb" // Heart Rate Measurement
	HealbeStressCharUUID     = "0000ffe1-0000-1000-8000-00805f9b34fb" // Stress Level
	HealbeBatteryCharUUID    = "00002a19-0000-1000-8000-00805f9b34fb" // Battery Level
)

// NewHealbeClient создает новый клиент для часов Healbe
func NewHealbeClient(deviceAddress string, healbeRepo *repository.HealbeRepository, deviceRepo *repository.DeviceRepository) *HealbeClient {
	return &HealbeClient{
		deviceAddress: deviceAddress,
		stopChan:      make(chan struct{}),
		healbeRepo:    healbeRepo,
		deviceRepo:    deviceRepo,
		heartRateChar: HealbeHeartRateCharUUID,
		stressChar:    HealbeStressCharUUID,
	}
}

// Connect подключается к часам Healbe через BLE
func (c *HealbeClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// В реальной реализации здесь будет BLE подключение
	// Для Windows/Linux можно использовать github.com/paypal/gatt или similar
	log.Printf("Healbe: Connecting to device %s...", c.deviceAddress)

	// Имитация подключения (заменить на реальное BLE)
	c.connected = true

	// Запускаем горутину для чтения данных
	go c.readDataLoop()

	log.Printf("Healbe: Connected to %s", c.deviceAddress)
	return nil
}

// Disconnect отключается от часов
func (c *HealbeClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return
	}

	close(c.stopChan)
	c.stopChan = make(chan struct{})
	c.connected = false

	log.Printf("Healbe: Disconnected from %s", c.deviceAddress)
}

// IsConnected проверяет статус подключения
func (c *HealbeClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetDeviceAddress возвращает MAC адрес устройства
func (c *HealbeClient) GetDeviceAddress() string {
	return c.deviceAddress
}

// SetDataCallback устанавливает callback для получения данных
func (c *HealbeClient) SetDataCallback(callback func(data *HealbeData)) {
	c.dataCallback = callback
}

// readDataLoop цикл чтения данных с часов
func (c *HealbeClient) readDataLoop() {
	ticker := time.NewTicker(5 * time.Second) // Читаем данные каждые 5 секунд
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			data := c.readSensorData()
			if data != nil {
				c.saveData(data)
				if c.dataCallback != nil {
					c.dataCallback(data)
				}
			}
		}
	}
}

// readSensorData читает данные с сенсоров часов
func (c *HealbeClient) readSensorData() *HealbeData {
	// В реальной реализации здесь будет чтение BLE характеристик
	// Для демонстрации генерируем тестовые данные

	// Парсинг данных Heart Rate
	heartRate := c.parseHeartRateData()

	// Парсинг данных Stress
	stressLevel := c.parseStressData()

	return &HealbeData{
		DeviceID:    c.deviceAddress,
		HeartRate:   heartRate,
		StressLevel: stressLevel,
		Timestamp:   time.Now(),
		Battery:     85, // Тестовое значение
	}
}

// parseHeartRateData парсит данные пульса из BLE характеристики
func (c *HealbeClient) parseHeartRateData() int {
	// Формат данных Heart Rate Measurement (Bluetooth SIG):
	// Flags (1 byte) + Heart Rate (1-2 bytes) + ...
	// Если флаг 0 - heart rate в 1 байте, если 1 - в 2 байтах

	// В реальной реализации читаем из BLE:
	// data, err := c.bleClient.ReadCharacteristic(HealbeHeartRateCharUUID)
	// if err != nil { return 0 }
	// flags := data[0]
	// if flags & 0x01 == 0 {
	//     return int(data[1])
	// }
	// return int(binary.LittleEndian.Uint16(data[1:3]))

	// Тестовые данные
	return 72 + (time.Now().Second() % 20) // 72-92 bpm
}

// parseStressData парсит данные стресса из BLE характеристики
func (c *HealbeClient) parseStressData() int {
	// Healbe использует свой собственный протокол для стресса
	// Обычно это значение от 0 до 100 или категории 0-4

	// В реальной реализации:
	// data, err := c.bleClient.ReadCharacteristic(HealbeStressCharUUID)
	// if err != nil { return 0 }
	// return int(data[0])

	// Тестовые данные: 0-4 (No stress, Light, Elevated, High, Very high)
	return time.Now().Second() % 5
}

// saveData сохраняет данные в базу
func (c *HealbeClient) saveData(data *HealbeData) {
	if c.healbeRepo == nil {
		return
	}

	// Находим или создаем устройство
	device, err := c.deviceRepo.GetByNodeID(data.DeviceID)
	if err != nil {
		// Создаем новое устройство
		device = &models.Device{
			NodeID:   data.DeviceID,
			Name:     "Healbe GoBe",
			LastSeen: time.Now(),
		}
		if err := c.deviceRepo.Create(device); err != nil {
			log.Printf("Healbe: failed to create device: %v", err)
			return
		}
	}

	// Обновляем last_seen
	c.deviceRepo.UpdateLastSeen(device.ID)

	// Сохраняем метрики Healbe
	metrics := &models.HealbeMetrics{
		DeviceID:    device.ID,
		HeartRate:   data.HeartRate,
		StressLevel: data.StressLevel,
		Battery:     data.Battery,
		Timestamp:   data.Timestamp,
	}

	if err := c.healbeRepo.Create(metrics); err != nil {
		log.Printf("Healbe: failed to save metrics: %v", err)
		return
	}

	log.Printf("Healbe: saved data - HR: %d, Stress: %d, Battery: %d%%", data.HeartRate, data.StressLevel, data.Battery)
}

// GetLastData возвращает последние полученные данные
func (c *HealbeClient) GetLastData() *HealbeData {
	// Получаем последние данные из базы
	return nil
}

// ParseHeartRateCharacteristic парсит стандартную BLE характеристику Heart Rate
func ParseHeartRateCharacteristic(data []byte) (int, error) {
	if len(data) < 2 {
		return 0, fmt.Errorf("invalid heart rate data length")
	}

	flags := data[0]

	// Если бит 0 установлен - heart rate в формате uint16
	if flags&0x01 != 0 {
		if len(data) < 3 {
			return 0, fmt.Errorf("invalid heart rate data length for uint16")
		}
		return int(binary.LittleEndian.Uint16(data[1:3])), nil
	}

	// Иначе heart rate в формате uint8
	return int(data[1]), nil
}

// ParseStressLevel преобразует уровень стресса в текстовое описание
func ParseStressLevel(level int) string {
	switch level {
	case 0:
		return "No stress"
	case 1:
		return "Light"
	case 2:
		return "Elevated"
	case 3:
		return "High"
	case 4:
		return "Very high"
	default:
		return "Unknown"
	}
}