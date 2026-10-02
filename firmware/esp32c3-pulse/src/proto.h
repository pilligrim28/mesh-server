#pragma once
// Минимальный ручной энкодер protobuf для сообщений Meshtastic, которые отправляет датчик.
// Совместимость проверяется тестом сервера internal/mesh TestFirmwareTelemetryEncoding.
#include <stdint.h>
#include <stddef.h>

namespace pbw {

struct Buf {
  uint8_t d[200];
  size_t n = 0;
  void byte(uint8_t b) { if (n < sizeof(d)) d[n++] = b; }
  void varint(uint32_t v) { while (v >= 0x80) { byte((v & 0x7F) | 0x80); v >>= 7; } byte(v); }
  void tag(uint32_t field, uint8_t wire) { varint((field << 3) | wire); }
  void fixed32(uint32_t field, uint32_t v) { tag(field, 5); for (int i = 0; i < 4; i++) byte(v >> (8 * i)); }
  void uvarint(uint32_t field, uint32_t v) { tag(field, 0); varint(v); }
  void bytes(uint32_t field, const Buf &b) { tag(field, 2); varint(b.n); for (size_t i = 0; i < b.n; i++) byte(b.d[i]); }
};

// ToRadio{ packet: MeshPacket{ to: broadcast, decoded: Data{ portnum: TELEMETRY_APP,
//   payload: Telemetry{ health_metrics: { heart_bpm, spO2 } } }, id, hop_limit } }
// bpm == 0 -> пустые health_metrics: «датчик не на коже».
inline Buf healthToRadio(uint32_t packetId, uint32_t bpm, uint32_t spo2, uint8_t hopLimit) {
  Buf health; if (bpm) health.uvarint(1, bpm); if (bpm && spo2) health.uvarint(2, spo2);
  Buf tel; tel.bytes(7, health);                       // Telemetry.health_metrics = 7
  Buf data; data.uvarint(1, 67); data.bytes(2, tel);   // Data.portnum = TELEMETRY_APP(67), payload = 2
  Buf pkt;
  pkt.fixed32(2, 0xFFFFFFFF);                          // MeshPacket.to = broadcast
  pkt.bytes(4, data);                                  // MeshPacket.decoded = 4
  pkt.fixed32(6, packetId);                            // MeshPacket.id = 6
  pkt.uvarint(9, hopLimit);                            // MeshPacket.hop_limit = 9
  Buf to; to.bytes(1, pkt);                            // ToRadio.packet = 1
  return to;
}

inline Buf wantConfig(uint32_t nonce) { Buf b; b.uvarint(3, nonce); return b; }   // ToRadio.want_config_id = 3
inline Buf heartbeat() { Buf b; b.tag(7, 2); b.varint(0); return b; }            // ToRadio.heartbeat = 7

}  // namespace pbw
