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
	HealbeMAC            string
	HealbeMode           string
	HealbeForwardMesh    bool
	HealbeBridgeURL      string
	DemoMode             bool
	PeopleSimEnabled     bool
	PeopleSimCount       int
	PeopleSimRadiusKm    float64
	PeopleSimCenterLat   float64
	PeopleSimCenterLon   float64
	PeopleSimInterval    int
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
		HealbeMAC:          getEnv("HEALBE_MAC", ""),
		HealbeMode:         getEnv("HEALBE_MODE", "auto"),
		HealbeForwardMesh:  getEnvBool("HEALBE_FORWARD_MESH", false),
		HealbeBridgeURL:    getEnv("HEALBE_BRIDGE_URL", ""),
		DemoMode:           getEnvBool("DEMO_MODE", false),
		PeopleSimEnabled:   getEnvBool("PEOPLE_SIM_ENABLED", false),
		PeopleSimCount:     getEnvInt("PEOPLE_SIM_COUNT", 8),
		PeopleSimRadiusKm:  getEnvFloat("PEOPLE_SIM_RADIUS_KM", 2),
		PeopleSimCenterLat: getEnvFloat("PEOPLE_SIM_CENTER_LAT", 59.9343),
		PeopleSimCenterLon: getEnvFloat("PEOPLE_SIM_CENTER_LON", 30.3351),
		PeopleSimInterval:  getEnvInt("PEOPLE_SIM_INTERVAL", 5),
	}

	if cfg.DemoMode {
		applyDemoDefaults(cfg)
	}

	return cfg
}

func applyDemoDefaults(cfg *Config) {
	cfg.EnableBluetooth = false
	cfg.EnableWiFi = false
	cfg.ESP32HubEnabled = false
	cfg.MQTTEnabled = false
	cfg.DiscoveryInterval = 60

	if cfg.ESP32COMPort == "" {
		cfg.ESP32COMPort = "COM4"
	}
	if cfg.HealbeMAC == "" {
		cfg.HealbeMAC = "8B:20:91:8E:F5:CB"
	}
	if cfg.HealbeMode == "" || cfg.HealbeMode == "auto" {
		cfg.HealbeMode = "demo"
	}
	cfg.HealbeForwardMesh = true
	cfg.PeopleSimEnabled = true
	if cfg.PeopleSimCount < 6 {
		cfg.PeopleSimCount = 8
	}
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

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		f, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return f
		}
	}
	return defaultValue
}
