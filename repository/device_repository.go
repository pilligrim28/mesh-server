package repository

import (
	"database/sql"
	"time"

	"mesh-server/models"
)

type DeviceRepository struct {
	db *sql.DB
}

func NewDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) Create(device *models.Device) error {
	query := `
		INSERT INTO devices (node_id, name, latitude, longitude, altitude, last_seen)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(query, device.NodeID, device.Name, device.Latitude, device.Longitude, device.Altitude, device.LastSeen)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	device.ID = id
	return nil
}

func (r *DeviceRepository) GetByID(id int64) (*models.Device, error) {
	query := `SELECT id, node_id, name, latitude, longitude, altitude, last_seen, created_at, updated_at 
			  FROM devices WHERE id = ?`
	return r.scanDevice(r.db.QueryRow(query, id))
}

func (r *DeviceRepository) GetByNodeID(nodeID string) (*models.Device, error) {
	query := `SELECT id, node_id, name, latitude, longitude, altitude, last_seen, created_at, updated_at 
			  FROM devices WHERE node_id = ?`
	return r.scanDevice(r.db.QueryRow(query, nodeID))
}

func (r *DeviceRepository) GetAll() ([]models.Device, error) {
	query := `SELECT id, node_id, name, latitude, longitude, altitude, last_seen, created_at, updated_at 
			  FROM devices ORDER BY last_seen DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		device, err := r.scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (r *DeviceRepository) UpdatePosition(id int64, lat, lon, alt float64) error {
	query := `UPDATE devices SET latitude = ?, longitude = ?, altitude = ?, last_seen = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, lat, lon, alt, time.Now(), time.Now(), id)
	return err
}

func (r *DeviceRepository) UpdateLastSeen(id int64) error {
	query := `UPDATE devices SET last_seen = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, time.Now(), time.Now(), id)
	return err
}

func (r *DeviceRepository) scanDevice(row *sql.Row) (*models.Device, error) {
	var d models.Device
	err := row.Scan(&d.ID, &d.NodeID, &d.Name, &d.Latitude, &d.Longitude, &d.Altitude, &d.LastSeen, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeviceRepository) scanDeviceRow(rows *sql.Rows) (models.Device, error) {
	var d models.Device
	err := rows.Scan(&d.ID, &d.NodeID, &d.Name, &d.Latitude, &d.Longitude, &d.Altitude, &d.LastSeen, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
