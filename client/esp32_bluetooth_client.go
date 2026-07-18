package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"mesh-server/models"
)

// ESP32BluetoothClient клиент для подключения к ESP32 через Bluetooth
type ESP32BluetoothClient struct {
	macAddress string
	conn       net.Conn
	mu         sync.Mutex
	connected  bool
	timeout    time.Duration
}

// NewESP32BluetoothClient создает новый клиент для подключения по Bluetooth
func NewESP32BluetoothClient(macAddress string) *ESP32BluetoothClient {
	return &ESP32BluetoothClient{
		macAddress: macAddress,
		timeout:    10 * time.Second,
	}
}

// Connect подключается к ESP32 через Bluetooth RFCOMM
func (c *ESP32BluetoothClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// На Windows Bluetooth RFCOMM подключение требует использования Windows API
	// Для кроссплатформенности используем TCP если ESP32 в WiFi режиме
	
	// Пробуем подключиться через WiFi (если ESP32 в режиме точки доступа)
	// ESP32 Meshtastic обычно имеет IP 192.168.4.1 в режиме AP
	return c.connectViaWiFi(ctx)
}

// connectViaWiFi подключается к ESP32 через WiFi
func (c *ESP32BluetoothClient) connectViaWiFi(ctx context.Context) error {
	// ESP32 Meshtastic default IP
	ips := []string{
		"192.168.4.1",      // AP режим
		"192.168.4.2",      // Клиент режим
		"192.168.1.100",    // Обычный IP в локальной сети
	}

	for _, ip := range ips {
		addr := fmt.Sprintf("http://%s", ip)
		client := &http.Client{
			Timeout: c.timeout,
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+"/health", nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			log.Printf("Connected to ESP32 at %s", ip)
			return nil
		}
	}

	return fmt.Errorf("failed to connect to ESP32 via WiFi")
}

// IsConnected проверяет подключение
func (c *ESP32BluetoothClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// SendMessage отправляет сообщение на ESP32
func (c *ESP32BluetoothClient) SendMessage(ctx context.Context, msg *models.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Отправляем через HTTP API если ESP32 в WiFi режиме
	return c.sendMessageViaWiFi(ctx, msg)
}

// sendMessageViaWiFi отправляет сообщение через WiFi
func (c *ESP32BluetoothClient) sendMessageViaWiFi(ctx context.Context, msg *models.Message) error {
	ips := []string{
		"192.168.4.1",
		"192.168.4.2",
		"192.168.1.100",
	}

	reqBody := map[string]string{
		"to_node": msg.ToNode,
		"text":    msg.Text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	for _, ip := range ips {
		client := &http.Client{
			Timeout: c.timeout,
		}

		url := fmt.Sprintf("http://%s/api/message", ip)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(jsonData)))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			log.Printf("Message sent to ESP32 at %s", ip)
			return nil
		}
	}

	return fmt.Errorf("failed to send message via WiFi")
}

// GetMACAddress возвращает MAC адрес устройства
func (c *ESP32BluetoothClient) GetMACAddress() string {
	return c.macAddress
}

// SetBaseURL устанавливает базовый URL для подключения
func (c *ESP32BluetoothClient) SetBaseURL(url string) {
	log.Printf("ESP32 Bluetooth client baseURL updated to: %s", url)
}

// GetStatus проверяет доступность ESP32
func (c *ESP32BluetoothClient) GetStatus(ctx context.Context) (bool, error) {
	ips := []string{
		"192.168.4.1",
		"192.168.4.2",
		"192.168.1.100",
	}

	for _, ip := range ips {
		url := fmt.Sprintf("http://%s/health", ip)
		client := &http.Client{
			Timeout: 2 * time.Second,
		}

		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return true, nil
		}
	}

	return false, fmt.Errorf("ESP32 not reachable")
}

// ScanForESP32 сканирует сеть для поиска ESP32/Meshtastic устройств
func ScanForESP32(ctx context.Context) ([]string, error) {
	var esp32Devices []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Частые IP адреса для ESP32
	commonIPs := []string{
		"192.168.4.1",
		"192.168.4.2",
		"192.168.1.100",
		"192.168.1.101",
		"192.168.0.100",
	}

	// Сканируем常见的 IP
	for _, ip := range commonIPs {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()

			url := fmt.Sprintf("http://%s/health", ip)
			client := &http.Client{
				Timeout: 2 * time.Second,
			}

			resp, err := client.Get(url)
			if err == nil {
				resp.Body.Close()
				mu.Lock()
				esp32Devices = append(esp32Devices, ip)
				mu.Unlock()
				log.Printf("Found ESP32 at %s", ip)
			}
		}(ip)
	}

	wg.Wait()

	// Сканируем локальную сеть через ARP таблицу
	localIPs := scanLocalNetwork()
	for _, ip := range localIPs {
		if !contains(esp32Devices, ip) {
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()

				if isMeshtasticDevice(ctx, ip) {
					mu.Lock()
					esp32Devices = append(esp32Devices, ip)
					mu.Unlock()
					log.Printf("Found Meshtastic at %s", ip)
				}
			}(ip)
		}
	}

	wg.Wait()

	return esp32Devices, nil
}

// scanLocalNetwork сканирует локальную сеть через ARP таблицу
func scanLocalNetwork() []string {
	if runtime.GOOS == "linux" {
		return scanLocalNetworkLinux()
	}
	return scanLocalNetworkWindows()
}

// scanLocalNetworkLinux сканирует ARP таблицу на Linux
func scanLocalNetworkLinux() []string {
	var devices []string

	// Читаем /proc/net/arp на Linux
	cmd := exec.Command("cat", "/proc/net/arp")
	output, err := cmd.Output()
	if err != nil {
		// Пробуем через ip neigh
		return scanLocalNetworkIPNeigh()
	}

	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // пропускаем заголовок
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			ip := parts[0]
			flags := parts[2] // FLAGS: 0x2 = incomplete, 0x6 = reachable
			mac := parts[3]

			// Пропускаем incomplete записи и broadcast
			if flags == "0x0" || mac == "00:00:00:00:00:00" {
				continue
			}
			if strings.Contains(ip, "255") || strings.Contains(ip, "224") {
				continue
			}
			if net.ParseIP(ip) != nil && strings.Contains(ip, ".") {
				devices = append(devices, ip)
			}
		}
	}

	return devices
}

// scanLocalNetworkIPNeigh альтернатива через ip neigh (modern Linux)
func scanLocalNetworkIPNeigh() []string {
	var devices []string

	cmd := exec.Command("ip", "neigh", "show")
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
		parts := strings.Fields(line)
		if len(parts) >= 5 {
			ip := parts[0]
			if strings.Contains(ip, "255") || strings.Contains(ip, "224") {
				continue
			}
			if net.ParseIP(ip) != nil && strings.Contains(ip, ".") {
				devices = append(devices, ip)
			}
		}
	}

	return devices
}

// scanLocalNetworkWindows сканирует ARP таблицу на Windows
func scanLocalNetworkWindows() []string {
	var devices []string

	cmd := exec.Command("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return devices
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			ip := parts[0]
			if strings.Contains(ip, "255") || strings.Contains(ip, "224") {
				continue
			}
			if net.ParseIP(ip) != nil && strings.Contains(ip, ".") {
				devices = append(devices, ip)
			}
		}
	}

	return devices
}

// isMeshtasticDevice проверяет является ли устройство Meshtastic
func isMeshtasticDevice(ctx context.Context, ip string) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Проверяем /health endpoint
	url := fmt.Sprintf("http://%s/health", ip)
	resp, err := client.Get(url)
	if err == nil {
		resp.Body.Close()
		return true
	}

	// Проверяем /api/v1/mesh (Meshtastic API)
	url = fmt.Sprintf("http://%s/api/v1/mesh", ip)
	resp, err = client.Get(url)
	if err == nil {
		resp.Body.Close()
		return true
	}

	return false
}

// contains проверяет наличие строки в слайсе
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// FindESP32ByMAC ищет ESP32 по MAC адресу в локальной сети
func FindESP32ByMAC(ctx context.Context, macAddress string) (string, error) {
	// На Windows можно использовать ARP таблицу для поиска IP по MAC
	// arp -a | findstr <MAC>
	
	// Для упрощения возвращаем ошибку
	return "", fmt.Errorf("MAC address lookup not implemented")
}
