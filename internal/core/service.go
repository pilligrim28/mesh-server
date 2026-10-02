// Package core — бизнес-логика: приём событий из mesh, сохранение, алерты, рассылка.
package core

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"mesh-server/internal/config"
	"mesh-server/internal/mesh"
	"mesh-server/internal/store"
)

// GatewayStatus — состояние подключения к шлюзу (BLE / USB / симулятор).
type GatewayStatus struct {
	ID        string    `json:"id"`        // адрес BLE или имя порта
	Name      string    `json:"name"`      // имя узла
	Transport string    `json:"transport"` // ble | serial | sim
	State     string    `json:"state"`     // scanning | connecting | connected | disconnected | error
	Error     string    `json:"error,omitempty"`
	NodeNum   uint32    `json:"node_num,omitempty"`
	Since     time.Time `json:"since"`
	Packets   int       `json:"packets"`
}

type Service struct {
	cfg   config.Config
	store *store.Store
	hub   *Hub

	mu       sync.Mutex
	gateways map[string]*GatewayStatus
	seen     map[uint64]time.Time // дедупликация (from,id): один пакет может прийти через несколько шлюзов
}

func NewService(cfg config.Config, st *store.Store, hub *Hub) *Service {
	return &Service{cfg: cfg, store: st, hub: hub, gateways: map[string]*GatewayStatus{}, seen: map[uint64]time.Time{}}
}

func (s *Service) Hub() *Hub             { return s.hub }
func (s *Service) Store() *store.Store   { return s.store }
func (s *Service) Config() config.Config { return s.cfg }

// SetGateway обновляет статус шлюза (вызывается транспортами).
func (s *Service) SetGateway(id, transport, state, name, errText string) {
	s.mu.Lock()
	g, ok := s.gateways[id]
	if !ok {
		g = &GatewayStatus{ID: id, Transport: transport}
		s.gateways[id] = g
	}
	if g.State != state {
		g.Since = time.Now()
	}
	g.State, g.Error = state, errText
	if name != "" {
		g.Name = name
	}
	cp := *g
	s.mu.Unlock()
	log.Printf("[gateway %s/%s] %s %s", transport, id, state, errText)
	s.hub.Publish("gateway", cp)
}

// DropGateway убирает служебный статус (например, «поиск по Bluetooth» после подключения).
func (s *Service) DropGateway(id string) {
	s.mu.Lock()
	_, ok := s.gateways[id]
	delete(s.gateways, id)
	s.mu.Unlock()
	if ok {
		s.hub.Publish("gateway_removed", id)
	}
}

func (s *Service) Gateways() []GatewayStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]GatewayStatus, 0, len(s.gateways))
	for _, g := range s.gateways {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// HandleFrame — точка входа для всех транспортов: сырое сообщение FromRadio.
func (s *Service) HandleFrame(gatewayID string, frame []byte) {
	events, err := mesh.Decode(frame, gatewayID)
	if err != nil {
		log.Printf("[gateway %s] %v", gatewayID, err)
		return
	}
	s.mu.Lock()
	if g := s.gateways[gatewayID]; g != nil {
		g.Packets++
	}
	s.mu.Unlock()
	for _, e := range events {
		s.handle(e)
	}
}

func (s *Service) handle(e mesh.Event) {
	switch ev := e.(type) {
	case mesh.MyInfo:
		s.mu.Lock()
		if g := s.gateways[ev.Gateway]; g != nil {
			g.NodeNum = ev.NodeNum
		}
		s.mu.Unlock()
	case mesh.NodeInfo:
		if err := s.store.UpsertNodeInfo(ev.NodeNum, ev.LongName, ev.ShortName, ev.HWModel, ev.Gateway); err != nil {
			log.Printf("store nodeinfo: %v", err)
		}
		s.mu.Lock()
		if g := s.gateways[ev.Gateway]; g != nil && g.NodeNum == ev.NodeNum && g.Name == "" {
			g.Name = ev.LongName
		}
		s.mu.Unlock()
		s.hub.Publish("nodes_changed", nil)
	case mesh.Battery:
		if err := s.store.SetBattery(ev.NodeNum, ev.Level); err == nil {
			s.hub.Publish("battery", map[string]any{"node_num": ev.NodeNum, "battery": ev.Level})
		}
	case mesh.Health:
		s.handleHealth(ev)
	}
}

func (s *Service) duplicate(from, id uint32) bool {
	if id == 0 {
		return false
	}
	key := uint64(from)<<32 | uint64(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if t, ok := s.seen[key]; ok && now.Sub(t) < 10*time.Minute {
		return true
	}
	s.seen[key] = now
	if len(s.seen) > 5000 {
		for k, t := range s.seen {
			if now.Sub(t) > 10*time.Minute {
				delete(s.seen, k)
			}
		}
	}
	return false
}

func (s *Service) handleHealth(h mesh.Health) {
	if s.duplicate(h.NodeNum, h.PacketID) {
		return
	}
	r := store.Reading{NodeNum: h.NodeNum, Time: h.Time, HeartBPM: h.HeartBPM, SpO2: h.SpO2, RSSI: h.RSSI, SNR: h.SNR, Gateway: h.Gateway}
	id, err := s.store.AddReading(r, h.HopsAway)
	if err != nil {
		log.Printf("store reading: %v", err)
		return
	}
	r.ID = id
	s.hub.Publish("reading", r)
	s.evaluate(h)
}

// evaluate открывает/закрывает алерты по порогам.
func (s *Service) evaluate(h mesh.Health) {
	n := h.NodeNum
	s.resolve(n, "offline")
	if h.HeartBPM == nil {
		s.open(n, "no_contact", "Датчик не касается кожи — нет пульса", nil)
		s.resolve(n, "hr_high", "hr_low")
	} else {
		s.resolve(n, "no_contact")
		bpm := *h.HeartBPM
		switch {
		case bpm > s.cfg.HRHigh:
			s.open(n, "hr_high", fmt.Sprintf("Высокий пульс: %d уд/мин (порог %d)", bpm, s.cfg.HRHigh), &bpm)
			s.resolve(n, "hr_low")
		case bpm < s.cfg.HRLow:
			s.open(n, "hr_low", fmt.Sprintf("Низкий пульс: %d уд/мин (порог %d)", bpm, s.cfg.HRLow), &bpm)
			s.resolve(n, "hr_high")
		default:
			s.resolve(n, "hr_high", "hr_low")
		}
	}
	if h.SpO2 != nil && *h.SpO2 > 0 && *h.SpO2 < s.cfg.SpO2Low {
		v := *h.SpO2
		s.open(n, "spo2_low", fmt.Sprintf("Низкий SpO₂: %d%% (порог %d%%)", v, s.cfg.SpO2Low), &v)
	} else if h.SpO2 != nil {
		s.resolve(n, "spo2_low")
	}
}

func (s *Service) open(n uint32, kind, msg string, v *int) {
	a, err := s.store.OpenAlert(n, kind, msg, v)
	if err != nil {
		log.Printf("alert: %v", err)
		return
	}
	if a != nil {
		log.Printf("[ALERT] %s %s", mesh.NodeIDString(n), msg)
		s.hub.Publish("alert", a)
	}
}

func (s *Service) resolve(n uint32, kinds ...string) {
	if c, _ := s.store.ResolveAlerts(n, kinds...); c > 0 {
		s.hub.Publish("alerts_changed", nil)
	}
}

// Watchdog: алерт, если от узла с датчиком давно нет данных; плюс очистка старых данных.
func (s *Service) Watchdog(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	lastPrune := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		nodes, err := s.store.Nodes()
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if n.LastHealth != nil && time.Since(*n.LastHealth) > s.cfg.OfflineAfter {
				s.open(n.Num, "offline", fmt.Sprintf("Нет данных пульса более %d с", int(s.cfg.OfflineAfter.Seconds())), nil)
			}
		}
		if s.cfg.RetentionDays > 0 && time.Since(lastPrune) > time.Hour {
			lastPrune = time.Now()
			s.store.Prune(time.Now().AddDate(0, 0, -s.cfg.RetentionDays))
		}
	}
}
