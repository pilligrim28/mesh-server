package discovery

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

// BLEScanner сканирует Bluetooth LE устройства
type BLEScanner struct {
	repo         *repository.DiscoveryRepository
	isScanning   bool
	mu           sync.Mutex
	cancel       context.CancelFunc
	scanInterval int
	esp32MAC     string
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
	devices := s.scanBLEDevices(ctx)

	for _, device := range devices {
		s.foundDevice(device.Address, "bluetooth", device.RSSI, device.Name)

		if device.IsESP32 {
			s.esp32MAC = device.Address
			log.Printf("Found ESP32 MAC: %s", device.Address)
		}
	}
}

// scanBLEDevices сканирует BLE устройства (Linux или Windows)
func (s *BLEScanner) scanBLEDevices(ctx context.Context) []BLEDevice {
	if runtime.GOOS == "linux" {
		return s.scanBLELinux(ctx)
	}
	return s.scanBLEWindows(ctx)
}

// scanBLELinux сканирует BLE устройства через bluetoothctl / hcitool
func (s *BLEScanner) scanBLELinux(ctx context.Context) []BLEDevice {
	// Способ 1: bluetoothctl devices
	devices := s.scanViaBluetoothctl(ctx)
	if len(devices) > 0 {
		return devices
	}

	// Способ 2: hcitool lescan (если bluetoothctl не дал результатов)
	return s.scanViaHcitool(ctx)
}

// scanViaBluetoothctl сканирует через bluetoothctl devices
func (s *BLEScanner) scanViaBluetoothctl(ctx context.Context) []BLEDevice {
	var devices []BLEDevice

	// Проверяем включён ли Bluetooth
	if !s.isBluetoothEnabled() {
		log.Println("Bluetooth is not enabled, attempting to power on...")
		s.enableBluetooth()
	}

	// Получаем список уже известных BLE устройств
	cmd := exec.Command("bluetoothctl", "devices")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("bluetoothctl devices error: %v", err)
		return devices
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		device := s.parseBluetoothctlLine(line)
		if device.Address != "" {
			devices = append(devices, device)
		}
	}

	// Запускаем сканирование на 5 секунд для обнаружения новых устройств
	scanDevices := s.triggerScan(ctx, 5*time.Second)
	devices = append(devices, scanDevices...)

	return devices
}

// triggerScan запускает.bluetoothctl scan on, ждёт, затем scan off
func (s *BLEScanner) triggerScan(ctx context.Context, duration time.Duration) []BLEDevice {
	var devices []BLEDevice

	// Включаем сканирование
	startCmd := exec.Command("bluetoothctl", "scan", "on")
	if err := startCmd.Start(); err != nil {
		log.Printf("Failed to start bluetoothctl scan: %v", err)
		return devices
	}

	// Ждём указанное время
	select {
	case <-ctx.Done():
	case <-time.After(duration):
	}

	// Останавливаем сканирование
	stopCmd := exec.Command("bluetoothctl", "scan", "off")
	stopCmd.Run()

	// Читаем результаты
	cmd := exec.Command("bluetoothctl", "devices")
	output, err := cmd.Output()
	if err != nil {
		return devices
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		device := s.parseBluetoothctlLine(line)
		if device.Address != "" {
			devices = append(devices, device)
		}
	}

	return devices
}

// parseBluetoothctlLine парсит строку "Device AA:BB:CC:DD:EE:FF Name"
func (s *BLEScanner) parseBluetoothctlLine(line string) BLEDevice {
	device := BLEDevice{}

	// Формат: "Device AA:BB:CC:DD:DD:EE Name" или "Device XX:XX:XX:XX:XX:XX"
	if !strings.HasPrefix(line, "Device ") {
		return device
	}

	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		return device
	}

	device.Address = parts[1]
	if len(parts) >= 3 {
		device.Name = parts[2]
	}

	// Проверяем является ли устройство ESP32/Meshtastic
	lowerName := strings.ToLower(device.Name)
	device.IsESP32 = strings.Contains(lowerName, "esp32") ||
		strings.Contains(lowerName, "meshtastic") ||
		strings.Contains(lowerName, "lilygo") ||
		strings.Contains(lowerName, "heltec") ||
		strings.Contains(lowerName, "tbeam") ||
		strings.Contains(lowerName, "tlora")
	device.IsMeshtastic = device.IsESP32

	return device
}

// isBluetoothEnabled проверяет включён ли Bluetooth
func (s *BLEScanner) isBluetoothEnabled() bool {
	cmd := exec.Command("bluetoothctl", "show")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Ищем "Powered: yes" или "Powered: no"
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Powered:") {
			return strings.Contains(line, "yes")
		}
	}
	return false
}

// enableBluetooth включает Bluetooth
func (s *BLEScanner) enableBluetooth() {
	cmd := exec.Command("bluetoothctl", "power", "on")
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to power on bluetooth: %v", err)
		return
	}
	log.Println("Bluetooth powered on")
	time.Sleep(2 * time.Second)
}

// scanViaHcitool сканирует через hcitool lescan (альтернатива)
func (s *BLEScanner) scanViaHcitool(ctx context.Context) []BLEDevice {
	var devices []BLEDevice

	// Проверяем доступность hcitool
	if _, err := exec.LookPath("hcitool"); err != nil {
		log.Println("hcitool not found, skipping")
		return devices
	}

	// Запускаем lescan на 5 секунд
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx2, "hcitool", "lescan")
	output, err := cmd.CombinedOutput()
	if err != nil && ctx2.Err() == nil {
		log.Printf("hcitool lescan error: %v", err)
		return devices
	}

	// Парсим вывод hcitool lescan
	// Формат: "XX:XX:XX:XX:XX:XX (unknown)"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "LE") || strings.HasPrefix(line, "OK") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 1 {
			continue
		}

		address := parts[0]
		name := ""
		if len(parts) >= 2 {
			// Убираем скобки: "(unknown)" -> ""
			name = strings.Trim(parts[1], "()")
			if name == "unknown" || name == "" {
				name = ""
			}
		}

		isESP32 := strings.Contains(strings.ToLower(name), "esp32") ||
			strings.Contains(strings.ToLower(name), "meshtastic") ||
			strings.Contains(strings.ToLower(name), "lilygo") ||
			strings.Contains(strings.ToLower(name), "heltec")

		devices = append(devices, BLEDevice{
			Address:      address,
			Name:         name,
			RSSI:         -50, // hcitool не показывает RSSI в lescan
			IsESP32:      isESP32,
			IsMeshtastic: isESP32,
		})
	}

	return devices
}

// scanBLEWindows сканирует BLE устройства через PowerShell (оригинальная реализация)
func (s *BLEScanner) scanBLEWindows(ctx context.Context) []BLEDevice {
	var devices []BLEDevice

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

	output, err := s.runCommand("powershell", "-Command", powerShellScript)
	if err != nil {
		log.Printf("PowerShell BLE scan error: %v", err)
		return s.scanBLEWindowsFallback()
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

// scanBLEWindowsFallback альтернативный метод через Get-PnpDevice
func (s *BLEScanner) scanBLEWindowsFallback() []BLEDevice {
	var devices []BLEDevice

	cmd := exec.Command("powershell", "-Command", `
Get-PnpDevice -Class Bluetooth | Where-Object {$_.Status -eq 'OK'} | Select-Object FriendlyName
`)

	output, err := cmd.Output()
	if err != nil {
		log.Printf("Windows fallback BLE scan error: %v", err)
		return devices
	}

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
				Address:      "UNKNOWN",
				Name:         line,
				RSSI:         -50,
				IsESP32:      true,
				IsMeshtastic: true,
			})
		}
	}

	return devices
}

// runCommand выполняет команду (powershell / bat / sh)
func (s *BLEScanner) runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
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

	if s.esp32MAC != "" {
		return s.esp32MAC, nil
	}

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
