package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

// BluetoothScanner сканирует Bluetooth LE устройства
// Примечание: Для работы Bluetooth на Windows требуется поддержка BLE
type BluetoothScanner struct {
	repo         *repository.DiscoveryRepository
	isScanning   bool
	mu           sync.Mutex
	cancel       context.CancelFunc
	scanInterval int // интервал сканирования в секундах
}

func NewBluetoothScanner(repo *repository.DiscoveryRepository) *BluetoothScanner {
	return &BluetoothScanner{
		repo:         repo,
		scanInterval: 30, // интервал по умолчанию
	}
}

// SetScanInterval устанавливает интервал сканирования в секундах
func (s *BluetoothScanner) SetScanInterval(interval int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if interval > 0 {
		s.scanInterval = interval
	}
}

// Start начинает сканирование Bluetooth устройств
func (s *BluetoothScanner) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.isScanning {
		s.mu.Unlock()
		return nil
	}
	s.isScanning = true
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	log.Println("Bluetooth scanner started (simulated mode - requires BLE hardware)")

	// Запускаем сканирование в горутине
	go s.scanLoop(ctx)

	return nil
}

// Stop останавливает сканирование
func (s *BluetoothScanner) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isScanning {
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}

	s.isScanning = false
	log.Println("Bluetooth scanner stopped")
	return nil
}

// IsScanning возвращает статус сканирования
func (s *BluetoothScanner) IsScanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isScanning
}

func (s *BluetoothScanner) scanLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.scanOnce()
			time.Sleep(time.Duration(s.scanInterval) * time.Second)
		}
	}
}

func (s *BluetoothScanner) scanOnce() {
	// Сканирование Bluetooth устройств через обнаружение BLE
	// На Windows это требует использования Windows Runtime API
	// Для кроссплатформенности используем эмуляцию через UDP multicast

	// Meshtastic использует UUID службы: 0x531e5f7c122b66399d28d1a44cd50000
	// Также ищем устройства с именем содержащим "Meshtastic" или "ESP32"

	// Отправляем multicast запрос для обнаружения BLE устройств
	s.scanBLEMulticast()
}

func (s *BluetoothScanner) scanBLEMulticast() {
	// Используем mDNS/Bonjour для обнаружения Meshtastic устройств
	// Meshtastic устройства могут рекламировать себя через mDNS

	addr, err := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
	if err != nil {
		return
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// mDNS запрос для поиска Meshtastic устройств
	query := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x09, 0x5f, 0x6d, 0x65, 0x73, 0x68, 0x74, 0x61, 0x73, 0x74, 0x69, 0x63,
		0x04, 0x5f, 0x74, 0x63, 0x70, 0x05, 0x6c, 0x6f, 0x63, 0x61, 0x6c, 0x00,
		0x00, 0x0c, 0x00, 0x01,
	}

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.WriteToUDP(query, addr)

	// Ждем ответы
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buffer := make([]byte, 1024)

	for {
		_, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return
			}
			continue
		}

		// Парсим ответ (упрощенно)
		ip := strings.Split(remoteAddr.String(), ":")[0]
		s.foundDevice(ip, "bluetooth", -50)
	}
}

func (s *BluetoothScanner) foundDevice(address, deviceType string, rssi int) {
	name := fmt.Sprintf("Meshtastic-%s", address)

	discoveredDevice := &models.DiscoveredDevice{
		Address:      address,
		Name:         name,
		Type:         deviceType,
		RSSI:         rssi,
		Meshtastic:   true,
		LastSeen:     time.Now(),
		DiscoveredAt: time.Now(),
	}

	// Проверяем, есть ли уже такое устройство
	existing, err := s.repo.GetByAddress(discoveredDevice.Address)
	if err != nil {
		// Устройство не найдено, создаем новое
		if err := s.repo.Create(discoveredDevice); err != nil {
			log.Printf("Error creating discovered device: %v", err)
		}
	} else {
		// Обновляем last_seen и RSSI
		if err := s.repo.UpdateLastSeen(existing.Address, rssi); err != nil {
			log.Printf("Error updating discovered device: %v", err)
		}
	}

	log.Printf("Found %s device: %s (%s) RSSI: %d", deviceType, name, address, rssi)
}

// ScanNearbyDevices сканирует устройства поблизости через UDP multicast
func (s *BluetoothScanner) ScanNearbyDevices() error {
	// Отправляем UDP пакет на multicast адрес для обнаружения устройств
	multicastAddr := "239.255.255.250:1900" // SSDP адрес

	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// SSDP M-SEARCH запрос
	message := []byte("M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 3\r\n" +
		"ST: ssdp:all\r\n\r\n")

	if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}

	_, err = conn.WriteToUDP(message, addr)
	if err != nil {
		return err
	}

	// Ждем ответы
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}

	buffer := make([]byte, 2048)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil
			}
			continue
		}

		response := string(buffer[:n])
		if strings.Contains(strings.ToLower(response), "meshtastic") ||
			strings.Contains(strings.ToLower(response), "esp32") {
			ip := strings.Split(remoteAddr.String(), ":")[0]
			s.foundDevice(ip, "network", -60) // Сетевое обнаружение
		}
	}
}
