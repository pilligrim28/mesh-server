package simulator

import (
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"mesh-server/models"
	"mesh-server/repository"
)

// Simulator генерирует тестовые данные для носимого устройства
type Simulator struct {
	deviceRepo    *repository.DeviceRepository
	metricsRepo   *repository.MetricsRepository
	alertRepo     *repository.AlertRepository
	device        *models.Device
	ticker        *time.Ticker
	stopChan      chan struct{}
	mu            sync.RWMutex
	isRunning     bool

	// Параметры симуляции
	baseLat       float64
	baseLon       float64
	baseHeartRate int
	baseTemp      float64
	baseCO2       int
}

// NewSimulator создаёт новый симулятор
func NewSimulator(
	deviceRepo *repository.DeviceRepository,
	metricsRepo *repository.MetricsRepository,
	alertRepo *repository.AlertRepository,
) *Simulator {
	return &Simulator{
		deviceRepo:  deviceRepo,
		metricsRepo: metricsRepo,
		alertRepo:   alertRepo,
		stopChan:    make(chan struct{}),

		// Базовые координаты (Санкт-Петербург, центр города)
		baseLat: 59.9343,
		baseLon: 30.3351,

		// Базовые показатели человека в покое
		baseHeartRate: 72,
		baseTemp:      36.6,
		baseCO2:       400,
	}
}

// Start запускает симуляцию
func (s *Simulator) Start(interval time.Duration) {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.mu.Unlock()

	// Создаём устройство если нет
	if err := s.initDevice(); err != nil {
		log.Printf("Simulator: failed to init device: %v", err)
		return
	}

	s.ticker = time.NewTicker(interval)
	go s.run()
	log.Printf("Simulator: started with interval %v", interval)
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
	log.Printf("Simulator: stopped")
}

// IsRunning возвращает статус симуляции
func (s *Simulator) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

func (s *Simulator) initDevice() error {
	// Проверяем существует ли устройство
	device, err := s.deviceRepo.GetByNodeID("!SIMULATOR01")
	if err == nil {
		s.device = device
		return nil
	}

	// Создаём новое устройство
	s.device = &models.Device{
		NodeID:   "!SIMULATOR01",
		Name:     "Носимое устройство (Тест)",
		Latitude: s.baseLat,
		Longitude: s.baseLon,
		Altitude:  150,
		LastSeen:  time.Now(),
	}

	if err := s.deviceRepo.Create(s.device); err != nil {
		return err
	}

	log.Printf("Simulator: created device %s", s.device.NodeID)
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.device == nil {
		return
	}

	now := time.Now()

	// Генерируем метрики
	metrics := s.generateMetrics(now)
	if err := s.metricsRepo.Create(metrics); err != nil {
		log.Printf("Simulator: failed to create metrics: %v", err)
	}

	// Генерируем перемещение
	s.simulateMovement(now)

	// Генерируем алерты с вероятностью
	s.generateAlerts()

	// Обновляем устройство
	s.device.LastSeen = now
	s.deviceRepo.UpdateLastSeen(s.device.ID)

	log.Printf("Simulator: generated data - HR: %d, Temp: %.1f, CO2: %d",
		metrics.HeartRate, metrics.Temp, metrics.CO2)
}

func (s *Simulator) generateMetrics(now time.Time) *models.Metrics {
	// Симуляция активности человека (циркадные ритмы + случайность)
	hour := float64(now.Hour())

	// Пульс: выше днём, ниже ночью
	activityFactor := 1.0 + 0.3*math.Sin((hour-6)*math.Pi/12)
	if hour < 6 || hour > 23 {
		activityFactor = 0.85 // Ночью пульс ниже
	}

	// Добавляем случайные всплески (физическая активность)
	if rand.Float32() < 0.1 {
		activityFactor *= 1.2 + rand.Float64()*0.3
	}

	heartRate := int(float64(s.baseHeartRate) * activityFactor)
	heartRate += rand.Intn(7) - 3 // Небольшой шум

	// Температура тела (циркадный ритм)
	tempVariation := 0.3 * math.Sin((hour-5)*math.Pi/12)
	temp := s.baseTemp + tempVariation + (rand.Float64()*0.2 - 0.1)

	// CO2 (зависит от активности)
	co2Base := s.baseCO2
	if heartRate > 90 {
		co2Base += (heartRate - 72) * 2 // При активности CO2 выше
	}
	co2 := co2Base + rand.Intn(30) - 15

	// Влажность (комфортный диапазон)
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

func (s *Simulator) simulateMovement(now time.Time) {
	// Симуляция ходьбы по району
	speed := 0.0001 // ~10 метров за тик

	// Используем Perlin-like шум для плавного движения
	t := now.Unix()

	dx := math.Sin(float64(t)/100) * math.Cos(float64(t)/300)
	dy := math.Cos(float64(t)/100) * math.Sin(float64(t)/250)

	newLat := s.baseLat + dy*speed*10
	newLon := s.baseLon + dx*speed*10

	s.device.Latitude = newLat
	s.device.Longitude = newLon

	s.deviceRepo.UpdatePosition(s.device.ID, newLat, newLon, s.device.Altitude)
}

func (s *Simulator) generateAlerts() {
	// Алерт "вне зоны" с вероятностью 2%
	if rand.Float32() < 0.02 {
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "out_of_zone",
			Message:   "Пользователь покинул разрешенную зону",
			Severity:  "critical",
			IsRead:    false,
			CreatedAt: time.Now(),
		}
		if err := s.alertRepo.Create(alert); err != nil {
			log.Printf("Simulator: failed to create alert: %v", err)
		} else {
			log.Printf("Simulator: generated out_of_zone alert")
		}
	}

	// Алерт "низкий заряд" с вероятностью 0.5%
	if rand.Float32() < 0.005 {
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "low_battery",
			Message:   "Низкий заряд батареи (15%)",
			Severity:  "warning",
			IsRead:    false,
			CreatedAt: time.Now(),
		}
		if err := s.alertRepo.Create(alert); err != nil {
			log.Printf("Simulator: failed to create alert: %v", err)
		}
	}
}

// GetDevice возвращает устройство симулятора
func (s *Simulator) GetDevice() *models.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.device
}

// GenerateSingleEvent генерирует единичное событие (для тестирования)
func (s *Simulator) GenerateSingleEvent(eventType string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.device == nil {
		log.Printf("Simulator: cannot generate event, device is nil")
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
		if err := s.metricsRepo.Create(metrics); err != nil {
			log.Printf("Simulator: failed to create high_heart_rate metrics: %v", err)
		}

	case "low_battery":
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "low_battery",
			Message:   "Критически низкий заряд батареи (5%)",
			Severity:  "critical",
			IsRead:    false,
			CreatedAt: now,
		}
		if err := s.alertRepo.Create(alert); err != nil {
			log.Printf("Simulator: failed to create low_battery alert: %v", err)
		}

	case "signal_lost":
		alert := &models.Alert{
			DeviceID:  s.device.ID,
			Type:      "signal_lost",
			Message:   "Потеря сигнала с устройством",
			Severity:  "warning",
			IsRead:    false,
			CreatedAt: now,
		}
		if err := s.alertRepo.Create(alert); err != nil {
			log.Printf("Simulator: failed to create signal_lost alert: %v", err)
		}
	}
}
