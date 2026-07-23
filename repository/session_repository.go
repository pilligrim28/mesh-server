package repository

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"mesh-server/models"
)

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// GenerateToken creates a cryptographically random session token
func (r *SessionRepository) GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (r *SessionRepository) Create(userID int64, expiresAt time.Time) (*models.Session, error) {
	token, err := r.GenerateToken()
	if err != nil {
		return nil, err
	}

	result, err := r.db.Exec(
		`INSERT INTO sessions (user_id, token, expires_at, created_at)
		 VALUES (?, ?, ?, ?)`,
		userID, token, expiresAt, time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	return &models.Session{
		ID:        id,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

func (r *SessionRepository) GetByToken(token string) (*models.Session, error) {
	s := &models.Session{}
	err := r.db.QueryRow(
		`SELECT id, user_id, token, expires_at, created_at
		 FROM sessions WHERE token = ? AND expires_at > datetime('now')`, token,
	).Scan(&s.ID, &s.UserID, &s.Token, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found or expired")
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	return s, nil
}

func (r *SessionRepository) Delete(token string) error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *SessionRepository) DeleteExpired() error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE expires_at <= datetime('now')`)
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
