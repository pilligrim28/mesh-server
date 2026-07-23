package repository

import (
	"database/sql"
	"fmt"
	"time"

	"mesh-server/models"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user *models.User) error {
	now := time.Now()
	result, err := r.db.Exec(
		`INSERT INTO users (username, email, name, password, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		user.Username, user.Email, user.Name, user.Password, now, now,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	user.ID = id
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

func (r *UserRepository) GetByID(id int64) (*models.User, error) {
	user := &models.User{}
	err := r.db.QueryRow(
		`SELECT id, username, COALESCE(email,''), COALESCE(name,''), password, created_at, updated_at
		 FROM users WHERE id = ?`, id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Name, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	user := &models.User{}
	err := r.db.QueryRow(
		`SELECT id, username, COALESCE(email,''), COALESCE(name,''), password, created_at, updated_at
		 FROM users WHERE username = ?`, username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Name, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return user, nil
}

func (r *UserRepository) Update(user *models.User) error {
	user.UpdatedAt = time.Now()
	_, err := r.db.Exec(
		`UPDATE users SET email = ?, name = ?, updated_at = ? WHERE id = ?`,
		user.Email, user.Name, user.UpdatedAt, user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdatePassword(id int64, hash string) error {
	_, err := r.db.Exec(
		`UPDATE users SET password = ?, updated_at = ? WHERE id = ?`,
		hash, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}
