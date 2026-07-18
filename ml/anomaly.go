package ml

import (
	"fmt"
	"math"
	"sync"
	"time"

	"mesh-server/models"
)

// WindowSize число последних замеров для анализа
const WindowSize = 60

// Пороги для разных типов метрик
var metricThresholds = map[string]struct {
	Min       float64
	Max       float64
	StdDevMul float64 // множитель стандартного отклонения для Z-score
}{
	"heart_rate": {Min: 50, Max: 120, StdDevMul: 2.0},
	"co2":        {Min: 350, Max: 800, StdDevMul: 2.0},
	"temp":       {Min: 35.5, Max: 38.0, StdDevMul: 2.5},
	"humidity":   {Min: 20, Max: 80, StdDevMul: 2.0},
}

// AnomalyResult результат проверки на аномалию
type AnomalyResult struct {
	DeviceID     int64   `json:"device_id"`
	Type         string  `json:"type"`
	MetricValue  float64 `json:"metric_value"`
	ExpectedMin  float64 `json:"expected_min"`
	ExpectedMax  float64 `json:"expected_max"`
	Severity     string  `json:"severity"`
	Description  string  `json:"description"`
	Timestamp    time.Time `json:"timestamp"`
}

// MetricWindow скользящее окно замеров одного типа
type MetricWindow struct {
	mu     sync.RWMutex
	values []float64
}

// AnomalyDetector детектор аномалий на основе Z-score
type AnomalyDetector struct {
	mu      sync.RWMutex
	windows map[string]*MetricWindow // key: "deviceID:metricType"
}

// NewAnomalyDetector создаёт детектор аномалий
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{
		windows: make(map[string]*MetricWindow),
	}
}

// windowKey формирует ключ для окна
func windowKey(deviceID int64, metricType string) string {
	return string(rune(deviceID)) + ":" + metricType
}

// getOrCreateWindow возвращает или создаёт окно для метрики
func (d *AnomalyDetector) getOrCreateWindow(key string) *MetricWindow {
	d.mu.Lock()
	defer d.mu.Unlock()

	if w, ok := d.windows[key]; ok {
		return w
	}
	w := &MetricWindow{
		values: make([]float64, 0, WindowSize),
	}
	d.windows[key] = w
	return w
}

// Push добавляет значение в окно и проверяет на аномалию
func (d *AnomalyDetector) Push(deviceID int64, metrics *models.Metrics) []AnomalyResult {
	var results []AnomalyResult
	now := time.Now()

	check := func(metricType string, value float64) {
		if math.IsNaN(value) {
			return
		}

		threshold, ok := metricThresholds[metricType]
		if !ok {
			return
		}

		key := windowKey(deviceID, metricType)
		window := d.getOrCreateWindow(key)

		result := d.checkAnomaly(window, metricType, value, threshold, deviceID, now)
		if result != nil {
			results = append(results, *result)
		}
	}

	if metrics.HeartRate > 0 {
		check("heart_rate", float64(metrics.HeartRate))
	}
	if metrics.CO2 > 0 {
		check("co2", float64(metrics.CO2))
	}
	if metrics.Temp > 0 {
		check("temp", metrics.Temp)
	}
	if metrics.Humidity > 0 {
		check("humidity", metrics.Humidity)
	}

	return results
}

// CheckStuckPosition проверяет застревание устройства (координаты не меняются)
func (d *AnomalyDetector) CheckStuckPosition(deviceID int64, lat, lon float64, lastUpdate time.Time, activity string) *AnomalyResult {
	if activity != "walking" && activity != "running" {
		return nil
	}
	if time.Since(lastUpdate) > 10*time.Minute {
		return &AnomalyResult{
			DeviceID:    deviceID,
			Type:        "position_stuck",
			MetricValue: 0,
			ExpectedMin: 0,
			ExpectedMax: float64(time.Since(lastUpdate).Seconds()),
			Severity:    "warning",
			Description: "Устройство не двигается более 10 минут",
			Timestamp:   time.Now(),
		}
	}
	return nil
}

// checkAnomaly проверяет одно значение на аномалию
func (d *AnomalyDetector) checkAnomaly(window *MetricWindow, metricType string, value float64, threshold struct {
	Min       float64
	Max       float64
	StdDevMul float64
}, deviceID int64, now time.Time) *AnomalyResult {
	// Добавляем значение в окно
	window.mu.Lock()
	window.values = append(window.values, value)
	if len(window.values) > WindowSize {
		window.values = window.values[len(window.values)-WindowSize:]
	}
	window.mu.Unlock()

	// Если данных для анализа недостаточно, пропускаем
	window.mu.RLock()
	n := len(window.values)
	window.mu.RUnlock()
	if n < 10 {
		return nil
	}

	// Вычисляем среднее и стандартное отклонение
	window.mu.RLock()
	mean, stddev := meanStddev(window.values)
	window.mu.RUnlock()

	// Z-score аномалия
	zScore := math.Abs(value-mean) / math.Max(stddev, 0.001)

	// Проверка по порогам
	belowMin := value < threshold.Min
	aboveMax := value > threshold.Max
	zAnomaly := zScore > threshold.StdDevMul

	if !belowMin && !aboveMax && !zAnomaly {
		return nil
	}

	severity := "warning"
	if belowMin || aboveMax {
		severity = "critical"
	}

	desc := formatAnomalyDesc(metricType, value, mean, stddev)

	return &AnomalyResult{
		DeviceID:    deviceID,
		Type:        metricType + "_anomaly",
		MetricValue: value,
		ExpectedMin: threshold.Min,
		ExpectedMax: threshold.Max,
		Severity:    severity,
		Description: desc,
		Timestamp:   now,
	}
}

func meanStddev(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(values))
	return mean, math.Sqrt(variance)
}

func formatAnomalyDesc(metricType string, value, mean, stddev float64) string {
	switch metricType {
	case "heart_rate":
		return formatMetric("Пульс", int(value), "bpm", int(mean))
	case "co2":
		return formatMetric("CO₂", int(value), "ppm", int(mean))
	case "temp":
		return formatMetric("Температура", value, "°C", mean)
	case "humidity":
		return formatMetric("Влажность", value, "%", mean)
	default:
		return formatMetric(metricType, value, "", mean)
	}
}

func formatMetric(name string, value interface{}, unit string, mean interface{}) string {
	return fmt.Sprintf("%s: %v %s (среднее: %v %s)", name, value, unit, mean, unit)
}
