package meshtastic

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Meshtastic BLE UUIDs (for reference)
const (
	MeshtasticServiceUUID = "6ba1b218-15a8-461f-9fa8-511d75550f30"
	MeshtasticToRadioChar = "f8925b27-2b8f-4f60-94dc-9e0810c91b8f"
	MeshtasticFromRadioChar = "85b01054-68e8-45b0-8c7b-db15387dd36c"
)

// BLEServer управляет BLE GATT сервером для Meshtastic
type BLEServer struct {
	deviceName string
	isRunning  bool
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	cmd        *exec.Cmd
}

// NewBLEServer создаёт новый BLE сервер
func NewBLEServer(deviceName string) *BLEServer {
	return &BLEServer{
		deviceName: deviceName,
	}
}

// Start запускает BLE GATT сервер
func (b *BLEServer) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.isRunning {
		return nil
	}

	if runtime.GOOS != "linux" {
		log.Printf("BLE Server: only supported on Linux, current OS: %s", runtime.GOOS)
		return fmt.Errorf("BLE server only supported on Linux")
	}

	// Проверяем наличие python3 и dbus
	if _, err := exec.LookPath("python3"); err != nil {
		log.Println("BLE Server: python3 not found")
		return fmt.Errorf("python3 not found")
	}

	// Проверяем наличие bluetoothctl
	if _, err := exec.LookPath("bluetoothctl"); err != nil {
		log.Println("BLE Server: bluetoothctl not found")
		return fmt.Errorf("bluetoothctl not found")
	}

	b.ctx, b.cancel = context.WithCancel(context.Background())

	// Находим путь к Python скрипту
	scriptPath := b.findScript()
	if scriptPath == "" {
		log.Println("BLE Server: ble_gatt_server.py not found")
		return fmt.Errorf("ble_gatt_server.py not found")
	}

	// Включаем Bluetooth
	b.enableBluetooth()

	// Запускаем Python GATT сервер
	b.cmd = exec.CommandContext(b.ctx, "python3", scriptPath, b.deviceName)
	b.cmd.Dir = filepath.Dir(scriptPath)

	// Логируем вывод
	stdout, _ := b.cmd.StdoutPipe()
	stderr, _ := b.cmd.StderrPipe()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "ERROR") {
				log.Printf("BLE GATT: %s", line)
			} else {
				log.Printf("BLE GATT: %s", line)
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("BLE GATT ERR: %s", scanner.Text())
		}
	}()

	if err := b.cmd.Start(); err != nil {
		log.Printf("BLE Server: failed to start Python GATT server: %v", err)
		return err
	}

	b.isRunning = true
	log.Printf("BLE Server: started, name='%s', pid=%d", b.deviceName, b.cmd.Process.Pid)

	// Фоновый мониторинг
	go b.monitorLoop()

	return nil
}

// Stop останавливает BLE сервер
func (b *BLEServer) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isRunning {
		return
	}

	b.isRunning = false
	b.cancel()

	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd.Wait()
	}

	log.Println("BLE Server: stopped")
}

// IsRunning возвращает статус
func (b *BLEServer) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isRunning
}

// GetMACAddress возвращает MAC адрес Bluetooth адаптера
func (b *BLEServer) GetMACAddress() string {
	cmd := exec.Command("bluetoothctl", "show")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Address:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// GetInfo возвращает информацию BLE сервера
func (b *BLEServer) GetInfo() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	info := map[string]interface{}{
		"running":     b.isRunning,
		"device_name": b.deviceName,
		"mac_address": b.GetMACAddress(),
		"service_uuid": MeshtasticServiceUUID,
		"supported":   runtime.GOOS == "linux",
	}

	if b.cmd != nil && b.cmd.Process != nil {
		info["pid"] = b.cmd.Process.Pid
	}

	return info
}

// enableBluetooth включает Bluetooth
func (b *BLEServer) enableBluetooth() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bluetoothctl", "power", "on")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("BLE Server: power on error: %v, output: %s", err, string(output))
	} else {
		log.Println("BLE Server: Bluetooth powered on")
	}
	time.Sleep(1 * time.Second)
}

// findScript находит Python скрипт
func (b *BLEServer) findScript() string {
	// Ищем относительно текущего файла
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)

	paths := []string{
		filepath.Join(dir, "ble_gatt_server.py"),
		filepath.Join(dir, "..", "meshtastic", "ble_gatt_server.py"),
		"meshtastic/ble_gatt_server.py",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			absPath, _ := filepath.Abs(p)
			return absPath
		}
	}

	return ""
}

// monitorLoop проверяет состояние процесса
func (b *BLEServer) monitorLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.mu.RLock()
			running := b.isRunning
			cmd := b.cmd
			b.mu.RUnlock()

			if running && cmd != nil && cmd.Process != nil {
				// Проверяем жив ли процесс
				if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
					log.Println("BLE Server: process died")
					b.mu.Lock()
					b.isRunning = false
					b.mu.Unlock()
				}
			}
		}
	}
}
