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

const earthRadiusM = 6371000.0

const defaultOutsideAlertDelay = 5 * time.Minute

// PeopleMovementConfig параметры симуляции передвижения людей.
type PeopleMovementConfig struct {
	CenterLat          float64
	CenterLon          float64
	RadiusKm           float64
	PersonCount        int
	Interval           time.Duration
	OutsideAlertDelay  time.Duration
}

// PeopleMovementSimulator симулирует перемещение нескольких людей.
type PeopleMovementSimulator struct {
	deviceRepo  *repository.DeviceRepository
	metricsRepo *repository.MetricsRepository
	alertRepo   *repository.AlertRepository
	config      PeopleMovementConfig

	people    []*simulatedPerson
	ticker    *time.Ticker
	stopChan  chan struct{}
	mu        sync.RWMutex
	isRunning bool
}

type simulatedPerson struct {
	device           *models.Device
	lat              float64
	lon              float64
	heading          float64
	speed            float64
	outsideSince     time.Time
	outsideAlertSent bool
}

var demoNames = []string{
	"Алексей", "Мария", "Дмитрий", "Елена", "Иван",
	"Ольга", "Сергей", "Анна", "Павел", "Наталья",
	"Андрей", "Татьяна", "Михаил", "Юлия", "Николай",
}

// NewPeopleMovementSimulator создаёт симулятор передвижения людей.
func NewPeopleMovementSimulator(
	deviceRepo *repository.DeviceRepository,
	metricsRepo *repository.MetricsRepository,
	alertRepo *repository.AlertRepository,
	config PeopleMovementConfig,
) *PeopleMovementSimulator {
	if config.CenterLat == 0 && config.CenterLon == 0 {
		config.CenterLat = 59.9343
		config.CenterLon = 30.3351
	}
	if config.RadiusKm <= 0 {
		config.RadiusKm = 2
	}
	if config.PersonCount <= 0 {
		config.PersonCount = 8
	}
	if config.Interval <= 0 {
		config.Interval = 5 * time.Second
	}
	if config.OutsideAlertDelay <= 0 {
		config.OutsideAlertDelay = defaultOutsideAlertDelay
	}

	return &PeopleMovementSimulator{
		deviceRepo:  deviceRepo,
		metricsRepo: metricsRepo,
		alertRepo:   alertRepo,
		config:      config,
		stopChan:    make(chan struct{}),
	}
}

// Start запускает симуляцию.
func (s *PeopleMovementSimulator) Start() error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return nil
	}

	if err := s.initPeople(); err != nil {
		s.mu.Unlock()
		return err
	}

	s.isRunning = true
	s.ticker = time.NewTicker(s.config.Interval)
	s.mu.Unlock()

	go s.run()
	log.Printf("People movement sim: %d people, zone r=%.1f km, alert after %v outside",
		s.config.PersonCount, s.config.RadiusKm, s.config.OutsideAlertDelay)
	return nil
}

// Stop останавливает симуляцию.
func (s *PeopleMovementSimulator) Stop() {
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
	log.Println("People movement sim: stopped")
}

// IsRunning возвращает статус.
func (s *PeopleMovementSimulator) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// GetConfig возвращает параметры зоны.
func (s *PeopleMovementSimulator) GetConfig() PeopleMovementConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// SetZone обновляет геозону вручную (центр и радиус в км).
func (s *PeopleMovementSimulator) SetZone(centerLat, centerLon, radiusKm float64) error {
	if radiusKm <= 0 {
		return fmt.Errorf("radius must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.CenterLat = centerLat
	s.config.CenterLon = centerLon
	s.config.RadiusKm = radiusKm
	for _, p := range s.people {
		p.outsideSince = time.Time{}
		p.outsideAlertSent = false
	}
	log.Printf("People movement sim: zone updated center=(%.4f, %.4f) radius=%.2f km",
		centerLat, centerLon, radiusKm)
	return nil
}

// GetPeopleCount возвращает число симулируемых людей.
func (s *PeopleMovementSimulator) GetPeopleCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.people)
}

// GetOutsideCount число людей за пределами зоны.
func (s *PeopleMovementSimulator) GetOutsideCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	radiusM := s.config.RadiusKm * 1000
	count := 0
	for _, p := range s.people {
		if distanceMeters(s.config.CenterLat, s.config.CenterLon, p.lat, p.lon) > radiusM {
			count++
		}
	}
	return count
}

func (s *PeopleMovementSimulator) initPeople() error {
	s.people = make([]*simulatedPerson, 0, s.config.PersonCount)
	radiusM := s.config.RadiusKm * 1000

	for i := 0; i < s.config.PersonCount; i++ {
		nodeID := fmt.Sprintf("!SIM_P%02d", i+1)
		name := demoNames[i%len(demoNames)]
		if s.config.PersonCount > len(demoNames) {
			name = fmt.Sprintf("%s %d", name, i+1)
		}

		lat, lon := randomPointInCircle(s.config.CenterLat, s.config.CenterLon, radiusM)

		device, err := s.deviceRepo.GetByNodeID(nodeID)
		if err != nil {
			device = &models.Device{
				NodeID:    nodeID,
				Name:      name + " (симуляция)",
				Latitude:  lat,
				Longitude: lon,
				Altitude:  10 + rand.Float64()*30,
				LastSeen:  time.Now(),
			}
			if err := s.deviceRepo.Create(device); err != nil {
				return fmt.Errorf("create person device %s: %w", nodeID, err)
			}
		} else {
			device.Name = name + " (симуляция)"
			_ = s.deviceRepo.UpdatePosition(device.ID, lat, lon, device.Altitude)
		}

		s.people = append(s.people, &simulatedPerson{
			device:  device,
			lat:     lat,
			lon:     lon,
			heading: rand.Float64() * 2 * math.Pi,
			speed:   0.5 + rand.Float64()*1.5,
		})
	}
	return nil
}

func (s *PeopleMovementSimulator) run() {
	s.tick()
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.ticker.C:
			s.tick()
		}
	}
}

func (s *PeopleMovementSimulator) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	dt := s.config.Interval.Seconds()
	radiusM := s.config.RadiusKm * 1000
	alertDelay := s.config.OutsideAlertDelay

	for _, person := range s.people {
		s.moveChaotic(person, dt)

		person.device.Latitude = person.lat
		person.device.Longitude = person.lon
		person.device.LastSeen = now
		_ = s.deviceRepo.UpdatePosition(person.device.ID, person.lat, person.lon, person.device.Altitude)
		_ = s.deviceRepo.UpdateLastSeen(person.device.ID)

		metrics := &models.Metrics{
			DeviceID:  person.device.ID,
			HeartRate: 60 + rand.Intn(45),
			CO2:       360 + rand.Intn(120),
			Temp:      36.3 + rand.Float64()*0.8,
			Humidity:  35 + rand.Float64()*25,
			Timestamp: now,
		}
		if err := s.metricsRepo.Create(metrics); err != nil {
			log.Printf("People movement sim: metrics error for %s: %v", person.device.NodeID, err)
		}

		s.checkGeofence(person, now, radiusM, alertDelay)
	}
}

func (s *PeopleMovementSimulator) moveChaotic(person *simulatedPerson, dt float64) {
	switch {
	case rand.Float32() < 0.15:
		person.heading = rand.Float64() * 2 * math.Pi
	case rand.Float32() < 0.10:
		person.speed = 0
	default:
		person.heading += (rand.Float64() - 0.5) * 1.8
		person.speed = clamp(person.speed+(rand.Float64()-0.5)*0.5, 0.2, 3.0)
	}

	if rand.Float32() < 0.07 {
		person.speed = 2.0 + rand.Float64()*1.2
	}

	distM := person.speed * dt
	person.lat, person.lon = moveByMeters(person.lat, person.lon, person.heading, distM)

	if rand.Float32() < 0.04 {
		jump := 30 + rand.Float64()*120
		person.lat, person.lon = moveByMeters(person.lat, person.lon, rand.Float64()*2*math.Pi, jump)
	}
}

func (s *PeopleMovementSimulator) checkGeofence(person *simulatedPerson, now time.Time, radiusM float64, alertDelay time.Duration) {
	dist := distanceMeters(s.config.CenterLat, s.config.CenterLon, person.lat, person.lon)
	outside := dist > radiusM

	if !outside {
		person.outsideSince = time.Time{}
		person.outsideAlertSent = false
		return
	}

	if person.outsideSince.IsZero() {
		person.outsideSince = now
		person.outsideAlertSent = false
		log.Printf("People movement sim: %s left zone (%.0f m from center)", person.device.Name, dist)
		return
	}

	if person.outsideAlertSent {
		return
	}

	if now.Sub(person.outsideSince) < alertDelay {
		return
	}

	person.outsideAlertSent = true
	minutes := int(alertDelay.Minutes())
	alert := &models.Alert{
		DeviceID:  person.device.ID,
		Type:      "out_of_zone",
		Message:   fmt.Sprintf("%s вне зоны более %d мин (%.0f м от центра, лимит %.0f м)", person.device.Name, minutes, dist, radiusM),
		Severity:  "critical",
		IsRead:    false,
		CreatedAt: now,
	}
	if err := s.alertRepo.Create(alert); err != nil {
		log.Printf("People movement sim: alert error: %v", err)
		return
	}
	log.Printf("People movement sim: ALERT %s outside zone for %d+ min", person.device.Name, minutes)
}

func randomPointInCircle(centerLat, centerLon, radiusM float64) (float64, float64) {
	angle := rand.Float64() * 2 * math.Pi
	r := radiusM * math.Sqrt(rand.Float64()) * 0.9
	return moveByMeters(centerLat, centerLon, angle, r)
}

func moveByMeters(lat, lon, headingRad, distM float64) (float64, float64) {
	dLat := (distM * math.Sin(headingRad)) / earthRadiusM * (180 / math.Pi)
	dLon := (distM * math.Cos(headingRad)) / (earthRadiusM * math.Cos(lat*math.Pi/180)) * (180 / math.Pi)
	return lat + dLat, lon + dLon
}

func distanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
