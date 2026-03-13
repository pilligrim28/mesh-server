package discovery

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

// BLEScanner сканирует Bluetooth LE устройства через Windows Runtime API
type BLEScanner struct {
	repo         *repository.DiscoveryRepository
	isScanning   bool
	mu           sync.Mutex
	cancel       context.CancelFunc
	scanInterval int
	esp32MAC     string // Сохраняем найденный MAC адрес ESP32
}

// BLEDevice представляет BLE устройство
type BLEDevice struct {
	Address      string
	Name         string
	RSSI         int
	ServiceIDs   []string
	IsESP32      bool
	IsMeshtastic bool
}

func NewBLEScanner(repo *repository.DiscoveryRepository) *BLEScanner {
	return &BLEScanner{
		repo:         repo,
		scanInterval: 10,
	}
}

// SetScanInterval устанавливает интервал сканирования
func (s *BLEScanner) SetScanInterval(interval int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if interval > 0 {
		s.scanInterval = interval
	}
}

// Start начинает сканирование BLE устройств
func (s *BLEScanner) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.isScanning {
		s.mu.Unlock()
		return nil
	}
	s.isScanning = true
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	log.Println("BLE scanner started")
	go s.scanLoop(ctx)

	return nil
}

// Stop останавливает сканирование
func (s *BLEScanner) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isScanning {
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}
	s.isScanning = false

	log.Println("BLE scanner stopped")
	return nil
}

// IsScanning возвращает статус сканирования
func (s *BLEScanner) IsScanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isScanning
}

func (s *BLEScanner) scanLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.scanOnce(ctx)
			time.Sleep(time.Duration(s.scanInterval) * time.Second)
		}
	}
}

func (s *BLEScanner) scanOnce(ctx context.Context) {
	// Сканируем BLE устройства через PowerShell
	devices := s.scanBLEDevices(ctx)

	for _, device := range devices {
		s.foundDevice(device.Address, "bluetooth", device.RSSI, device.Name)

		// Сохраняем MAC адрес если это ESP32
		if device.IsESP32 {
			s.esp32MAC = device.Address
			log.Printf("Found ESP32 MAC: %s", device.Address)
		}
	}
}

// scanBLEDevices сканирует BLE устройства через PowerShell
func (s *BLEScanner) scanBLEDevices(ctx context.Context) []BLEDevice {
	var devices []BLEDevice

	// PowerShell скрипт для сканирования BLE устройств
	powerShellScript := `
$watcher = [Windows.Devices.Bluetooth.Advertisement.BluetoothLEAdvertisementWatcher]::new()
$devices = @()
$timeout = 5000

$watcher.add_Received({
    param($sender, $args)
    $device = @{
        Address = $args.BluetoothAddress.ToString("X12")
        Name = $args.Advertisement.LocalName
        RSSI = $args.RawSignalStrengthInDBm
    }
    $devices += $device
})

$watcher.Start()
Start-Sleep -Milliseconds $timeout
$watcher.Stop()

foreach ($d in $devices) {
    Write-Output "$($d.Address)|$($d.Name)|$($d.RSSI)"
}
`

	// Выполняем PowerShell скрипт
	output, err := s.runPowerShell(powerShellScript)
	if err != nil {
		log.Printf("PowerShell BLE scan error: %v", err)
		// Пробуем альтернативный метод через cmd
		return s.scanBLEViaCMD()
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		address := parts[0]
		name := parts[1]
		rssi := 0
		fmt.Sscanf(parts[2], "%d", &rssi)

		// Проверяем является ли устройство ESP32 или Meshtastic
		isESP32 := strings.Contains(strings.ToLower(name), "esp32") ||
			strings.Contains(strings.ToLower(name), "meshtastic") ||
			strings.Contains(strings.ToLower(name), "lilygo") ||
			strings.Contains(strings.ToLower(name), "heltec")

		devices = append(devices, BLEDevice{
			Address:      address,
			Name:         name,
			RSSI:         rssi,
			IsESP32:      isESP32,
			IsMeshtastic: isESP32,
		})
	}

	return devices
}

// scanBLEViaCMD альтернативный метод сканирования через cmd
func (s *BLEScanner) scanBLEViaCMD() []BLEDevice {
	var devices []BLEDevice

	// Используем PowerShell для получения Bluetooth устройств
	cmd := exec.Command("powershell", "-Command", `
Get-PnpDevice -Class Bluetooth | Where-Object {$_.Status -eq 'OK'} | Select-Object FriendlyName
`)

	output, err := cmd.Output()
	if err != nil {
		log.Printf("CMD BLE scan error: %v", err)
		return devices
	}

	// Парсим вывод
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "FriendlyName") {
			continue
		}

		isESP32 := strings.Contains(strings.ToLower(line), "esp32") ||
			strings.Contains(strings.ToLower(line), "meshtastic")

		if isESP32 {
			devices = append(devices, BLEDevice{
				Address:      "UNKNOWN", // MAC адрес через CMD получить сложно
				Name:         line,
				RSSI:         -50,
				IsESP32:      true,
				IsMeshtastic: true,
			})
		}
	}

	return devices
}

// runPowerShell выполняет PowerShell скрипт
func (s *BLEScanner) runPowerShell(script string) (string, error) {
	cmd := exec.Command("powershell", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (s *BLEScanner) foundDevice(address, deviceType string, rssi int, name string) {
	if name == "" {
		name = fmt.Sprintf("Meshtastic-%s", address)
	}

	discoveredDevice := &models.DiscoveredDevice{
		Address:      address,
		Name:         name,
		Type:         deviceType,
		RSSI:         rssi,
		Meshtastic: strings.Contains(strings.ToLower(name), "meshtastic") ||
			strings.Contains(strings.ToLower(name), "esp32"),
		LastSeen:     time.Now(),
		DiscoveredAt: time.Now(),
	}

	// Проверяем, есть ли уже такое устройство
	existing, err := s.repo.GetByAddress(discoveredDevice.Address)
	if err != nil {
		if err := s.repo.Create(discoveredDevice); err != nil {
			log.Printf("Error creating discovered device: %v", err)
		}
	} else {
		if err := s.repo.UpdateLastSeen(existing.Address, rssi); err != nil {
			log.Printf("Error updating discovered device: %v", err)
		}
	}

	log.Printf("Found %s device: %s (%s) RSSI: %d", deviceType, name, address, rssi)
}

// ScanNearbyDevices сканирует устройства поблизости
func (s *BLEScanner) ScanNearbyDevices() error {
	ctx := context.Background()
	s.scanOnce(ctx)
	return nil
}

// GetESP32MACAddress возвращает MAC адрес ESP32 если он найден
func (s *BLEScanner) GetESP32MACAddress() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Сначала пробуем сохраненный MAC
	if s.esp32MAC != "" {
		return s.esp32MAC, nil
	}

	// Ищем в базе данных
	devices, err := s.repo.GetByType("bluetooth")
	if err != nil {
		return "", err
	}

	for _, device := range devices {
		if device.Meshtastic {
			return device.Address, nil
		}
	}

	return "", fmt.Errorf("ESP32 device not found")
}

// GetAllBLEDevices возвращает все найденные BLE устройства
func (s *BLEScanner) GetAllBLEDevices() []BLEDevice {
	ctx := context.Background()
	return s.scanBLEDevices(ctx)
}
