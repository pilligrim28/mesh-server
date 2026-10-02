package core

import (
	"path/filepath"
	"testing"
	"time"

	"mesh-server/internal/config"
	"mesh-server/internal/mesh"
	"mesh-server/internal/store"
)

func u32(v uint32) *uint32 { return &v }

func newSvc(t *testing.T) *Service {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Config{HRLow: 45, HRHigh: 150, SpO2Low: 90, OfflineAfter: time.Minute}
	return NewService(cfg, st, NewHub())
}

func activeKinds(t *testing.T, s *Service) map[string]bool {
	as, err := s.store.Alerts(false, 100)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, a := range as {
		if a.Resolved == nil {
			m[a.Kind] = true
		}
	}
	return m
}

func TestAlertsLifecycle(t *testing.T) {
	s := newSvc(t)
	s.HandleFrame("gw", mesh.EncodeHealthPacket(9, 1, u32(170), u32(97), -70, 5))
	if k := activeKinds(t, s); !k["hr_high"] {
		t.Fatalf("expected hr_high, got %v", k)
	}
	s.HandleFrame("gw", mesh.EncodeHealthPacket(9, 2, u32(175), u32(97), -70, 5)) // не дублирует
	if as, _ := s.store.Alerts(false, 100); len(as) != 1 {
		t.Fatalf("alert duplicated: %d", len(as))
	}
	s.HandleFrame("gw", mesh.EncodeHealthPacket(9, 3, nil, nil, -70, 5))
	if k := activeKinds(t, s); k["hr_high"] || !k["no_contact"] {
		t.Fatalf("expected only no_contact, got %v", k)
	}
	s.HandleFrame("gw", mesh.EncodeHealthPacket(9, 4, u32(70), u32(85), -70, 5))
	if k := activeKinds(t, s); k["no_contact"] || !k["spo2_low"] {
		t.Fatalf("expected spo2_low, got %v", k)
	}
}

func TestDeduplicationAcrossGateways(t *testing.T) {
	s := newSvc(t)
	f := mesh.EncodeHealthPacket(9, 42, u32(80), u32(98), -70, 5)
	s.HandleFrame("gw1", f)
	s.HandleFrame("gw2", f)
	rs, _ := s.store.Readings(9, time.Now().Add(-time.Hour), 100)
	if len(rs) != 1 {
		t.Fatalf("expected 1 reading, got %d", len(rs))
	}
}

func TestNodeInfoStored(t *testing.T) {
	s := newSvc(t)
	s.HandleFrame("gw", mesh.EncodeNodeInfoPacket(9, "Пётр", "PT"))
	s.HandleFrame("gw", mesh.EncodeBatteryPacket(9, 55))
	s.HandleFrame("gw", mesh.EncodeHealthPacket(9, 1, u32(66), nil, -70, 5))
	ns, _ := s.store.Nodes()
	if len(ns) != 1 || ns[0].LongName != "Пётр" || *ns[0].Battery != 55 || *ns[0].HeartBPM != 66 || ns[0].NodeID != "!00000009" {
		t.Fatalf("bad node %+v", ns)
	}
}
