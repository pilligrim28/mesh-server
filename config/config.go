package config

import (
	"os"
	"strconv"
)

type Config struct {
	ServerPort           string
	DatabasePath         string
	EnableBluetooth      bool
	EnableWiFi           bool
	DiscoveryInterval    int
	MeshtasticIP         string
	ESP32URL             string
	ESP32COMPort         string
	MQTTEnabled          bool
	MQTTServer           string
	MQTTUsername         string
	MQTTPassword         string
	MQTTRootTopic        string
	MQTTMapReporting     bool
	ESP32HubEnabled      bool
	ESP32HubPollInterval int
}

func Load() *Config {
	cfg := &Config{
		ServerPort:         getEnv("SERVER_PORT", "8080"),
		DatabasePath:       getEnv("DATABASE_PATH", "mesh-server.db"),
		EnableBluetooth:    getEnvBool("ENABLE_BLUETOOTH", true),
		EnableWiFi:         getEnvBool("ENABLE_WIFI", true),
		DiscoveryInterval:  getEnvInt("DISCOVERY_INTERVAL", 30),
		MeshtasticIP:       getEnv("MESHTASTIC_IP", ""),
		ESP32URL:           getEnv("ESP32_URL", ""),
		ESP32COMPort:       getEnv("ESP32_COM_PORT", ""),
		MQTTEnabled:        getEnvBool("MQTT_ENABLED", false),
		MQTTServer:         getEnv("MQTT_SERVER", "mqtt.meshtastic.org"),
		MQTTUsername:       getEnv("MQTT_USERNAME", "meshdev"),
		MQTTPassword:       getEnv("MQTT_PASSWORD", "large4cats"),
		MQTTRootTopic:      getEnv("MQTT_ROOT_TOPIC", "msh/RU"),
		MQTTMapReporting:   getEnvBool("MQTT_MAP_REPORTING", false),
		ESP32HubEnabled:    getEnvBool("ESP32_HUB_ENABLED", true),
		ESP32HubPollInterval: getEnvInt("ESP32_HUB_POLL_INTERVAL", 5),
	}
	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		b, err := strconv.ParseBool(value)
		if err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		i, err := strconv.Atoi(value)
		if err == nil {
			return i
		}
	}
	return defaultValue
}
