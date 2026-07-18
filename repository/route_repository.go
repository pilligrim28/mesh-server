package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"mesh-server/models"
)

// RouteRepository работа с маршрутами и точками
type RouteRepository struct {
	db *sql.DB
}

func NewRouteRepository(db *sql.DB) *RouteRepository {
	return &RouteRepository{db: db}
}

// Create создаёт новый маршрут
func (r *RouteRepository) Create(route *models.Route) error {
	waypointsJSON, err := json.Marshal(route.Waypoints)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(
		`INSERT INTO routes (name, description, device_id, waypoints) VALUES (?, ?, ?, ?)`,
		route.Name, route.Description, route.DeviceID, string(waypointsJSON),
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	route.ID = id
	return nil
}

// GetAll возвращает все маршруты
func (r *RouteRepository) GetAll() ([]models.Route, error) {
	rows, err := r.db.Query(`SELECT id, name, description, device_id, waypoints, created_at FROM routes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []models.Route
	for rows.Next() {
		var route models.Route
		var waypointsStr string
		if err := rows.Scan(&route.ID, &route.Name, &route.Description, &route.DeviceID, &waypointsStr, &route.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(waypointsStr), &route.Waypoints); err != nil {
			route.Waypoints = nil
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

// GetByID возвращает маршрут по ID
func (r *RouteRepository) GetByID(id int64) (*models.Route, error) {
	var route models.Route
	var waypointsStr string
	err := r.db.QueryRow(
		`SELECT id, name, description, device_id, waypoints, created_at FROM routes WHERE id = ?`, id,
	).Scan(&route.ID, &route.Name, &route.Description, &route.DeviceID, &waypointsStr, &route.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(waypointsStr), &route.Waypoints); err != nil {
		route.Waypoints = nil
	}
	return &route, nil
}

// Delete удаляет маршрут
func (r *RouteRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM routes WHERE id = ?`, id)
	return err
}

// RecordPoint записывает точку маршрута
func (r *RouteRepository) RecordPoint(deviceID int64, routeID *int64, lat, lon float64) error {
	_, err := r.db.Exec(
		`INSERT INTO route_points (device_id, route_id, latitude, longitude) VALUES (?, ?, ?, ?)`,
		deviceID, routeID, lat, lon,
	)
	return err
}

// GetPointsByPeriod возвращает точки маршрута за период
func (r *RouteRepository) GetPointsByPeriod(deviceID int64, from, to time.Time) ([]models.RoutePoint, error) {
	rows, err := r.db.Query(
		`SELECT id, device_id, route_id, latitude, longitude, timestamp
		 FROM route_points
		 WHERE device_id = ? AND timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`, deviceID, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []models.RoutePoint
	for rows.Next() {
		var p models.RoutePoint
		if err := rows.Scan(&p.ID, &p.DeviceID, &p.RouteID, &p.Latitude, &p.Longitude, &p.Timestamp); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetRecentPoints возвращает последние N точек устройства
func (r *RouteRepository) GetRecentPoints(deviceID int64, limit int) ([]models.RoutePoint, error) {
	rows, err := r.db.Query(
		`SELECT id, device_id, route_id, latitude, longitude, timestamp
		 FROM route_points WHERE device_id = ?
		 ORDER BY timestamp DESC LIMIT ?`, deviceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []models.RoutePoint
	for i := 0; rows.Next(); i++ {
		var p models.RoutePoint
		if err := rows.Scan(&p.ID, &p.DeviceID, &p.RouteID, &p.Latitude, &p.Longitude, &p.Timestamp); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	// Разворачиваем в хронологическом порядке
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points, rows.Err()
}

// SaveAnomaly сохраняет аномалию в БД
func (r *RouteRepository) SaveAnomaly(anomaly *models.Anomaly) error {
	result, err := r.db.Exec(
		`INSERT INTO anomalies (device_id, type, metric_value, expected_range, severity, description)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		anomaly.DeviceID, anomaly.Type, anomaly.MetricValue,
		anomaly.ExpectedRange, anomaly.Severity, anomaly.Description,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	anomaly.ID = id
	return nil
}

// GetAnomalies возвращает аномалии с фильтрами
func (r *RouteRepository) GetAnomalies(deviceID *int64, limit int) ([]models.Anomaly, error) {
	query := `SELECT id, device_id, type, metric_value, expected_range, severity, description, timestamp
		 FROM anomalies`
	var args []interface{}
	if deviceID != nil {
		query += ` WHERE device_id = ?`
		args = append(args, *deviceID)
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.Anomaly
	for rows.Next() {
		var a models.Anomaly
		if err := rows.Scan(&a.ID, &a.DeviceID, &a.Type, &a.MetricValue, &a.ExpectedRange, &a.Severity, &a.Description, &a.Timestamp); err != nil {
			return nil, err
		}
		results = append(results, a)
	}
	return results, rows.Err()
}
