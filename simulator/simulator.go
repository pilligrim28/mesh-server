package simulator

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

// RoutePoint точка маршрута для отрисовки на карте
type RoutePoint struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Name string  `json:"name,omitempty"`
	Ts   int64   `json:"ts"`
}

// Location названная точка на карте
type Location struct {
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Alt     float64 `json:"alt"`
	Zone    string  `json:"zone"`
	Activity string  `json:"activity"`
}

var predefinedLocations = []Location{
	{Name: "Невский проспект (старт)", Lat: 59.9343, Lon: 30.3351, Alt: 10, Zone: "safe", Activity: "walking"},
	{Name: "Казанский собор", Lat: 59.9341, Lon: 30.3242, Alt: 10, Zone: "safe", Activity: "walking"},
	{Name: "Площадь Восстания", Lat: 59.9316, Lon: 30.3604, Alt: 10, Zone: "safe", Activity: "walking"},
	{Name: "Московский вокзал", Lat: 59.9302, Lon: 30.3618, Alt: 12, Zone: "safe", Activity: "standing"},
	{Name: "Садовая улица", Lat: 59.9271, Lon: 30.3185, Alt: 10, Zone: "safe", Activity: "walking"},
	{Name: "Мариинский театр", Lat: 59.9258, Lon: 30.2956, Alt: 10, Zone: "safe", Activity: "standing"},
	{Name: "Эрмитаж", Lat: 59.9398, Lon: 30.3146, Alt: 10, Zone: "safe", Activity: "walking"},
	{Name: "Дворцовая площадь", Lat: 59.9403, Lon: 30.3150, Alt: 10, Zone: "safe", Activity: "standing"},
	{Name: "Петропавловская крепость", Lat: 59.9497, Lon: 30.3162, Alt: 12, Zone: "safe", Activity: "walking"},
	{Name: "Исаакиевский собор", Lat: 59.9341, Lon: 30.3062, Alt: 15, Zone: "safe", Activity: "walking"},
	{Name: "Спас на Крови", Lat: 59.9400, Lon: 30.3289, Alt: 10, Zone: "safe", Activity: "walking"},
	{Name: "Смольный собор", Lat: 59.9490, Lon: 30.3951, Alt: 15, Zone: "safe", Activity: "walking"},
	{Name: "Летний сад", Lat: 59.9470, Lon: 30.3362, Alt: 5, Zone: "safe", Activity: "walking"},
	{Name: "Адмиралтейство", Lat: 59.9367, Lon: 30.3120, Alt: 10, Zone: "safe", Activity: "standing"},
	{Name: "Вне зоны (тест)", Lat: 59.8500, Lon: 30.2000, Alt: 10, Zone: "restricted", Activity: "running"},
}

var predefinedRoutes = map[string][]string{
	"Невский проспект": {
		"Невский проспект (старт)", "Казанский собор", "Площадь Восстания",
		"Московский вокзал", "Садовая улица", "Мариинский театр",
		"Садовая улица", "Казанский собор", "Невский проспект (старт)",
	},
	"Эрмитаж": {
		"Эрмитаж", "Дворцовая площадь", "Адмиралтейство",
		"Исаакиевский собор", "Спас на Крови", "Невский проспект (старт)",
		"Казанский собор", "Эрмитаж",
	},
	"Петропавловка": {
		"Петропавловская крепость", "Летний сад", "Невский проспект (старт)",
		"Казанский собор", "Площадь Восстания", "Летний сад",
		"Петропавловская крепость",
	},
	"Забег": {
		"Невский проспект (старт)", "Площадь Восстания", "Садовая улица",
		"Мариинский театр", "Садовая улица", "Площадь Восстания",
		"Невский проспект (старт)",
	},
	"Вне зоны": {
		"Невский проспект (старт)", "Вне зоны (тест)", "Невский проспект (старт)",
	},
}

// 8 цветов для маркеров на карте
var simColors = []string{
	"#4ecca3", "#ff6b6b", "#ffd93d", "#6bcbff",
	"#c084fc", "#fb923c", "#34d399", "#f472b6",
}

// 8 имен для симуляторов
var simNames = []string{
	"Алексей", "Мария", "Дмитрий", "Анна",
	"Сергей", "Елена", "Николай", "Ольга",
}

const maxRouteHistory = 500

// Simulator генерирует тестовые данные для одного устройства
type Simulator struct {
	ID          int
	deviceRepo  *repository.DeviceRepository
	metricsRepo *repository.MetricsRepository
	alertRepo   *repository.AlertRepository
	device      *models.Device
	ticker      *time.Ticker
	stopChan    chan struct{}
	mu          sync.RWMutex
	isRunning   bool

	baseHeartRate int
	baseTemp      float64
	baseCO2       int

	currentRoute    []string
	routeIndex      int
	currentLocation *Location
	targetLocation  *Location
	moveProgress    float64
	moveSpeed       float64
	isRandomMode    bool

	routeHistory []RoutePoint
}

// NewSimulator создаёт новый симулятор
func NewSimulator(
	id int,
	deviceRepo *repository.DeviceRepository,
	metricsRepo *repository.MetricsRepository,
	alertRepo *repository.AlertRepository,
) *Simulator {
	return &Simulator{
		ID:            id,
		deviceRepo:    deviceRepo,
		metricsRepo:   metricsRepo,
		alertRepo:     alertRepo,
		stopChan:      make(chan struct{}),
		baseHeartRate: 65 + rand.Intn(20), // каждый со своим базовым пульсом
		baseTemp:      36.4 + rand.Float64()*0.4,
		baseCO2:       380 + rand.Intn(40),
		moveSpeed:     0.8 + rand.Float64()*0.8, // 0.8 - 1.6
		routeHistory:  make([]RoutePoint, 0),
	}
}

// Name возвращает имя симулятора
func (s *Simulator) Name() string {
	if s.ID >= 0 && s.ID < len(simNames) {
		return simNames[s.ID]
	}
	return fmt.Sprintf("Sim-%d", s.ID)
}

// Color возвращает цвет маркера
func (s *Simulator) Color() string {
	if s.ID >= 0 && s.ID < len(simColors) {
		return simColors[s.ID]
	}
	return "#ffffff"
}

// Start запускает симуляцию
func (s *Simulator) Start(interval time.Duration) {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.routeHistory = make([]RoutePoint, 0)
	s.mu.Unlock()

	if err := s.initDevice(); err != nil {
		log.Printf("Simulator[%s]: failed to init device: %v", s.Name(), err)
		return
	}

	// Каждый стартует с рандомной точки
	startIdx := rand.Intn(len(predefinedLocations))
	startLoc := &predefinedLocations[startIdx]
	s.mu.Lock()
	s.currentLocation = startLoc
	s.targetLocation = startLoc
	s.device.Latitude = startLoc.Lat
	s.device.Longitude = startLoc.Lon
	s.device.Altitude = startLoc.Alt
	s.routeHistory = append(s.routeHistory, RoutePoint{
		Lat: startLoc.Lat, Lon: startLoc.Lon, Name: startLoc.Name, Ts: time.Now().Unix(),
	})
	s.mu.Unlock()

	s.deviceRepo.UpdatePosition(s.device.ID, startLoc.Lat, startLoc.Lon, startLoc.Alt)

	s.ticker = time.NewTicker(interval)
	go s.run()
	log.Printf("Simulator[%s]: started, color=%s, start=%s", s.Name(), s.Color(), startLoc.Name)
}

// Stop останавливает симуляцию
func (s *Simulator) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	s.isRunning = false
	close(s.stopChan)
	s.stopChan = make(chan struct{})
	if s.ticker != nil {
		s.ticker.Stop()
	}
	log.Printf("Simulator[%s]: stopped", s.Name())
}

// IsRunning возвращает статус
func (s *Simulator) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// SetRoute устанавливает маршрут
func (s *Simulator) SetRoute(routeName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if routeName == "Случайный" {
		s.isRandomMode = true
		s.currentRoute = nil
		s.routeIndex = 0
		s.moveProgress = 0

		startIdx := rand.Intn(len(predefinedLocations))
		startLoc := &predefinedLocations[startIdx]
		s.currentLocation = startLoc
		s.targetLocation = startLoc
		s.device.Latitude = startLoc.Lat
		s.device.Longitude = startLoc.Lon
		s.device.Altitude = startLoc.Alt
		s.deviceRepo.UpdatePosition(s.device.ID, startLoc.Lat, startLoc.Lon, startLoc.Alt)
		s.routeHistory = append(s.routeHistory, RoutePoint{
			Lat: startLoc.Lat, Lon: startLoc.Lon, Name: startLoc.Name, Ts: time.Now().Unix(),
		})
		log.Printf("Simulator[%s]: random mode, start: %s", s.Name(), startLoc.Name)
		return
	}

	s.isRandomMode = false
	route, ok := predefinedRoutes[routeName]
	if !ok {
		return
	}

	s.currentRoute = route
	s.routeIndex = 0
	s.moveProgress = 0
	s.routeHistory = make([]RoutePoint, 0)

	if loc := s.findLocation(route[0]); loc != nil {
		s.currentLocation = loc
		s.targetLocation = loc
		s.device.Latitude = loc.Lat
		s.device.Longitude = loc.Lon
		s.device.Altitude = loc.Alt
		s.deviceRepo.UpdatePosition(s.device.ID, loc.Lat, loc.Lon, loc.Alt)
		s.routeHistory = append(s.routeHistory, RoutePoint{
			Lat: loc.Lat, Lon: loc.Lon, Name: loc.Name, Ts: time.Now().Unix(),
		})
	}
}

// GetRouteHistory возвращает историю маршрута
func (s *Simulator) GetRouteHistory() []RoutePoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]RoutePoint, len(s.routeHistory))
	copy(result, s.routeHistory)
	return result
}

// GetStatus возвращает статус симулятора
func (s *Simulator) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"id":             s.ID,
		"name":           s.Name(),
		"color":          s.Color(),
		"running":        s.isRunning,
		"move_speed":     s.moveSpeed,
		"is_random":      s.isRandomMode,
		"history_count":  len(s.routeHistory),
		"base_heart_rate": s.baseHeartRate,
	}

	if s.currentLocation != nil {
		status["current_location"] = s.currentLocation.Name
	}
	if s.targetLocation != nil {
		status["target_location"] = s.targetLocation.Name
	}
	if s.isRandomMode {
		status["route_name"] = "Случайный"
	} else if len(s.currentRoute) > 0 {
		status["route_name"] = s.getRouteName()
	}
	if s.device != nil {
		status["device_id"] = s.device.ID
		status["node_id"] = s.device.NodeID
		status["lat"] = s.device.Latitude
		status["lon"] = s.device.Longitude
	}

	return status
}

func (s *Simulator) getRouteName() string {
	for name, route := range predefinedRoutes {
		if len(route) == len(s.currentRoute) {
			same := true
			for i := range route {
				if route[i] != s.currentRoute[i] {
					same = false
					break
				}
			}
			if same {
				return name
			}
		}
	}
	return "custom"
}

// ClearRouteHistory очищает историю
func (s *Simulator) ClearRouteHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routeHistory = make([]RoutePoint, 0)
}

func (s *Simulator) initDevice() error {
	nodeID := fmt.Sprintf("!SIM_%02d", s.ID+1)
	device, err := s.deviceRepo.GetByNodeID(nodeID)
	if err == nil {
		s.device = device
		return nil
	}

	s.device = &models.Device{
		NodeID:    nodeID,
		Name:      s.Name(),
		Latitude:  59.9343,
		Longitude: 30.3351,
		Altitude:  10,
		LastSeen:  time.Now(),
	}

	if err := s.deviceRepo.Create(s.device); err != nil {
		return err
	}

	log.Printf("Simulator[%s]: created device %s", s.Name(), nodeID)
	return nil
}

func (s *Simulator) run() {
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.ticker.C:
			s.generateData()
		}
	}
}

func (s *Simulator) generateData() {
	s.mu.Lock()

	if s.device == nil {
		s.mu.Unlock()
		return
	}

	now := time.Now()

	s.simulateMovement(now)

	s.routeHistory = append(s.routeHistory, RoutePoint{
		Lat:  s.device.Latitude,
		Lon:  s.device.Longitude,
		Name: s.currentLocationName(),
		Ts:   now.Unix(),
	})
	if len(s.routeHistory) > maxRouteHistory {
		s.routeHistory = s.routeHistory[len(s.routeHistory)-maxRouteHistory:]
	}

	s.mu.Unlock()

	metrics := s.generateMetrics(now)
	if err := s.metricsRepo.Create(metrics); err != nil {
		log.Printf("Simulator[%s]: failed to create metrics: %v", s.Name(), err)
	}

	s.generateAlerts()

	s.device.LastSeen = now
	s.deviceRepo.UpdateLastSeen(s.device.ID)
}

func (s *Simulator) simulateMovement(now time.Time) {
	if s.isRandomMode {
		s.simulateRandomMovement()
	} else {
		s.simulateRouteMovement()
	}
}

func (s *Simulator) simulateRandomMovement() {
	if s.targetLocation == nil || s.moveProgress >= 1.0 {
		s.currentLocation = s.targetLocation

		var nextLoc *Location
		for {
			idx := rand.Intn(len(predefinedLocations))
			loc := &predefinedLocations[idx]
			if s.currentLocation == nil || loc.Name != s.currentLocation.Name {
				nextLoc = loc
				break
			}
		}

		s.targetLocation = nextLoc
		s.moveProgress = 0.0
	}

	s.interpolatePosition()
}

func (s *Simulator) simulateRouteMovement() {
	if len(s.currentRoute) < 2 {
		return
	}

	if s.targetLocation == nil || s.moveProgress >= 1.0 {
		s.routeIndex++
		if s.routeIndex >= len(s.currentRoute) {
			s.routeIndex = 0
		}

		s.currentLocation = s.targetLocation
		s.targetLocation = s.findLocation(s.currentRoute[s.routeIndex])
		s.moveProgress = 0.0
	}

	s.interpolatePosition()
}

func (s *Simulator) interpolatePosition() {
	if s.currentLocation == nil || s.targetLocation == nil {
		return
	}

	distLat := s.targetLocation.Lat - s.currentLocation.Lat
	distLon := s.targetLocation.Lon - s.currentLocation.Lon

	distKm := math.Sqrt(math.Pow(distLat*111, 2) + math.Pow(distLon*66, 2))

	var speedKmPerTick float64
	switch s.currentLocation.Activity {
	case "running":
		speedKmPerTick = 0.003 * s.moveSpeed
	case "walking":
		speedKmPerTick = 0.0015 * s.moveSpeed
	case "standing":
		speedKmPerTick = 0.0001 * s.moveSpeed
	case "resting":
		speedKmPerTick = 0.0
	default:
		speedKmPerTick = 0.001 * s.moveSpeed
	}

	if distKm > 0 {
		s.moveProgress += speedKmPerTick / distKm
	} else {
		s.moveProgress = 1.0
	}

	if s.moveProgress > 1.0 {
		s.moveProgress = 1.0
	}

	t := s.moveProgress
	noiseLat := (rand.Float64() - 0.5) * 0.00005
	noiseLon := (rand.Float64() - 0.5) * 0.00005

	newLat := s.currentLocation.Lat + distLat*t + noiseLat
	newLon := s.currentLocation.Lon + distLon*t + noiseLon

	s.device.Latitude = newLat
	s.device.Longitude = newLon
	s.device.Altitude = s.currentLocation.Alt + (s.targetLocation.Alt-s.currentLocation.Alt)*t

	s.deviceRepo.UpdatePosition(s.device.ID, newLat, newLon, s.device.Altitude)
}

func (s *Simulator) generateMetrics(now time.Time) *models.Metrics {
	hour := float64(now.Hour())

	activityFactor := 1.0 + 0.3*math.Sin((hour-6)*math.Pi/12)
	if hour < 6 || hour > 23 {
		activityFactor = 0.85
	}

	if s.currentLocation != nil {
		switch s.currentLocation.Activity {
		case "running":
			activityFactor *= 1.8 + rand.Float64()*0.3
		case "walking":
			activityFactor *= 1.15 + rand.Float64()*0.1
		case "standing":
			activityFactor *= 1.05
		case "resting":
			activityFactor *= 0.9
		}
	}

	if rand.Float32() < 0.1 {
		activityFactor *= 1.1 + rand.Float64()*0.2
	}

	heartRate := int(float64(s.baseHeartRate) * activityFactor)
	heartRate += rand.Intn(7) - 3

	tempVariation := 0.3 * math.Sin((hour-5)*math.Pi/12)
	temp := s.baseTemp + tempVariation + (rand.Float64()*0.2 - 0.1)

	co2Base := s.baseCO2
	if heartRate > 90 {
		co2Base += (heartRate - 72) * 2
	}
	co2 := co2Base + rand.Intn(30) - 15

	humidity := 45.0 + rand.Float64()*10 - 5

	return &models.Metrics{
		DeviceID:  s.device.ID,
		HeartRate: heartRate,
		CO2:       co2,
		Temp:      temp,
		Humidity:  humidity,
		Timestamp: now,
	}
}

func (s *Simulator) generateAlerts() {
	if s.currentLocation != nil && s.currentLocation.Zone == "restricted" {
		if rand.Float32() < 0.3 {
			alert := &models.Alert{
				DeviceID:  s.device.ID,
				Type:      "out_of_zone",
				Message:   fmt.Sprintf("%s покинул разрешенную зону: %s", s.Name(), s.currentLocation.Name),
				Severity:  "critical",
				IsRead:    false,
				CreatedAt: time.Now(),
			}
			s.alertRepo.Create(alert)
		}
	}

	if rand.Float32() < 0.02 {
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "out_of_zone",
			Message:   fmt.Sprintf("%s покинул разрешенную зону", s.Name()),
			Severity:  "critical",
			IsRead:    false,
			CreatedAt: time.Now(),
		}
		s.alertRepo.Create(alert)
	}

	if rand.Float32() < 0.005 {
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "low_battery",
			Message:   fmt.Sprintf("%s: низкий заряд батареи (15%%)", s.Name()),
			Severity:  "warning",
			IsRead:    false,
			CreatedAt: time.Now(),
		}
		s.alertRepo.Create(alert)
	}
}

func (s *Simulator) findLocation(name string) *Location {
	for i := range predefinedLocations {
		if predefinedLocations[i].Name == name {
			return &predefinedLocations[i]
		}
	}
	return nil
}

func (s *Simulator) currentLocationName() string {
	if s.currentLocation != nil {
		return s.currentLocation.Name
	}
	return "unknown"
}

// GetDevice возвращает устройство
func (s *Simulator) GetDevice() *models.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.device
}

// GenerateSingleEvent генерирует единичное событие
func (s *Simulator) GenerateSingleEvent(eventType string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.device == nil {
		return
	}

	now := time.Now()

	switch eventType {
	case "high_heart_rate":
		metrics := &models.Metrics{
			DeviceID:  s.device.ID,
			HeartRate: 140 + rand.Intn(20),
			CO2:       s.baseCO2 + rand.Intn(50),
			Temp:      37.2 + rand.Float64()*0.3,
			Humidity:  50 + rand.Float64()*5,
			Timestamp: now,
		}
		s.metricsRepo.Create(metrics)

	case "low_battery":
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "low_battery",
			Message:   fmt.Sprintf("%s: критически низкий заряд (5%%)", s.Name()),
			Severity:  "critical",
			IsRead:    false,
			CreatedAt: now,
		}
		s.alertRepo.Create(alert)

	case "signal_lost":
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "signal_lost",
			Message:   fmt.Sprintf("%s: потеря сигнала", s.Name()),
			Severity:  "warning",
			IsRead:    false,
			CreatedAt: now,
		}
		s.alertRepo.Create(alert)
	}
}

// GetLocationByName возвращает координаты точки по имени
func GetLocationByName(name string) *Location {
	for i := range predefinedLocations {
		if predefinedLocations[i].Name == name {
			return &predefinedLocations[i]
		}
	}
	return nil
}

// GetAllLocations возвращает все доступные точки
func GetAllLocations() []Location {
	return predefinedLocations
}

// GetAllRoutes возвращает все маршруты
func GetAllRoutes() map[string][]string {
	return predefinedRoutes
}

// GetSimColors возвращает цвета симуляторов
func GetSimColors() []string {
	return simColors
}

// GetSimNames возвращает имена симуляторов
func GetSimNames() []string {
	return simNames
}
