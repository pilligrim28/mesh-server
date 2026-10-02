// Package store — хранилище SQLite (pure Go, без CGO): узлы, показания, алерты, пользователи.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type Node struct {
	Num        uint32     `json:"num"`
	NodeID     string     `json:"node_id"`
	LongName   string     `json:"long_name"`
	ShortName  string     `json:"short_name"`
	Alias      string     `json:"alias"`
	HWModel    string     `json:"hw_model"`
	Battery    *int       `json:"battery"`
	LastSeen   time.Time  `json:"last_seen"`
	LastRSSI   int        `json:"last_rssi"`
	LastSNR    float64    `json:"last_snr"`
	HopsAway   int        `json:"hops_away"`
	Gateway    string     `json:"gateway"`
	HeartBPM   *int       `json:"heart_bpm"`
	SpO2       *int       `json:"spo2"`
	LastHealth *time.Time `json:"last_health"`
}

type Reading struct {
	ID       int64     `json:"id"`
	NodeNum  uint32    `json:"node_num"`
	Time     time.Time `json:"time"`
	HeartBPM *int      `json:"heart_bpm"`
	SpO2     *int      `json:"spo2"`
	RSSI     int       `json:"rssi"`
	SNR      float64   `json:"snr"`
	Gateway  string    `json:"gateway"`
}

type Alert struct {
	ID       int64      `json:"id"`
	NodeNum  uint32     `json:"node_num"`
	Time     time.Time  `json:"time"`
	Kind     string     `json:"kind"`
	Message  string     `json:"message"`
	Value    *int       `json:"value"`
	Resolved *time.Time `json:"resolved"`
	Acked    bool       `json:"acked"`
}

var ErrNotFound = errors.New("not found")

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS nodes (
  num INTEGER PRIMARY KEY, long_name TEXT DEFAULT '', short_name TEXT DEFAULT '', alias TEXT DEFAULT '',
  hw_model TEXT DEFAULT '', battery INTEGER, last_seen INTEGER DEFAULT 0, last_rssi INTEGER DEFAULT 0,
  last_snr REAL DEFAULT 0, hops_away INTEGER DEFAULT 0, gateway TEXT DEFAULT '',
  heart_bpm INTEGER, spo2 INTEGER, last_health INTEGER
);
CREATE TABLE IF NOT EXISTS readings (
  id INTEGER PRIMARY KEY AUTOINCREMENT, node_num INTEGER NOT NULL, ts INTEGER NOT NULL,
  heart_bpm INTEGER, spo2 INTEGER, rssi INTEGER, snr REAL, gateway TEXT
);
CREATE INDEX IF NOT EXISTS idx_readings_node_ts ON readings(node_num, ts);
CREATE TABLE IF NOT EXISTS alerts (
  id INTEGER PRIMARY KEY AUTOINCREMENT, node_num INTEGER NOT NULL, ts INTEGER NOT NULL, kind TEXT NOT NULL,
  message TEXT NOT NULL, value INTEGER, resolved INTEGER, acked INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_alerts_open ON alerts(node_num, kind, resolved);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires INTEGER NOT NULL
);`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: одна запись за раз, без "database is locked"
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---------- узлы ----------

func (s *Store) UpsertNodeInfo(num uint32, long, short, hw, gw string) error {
	_, err := s.db.Exec(`INSERT INTO nodes(num, long_name, short_name, hw_model, gateway) VALUES(?,?,?,?,?)
	  ON CONFLICT(num) DO UPDATE SET long_name=excluded.long_name, short_name=excluded.short_name,
	  hw_model=CASE WHEN excluded.hw_model<>'' THEN excluded.hw_model ELSE hw_model END`, num, long, short, hw, gw)
	return err
}

func (s *Store) SetBattery(num uint32, level int) error {
	_, err := s.db.Exec(`INSERT INTO nodes(num, battery) VALUES(?,?) ON CONFLICT(num) DO UPDATE SET battery=excluded.battery`, num, level)
	return err
}

func (s *Store) SetAlias(num uint32, alias string) error {
	r, err := s.db.Exec(`UPDATE nodes SET alias=? WHERE num=?`, alias, num)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteNode(num uint32) error {
	_, err := s.db.Exec(`DELETE FROM readings WHERE node_num=?; DELETE FROM alerts WHERE node_num=?; DELETE FROM nodes WHERE num=?`, num, num, num)
	return err
}

// AddReading сохраняет показание и обновляет «последнее состояние» узла.
func (s *Store) AddReading(r Reading, hops int) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	ts := r.Time.UnixMilli()
	res, err := tx.Exec(`INSERT INTO readings(node_num, ts, heart_bpm, spo2, rssi, snr, gateway) VALUES(?,?,?,?,?,?,?)`,
		r.NodeNum, ts, r.HeartBPM, r.SpO2, r.RSSI, r.SNR, r.Gateway)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`INSERT INTO nodes(num, last_seen, last_rssi, last_snr, hops_away, gateway, heart_bpm, spo2, last_health)
	  VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(num) DO UPDATE SET last_seen=excluded.last_seen, last_rssi=excluded.last_rssi,
	  last_snr=excluded.last_snr, hops_away=excluded.hops_away, gateway=excluded.gateway, heart_bpm=excluded.heart_bpm,
	  spo2=excluded.spo2, last_health=excluded.last_health`,
		r.NodeNum, ts, r.RSSI, r.SNR, hops, r.Gateway, r.HeartBPM, r.SpO2, ts); err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, tx.Commit()
}

func (s *Store) Nodes() ([]Node, error) {
	rows, err := s.db.Query(`SELECT num, long_name, short_name, alias, hw_model, battery, last_seen, last_rssi, last_snr,
	  hops_away, gateway, heart_bpm, spo2, last_health FROM nodes ORDER BY COALESCE(NULLIF(alias,''), long_name, num)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		var n Node
		var seen int64
		var health sql.NullInt64
		if err := rows.Scan(&n.Num, &n.LongName, &n.ShortName, &n.Alias, &n.HWModel, &n.Battery, &seen, &n.LastRSSI,
			&n.LastSNR, &n.HopsAway, &n.Gateway, &n.HeartBPM, &n.SpO2, &health); err != nil {
			return nil, err
		}
		n.NodeID = fmt.Sprintf("!%08x", n.Num)
		n.LastSeen = time.UnixMilli(seen)
		if health.Valid {
			t := time.UnixMilli(health.Int64)
			n.LastHealth = &t
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) Readings(num uint32, since time.Time, limit int) ([]Reading, error) {
	rows, err := s.db.Query(`SELECT id, node_num, ts, heart_bpm, spo2, rssi, snr, gateway FROM
	  (SELECT * FROM readings WHERE node_num=? AND ts>=? ORDER BY ts DESC LIMIT ?) ORDER BY ts`, num, since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reading{}
	for rows.Next() {
		var r Reading
		var ts int64
		if err := rows.Scan(&r.ID, &r.NodeNum, &ts, &r.HeartBPM, &r.SpO2, &r.RSSI, &r.SNR, &r.Gateway); err != nil {
			return nil, err
		}
		r.Time = time.UnixMilli(ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Prune(olderThan time.Time) error {
	_, err := s.db.Exec(`DELETE FROM readings WHERE ts<?; DELETE FROM alerts WHERE resolved IS NOT NULL AND ts<?`,
		olderThan.UnixMilli(), olderThan.UnixMilli())
	return err
}

// ---------- алерты ----------

// OpenAlert создаёт алерт, если такого же открытого ещё нет. Возвращает nil, если алерт уже открыт.
func (s *Store) OpenAlert(num uint32, kind, msg string, value *int) (*Alert, error) {
	var exists int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE node_num=? AND kind=? AND resolved IS NULL`, num, kind).Scan(&exists)
	if exists > 0 {
		return nil, nil
	}
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO alerts(node_num, ts, kind, message, value) VALUES(?,?,?,?,?)`, num, now.UnixMilli(), kind, msg, value)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Alert{ID: id, NodeNum: num, Time: now, Kind: kind, Message: msg, Value: value}, nil
}

// ResolveAlerts закрывает открытые алерты указанных типов. Возвращает число закрытых.
func (s *Store) ResolveAlerts(num uint32, kinds ...string) (int64, error) {
	var total int64
	for _, k := range kinds {
		r, err := s.db.Exec(`UPDATE alerts SET resolved=? WHERE node_num=? AND kind=? AND resolved IS NULL`, time.Now().UnixMilli(), num, k)
		if err != nil {
			return total, err
		}
		n, _ := r.RowsAffected()
		total += n
	}
	return total, nil
}

func (s *Store) AckAlert(id int64) error {
	r, err := s.db.Exec(`UPDATE alerts SET acked=1 WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Alerts(onlyActive bool, limit int) ([]Alert, error) {
	q := `SELECT id, node_num, ts, kind, message, value, resolved, acked FROM alerts`
	if onlyActive {
		q += ` WHERE resolved IS NULL OR acked=0`
	}
	rows, err := s.db.Query(q+` ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		var ts int64
		var resolved sql.NullInt64
		if err := rows.Scan(&a.ID, &a.NodeNum, &ts, &a.Kind, &a.Message, &a.Value, &resolved, &a.Acked); err != nil {
			return nil, err
		}
		a.Time = time.UnixMilli(ts)
		if resolved.Valid {
			t := time.UnixMilli(resolved.Int64)
			a.Resolved = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------- пользователи и сессии ----------

// EnsureUser создаёт пользователя, если его ещё нет (пароль существующего не меняется).
func (s *Store) EnsureUser(username, password string) (created bool, err error) {
	var n int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, username).Scan(&n); err != nil || n > 0 {
		return false, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO users(username, password_hash) VALUES(?,?)`, username, string(hash))
	return err == nil, err
}

func (s *Store) SetPassword(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE users SET password_hash=? WHERE username=?`, string(hash), username)
	return err
}

func (s *Store) CheckPassword(username, password string) (int64, bool) {
	var id int64
	var hash string
	if s.db.QueryRow(`SELECT id, password_hash FROM users WHERE username=?`, username).Scan(&id, &hash) != nil {
		bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinva"), []byte(password))
		return 0, false
	}
	return id, bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Store) CreateSession(userID int64, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := s.db.Exec(`INSERT INTO sessions(token, user_id, expires) VALUES(?,?,?)`, token, userID, time.Now().Add(ttl).Unix())
	return token, err
}

// SessionUser возвращает имя пользователя по токену сессии.
func (s *Store) SessionUser(token string) (string, bool) {
	var name string
	err := s.db.QueryRow(`SELECT u.username FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND s.expires>?`,
		token, time.Now().Unix()).Scan(&name)
	return name, err == nil
}

func (s *Store) DeleteSession(token string) {
	s.db.Exec(`DELETE FROM sessions WHERE token=? OR expires<?`, token, time.Now().Unix())
}
