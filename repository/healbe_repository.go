package repository

import (
	"database/sql"
	"time"

	"mesh-server/models"
)

// HealbeRepository работает с метриками часов Healbe
type HealbeRepository struct {
	db *sql.DB
}

// NewHealbeRepository создает новый репозиторий
func NewHealbeRepository(db *sql.DB) *HealbeRepository {
	return &HealbeRepository{db: db}
}

// CreateTable создает таблицу для Healbe метрик
func (r *HealbeRepository) CreateTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS healbe_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id INTEGER NOT NULL,
		heart_rate INTEGER,
		stress_level INTEGER,
		battery INTEGER,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (device_id) REFERENCES devices(id)
	)`
	_, err := r.db.Exec(query)
	return err
}

// Create сохраняет новые метрики Healbe
func (r *HealbeRepository) Create(metrics *models.HealbeMetrics) error {
	query := `
		INSERT INTO healbe_metrics (device_id, heart_rate, stress_level, battery, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(query, metrics.DeviceID, metrics.HeartRate, metrics.StressLevel, metrics.Battery, metrics.Timestamp)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	metrics.ID = id
	return nil
}

// GetLatest возвращает последние метрики для устройства
func (r *HealbeRepository) GetLatest(deviceID int64, limit int) ([]*models.HealbeMetrics, error) {
	query := `
		SELECT id, device_id, heart_rate, stress_level, battery, timestamp
		FROM healbe_metrics
		WHERE device_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := r.db.Query(query, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []*models.HealbeMetrics
	for rows.Next() {
		m := &models.HealbeMetrics{}
		var timestampStr string
		err := rows.Scan(&m.ID, &m.DeviceID, &m.HeartRate, &m.StressLevel, &m.Battery, &timestampStr)
		if err != nil {
			continue
		}
		m.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestampStr)
		metrics = append(metrics, m)
	}

	return metrics, nil
}

// GetLatestForAll возвращает последние метрики для всех устройств
func (r *HealbeRepository) GetLatestForAll(limit int) ([]*models.HealbeMetrics, error) {
	query := `
		SELECT hm.id, hm.device_id, hm.heart_rate, hm.stress_level, hm.battery, hm.timestamp
		FROM healbe_metrics hm
		INNER JOIN (
			SELECT device_id, MAX(timestamp) as max_ts
			FROM healbe_metrics
			GROUP BY device_id
		) latest ON hm.device_id = latest.device_id AND hm.timestamp = latest.max_ts
		ORDER BY hm.timestamp DESC
		LIMIT ?
	`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []*models.HealbeMetrics
	for rows.Next() {
		m := &models.HealbeMetrics{}
		var timestampStr string
		err := rows.Scan(&m.ID, &m.DeviceID, &m.HeartRate, &m.StressLevel, &m.Battery, &timestampStr)
		if err != nil {
			continue
		}
		m.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestampStr)
		metrics = append(metrics, m)
	}

	return metrics, nil
}