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

// Landmark представляет достопримечательность Санкт-Петербурга
type Landmark struct {
	Name      string
	Latitude  float64
	Longitude float64
}

// Person представляет человека в симуляции
type Person struct {
	ID              string
	Name            string
	CurrentLandmark *Landmark
	TargetLandmark  *Landmark
	Latitude        float64
	Longitude       float64
	Speed           float64 // градусов в секунду
	Device          *models.Device
	State           string // "walking", "visiting", "resting"
	VisitStartTime  time.Time
	VisitDuration   time.Duration
}

// PeopleSimulation симулирует множество людей в Санкт-Петербурге
type PeopleSimulation struct {
	deviceRepo  *repository.DeviceRepository
	metricsRepo *repository.MetricsRepository
	landmarks   []Landmark
	people      []*Person
	ticker      *time.Ticker
	stopChan    chan struct{}
	mu          sync.RWMutex
	isRunning   bool
	interval    time.Duration
}

// Основные достопримечательности Санкт-Петербурга
var spbLandmarks = []Landmark{
	{Name: "Дворцовая площадь", Latitude: 59.9398, Longitude: 30.3146},
	{Name: "Эрмитаж", Latitude: 59.9417, Longitude: 30.3127},
	{Name: "Исаакиевский собор", Latitude: 59.9335, Longitude: 30.3048},
	{Name: "Спас на Крови", Latitude: 59.9402, Longitude: 30.3269},
	{Name: "Казанский собор", Latitude: 59.9345, Longitude: 30.3224},
	{Name: "Петропавловская крепость", Latitude: 59.9502, Longitude: 30.3159},
	{Name: "Невский проспект (начало)", Latitude: 59.9341, Longitude: 30.3163},
	{Name: "Невский проспект (конец)", Latitude: 59.9295, Longitude: 30.3586},
	{Name: "Марсово поле", Latitude: 59.9445, Longitude: 30.3208},
	{Name: "Летний сад", Latitude: 59.9466, Longitude: 30.3291},
	{Name: "Стрелка Васильевского острова", Latitude: 59.9429, Longitude: 30.3067},
	{Name: "Медный всадник", Latitude: 59.9365, Longitude: 30.3058},
	{Name: "Новая Голландия", Latitude: 59.9208, Longitude: 30.2936},
	{Name: "Смоленское кладбище", Latitude: 59.9408, Longitude: 30.2755},
	{Name: "Елагин остров", Latitude: 59.9676, Longitude: 30.2926},
}

// NewPeopleSimulation создаёт новую симуляцию людей
func NewPeopleSimulation(
	deviceRepo *repository.DeviceRepository,
	metricsRepo *repository.MetricsRepository,
) *PeopleSimulation {
	return &PeopleSimulation{
		deviceRepo:  deviceRepo,
		metricsRepo: metricsRepo,
		landmarks:   spbLandmarks,
		people:      make([]*Person, 0),
		stopChan:    make(chan struct{}),
		interval:    time.Second * 2,
	}
}

// Start запускает симуляцию с указанным количеством людей
func (ps *PeopleSimulation) Start(numPeople int, interval time.Duration) {
	ps.mu.Lock()
	if ps.isRunning {
		ps.mu.Unlock()
		return
	}
	ps.isRunning = true
	ps.interval = interval
	ps.mu.Unlock()

	// Инициализируем людей
	ps.initPeople(numPeople)

	ps.ticker = time.NewTicker(interval)
	go ps.run()
	log.Printf("PeopleSimulation: started with %d people, interval %v", numPeople, interval)
}

// Stop останавливает симуляцию
func (ps *PeopleSimulation) Stop() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if !ps.isRunning {
		return
	}

	ps.isRunning = false
	close(ps.stopChan)
	ps.stopChan = make(chan struct{})
	if ps.ticker != nil {
		ps.ticker.Stop()
	}
	log.Printf("PeopleSimulation: stopped")
}

// IsRunning возвращает статус симуляции
func (ps *PeopleSimulation) IsRunning() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.isRunning
}

// GetPeopleCount возвращает количество людей в симуляции
func (ps *PeopleSimulation) GetPeopleCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.people)
}

// initPeople инициализирует людей в случайных точках города
func (ps *PeopleSimulation) initPeople(numPeople int) {
	ps.people = make([]*Person, numPeople)

	for i := 0; i < numPeople; i++ {
		person := ps.createPerson(i)
		ps.people[i] = person
	}
}

// createPerson создаёт нового человека
func (ps *PeopleSimulation) createPerson(index int) *Person {
	// Выбираем случайную начальную достопримечательность
	startLandmark := ps.landmarks[rand.Intn(len(ps.landmarks))]

	// Небольшое смещение от достопримечательности
	offsetLat := (rand.Float64() - 0.5) * 0.002
	offsetLon := (rand.Float64() - 0.5) * 0.002

	lat := startLandmark.Latitude + offsetLat
	lon := startLandmark.Longitude + offsetLon

	// Создаём устройство для человека
	nodeID := fmt.Sprintf("!PERSON%03d", index+1)
	name := fmt.Sprintf("Турист %d", index+1)

	device := &models.Device{
		NodeID:    nodeID,
		Name:      name,
		Latitude:  lat,
		Longitude: lon,
		Altitude:  10 + rand.Float64()*20,
		LastSeen:  time.Now(),
	}

	// Пытаемся создать устройство, или используем существующее
	existingDevice, err := ps.deviceRepo.GetByNodeID(nodeID)
	if err != nil {
		if err := ps.deviceRepo.Create(device); err != nil {
			log.Printf("PeopleSimulation: failed to create device for person %d: %v", index, err)
		}
	} else {
		device = existingDevice
		ps.deviceRepo.UpdatePosition(device.ID, lat, lon, device.Altitude)
	}

	// Скорость ходьбы: 4-6 км/ч -> ~0.00001-0.000015 градусов в секунду
	speed := 0.00001 + rand.Float64()*0.000005

	return &Person{
		ID:              fmt.Sprintf("person_%d", index+1),
		Name:            name,
		CurrentLandmark: &startLandmark,
		TargetLandmark:  ps.selectNewTarget(&startLandmark),
		Latitude:        lat,
		Longitude:       lon,
		Speed:           speed,
		Device:          device,
		State:           "walking",
		VisitDuration:   time.Duration(2+rand.Intn(5)) * time.Minute,
	}
}

// selectNewTarget выбирает новую цель для посещения
func (ps *PeopleSimulation) selectNewTarget(current *Landmark) *Landmark {
	// Выбираем случайную достопримечательность, отличную от текущей
	var target *Landmark
	for {
		candidate := ps.landmarks[rand.Intn(len(ps.landmarks))]
		if candidate.Name != current.Name {
			target = &candidate
			break
		}
	}
	return target
}

// calculateDistance вычисляет расстояние между двумя точками (в градусах)
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := lat2 - lat1
	dLon := lon2 - lon1
	return math.Sqrt(dLat*dLat + dLon*dLon)
}

// run запускает основной цикл симуляции
func (ps *PeopleSimulation) run() {
	for {
		select {
		case <-ps.stopChan:
			return
		case <-ps.ticker.C:
			ps.updatePeople()
		}
	}
}

// updatePeople обновляет состояние всех людей
func (ps *PeopleSimulation) updatePeople() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()

	for _, person := range ps.people {
		ps.updatePerson(person, now)
	}
}

// updatePerson обновляет состояние одного человека
func (ps *PeopleSimulation) updatePerson(person *Person, now time.Time) {
	switch person.State {
	case "walking":
		ps.updateWalking(person, now)
	case "visiting":
		ps.updateVisiting(person, now)
	case "resting":
		ps.updateResting(person, now)
	}

	// Обновляем позицию устройства
	if person.Device != nil {
		person.Device.Latitude = person.Latitude
		person.Device.Longitude = person.Longitude
		person.Device.LastSeen = now
		ps.deviceRepo.UpdatePosition(person.Device.ID, person.Latitude, person.Longitude, person.Device.Altitude)
	}

	// Генерируем метрики
	ps.generateMetricsForPerson(person, now)
}

// updateWalking обновляет состояние идущего человека
func (ps *PeopleSimulation) updateWalking(person *Person, now time.Time) {
	if person.TargetLandmark == nil {
		person.TargetLandmark = ps.selectNewTarget(person.CurrentLandmark)
		return
	}

	// Вычисляем направление к цели
	dLat := person.TargetLandmark.Latitude - person.Latitude
	dLon := person.TargetLandmark.Longitude - person.Longitude
	distance := calculateDistance(person.Latitude, person.Longitude,
		person.TargetLandmark.Latitude, person.TargetLandmark.Longitude)

	// Если достигли цели (расстояние < 10 метров ~ 0.0001 градуса)
	if distance < 0.0001 {
		person.Latitude = person.TargetLandmark.Latitude
		person.Longitude = person.TargetLandmark.Longitude
		person.CurrentLandmark = person.TargetLandmark
		person.State = "visiting"
		person.VisitStartTime = now
		log.Printf("PeopleSimulation: %s arrived at %s", person.Name, person.CurrentLandmark.Name)
		return
	}

	// Нормализуем направление и двигаемся
	directionLat := dLat / distance
	directionLon := dLon / distance

	person.Latitude += directionLat * person.Speed
	person.Longitude += directionLon * person.Speed
}

// updateVisiting обновляет состояние посещающего человека
func (ps *PeopleSimulation) updateVisiting(person *Person, now time.Time) {
	elapsed := now.Sub(person.VisitStartTime)

	// Если провели достаточно времени у достопримечательности
	if elapsed >= person.VisitDuration {
		// С вероятностью 30% переходим в состояние отдыха
		if rand.Float32() < 0.3 {
			person.State = "resting"
			person.VisitStartTime = now
			log.Printf("PeopleSimulation: %s is resting near %s", person.Name, person.CurrentLandmark.Name)
		} else {
			// Выбираем новую цель и идём
			person.TargetLandmark = ps.selectNewTarget(person.CurrentLandmark)
			person.State = "walking"
			log.Printf("PeopleSimulation: %s heading to %s", person.Name, person.TargetLandmark.Name)
		}
	}
}

// updateResting обновляет состояние отдыхающего человека
func (ps *PeopleSimulation) updateResting(person *Person, now time.Time) {
	elapsed := now.Sub(person.VisitStartTime)

	// Отдыхаем 1-3 минуты
	if elapsed >= time.Duration(1+rand.Intn(2))*time.Minute {
		person.TargetLandmark = ps.selectNewTarget(person.CurrentLandmark)
		person.State = "walking"
		log.Printf("PeopleSimulation: %s finished resting, heading to %s", person.Name, person.TargetLandmark.Name)
	}
}

// generateMetricsForPerson генерирует метрики для человека
func (ps *PeopleSimulation) generateMetricsForPerson(person *Person, now time.Time) {
	if person.Device == nil {
		return
	}

	// Базовые показатели
	baseHeartRate := 72
	baseTemp := 36.6
	baseCO2 := 400

	// Корректировка в зависимости от состояния
	switch person.State {
	case "walking":
		baseHeartRate += 20 + rand.Intn(15) // При ходьбе пульс выше
		baseCO2 += 50 + rand.Intn(30)
	case "visiting":
		baseHeartRate += 5 + rand.Intn(10)
	case "resting":
		baseHeartRate -= 5 + rand.Intn(5)
	}

	// Добавляем небольшой шум
	heartRate := baseHeartRate + rand.Intn(5) - 2
	temp := baseTemp + (rand.Float64()*0.3 - 0.15)
	co2 := baseCO2 + rand.Intn(20) - 10
	humidity := 45.0 + rand.Float64()*10 - 5

	metrics := &models.Metrics{
		DeviceID:  person.Device.ID,
		HeartRate: heartRate,
		CO2:       co2,
		Temp:      temp,
		Humidity:  humidity,
		Timestamp: now,
	}

	if err := ps.metricsRepo.Create(metrics); err != nil {
		log.Printf("PeopleSimulation: failed to create metrics for %s: %v", person.Name, err)
	}
}

// GetPeopleStatus возвращает статус всех людей
func (ps *PeopleSimulation) GetPeopleStatus() []map[string]interface{} {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	status := make([]map[string]interface{}, len(ps.people))

	for i, person := range ps.people {
		currentLandmark := ""
		targetLandmark := ""
		if person.CurrentLandmark != nil {
			currentLandmark = person.CurrentLandmark.Name
		}
		if person.TargetLandmark != nil {
			targetLandmark = person.TargetLandmark.Name
		}

		status[i] = map[string]interface{}{
			"id":              person.ID,
			"name":            person.Name,
			"latitude":        person.Latitude,
			"longitude":       person.Longitude,
			"current_landmark": currentLandmark,
			"target_landmark":  targetLandmark,
			"state":           person.State,
			"speed":           person.Speed,
		}
	}

	return status
}

// GetLandmarks возвращает список достопримечательностей
func (ps *PeopleSimulation) GetLandmarks() []Landmark {
	return ps.landmarks
}

// AddPerson добавляет нового человека в симуляцию
func (ps *PeopleSimulation) AddPerson() *Person {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	index := len(ps.people)
	person := ps.createPerson(index)
	ps.people = append(ps.people, person)

	log.Printf("PeopleSimulation: added %s", person.Name)
	return person
}

// RemovePerson удаляет человека из симуляции
func (ps *PeopleSimulation) RemovePerson(personID string) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for i, person := range ps.people {
		if person.ID == personID {
			ps.people = append(ps.people[:i], ps.people[i+1:]...)
			log.Printf("PeopleSimulation: removed %s", person.Name)
			return true
		}
	}
	return false
}

// TeleportPerson телепортирует человека к указанной достопримечательности
func (ps *PeopleSimulation) TeleportPerson(personID, landmarkName string) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var targetLandmark *Landmark
	for _, landmark := range ps.landmarks {
		if landmark.Name == landmarkName {
			targetLandmark = &landmark
			break
		}
	}

	if targetLandmark == nil {
		return false
	}

	for _, person := range ps.people {
		if person.ID == personID {
			person.Latitude = targetLandmark.Latitude
			person.Longitude = targetLandmark.Longitude
			person.CurrentLandmark = targetLandmark
			person.TargetLandmark = nil
			person.State = "visiting"
			person.VisitStartTime = time.Now()

			if person.Device != nil {
				ps.deviceRepo.UpdatePosition(person.Device.ID, person.Latitude, person.Longitude, person.Device.Altitude)
			}

			log.Printf("PeopleSimulation: teleported %s to %s", person.Name, landmarkName)
			return true
		}
	}

	return false
}
