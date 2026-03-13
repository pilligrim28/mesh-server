package repository

import (
	"database/sql"
	"time"

	"mesh-server/models"
)

type DiscoveryRepository struct {
	db *sql.DB
}

func NewDiscoveryRepository(db *sql.DB) *DiscoveryRepository {
	return &DiscoveryRepository{db: db}
}

// Create создает запись об обнаруженном устройстве
func (r *DiscoveryRepository) Create(device *models.DiscoveredDevice) error {
	query := `
		INSERT INTO discovered_devices (address, name, type, rssi, meshtastic, last_seen, discovered_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(query, device.Address, device.Name, device.Type, device.RSSI, device.Meshtastic, device.LastSeen, device.DiscoveredAt)
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

// GetByAddress находит устройство по адресу (MAC или IP)
func (r *DiscoveryRepository) GetByAddress(address string) (*models.DiscoveredDevice, error) {
	query := `SELECT id, address, name, type, rssi, meshtastic, last_seen, discovered_at
			  FROM discovered_devices WHERE address = ?`
	return r.scanDevice(r.db.QueryRow(query, address))
}

// GetAll возвращает все обнаруженные устройства
func (r *DiscoveryRepository) GetAll() ([]models.DiscoveredDevice, error) {
	query := `SELECT id, address, name, type, rssi, meshtastic, last_seen, discovered_at
			  FROM discovered_devices ORDER BY discovered_at DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.DiscoveredDevice
	for rows.Next() {
		device, err := r.scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

// GetByType возвращает устройства по типу сканирования
func (r *DiscoveryRepository) GetByType(scanType string) ([]models.DiscoveredDevice, error) {
	query := `SELECT id, address, name, type, rssi, meshtastic, last_seen, discovered_at
			  FROM discovered_devices WHERE type = ? ORDER BY discovered_at DESC`
	rows, err := r.db.Query(query, scanType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.DiscoveredDevice
	for rows.Next() {
		device, err := r.scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

// GetMeshtasticDevices возвращает только Meshtastic устройства
func (r *DiscoveryRepository) GetMeshtasticDevices() ([]models.DiscoveredDevice, error) {
	query := `SELECT id, address, name, type, rssi, meshtastic, last_seen, discovered_at
			  FROM discovered_devices WHERE meshtastic = 1 ORDER BY discovered_at DESC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.DiscoveredDevice
	for rows.Next() {
		device, err := r.scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

// UpdateLastSeen обновляет время последнего обнаружения
func (r *DiscoveryRepository) UpdateLastSeen(address string, rssi int) error {
	query := `UPDATE discovered_devices SET last_seen = ?, rssi = ? WHERE address = ?`
	_, err := r.db.Exec(query, time.Now(), rssi, address)
	return err
}

// DeleteOld удаляет устройства старше указанного времени
func (r *DiscoveryRepository) DeleteOld(olderThan time.Time) error {
	query := `DELETE FROM discovered_devices WHERE discovered_at < ?`
	_, err := r.db.Exec(query, olderThan)
	return err
}

// Clear очищает таблицу обнаруженных устройств
func (r *DiscoveryRepository) Clear() error {
	_, err := r.db.Exec(`DELETE FROM discovered_devices`)
	return err
}

func (r *DiscoveryRepository) scanDevice(row *sql.Row) (*models.DiscoveredDevice, error) {
	var d models.DiscoveredDevice
	err := row.Scan(&d.ID, &d.Address, &d.Name, &d.Type, &d.RSSI, &d.Meshtastic, &d.LastSeen, &d.DiscoveredAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DiscoveryRepository) scanDeviceRow(rows *sql.Rows) (models.DiscoveredDevice, error) {
	var d models.DiscoveredDevice
	err := rows.Scan(&d.ID, &d.Address, &d.Name, &d.Type, &d.RSSI, &d.Meshtastic, &d.LastSeen, &d.DiscoveredAt)
	return d, err
}
