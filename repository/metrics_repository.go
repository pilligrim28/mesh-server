package repository

import (
	"database/sql"

	"mesh-server/models"
)

type MetricsRepository struct {
	db *sql.DB
}

func NewMetricsRepository(db *sql.DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

func (r *MetricsRepository) Create(metrics *models.Metrics) error {
	query := `
		INSERT INTO metrics (device_id, heart_rate, co2, temp, humidity, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(query, metrics.DeviceID, metrics.HeartRate, metrics.CO2, metrics.Temp, metrics.Humidity, metrics.Timestamp)
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

func (r *MetricsRepository) GetByDeviceID(deviceID int64, limit int) ([]models.Metrics, error) {
	query := `
		SELECT id, device_id, heart_rate, co2, temp, humidity, timestamp 
		FROM metrics 
		WHERE device_id = ? 
		ORDER BY timestamp DESC 
		LIMIT ?
	`
	rows, err := r.db.Query(query, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.Metrics
	for rows.Next() {
		m, err := r.scanMetricsRow(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

func (r *MetricsRepository) GetLatest(deviceID int64) (*models.Metrics, error) {
	query := `
		SELECT id, device_id, heart_rate, co2, temp, humidity, timestamp 
		FROM metrics 
		WHERE device_id = ? 
		ORDER BY timestamp DESC 
		LIMIT 1
	`
	row := r.db.QueryRow(query, deviceID)
	return r.scanMetrics(row)
}

func (r *MetricsRepository) GetAllLatest() ([]models.Metrics, error) {
	query := `
		SELECT m.id, m.device_id, m.heart_rate, m.co2, m.temp, m.humidity, m.timestamp
		FROM metrics m
		INNER JOIN (
			SELECT device_id, MAX(timestamp) as max_ts
			FROM metrics
			GROUP BY device_id
		) latest ON m.device_id = latest.device_id AND m.timestamp = latest.max_ts
		ORDER BY m.timestamp DESC
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.Metrics
	for rows.Next() {
		m, err := r.scanMetricsRow(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

func (r *MetricsRepository) scanMetrics(row *sql.Row) (*models.Metrics, error) {
	var m models.Metrics
	err := row.Scan(&m.ID, &m.DeviceID, &m.HeartRate, &m.CO2, &m.Temp, &m.Humidity, &m.Timestamp)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MetricsRepository) scanMetricsRow(rows *sql.Rows) (models.Metrics, error) {
	var m models.Metrics
	err := rows.Scan(&m.ID, &m.DeviceID, &m.HeartRate, &m.CO2, &m.Temp, &m.Humidity, &m.Timestamp)
	return m, err
}
