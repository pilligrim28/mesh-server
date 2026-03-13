package discovery

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"mesh-server/client"
	"mesh-server/models"
	"mesh-server/repository"
)

// DiscoveryService управляет сканированием Bluetooth и WiFi устройств
type DiscoveryService struct {
	bluetoothScanner *BluetoothScanner
	wifiScanner      *WiFiScanner
	repo             *repository.DiscoveryRepository
	deviceRepo       *repository.DeviceRepository
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	mu               sync.RWMutex
}

// DiscoveryConfig конфигурация сервиса обнаружения
type DiscoveryConfig struct {
	EnableBluetooth bool   `json:"enable_bluetooth"`
	EnableWiFi      bool   `json:"enable_wifi"`
	ScanInterval    int    `json:"scan_interval"` // в секундах
	MeshtasticIP    string `json:"meshtastic_ip"` // Прямой IP адрес устройства Meshtastic
}

func NewDiscoveryService(repo *repository.DiscoveryRepository, deviceRepo *repository.DeviceRepository) *DiscoveryService {
	return &DiscoveryService{
		repo:             repo,
		deviceRepo:       deviceRepo,
		bluetoothScanner: NewBluetoothScanner(repo),
		wifiScanner:      NewWiFiScanner(repo),
	}
}

// Start запускает сервис обнаружения
func (s *DiscoveryService) Start(config DiscoveryConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	// Устанавливаем интервал сканирования в сканеры
	s.bluetoothScanner.SetScanInterval(config.ScanInterval)
	s.wifiScanner.SetScanInterval(config.ScanInterval)

	// Если указан прямой IP адрес Meshtastic, запускаем опрос
	if config.MeshtasticIP != "" {
		log.Printf("Direct Meshtastic IP configured: %s", config.MeshtasticIP)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.pollMeshtasticDevice(ctx, config.MeshtasticIP)
		}()
	}

	if config.EnableBluetooth {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.bluetoothScanner.Start(ctx); err != nil {
				// Логгируем ошибку, но продолжаем работу
			}
		}()
	}

	if config.EnableWiFi {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.wifiScanner.Start(ctx); err != nil {
				// Логгируем ошибку, но продолжаем работу
			}
		}()
	}

	return nil
}

// Stop останавливает сервис обнаружения
func (s *DiscoveryService) Stop() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	// Останавливаем сканеры
	s.bluetoothScanner.Stop()
	s.wifiScanner.Stop()

	// Ждем завершения горутин
	s.wg.Wait()

	return nil
}

// StartBluetoothScan запускает сканирование Bluetooth
func (s *DiscoveryService) StartBluetoothScan() error {
	return s.bluetoothScanner.Start(s.ctx)
}

// StopBluetoothScan останавливает сканирование Bluetooth
func (s *DiscoveryService) StopBluetoothScan() error {
	return s.bluetoothScanner.Stop()
}

// StartWiFiScan запускает сканирование WiFi
func (s *DiscoveryService) StartWiFiScan() error {
	return s.wifiScanner.Start(s.ctx)
}

// StopWiFiScan останавливает сканирование WiFi
func (s *DiscoveryService) StopWiFiScan() error {
	return s.wifiScanner.Stop()
}

// GetDiscoveredDevices возвращает все обнаруженные устройства
func (s *DiscoveryService) GetDiscoveredDevices() ([]models.DiscoveredDevice, error) {
	return s.repo.GetAll()
}

// GetBluetoothDevices возвращает устройства, обнаруженные через Bluetooth
func (s *DiscoveryService) GetBluetoothDevices() ([]models.DiscoveredDevice, error) {
	return s.repo.GetByType("bluetooth")
}

// GetWiFiDevices возвращает устройства, обнаруженные через WiFi
func (s *DiscoveryService) GetWiFiDevices() ([]models.DiscoveredDevice, error) {
	return s.repo.GetByType("wifi")
}

// GetMeshtasticDevices возвращает только Meshtastic устройства
func (s *DiscoveryService) GetMeshtasticDevices() ([]models.DiscoveredDevice, error) {
	return s.repo.GetMeshtasticDevices()
}

// GetScanStatus возвращает статус сканирования
func (s *DiscoveryService) GetScanStatus() DiscoveryScanStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := DiscoveryScanStatus{
		BluetoothScanning: s.bluetoothScanner.IsScanning(),
		WiFiScanning:      s.wifiScanner.IsScanning(),
	}

	// Получаем количество найденных устройств
	devices, err := s.repo.GetAll()
	if err == nil {
		status.DevicesFound = len(devices)
	}

	return status
}

// ClearDevices очищает список обнаруженных устройств
func (s *DiscoveryService) ClearDevices() error {
	return s.repo.Clear()
}

// DiscoveryScanStatus статус сканирования
type DiscoveryScanStatus struct {
	BluetoothScanning bool `json:"bluetooth_scanning"`
	WiFiScanning      bool `json:"wifi_scanning"`
	DevicesFound      int  `json:"devices_found"`
}

// TriggerScan принудительно запускает однократное сканирование
func (s *DiscoveryService) TriggerScan(scanType string) error {
	switch scanType {
	case "bluetooth":
		// Для Bluetooth запускаем сканирование nearby устройств
		return s.bluetoothScanner.ScanNearbyDevices()
	case "wifi":
		// Для WiFi можно запустить быстрое сканирование
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.wifiScanner.scanOnce(ctx)
		return nil
	case "all":
		// Запускаем оба сканирования
		go func() {
			s.bluetoothScanner.ScanNearbyDevices()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.wifiScanner.scanOnce(ctx)
		return nil
	default:
		return nil
	}
}

// GetNetworkInfo возвращает информацию о сетевых интерфейсах
func (s *DiscoveryService) GetNetworkInfo() ([]NetworkInfo, error) {
	return s.wifiScanner.GetNetworkInfo()
}

// pollMeshtasticDevice опрашивает устройство Meshtastic по прямому IP
func (s *DiscoveryService) pollMeshtasticDevice(ctx context.Context, ip string) {
	client := client.NewMeshtasticClient(ip)
	interval := time.Duration(s.wifiScanner.scanInterval) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	log.Printf("Starting Meshtastic poller for %s (interval: %v)", ip, interval)

	// Первый опрос сразу
	s.pollMeshtasticOnce(ctx, client, ip)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopping Meshtastic poller for %s", ip)
			return
		case <-ticker.C:
			s.pollMeshtasticOnce(ctx, client, ip)
		}
	}
}

// pollMeshtasticOnce выполняет однократный опрос устройства Meshtastic
func (s *DiscoveryService) pollMeshtasticOnce(ctx context.Context, client *client.MeshtasticClient, ip string) {
	// Проверяем, является ли устройство Meshtastic
	if !client.IsMeshtasticDevice(ctx) {
		log.Printf("Device at %s is not responding as Meshtastic", ip)
		return
	}

	// Получаем информацию об устройстве
	device, err := client.GetDevice(ctx)
	if err != nil {
		log.Printf("Failed to get device info from %s: %v", ip, err)
		return
	}

	log.Printf("Polling Meshtastic device: %s (%s)", device.LongName, device.ShortName)

	// Сохраняем как обнаруженное устройство
	discoveredDevice := device.ConvertToDiscoveredDevice(ip)
	existing, err := s.repo.GetByAddress(discoveredDevice.Address)
	if err != nil {
		if err := s.repo.Create(&discoveredDevice); err != nil {
			log.Printf("Error creating discovered device: %v", err)
		}
	} else {
		if err := s.repo.UpdateLastSeen(existing.Address, 0); err != nil {
			log.Printf("Error updating discovered device: %v", err)
		}
	}

	// Получаем метрики
	metrics, err := client.GetMetrics(ctx)
	if err != nil {
		log.Printf("Failed to get metrics from %s: %v", ip, err)
		return
	}

	log.Printf("Meshtastic metrics: %+v", metrics)

	// Получаем узлы
	nodes, err := client.GetNodes(ctx)
	if err != nil {
		log.Printf("Failed to get nodes from %s: %v", ip, err)
		return
	}

	log.Printf("Found %d nodes in Meshtastic network", len(nodes))

	// Для каждого узла создаём/обновляем устройство в БД
	for _, node := range nodes {
		if node.Position != nil {
			lat, lon, alt := node.Position.ConvertPosition()

			// Ищем устройство по node_id
			nodeID := fmt.Sprintf("!%08x", node.Num)
			dev, err := s.deviceRepo.GetByNodeID(nodeID)
			if err != nil {
				// Создаём новое устройство
				dev = &models.Device{
					NodeID:    nodeID,
					Name:      node.User.LongName,
					Latitude:  lat,
					Longitude: lon,
					Altitude:  alt,
					LastSeen:  time.Now(),
				}
				if err := s.deviceRepo.Create(dev); err != nil {
					log.Printf("Error creating device %s: %v", nodeID, err)
					continue
				}
			} else {
				// Обновляем позицию
				if err := s.deviceRepo.UpdatePosition(dev.ID, lat, lon, alt); err != nil {
					log.Printf("Error updating position for %s: %v", nodeID, err)
				}
			}

			log.Printf("Synced node %s: lat=%.6f, lon=%.6f, alt=%.1f", nodeID, lat, lon, alt)
		}
	}
}
