package handler

import (
	"context"
	"log"
	"sync"
	"time"

	"mesh-server/client"
	"mesh-server/models"
	"mesh-server/repository"
)

// HealbeDataCallback обрабатывает входящие данные Healbe.
type HealbeDataCallback func(data *client.HealbeData)

// HealbeESP32Bridge получает данные Healbe через ESP32 (HTTP или ingest).
type HealbeESP32Bridge struct {
	esp32Client *client.ESP32HealbeClient
	healbeRepo  *repository.HealbeRepository
	deviceRepo  *repository.DeviceRepository
	onData      HealbeDataCallback

	mu              sync.RWMutex
	mac             string
	connected       bool
	forwardEnabled  bool
	useServerIngest bool
	lastDataAt      time.Time
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewHealbeESP32Bridge создаёт мост Healbe ↔ ESP32.
func NewHealbeESP32Bridge(
	esp32URL string,
	healbeRepo *repository.HealbeRepository,
	deviceRepo *repository.DeviceRepository,
	onData HealbeDataCallback,
) *HealbeESP32Bridge {
	return &HealbeESP32Bridge{
		esp32Client:     client.NewESP32HealbeClient(esp32URL),
		healbeRepo:      healbeRepo,
		deviceRepo:      deviceRepo,
		onData:          onData,
		useServerIngest: esp32URL == "",
	}
}

// Connect запускает получение данных Healbe через ESP32.
func (b *HealbeESP32Bridge) Connect(ctx context.Context, mac string) error {
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.mac = mac
	b.connected = true
	bridgeCtx, cancel := context.WithCancel(context.Background())
	b.ctx = bridgeCtx
	b.cancel = cancel
	b.mu.Unlock()

	if b.esp32Client.IsConfigured() {
		if err := b.esp32Client.Connect(ctx, mac); err != nil {
			log.Printf("Healbe ESP32 bridge: HTTP connect failed, waiting for ingest: %v", err)
		}
		go b.pollLoop()
	} else {
		log.Printf("Healbe ESP32 bridge: MAC %s registered, waiting for ESP32 ingest", mac)
	}

	return nil
}

// Disconnect останавливает мост.
func (b *HealbeESP32Bridge) Disconnect() {
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.connected = false
	mac := b.mac
	b.mu.Unlock()

	if b.esp32Client.IsConfigured() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.esp32Client.Disconnect(ctx)
	}

	log.Printf("Healbe ESP32 bridge: disconnected from %s", mac)
}

// IsConnected проверяет активность моста.
func (b *HealbeESP32Bridge) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// GetMAC возвращает MAC часов.
func (b *HealbeESP32Bridge) GetMAC() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.mac
}

// SetForwardEnabled включает пересылку в mesh.
func (b *HealbeESP32Bridge) SetForwardEnabled(enabled bool) {
	b.mu.Lock()
	b.forwardEnabled = enabled
	b.mu.Unlock()
}

// HasHTTPBridge сообщает, задан ли HTTP URL bridge-ESP32.
func (b *HealbeESP32Bridge) HasHTTPBridge() bool {
	return b.esp32Client.IsConfigured()
}

// IsHTTPBridgeReachable проверяет доступность HTTP bridge-ESP32.
func (b *HealbeESP32Bridge) IsHTTPBridgeReachable(ctx context.Context) bool {
	if !b.esp32Client.IsConfigured() {
		return false
	}
	_, err := b.esp32Client.GetStatus(ctx)
	return err == nil
}

// GetBridgeURL возвращает URL bridge-ESP32.
func (b *HealbeESP32Bridge) GetBridgeURL() string {
	return b.esp32Client.GetBaseURL()
}

// GetLastDataAt возвращает время последних данных.
func (b *HealbeESP32Bridge) GetLastDataAt() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastDataAt
}

// GetConfig возвращает конфигурацию для прошивки ESP32.
func (b *HealbeESP32Bridge) GetConfig() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]interface{}{
		"enabled":   b.connected,
		"mac":       b.mac,
		"forward":   b.forwardEnabled,
		"transport": "esp32",
	}
}

func (b *HealbeESP32Bridge) pollLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(b.ctx, 8*time.Second)
			data, err := b.esp32Client.GetData(ctx)
			cancel()
			if err != nil {
				continue
			}
			if data.HeartRate == 0 && data.StressLevel == 0 && data.Battery == 0 {
				continue
			}
			b.ingest(client.HealbeData{
				DeviceID:    healbeFirstNonEmpty(data.DeviceID, data.MAC, b.GetMAC()),
				HeartRate:   data.HeartRate,
				StressLevel: data.StressLevel,
				Battery:     data.Battery,
				Timestamp:   time.Now(),
			})
		}
	}
}

// Ingest принимает данные, отправленные ESP32 на сервер.
func (b *HealbeESP32Bridge) Ingest(data client.HealbeData) error {
	if data.DeviceID == "" {
		data.DeviceID = b.GetMAC()
	}
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}
	b.ingest(data)
	return nil
}

func (b *HealbeESP32Bridge) ingest(data client.HealbeData) {
	if data.DeviceID == "" {
		return
	}

	device, err := b.deviceRepo.GetByNodeID(data.DeviceID)
	if err != nil {
		device = &models.Device{
			NodeID:   data.DeviceID,
			Name:     "Healbe GoBe",
			LastSeen: time.Now(),
		}
		if err := b.deviceRepo.Create(device); err != nil {
			log.Printf("Healbe ESP32 bridge: failed to create device: %v", err)
			return
		}
	} else {
		_ = b.deviceRepo.UpdateLastSeen(device.ID)
	}

	metrics := &models.HealbeMetrics{
		DeviceID:    device.ID,
		HeartRate:   data.HeartRate,
		StressLevel: data.StressLevel,
		Battery:     data.Battery,
		Timestamp:   data.Timestamp,
	}

	if err := b.healbeRepo.Create(metrics); err != nil {
		log.Printf("Healbe ESP32 bridge: failed to save metrics: %v", err)
		return
	}

	b.mu.Lock()
	b.lastDataAt = time.Now()
	b.mu.Unlock()

	log.Printf("Healbe ESP32 bridge: HR=%d Stress=%d Battery=%d%%", data.HeartRate, data.StressLevel, data.Battery)

	if b.onData != nil {
		b.onData(&data)
	}
}

func healbeFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
