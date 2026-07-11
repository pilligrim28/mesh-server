package simulator

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"mesh-server/client"
	"mesh-server/models"
	"mesh-server/repository"
)

// HealbeDemo генерирует демонстрационные данные Healbe GoBe.
type HealbeDemo struct {
	healbeRepo *repository.HealbeRepository
	deviceRepo *repository.DeviceRepository
	mac        string
	onData     func(*client.HealbeData)

	ticker    *time.Ticker
	stopChan  chan struct{}
	mu        sync.RWMutex
	isRunning bool
}

// NewHealbeDemo создаёт симулятор Healbe для демонстраций.
func NewHealbeDemo(
	healbeRepo *repository.HealbeRepository,
	deviceRepo *repository.DeviceRepository,
	mac string,
	onData func(*client.HealbeData),
) *HealbeDemo {
	return &HealbeDemo{
		healbeRepo: healbeRepo,
		deviceRepo: deviceRepo,
		mac:        mac,
		onData:     onData,
		stopChan:   make(chan struct{}),
	}
}

// Start запускает генерацию демо-данных Healbe.
func (d *HealbeDemo) Start(interval time.Duration) {
	d.mu.Lock()
	if d.isRunning {
		d.mu.Unlock()
		return
	}
	d.isRunning = true
	d.mu.Unlock()

	d.ticker = time.NewTicker(interval)
	go d.run()
	log.Printf("Healbe demo: started for %s (interval %v)", d.mac, interval)
}

// Stop останавливает генерацию.
func (d *HealbeDemo) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.isRunning {
		return
	}
	close(d.stopChan)
	if d.ticker != nil {
		d.ticker.Stop()
	}
	d.isRunning = false
	log.Println("Healbe demo: stopped")
}

// IsRunning возвращает статус симулятора.
func (d *HealbeDemo) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isRunning
}

func (d *HealbeDemo) run() {
	d.generateOnce()
	for {
		select {
		case <-d.stopChan:
			return
		case <-d.ticker.C:
			d.generateOnce()
		}
	}
}

func (d *HealbeDemo) generateOnce() {
	now := time.Now()
	data := &client.HealbeData{
		DeviceID:    d.mac,
		HeartRate:   68 + rand.Intn(28),
		StressLevel: rand.Intn(4),
		Battery:     72 + rand.Intn(23),
		Timestamp:   now,
	}

	device, err := d.deviceRepo.GetByNodeID(d.mac)
	if err != nil {
		device = &models.Device{
			NodeID:   d.mac,
			Name:     "Healbe GoBe (демо)",
			LastSeen: now,
		}
		if err := d.deviceRepo.Create(device); err != nil {
			log.Printf("Healbe demo: failed to create device: %v", err)
			return
		}
	} else {
		_ = d.deviceRepo.UpdateLastSeen(device.ID)
	}

	metrics := &models.HealbeMetrics{
		DeviceID:    device.ID,
		HeartRate:   data.HeartRate,
		StressLevel: data.StressLevel,
		Battery:     data.Battery,
		Timestamp:   now,
	}
	if err := d.healbeRepo.Create(metrics); err != nil {
		log.Printf("Healbe demo: failed to save metrics: %v", err)
		return
	}

	if d.onData != nil {
		d.onData(data)
	}
}
