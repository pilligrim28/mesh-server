package ml

import (
	"math"
)

// Point2D точка на плоскости для предсказания
type Point2D struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// PredictResult результат предсказания
type PredictResult struct {
	Current Point2D `json:"current"`
	Predicted Point2D `json:"predicted"`
	Confidence float64 `json:"confidence"` // 0..1
	DirectionHeading float64 `json:"direction_heading"` // градусы
	SpeedMpS float64 `json:"speed_mps"` // м/с
}

// Predictor предсказывает следующую позицию на основе экспоненциального сглаживания
type Predictor struct {
	Alpha float64 // коэффициент сглаживания (0..1), по умолчанию 0.3
}

// NewPredictor создаёт предсказатель
func NewPredictor() *Predictor {
	return &Predictor{
		Alpha: 0.3,
	}
}

// NextPosition предсказывает следующую позицию на основе последних точек
func (p *Predictor) NextPosition(points []Point2D) *PredictResult {
	if len(points) < 2 {
		return nil
	}

	n := len(points)
	current := points[n-1]

	if n == 2 {
		// При 2 точках — просто экстраполяция по направлению
		dx := current.Lat - points[0].Lat
		dy := current.Lon - points[0].Lon
		predicted := Point2D{
			Lat: current.Lat + dx,
			Lon: current.Lon + dy,
		}
		return &PredictResult{
			Current:         current,
			Predicted:       predicted,
			Confidence:      0.3,
			DirectionHeading: heading(points[0], current),
			SpeedMpS:        speedMps(points[0], current),
		}
	}

	// Экспоненциальное сглаживание для последних K=10 точек
	k := 10
	if n < k {
		k = n
	}
	startN := n - k

	// Взвешенная сумма: более старые точки имеют меньший вес
	var sumWeight float64
	var predLat, predLon float64

	for i := 0; i < k; i++ {
		idx := startN + i
		weight := p.Alpha * math.Pow(1-p.Alpha, float64(k-1-i))

		predLat += weight * points[idx].Lat
		predLon += weight * points[idx].Lon
		sumWeight += weight
	}

	if sumWeight > 0 {
		predLat /= sumWeight
		predLon /= sumWeight
	}

	// Добавляем вектор скорости от последней точки к средней взвешенной
	dx := predLat - current.Lat
	dy := predLon - current.Lon

	predicted := Point2D{
		Lat: current.Lat + dx*1.5, // экстраполяция на 1.5 интервала вперёд
		Lon: current.Lon + dy*1.5,
	}

	// Вычисляем уверенность: чем больше точек, тем выше
	confidence := math.Min(float64(k)/10.0, 1.0) * 0.8

	return &PredictResult{
		Current:          current,
		Predicted:        predicted,
		Confidence:       confidence,
		DirectionHeading: heading(points[n-2], current),
		SpeedMpS:         speedMps(points[n-2], current),
	}
}

// InterpolatePoints генерирует промежуточные точки для плавной анимации
func (p *Predictor) InterpolatePoints(from, to Point2D, steps int) []Point2D {
	if steps < 2 {
		steps = 10
	}
	points := make([]Point2D, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		points[i] = Point2D{
			Lat: from.Lat + (to.Lat-from.Lat)*t,
			Lon: from.Lon + (to.Lon-from.Lon)*t,
		}
	}
	return points
}

// CatmullRomPoints генерирует плавную кривую через массив точек
func (p *Predictor) CatmullRomPoints(controlPoints []Point2D, segments int) []Point2D {
	if len(controlPoints) < 2 {
		return controlPoints
	}
	if segments < 4 {
		segments = 10
	}

	// Для каждой пары точек генерируем сегменты
	var result []Point2D

	for i := 0; i < len(controlPoints)-1; i++ {
		p0 := controlPoints[int(math.Max(0, float64(i-1)))]
		p1 := controlPoints[i]
		p2 := controlPoints[i+1]
		p3 := controlPoints[int(math.Min(float64(len(controlPoints)-1), float64(i+2)))]

		for s := 0; s < segments; s++ {
			t := float64(s) / float64(segments)
			result = append(result, catmullRom(p0, p1, p2, p3, t))
		}
	}

	// Добавляем последнюю точку
	result = append(result, controlPoints[len(controlPoints)-1])
	return result
}

// catmullRom вычисляет точку на Catmull-Rom сплайне
func catmullRom(p0, p1, p2, p3 Point2D, t float64) Point2D {
	t2 := t * t
	t3 := t2 * t

	return Point2D{
		Lat: 0.5 * (2*p1.Lat +
			(-p0.Lat+p2.Lat)*t +
			(2*p0.Lat-5*p1.Lat+4*p2.Lat-p3.Lat)*t2 +
			(-p0.Lat+3*p1.Lat-3*p2.Lat+p3.Lat)*t3),
		Lon: 0.5 * (2*p1.Lon +
			(-p0.Lon+p2.Lon)*t +
			(2*p0.Lon-5*p1.Lon+4*p2.Lon-p3.Lon)*t2 +
			(-p0.Lon+3*p1.Lon-3*p2.Lon+p3.Lon)*t3),
	}
}

// heading возвращает направление между двумя точками в градусах
func heading(from, to Point2D) float64 {
	dx := to.Lon - from.Lon
	dy := to.Lat - from.Lat
	angle := math.Atan2(dx, dy) * 180 / math.Pi
	if angle < 0 {
		angle += 360
	}
	return angle
}

// speedMps возвращает скорость между двумя точками в м/с
func speedMps(from, to Point2D) float64 {
	dLat := (to.Lat - from.Lat) * 111320
	dLon := (to.Lon - from.Lon) * 111320 * math.Cos(to.Lat*math.Pi/180)
	return math.Sqrt(dLat*dLat + dLon*dLon)
}
