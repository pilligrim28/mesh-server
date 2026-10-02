package mesh

import (
	"encoding/hex"
	"testing"
)

func u32(v uint32) *uint32 { return &v }

func TestDecodeHealth(t *testing.T) {
	ev, err := Decode(EncodeHealthPacket(0x1234abcd, 77, u32(72), u32(98), -80, 5.5), "gw")
	if err != nil || len(ev) != 1 {
		t.Fatalf("events=%v err=%v", ev, err)
	}
	h := ev[0].(Health)
	if h.NodeNum != 0x1234abcd || h.PacketID != 77 || *h.HeartBPM != 72 || *h.SpO2 != 98 || h.RSSI != -80 || h.HopsAway != 1 {
		t.Fatalf("bad health: %+v", h)
	}
}

func TestDecodeNoContact(t *testing.T) {
	ev, _ := Decode(EncodeHealthPacket(1, 2, nil, nil, -80, 1), "gw")
	if h := ev[0].(Health); h.HeartBPM != nil || h.SpO2 != nil {
		t.Fatalf("expected empty health, got %+v", h)
	}
}

func TestDecodeNodeInfoAndBattery(t *testing.T) {
	ev, _ := Decode(EncodeNodeInfoPacket(5, "Иван", "IV"), "gw")
	if ni := ev[0].(NodeInfo); ni.LongName != "Иван" || ni.ShortName != "IV" || ni.HWModel != "HELTEC_V3" {
		t.Fatalf("bad nodeinfo %+v", ni)
	}
	ev, _ = Decode(EncodeBatteryPacket(5, 77), "gw")
	if b := ev[0].(Battery); b.Level != 77 {
		t.Fatalf("bad battery %+v", b)
	}
}

// Байты в точности как их формирует прошивка ESP32-C3 (firmware/esp32c3-pulse/src/proto.h),
// но обёрнутые узлом в FromRadio.packet. Проверяем совместимость ручного энкодера прошивки.
func TestFirmwareTelemetryEncoding(t *testing.T) {
	// Telemetry{health_metrics{heart_bpm:75, spO2:97}}
	tel := []byte{0x3A, 0x04, 0x08, 75, 0x10, 97}
	data := append([]byte{0x08, 67, 0x12, byte(len(tel))}, tel...)                                        // Data{portnum:67,payload}
	pkt := append([]byte{0x0D, 1, 0, 0, 0, 0x15, 0xff, 0xff, 0xff, 0xff, 0x22, byte(len(data))}, data...) // from=1,to=bcast,decoded
	fr := append([]byte{0x12, byte(len(pkt))}, pkt...)                                                    // FromRadio{packet}
	ev, err := Decode(fr, "gw")
	if err != nil || len(ev) != 1 {
		t.Fatalf("decode: %v %v (%s)", ev, err, hex.EncodeToString(fr))
	}
	if h := ev[0].(Health); *h.HeartBPM != 75 || *h.SpO2 != 97 || h.NodeNum != 1 {
		t.Fatalf("bad %+v", h)
	}
}

func TestEncryptedPacketIgnored(t *testing.T) {
	// FromRadio{packet{from:1, encrypted:"xx"}}
	fr := []byte{0x12, 0x09, 0x0D, 1, 0, 0, 0, 0x2A, 0x02, 'x', 'x'}
	if ev, err := Decode(fr, "gw"); err != nil || len(ev) != 0 {
		t.Fatalf("expected nothing, got %v %v", ev, err)
	}
}

func TestToRadioMessages(t *testing.T) {
	if got := hex.EncodeToString(WantConfig(5)); got != "1805" {
		t.Fatalf("want_config = %s", got)
	}
	if got := hex.EncodeToString(Heartbeat()); got != "3a00" {
		t.Fatalf("heartbeat = %s", got)
	}
}
