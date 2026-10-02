package transport

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"mesh-server/internal/mesh"
)

// Sim — виртуальный шлюз с N носимыми узлами. Кадры кодируются в настоящий protobuf
// и проходят через тот же декодер, что и данные с реального железа.
type Sim struct {
	nodes    int
	interval time.Duration
	sink     Sink
}

func NewSim(nodes int, interval time.Duration, sink Sink) *Sim {
	return &Sim{nodes: max(nodes, 1), interval: interval, sink: sink}
}

var simNames = []string{"Алексей", "Мария", "Дмитрий", "Анна", "Сергей", "Елена", "Николай", "Ольга"}

func (s *Sim) Run(ctx context.Context) {
	const gw = "sim"
	s.sink.SetGateway(gw, "sim", "connected", "Симулятор", "")
	type st struct {
		num  uint32
		base float64
		bat  uint32
	}
	nodes := make([]*st, s.nodes)
	for i := range nodes {
		nodes[i] = &st{num: 0xA1B20000 + uint32(i+1), base: 65 + rand.Float64()*20, bat: 60 + rand.Uint32N(40)}
		name := simNames[i%len(simNames)]
		s.sink.HandleFrame(gw, mesh.EncodeNodeInfoPacket(nodes[i].num, name, fmt.Sprintf("P%02d", i+1)))
		s.sink.HandleFrame(gw, mesh.EncodeBatteryPacket(nodes[i].num, nodes[i].bat))
	}
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for tick := 0; ; tick++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		for i, n := range nodes {
			phase := float64(tick)/12 + float64(i)
			bpm := uint32(n.base + 8*math.Sin(phase) + rand.Float64()*4)
			spo2 := uint32(96 + rand.IntN(3))
			// сценарии для демонстрации алертов
			switch {
			case i == 1 && tick%40 >= 30: // нагрузка → высокий пульс
				bpm = 155 + rand.Uint32N(15)
			case i == 2 && tick%60 >= 50: // датчик снят
				s.sink.HandleFrame(gw, mesh.EncodeHealthPacket(n.num, rand.Uint32(), nil, nil, -70, 6))
				continue
			}
			s.sink.HandleFrame(gw, mesh.EncodeHealthPacket(n.num, rand.Uint32(), &bpm, &spo2,
				int32(-60-rand.IntN(40)), float32(2+rand.Float64()*8)))
			if tick%20 == 0 && n.bat > 5 {
				n.bat--
				s.sink.HandleFrame(gw, mesh.EncodeBatteryPacket(n.num, n.bat))
			}
		}
	}
}
