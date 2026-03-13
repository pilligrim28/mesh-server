package repository

import (
	"database/sql"

	"mesh-server/models"
)

type AlertRepository struct {
	db *sql.DB
}

func NewAlertRepository(db *sql.DB) *AlertRepository {
	return &AlertRepository{db: db}
}

func (r *AlertRepository) Create(alert *models.Alert) error {
	query := `
		INSERT INTO alerts (device_id, type, message, severity, is_read, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(query, alert.DeviceID, alert.Type, alert.Message, alert.Severity, alert.IsRead, alert.CreatedAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	alert.ID = id
	return nil
}

func (r *AlertRepository) GetByID(id int64) (*models.Alert, error) {
	query := `SELECT id, device_id, type, message, severity, is_read, created_at FROM alerts WHERE id = ?`
	return r.scanAlert(r.db.QueryRow(query, id))
}

func (r *AlertRepository) GetByDeviceID(deviceID int64) ([]models.Alert, error) {
	query := `
		SELECT id, device_id, type, message, severity, is_read, created_at 
		FROM alerts 
		WHERE device_id = ? 
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		a, err := r.scanAlertRow(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (r *AlertRepository) GetUnread() ([]models.Alert, error) {
	query := `
		SELECT id, device_id, type, message, severity, is_read, created_at 
		FROM alerts 
		WHERE is_read = FALSE 
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		a, err := r.scanAlertRow(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (r *AlertRepository) MarkAsRead(id int64) error {
	query := `UPDATE alerts SET is_read = TRUE WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *AlertRepository) MarkAllAsRead(deviceID int64) error {
	query := `UPDATE alerts SET is_read = TRUE WHERE device_id = ?`
	_, err := r.db.Exec(query, deviceID)
	return err
}

func (r *AlertRepository) scanAlert(row *sql.Row) (*models.Alert, error) {
	var a models.Alert
	err := row.Scan(&a.ID, &a.DeviceID, &a.Type, &a.Message, &a.Severity, &a.IsRead, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AlertRepository) scanAlertRow(rows *sql.Rows) (models.Alert, error) {
	var a models.Alert
	err := rows.Scan(&a.ID, &a.DeviceID, &a.Type, &a.Message, &a.Severity, &a.IsRead, &a.CreatedAt)
	return a, err
}
