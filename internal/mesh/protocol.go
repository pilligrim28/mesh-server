// Package mesh декодирует поток FromRadio от узла Meshtastic в события сервера
// и формирует служебные сообщения ToRadio. Протокол одинаков для BLE и USB.
package mesh

import (
	"fmt"
	"time"

	pb "buf.build/gen/go/meshtastic/protobufs/protocolbuffers/go/meshtastic"
	"google.golang.org/protobuf/proto"
)

// BLE UUID сервиса Meshtastic (src/BluetoothCommon.h в прошивке).
const (
	ServiceUUID   = "6ba1b218-15a8-461f-9fa8-5dcae273eafd"
	ToRadioUUID   = "f75c76d2-129e-4dad-a1dd-7866124401e7"
	FromRadioUUID = "2c55e69e-4993-11ed-b878-0242ac120002"
	FromNumUUID   = "ed9da18c-a800-4f66-a670-aa7547e34453"
)

// Health — показания датчика пульса, пришедшие по mesh.
type Health struct {
	NodeNum  uint32
	PacketID uint32
	Time     time.Time
	HeartBPM *int // nil — палец не на датчике
	SpO2     *int
	TempC    *float64
	RSSI     int
	SNR      float64
	HopsAway int
	Gateway  string
}

// NodeInfo — имя/модель узла.
type NodeInfo struct {
	NodeNum   uint32
	LongName  string
	ShortName string
	HWModel   string
	Gateway   string
}

// Battery — заряд носимого узла Meshtastic.
type Battery struct {
	NodeNum uint32
	Level   int
	Voltage float64
	Gateway string
}

// MyInfo — номер самого шлюза (узла, к которому подключён сервер).
type MyInfo struct {
	NodeNum uint32
	Gateway string
}

// ConfigComplete — шлюз закончил выдачу конфигурации, начинается поток пакетов.
type ConfigComplete struct{ Gateway string }

type Event any

// NodeIDString форматирует номер узла как в приложении Meshtastic: !a1b2c3d4.
func NodeIDString(n uint32) string { return fmt.Sprintf("!%08x", n) }

// Decode разбирает одно сообщение FromRadio.
func Decode(buf []byte, gateway string) ([]Event, error) {
	var fr pb.FromRadio
	if err := proto.Unmarshal(buf, &fr); err != nil {
		return nil, fmt.Errorf("decode FromRadio: %w", err)
	}
	var out []Event
	switch {
	case fr.GetMyInfo() != nil:
		out = append(out, MyInfo{NodeNum: fr.GetMyInfo().GetMyNodeNum(), Gateway: gateway})
	case fr.GetNodeInfo() != nil:
		ni := fr.GetNodeInfo()
		if u := ni.GetUser(); u != nil {
			out = append(out, NodeInfo{NodeNum: ni.GetNum(), LongName: u.GetLongName(), ShortName: u.GetShortName(),
				HWModel: u.GetHwModel().String(), Gateway: gateway})
		}
		if dm := ni.GetDeviceMetrics(); dm != nil && dm.BatteryLevel != nil {
			out = append(out, Battery{NodeNum: ni.GetNum(), Level: int(dm.GetBatteryLevel()), Voltage: float64(dm.GetVoltage()), Gateway: gateway})
		}
	case fr.GetConfigCompleteId() != 0:
		out = append(out, ConfigComplete{Gateway: gateway})
	case fr.GetPacket() != nil:
		out = append(out, decodePacket(fr.GetPacket(), gateway)...)
	}
	return out, nil
}

func decodePacket(p *pb.MeshPacket, gateway string) []Event {
	d := p.GetDecoded()
	if d == nil { // зашифровано другим ключом канала — не наш пакет
		return nil
	}
	switch d.GetPortnum() {
	case pb.PortNum_TELEMETRY_APP:
		var t pb.Telemetry
		if proto.Unmarshal(d.GetPayload(), &t) != nil {
			return nil
		}
		if hm := t.GetHealthMetrics(); hm != nil {
			h := Health{NodeNum: p.GetFrom(), PacketID: p.GetId(), Time: packetTime(p, &t),
				RSSI: int(p.GetRxRssi()), SNR: float64(p.GetRxSnr()), Gateway: gateway}
			if p.GetHopStart() >= p.GetHopLimit() {
				h.HopsAway = int(p.GetHopStart() - p.GetHopLimit())
			}
			if hm.HeartBpm != nil {
				v := int(hm.GetHeartBpm())
				h.HeartBPM = &v
			}
			if hm.SpO2 != nil {
				v := int(hm.GetSpO2())
				h.SpO2 = &v
			}
			if hm.Temperature != nil {
				v := float64(hm.GetTemperature())
				h.TempC = &v
			}
			return []Event{h}
		}
		if dm := t.GetDeviceMetrics(); dm != nil && dm.BatteryLevel != nil {
			return []Event{Battery{NodeNum: p.GetFrom(), Level: int(dm.GetBatteryLevel()), Voltage: float64(dm.GetVoltage()), Gateway: gateway}}
		}
	case pb.PortNum_NODEINFO_APP:
		var u pb.User
		if proto.Unmarshal(d.GetPayload(), &u) == nil {
			return []Event{NodeInfo{NodeNum: p.GetFrom(), LongName: u.GetLongName(), ShortName: u.GetShortName(),
				HWModel: u.GetHwModel().String(), Gateway: gateway}}
		}
	}
	return nil
}

func packetTime(p *pb.MeshPacket, t *pb.Telemetry) time.Time {
	now := time.Now()
	for _, ts := range []uint32{t.GetTime(), p.GetRxTime()} {
		if ts == 0 {
			continue
		}
		tm := time.Unix(int64(ts), 0)
		// часы узла могут быть не выставлены — берём время сервера
		if d := now.Sub(tm); d > -time.Minute && d < time.Hour {
			return tm
		}
	}
	return now
}

// WantConfig — запрос, после которого узел выдаёт базу узлов и начинает поток пакетов.
func WantConfig(nonce uint32) []byte {
	b, _ := proto.Marshal(pb.ToRadio_builder{WantConfigId: proto.Uint32(nonce)}.Build())
	return b
}

// Heartbeat держит соединение активным (иначе узел считает клиента отключённым).
func Heartbeat() []byte {
	b, _ := proto.Marshal(pb.ToRadio_builder{Heartbeat: &pb.Heartbeat{}}.Build())
	return b
}

// EncodeHealthPacket формирует FromRadio с телеметрией пульса — используется симулятором и тестами.
func EncodeHealthPacket(from, id uint32, bpm, spo2 *uint32, rssi int32, snr float32) []byte {
	tel, _ := proto.Marshal(pb.Telemetry_builder{
		Time:          uint32(time.Now().Unix()),
		HealthMetrics: pb.HealthMetrics_builder{HeartBpm: bpm, SpO2: spo2}.Build(),
	}.Build())
	return encodePacket(from, id, pb.PortNum_TELEMETRY_APP, tel, rssi, snr)
}

// EncodeNodeInfoPacket формирует FromRadio с NodeInfo узла.
func EncodeNodeInfoPacket(from uint32, long, short string) []byte {
	u, _ := proto.Marshal(pb.User_builder{Id: NodeIDString(from), LongName: long, ShortName: short,
		HwModel: pb.HardwareModel_HELTEC_V3}.Build())
	return encodePacket(from, 0, pb.PortNum_NODEINFO_APP, u, -60, 8)
}

// EncodeBatteryPacket формирует FromRadio с DeviceMetrics (заряд батареи).
func EncodeBatteryPacket(from uint32, level uint32) []byte {
	tel, _ := proto.Marshal(pb.Telemetry_builder{DeviceMetrics: pb.DeviceMetrics_builder{
		BatteryLevel: proto.Uint32(level), Voltage: proto.Float32(3.6 + float32(level)/200)}.Build()}.Build())
	return encodePacket(from, 0, pb.PortNum_TELEMETRY_APP, tel, -60, 8)
}

func encodePacket(from, id uint32, port pb.PortNum, payload []byte, rssi int32, snr float32) []byte {
	pkt := pb.MeshPacket_builder{
		From: from, To: 0xffffffff, Id: id, RxRssi: rssi, RxSnr: snr, HopStart: 3, HopLimit: 2,
		Decoded: pb.Data_builder{Portnum: port, Payload: payload}.Build(),
	}.Build()
	b, _ := proto.Marshal(pb.FromRadio_builder{Packet: pkt}.Build())
	return b
}
