package models

import "time"

// Device представляет устройство Meshtastic
type Device struct {
	ID          int64     `json:"id"`
	NodeID      string    `json:"node_id"`
	Name        string    `json:"name"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Altitude    float64   `json:"altitude"`
	LastSeen    time.Time `json:"last_seen"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DiscoveredDevice - обнаруженное устройство через Bluetooth/WiFi
type DiscoveredDevice struct {
	ID            int64     `json:"id"`
	Address       string    `json:"address"`        // MAC адрес или IP
	Name          string    `json:"name"`           // Имя устройства
	Type          string    `json:"type"`           // "bluetooth" или "wifi"
	RSSI          int       `json:"rssi,omitempty"` // Уровень сигнала (для Bluetooth)
	Meshtastic    bool      `json:"meshtastic"`     // Является ли устройством Meshtastic
	LastSeen      time.Time `json:"last_seen"`
	DiscoveredAt  time.Time `json:"discovered_at"`
}

// Metrics - физические показатели с датчиков
type Metrics struct {
	ID          int64     `json:"id"`
	DeviceID    int64     `json:"device_id"`
	HeartRate   int       `json:"heart_rate,omitempty"`   // Пульс
	StressLevel int       `json:"stress_level,omitempty"` // Уровень стресса (0-4)
	CO2         int       `json:"co2,omitempty"`          // CO2 в ppm
	Temp        float64   `json:"temp,omitempty"`         // Температура в °C
	Humidity    float64   `json:"humidity,omitempty"`     // Влажность в %
	Battery     int       `json:"battery,omitempty"`      // Заряд батареи (%)
	Timestamp   time.Time `json:"timestamp"`
}

// HealbeMetrics - метрики специфичные для часов Healbe
type HealbeMetrics struct {
	ID          int64     `json:"id"`
	DeviceID    int64     `json:"device_id"`
	HeartRate   int       `json:"heart_rate"`   // Пульс (bpm)
	StressLevel int       `json:"stress_level"` // Уровень стресса (0-4)
	Battery     int       `json:"battery"`      // Заряд батареи (%)
	Timestamp   time.Time `json:"timestamp"`
}

// Alert - уведомление о событии (например, человек вне зоны)
type Alert struct {
	ID        int64     `json:"id"`
	DeviceID  int64     `json:"device_id"`
	Type      string    `json:"type"` // "out_of_zone", "low_battery", "signal_lost"
	Message   string    `json:"message"`
	Severity  string    `json:"severity"` // "info", "warning", "critical"
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// Message - исходящее/входящее сообщение
type Message struct {
	ID        int64     `json:"id"`
	DeviceID  int64     `json:"device_id"`
	FromNode  string    `json:"from_node"`
	ToNode    string    `json:"to_node"`
	Text      string    `json:"text"`
	Direction string    `json:"direction"` // "inbound", "outbound"
	SentAt    time.Time `json:"sent_at"`
}

// Route - маршрут сотрудника
type Route struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	DeviceID    int64           `json:"device_id,omitempty"`
	Waypoints   interface{}     `json:"waypoints"` // []ml.Point2D as JSON
	CreatedAt   time.Time       `json:"created_at"`
}

// RoutePoint - точка на маршруте (история перемещений)
type RoutePoint struct {
	ID        int64     `json:"id"`
	DeviceID  int64     `json:"device_id"`
	RouteID   *int64    `json:"route_id,omitempty"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Timestamp time.Time `json:"timestamp"`
}

// Anomaly - обнаруженная аномалия
type Anomaly struct {
	ID            int64     `json:"id"`
	DeviceID      int64     `json:"device_id"`
	Type          string    `json:"type"`
	MetricValue   float64   `json:"metric_value"`
	ExpectedRange string    `json:"expected_range,omitempty"`
	Severity      string    `json:"severity"`
	Description   string    `json:"description"`
	Timestamp     time.Time `json:"timestamp"`
}
