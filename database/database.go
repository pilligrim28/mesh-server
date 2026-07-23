package database

import (
	"database/sql"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"

	_ "modernc.org/sqlite"
)

type Database struct {
	DB *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Оптимизация SQLite для локальной работы:
	// - WAL режим для конкурентных чтений/записей
	// - single writer для избегания блокировок
	// - busy timeout вместо мгновенных ошибок
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-8000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			log.Printf("Warning: PRAGMA %s failed: %v", p, err)
		}
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	database := &Database{DB: db}

	if err := database.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	// Очистка старых записей обнаруженных устройств при запуске
	if _, err := db.Exec(`DELETE FROM discovered_devices WHERE last_seen < datetime('now', '-7 days')`); err != nil {
		log.Printf("Warning: failed to cleanup old discovered_devices: %v", err)
	}

	return database, nil
}

func (d *Database) Close() error {
	return d.DB.Close()
}

func (d *Database) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id TEXT UNIQUE NOT NULL,
			name TEXT,
			latitude REAL,
			longitude REAL,
			altitude REAL,
			last_seen DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id INTEGER NOT NULL,
			heart_rate INTEGER,
			co2 INTEGER,
			temp REAL,
			humidity REAL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
		)`,

		`CREATE TABLE IF NOT EXISTS alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			message TEXT,
			severity TEXT DEFAULT 'info',
			is_read BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
		)`,

		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id INTEGER NOT NULL,
			from_node TEXT NOT NULL,
			to_node TEXT,
			text TEXT NOT NULL,
			direction TEXT DEFAULT 'outbound',
			sent_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
		)`,

		`CREATE INDEX IF NOT EXISTS idx_metrics_device_id ON metrics(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_device_id ON alerts(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_is_read ON alerts(is_read)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_device_id ON messages(device_id)`,

		`CREATE TABLE IF NOT EXISTS discovered_devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			address TEXT UNIQUE NOT NULL,
			name TEXT,
			type TEXT NOT NULL,
			rssi INTEGER,
			meshtastic BOOLEAN DEFAULT FALSE,
			last_seen DATETIME,
			discovered_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_discovered_devices_type ON discovered_devices(type)`,
		`CREATE INDEX IF NOT EXISTS idx_discovered_devices_meshtastic ON discovered_devices(meshtastic)`,
		`CREATE INDEX IF NOT EXISTS idx_discovered_devices_discovered_at ON discovered_devices(discovered_at)`,

		// Маршруты сотрудников
		`CREATE TABLE IF NOT EXISTS routes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			device_id INTEGER,
			waypoints TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE SET NULL
		)`,

		// История пройденных точек
		`CREATE TABLE IF NOT EXISTS route_points (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id INTEGER NOT NULL,
			route_id INTEGER,
			latitude REAL NOT NULL,
			longitude REAL NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
			FOREIGN KEY (route_id) REFERENCES routes(id) ON DELETE SET NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_route_points_device ON route_points(device_id, timestamp)`,

		// Обнаруженные аномалии
		`CREATE TABLE IF NOT EXISTS anomalies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			metric_value REAL,
			expected_range TEXT,
			severity TEXT DEFAULT 'warning',
			description TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_anomalies_device ON anomalies(device_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_anomalies_type ON anomalies(type)`,

		// Пользователи и сессии
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			email TEXT DEFAULT '',
			name TEXT DEFAULT '',
			password TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,

		// Healbe метрики
		`CREATE TABLE IF NOT EXISTS healbe_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id INTEGER NOT NULL,
			heart_rate INTEGER,
			stress_level INTEGER,
			battery INTEGER,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_healbe_metrics_device ON healbe_metrics(device_id)`,
	}

	for _, migration := range migrations {
		if _, err := d.DB.Exec(migration); err != nil {
			return fmt.Errorf("migration error: %w", err)
		}
	}

	log.Println("Database migrations completed successfully")

	// Создаем демо-пользователя при каждом запуске (доступен всегда)
	if err := d.createDemoUser(); err != nil {
		log.Printf("Warning: Failed to create demo user: %v", err)
	}

	// Создаем устройство по умолчанию для переписки
	if err := d.createDefaultDevice(); err != nil {
		log.Printf("Warning: Failed to create default device: %v", err)
	}

	return nil
}

// createDemoUser создает демо-пользователя если он не существует
func (d *Database) createDemoUser() error {
	var count int
	err := d.DB.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'demo'").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		_, err = d.DB.Exec(`
			INSERT INTO users (username, email, name, password)
			VALUES ('demo', 'demo@example.com', 'Демо-пользователь', ?)
		`, string(hash))
		if err != nil {
			return err
		}
		log.Println("Demo user created: demo / demo")
	} else {
		log.Println("Demo user already exists")
	}

	return nil
}

// createDefaultDevice создает устройство по умолчанию если оно не существует
func (d *Database) createDefaultDevice() error {
	// Проверяем существует ли уже устройство
	var count int
	err := d.DB.QueryRow("SELECT COUNT(*) FROM devices WHERE node_id = '!default'").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Создаем устройство по умолчанию
		_, err := d.DB.Exec(`
			INSERT INTO devices (node_id, name, latitude, longitude, altitude, last_seen)
			VALUES ('!default', 'Default ESP32 Device', 0, 0, 0, datetime('now'))
		`)
		if err != nil {
			return err
		}
		log.Println("Default device created with node_id: !default")
	} else {
		log.Println("Default device already exists")
	}

	return nil
}
