package repository

import (
	"database/sql"

	"mesh-server/models"
)

type MessageRepository struct {
	db         *sql.DB
	deviceRepo *DeviceRepository
}

func NewMessageRepository(db *sql.DB, deviceRepo *DeviceRepository) *MessageRepository {
	return &MessageRepository{
		db:         db,
		deviceRepo: deviceRepo,
	}
}

func (r *MessageRepository) Create(msg *models.Message) error {
	query := `
		INSERT INTO messages (device_id, from_node, to_node, text, direction, sent_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(query, msg.DeviceID, msg.FromNode, msg.ToNode, msg.Text, msg.Direction, msg.SentAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	msg.ID = id
	return nil
}

// GetDeviceByNodeID находит или создает устройство по node_id
func (r *MessageRepository) GetDeviceByNodeID(nodeID string) (*models.Device, error) {
	if r.deviceRepo != nil {
		return r.deviceRepo.GetByNodeID(nodeID)
	}
	return nil, sql.ErrNoRows
}

func (r *MessageRepository) GetByDeviceID(deviceID int64, limit int) ([]models.Message, error) {
	query := `
		SELECT id, device_id, from_node, to_node, text, direction, sent_at 
		FROM messages 
		WHERE device_id = ? 
		ORDER BY sent_at DESC 
		LIMIT ?
	`
	rows, err := r.db.Query(query, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		m, err := r.scanMessageRow(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *MessageRepository) GetOutbound(limit int) ([]models.Message, error) {
	query := `
		SELECT id, device_id, from_node, to_node, text, direction, sent_at 
		FROM messages 
		WHERE direction = 'outbound' 
		ORDER BY sent_at DESC 
		LIMIT ?
	`
	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		m, err := r.scanMessageRow(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *MessageRepository) scanMessage(row *sql.Row) (*models.Message, error) {
	var m models.Message
	err := row.Scan(&m.ID, &m.DeviceID, &m.FromNode, &m.ToNode, &m.Text, &m.Direction, &m.SentAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MessageRepository) scanMessageRow(rows *sql.Rows) (models.Message, error) {
	var m models.Message
	err := rows.Scan(&m.ID, &m.DeviceID, &m.FromNode, &m.ToNode, &m.Text, &m.Direction, &m.SentAt)
	return m, err
}
