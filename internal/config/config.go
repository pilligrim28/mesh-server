// Package config загружает настройки из переменных окружения и файла .env.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr string
	DBPath   string

	// Транспорты: ble, serial, sim (можно несколько через запятую).
	Transports []string

	// BLE: автопоиск шлюзов Meshtastic.
	BLENameFilter  string   // подстрока имени (пусто = любой узел Meshtastic)
	BLEAddresses   []string // разрешённые адреса (пусто = любой)
	BLEMaxGateways int      // сколько шлюзов держать подключёнными одновременно
	BLEScanTimeout time.Duration

	SerialPort string // COM4 или /dev/ttyUSB0 (USB-подключение шлюза)

	SimNodes int // количество виртуальных узлов в режиме sim

	AdminUser     string
	AdminPassword string

	HRLow         int
	HRHigh        int
	SpO2Low       int
	OfflineAfter  time.Duration
	RetentionDays int
}

func Load() Config {
	loadDotEnv(".env")
	return Config{
		HTTPAddr:       str("HTTP_ADDR", ":8080"),
		DBPath:         str("DB_PATH", "mesh-server.db"),
		Transports:     list("TRANSPORT", "ble"),
		BLENameFilter:  str("BLE_NAME_FILTER", "GW_"),
		BLEAddresses:   list("BLE_ADDRESSES", ""),
		BLEMaxGateways: num("BLE_MAX_GATEWAYS", 1),
		BLEScanTimeout: time.Duration(num("BLE_SCAN_SECONDS", 10)) * time.Second,
		SerialPort:     str("SERIAL_PORT", ""),
		SimNodes:       num("SIM_NODES", 3),
		AdminUser:      str("ADMIN_USER", "admin"),
		AdminPassword:  str("ADMIN_PASSWORD", "admin"),
		HRLow:          num("HR_LOW", 45),
		HRHigh:         num("HR_HIGH", 150),
		SpO2Low:        num("SPO2_LOW", 90),
		OfflineAfter:   time.Duration(num("OFFLINE_AFTER_SECONDS", 180)) * time.Second,
		RetentionDays:  num("RETENTION_DAYS", 30),
	}
}

func (c Config) Has(transport string) bool {
	for _, t := range c.Transports {
		if t == transport {
			return true
		}
	}
	return false
}

func str(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func num(k string, def int) int {
	if n, err := strconv.Atoi(str(k, "")); err == nil {
		return n
	}
	return def
}

func list(k, def string) []string {
	var out []string
	for _, p := range strings.Split(str(k, def), ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadDotEnv читает KEY=VALUE из файла, не перезаписывая уже заданные переменные.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}
