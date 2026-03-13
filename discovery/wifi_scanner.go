package discovery

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

// WiFiScanner сканирует WiFi сеть для поиска Meshtastic устройств
type WiFiScanner struct {
	repo         *repository.DiscoveryRepository
	isScanning   bool
	mu           sync.Mutex
	cancel       context.CancelFunc
	scanInterval int // интервал сканирования в секундах
}

// Meshtastic API порты
var meshtasticPorts = []string{":80", ":443", ":8080"}

// Meshtastic HTTP пути для проверки
var meshtasticPaths = []string{
	"/hotspot-detect.html",
	"/admin",
	"/api",
}

func NewWiFiScanner(repo *repository.DiscoveryRepository) *WiFiScanner {
	return &WiFiScanner{
		repo:         repo,
		scanInterval: 30, // интервал по умолчанию
	}
}

// SetScanInterval устанавливает интервал сканирования в секундах
func (s *WiFiScanner) SetScanInterval(interval int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if interval > 0 {
		s.scanInterval = interval
	}
}

// Start начинает сканирование WiFi сети
func (s *WiFiScanner) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.isScanning {
		s.mu.Unlock()
		return nil
	}
	s.isScanning = true
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	log.Println("WiFi scanner started, scanning for Meshtastic devices...")

	// Запускаем сканирование в горутине
	go s.scanLoop(ctx)

	return nil
}

// Stop останавливает сканирование
func (s *WiFiScanner) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isScanning {
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}

	s.isScanning = false
	log.Println("WiFi scanner stopped")
	return nil
}

// IsScanning возвращает статус сканирования
func (s *WiFiScanner) IsScanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isScanning
}

func (s *WiFiScanner) scanLoop(ctx context.Context) {
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

func (s *WiFiScanner) scanOnce(ctx context.Context) {
	// Получаем локальные IP адреса
	ips, err := s.getLocalIPs()
	if err != nil {
		log.Printf("Error getting local IPs: %v", err)
		return
	}

	for _, ip := range ips {
		if err := s.scanSubnet(ctx, ip); err != nil {
			log.Printf("Error scanning subnet %s: %v", ip, err)
		}
	}
}

func (s *WiFiScanner) getLocalIPs() ([]string, error) {
	var ips []string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				// Получаем базовый IP подсети
				ip := ipNet.IP.String()
				ips = append(ips, ip)
			}
		}
	}

	return ips, nil
}

func (s *WiFiScanner) scanSubnet(ctx context.Context, localIP string) error {
	// Определяем подсеть (например, 192.168.1.x)
	parts := strings.Split(localIP, ".")
	if len(parts) != 4 {
		return fmt.Errorf("invalid IP format")
	}

	subnet := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])

	// Сканируем IP адреса в подсети (1-254)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 50) // Ограничиваем количество одновременных сканирований

	for i := 1; i <= 254; i++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(hostNum int) {
			defer wg.Done()
			defer func() { <-sem }()

			ip := fmt.Sprintf("%s.%d", subnet, hostNum)
			s.checkIP(ctx, ip)
		}(i)
	}

	wg.Wait()
	return nil
}

func (s *WiFiScanner) checkIP(ctx context.Context, ip string) {
	// Проверяем, доступен ли хост
	timeout := 500 * time.Millisecond
	conn, err := net.DialTimeout("tcp", ip+":80", timeout)
	if err != nil {
		return
	}
	conn.Close()

	// Проверяем, является ли устройство Meshtastic
	if s.isMeshtasticDevice(ctx, ip) {
		log.Printf("Found Meshtastic device at %s", ip)

		discoveredDevice := &models.DiscoveredDevice{
			Address:      ip,
			Name:         fmt.Sprintf("Meshtastic-%s", ip),
			Type:         "wifi",
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
			// Обновляем last_seen
			if err := s.repo.UpdateLastSeen(existing.Address, 0); err != nil {
				log.Printf("Error updating discovered device: %v", err)
			}
		}
	}
}

func (s *WiFiScanner) isMeshtasticDevice(ctx context.Context, ip string) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Проверяем Meshtastic paths
	for _, path := range meshtasticPaths {
		url := fmt.Sprintf("http://%s%s", ip, path)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		// Проверяем заголовки и содержимое
		if s.checkMeshtasticResponse(resp) {
			return true
		}
	}

	// Проверяем по MAC адресу (vendor OUI)
	if s.checkMeshtasticMAC(ip) {
		return true
	}

	return false
}

func (s *WiFiScanner) checkMeshtasticResponse(resp *http.Response) bool {
	// Проверяем заголовки
	server := resp.Header.Get("Server")
	if strings.Contains(strings.ToLower(server), "meshtastic") {
		return true
	}

	// Проверяем WWW-Authenticate заголовок (Meshtastic использует Basic Auth)
	auth := resp.Header.Get("WWW-Authenticate")
	if strings.Contains(strings.ToLower(auth), "meshtastic") {
		return true
	}

	// Проверяем статус
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
		return true
	}

	return false
}

func (s *WiFiScanner) checkMeshtasticMAC(ip string) bool {
	// Получаем MAC адрес через ARP
	mac, err := s.getMACAddress(ip)
	if err != nil {
		return false
	}

	// Проверяем OUI производителя (Meshtastic использует ESP32)
	// OUI для Espressif: 24:0A:C4, 30:AE:A4, 60:01:94
	macPrefix := strings.ToUpper(mac[:8])
	esp32OUIs := []string{"24:0A:C4", "30:AE:A4", "60:01:94", "84:CC:A8", "A8:48:FA"}

	for _, oui := range esp32OUIs {
		if macPrefix == oui {
			return true
		}
	}

	return false
}

func (s *WiFiScanner) getMACAddress(ip string) (string, error) {
	// Используем ARP для получения MAC адреса
	// На Windows это можно сделать через arp -a команду
	// Для кроссплатформенности используем простой подход

	// Отправляем пакет для активации ARP
	conn, err := net.DialTimeout("udp", ip+":12345", 100*time.Millisecond)
	if err != nil {
		return "", err
	}
	conn.Close()

	// Читаем ARP таблицу (Windows специфично)
	return s.readARPTable(ip)
}

func (s *WiFiScanner) readARPTable(ip string) (string, error) {
	// Для Windows используем arp -a команду
	// В реальном приложении лучше использовать syscall для вызова SendARP
	// Но для простоты вернем пустой MAC

	// Примечание: В production лучше использовать библиотеку для ARP
	// или системные вызовы для получения MAC адреса
	return "", fmt.Errorf("ARP lookup not implemented")
}

// GetNetworkInfo возвращает информацию о текущей сети
func (s *WiFiScanner) GetNetworkInfo() ([]NetworkInfo, error) {
	var networks []NetworkInfo

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					networks = append(networks, NetworkInfo{
						Interface: iface.Name,
						IP:        ipNet.IP.String(),
						Mask:      net.IP(ipNet.Mask).String(),
					})
				}
			}
		}
	}

	return networks, nil
}

// NetworkInfo информация о сетевом интерфейсе
type NetworkInfo struct {
	Interface string `json:"interface"`
	IP        string `json:"ip"`
	Mask      string `json:"mask"`
}

// BroadcastMessage отправляет UDP широковещательное сообщение для поиска устройств
func (s *WiFiScanner) BroadcastMessage(ctx context.Context, port int) error {
	// Создаем UDP соединение для broadcast
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", port))
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Включаем broadcast режим
	if err := conn.SetWriteBuffer(65536); err != nil {
		// Игнорируем ошибку, пробуем продолжить
	}

	// Отправляем Meshtastic discovery пакет
	message := []byte("MeshtasticDiscovery")
	if _, err := conn.WriteToUDP(message, addr); err != nil {
		return err
	}

	// Ждем ответы
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	buffer := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			n, remoteAddr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					return nil
				}
				continue
			}

			log.Printf("Received broadcast response from %s: %s", remoteAddr, string(buffer[:n]))
			s.processBroadcastResponse(remoteAddr.String())
		}
	}
}

func (s *WiFiScanner) processBroadcastResponse(addr string) {
	ip := strings.Split(addr, ":")[0]

	discoveredDevice := &models.DiscoveredDevice{
		Address:      ip,
		Name:         fmt.Sprintf("Meshtastic-Broadcast-%s", ip),
		Type:         "wifi",
		Meshtastic:   true,
		LastSeen:     time.Now(),
		DiscoveredAt: time.Now(),
	}

	existing, err := s.repo.GetByAddress(discoveredDevice.Address)
	if err != nil {
		if err := s.repo.Create(discoveredDevice); err != nil {
			log.Printf("Error creating discovered device: %v", err)
		}
	} else {
		if err := s.repo.UpdateLastSeen(existing.Address, 0); err != nil {
			log.Printf("Error updating discovered device: %v", err)
		}
	}
}

// ConvertIPToUint32 конвертирует IP адрес в uint32
func ConvertIPToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}

// ConvertUint32ToIP конвертирует uint32 в IP адрес
func ConvertUint32ToIP(ip uint32) net.IP {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, ip)
	return net.IP(bytes)
}
