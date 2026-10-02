// Носимый датчик пульса: ESP32-C3 + MAX30102.
// Подключается по BLE к узлу Meshtastic (как приложение на телефоне) и отправляет
// стандартную телеметрию HealthMetrics. Узел передаёт её по LoRa mesh до шлюза сервера.
//
// Светодиод:  частое мигание — поиск узла; двойная вспышка — подключение;
//             вспышка на каждый удар — подключено, пульс есть; редкая вспышка — подключено, нет контакта;
//             SOS-серия при старте — датчик MAX30102 не найден.
#include <Arduino.h>
#include <NimBLEDevice.h>
#include <Preferences.h>
#include <esp_random.h>

#include "config.h"
#include "proto.h"
#include "pulse.h"

static const char *SERVICE_UUID = "6ba1b218-15a8-461f-9fa8-5dcae273eafd";
static const char *TORADIO_UUID = "f75c76d2-129e-4dad-a1dd-7866124401e7";
static const char *FROMRADIO_UUID = "2c55e69e-4993-11ed-b878-0242ac120002";
static const char *FROMNUM_UUID = "ed9da18c-a800-4f66-a670-aa7547e34453";

enum LinkState { LINK_SCANNING, LINK_CONNECTING, LINK_READY };
static volatile LinkState linkState = LINK_SCANNING;
static volatile bool fromNumPending = false;
static volatile bool disconnected = false;
static bool sensorOk = false;

static Preferences prefs;
static NimBLEClient *client = nullptr;
static NimBLERemoteCharacteristic *chTo = nullptr, *chFrom = nullptr;

// ---------- светодиод ----------
static void led(bool on) { digitalWrite(PIN_LED, (on ^ LED_ACTIVE_LOW) ? HIGH : LOW); }

static void ledTask(void *) {
  uint32_t lastBeat = 0;
  for (;;) {
    uint32_t t = millis();
    PulseReading r = pulseRead();
    switch (linkState) {
      case LINK_SCANNING: led((t / 150) % 2); break;
      case LINK_CONNECTING: led((t % 1000) < 100 || ((t % 1000) > 200 && (t % 1000) < 300)); break;
      case LINK_READY:
        if (r.bpm) {
          uint32_t period = 60000 / r.bpm;
          if (t - lastBeat >= period) lastBeat = t;
          led(t - lastBeat < 60);
        } else {
          led((t % 3000) < 40);
        }
        break;
    }
    vTaskDelay(pdMS_TO_TICKS(20));
  }
}

// ---------- BLE ----------
class ClientCB : public NimBLEClientCallbacks {
  void onDisconnect(NimBLEClient *c, int reason) override {
    Serial.printf("[BLE] отключено, причина %d\n", reason);
    disconnected = true;
  }
  void onPassKeyEntry(NimBLEConnInfo &info) override {
    Serial.println("[BLE] узел запросил PIN — отправляю MESH_BLE_PIN");
    NimBLEDevice::injectPassKey(info, MESH_BLE_PIN);
  }
  void onConfirmPasskey(NimBLEConnInfo &info, uint32_t pin) override { NimBLEDevice::injectConfirmPasskey(info, true); }
  void onAuthenticationComplete(NimBLEConnInfo &info) override {
    Serial.printf("[BLE] сопряжение: %s\n", info.isEncrypted() ? "успешно" : "не выполнено (узел без PIN?)");
  }
};
static ClientCB clientCB;

static void onFromNum(NimBLERemoteCharacteristic *, uint8_t *, size_t, bool) { fromNumPending = true; }

static bool writeToRadio(const pbw::Buf &b) {
  return chTo && chTo->writeValue(b.d, b.n, true);
}

// Вычитывает очередь FromRadio узла (базу узлов, чужие пакеты). Содержимое датчику не нужно,
// но узел ждёт, что клиент читает очередь.
static void drainFromRadio(int maxReads) {
  for (int i = 0; i < maxReads && chFrom && client->isConnected(); i++) {
    NimBLEAttValue v = chFrom->readValue();
    if (v.size() == 0) return;
  }
}

static bool savedTarget(std::string &addr, uint8_t &type) {
  addr = prefs.getString("addr", "").c_str();
  type = prefs.getUChar("type", 0);
  return !addr.empty();
}

// Поиск узла: сохранённый адрес > имя из config.h > ближайший по RSSI.
static bool findNode(NimBLEAddress &out, std::string &name) {
  std::string want; uint8_t wantType;
  bool haveSaved = savedTarget(want, wantType);
  NimBLEScan *scan = NimBLEDevice::getScan();
  scan->setActiveScan(true);
  scan->setInterval(100);
  scan->setWindow(99);
  NimBLEScanResults res = scan->getResults(5000, false);
  const NimBLEAdvertisedDevice *best = nullptr;
  NimBLEUUID svc(SERVICE_UUID);
  for (int i = 0; i < res.getCount(); i++) {
    const NimBLEAdvertisedDevice *d = res.getDevice(i);
    bool isMesh = d->isAdvertisingService(svc) || d->getName().rfind("Meshtastic", 0) == 0;
    // шлюз сервера (GW_xxxx) не занимаем: у узла Meshtastic только одно BLE-подключение
    if (d->getName().rfind("GW_", 0) == 0) continue;
    if (haveSaved) {
      if (d->getAddress().toString() == want) { best = d; break; }
      continue;
    }
    if (strlen(MESH_TARGET_NAME) > 0) {
      if (d->getName() == MESH_TARGET_NAME) { best = d; break; }
      continue;
    }
    if (isMesh && (!best || d->getRSSI() > best->getRSSI())) best = d;
  }
  if (!best) {
    Serial.printf("[BLE] узел не найден (%s)\n", haveSaved ? ("ищу сохранённый " + want).c_str() : "ищу ближайший");
    scan->clearResults();
    return false;
  }
  out = best->getAddress();
  name = best->getName();
  Serial.printf("[BLE] найден %s [%s] RSSI %d\n", name.c_str(), out.toString().c_str(), best->getRSSI());
  scan->clearResults();
  return true;
}

static bool connectNode() {
  NimBLEAddress addr; std::string name;
  linkState = LINK_SCANNING;
  if (!findNode(addr, name)) return false;
  linkState = LINK_CONNECTING;
  disconnected = false;
  if (!client) {
    client = NimBLEDevice::createClient();
    client->setClientCallbacks(&clientCB, false);
    client->setConnectTimeout(10 * 1000);
  }
  if (!client->connect(addr)) { Serial.println("[BLE] не удалось подключиться"); return false; }
  client->secureConnection();  // для FIXED_PIN — сопряжение с PIN; для NO_PIN ошибка не критична

  NimBLERemoteService *s = client->getService(SERVICE_UUID);
  if (!s) { Serial.println("[BLE] нет сервиса Meshtastic"); client->disconnect(); return false; }
  chTo = s->getCharacteristic(TORADIO_UUID);
  chFrom = s->getCharacteristic(FROMRADIO_UUID);
  NimBLERemoteCharacteristic *chNum = s->getCharacteristic(FROMNUM_UUID);
  if (!chTo || !chFrom) { Serial.println("[BLE] нет ToRadio/FromRadio"); client->disconnect(); return false; }
  if (chNum && chNum->canNotify()) chNum->subscribe(true, onFromNum);

  if (!writeToRadio(pbw::wantConfig(esp_random() | 1))) {
    Serial.println("[BLE] запись отклонена — неверный PIN? (проверьте MESH_BLE_PIN)");
    client->disconnect();
    return false;
  }
  drainFromRadio(1500);

  // запоминаем «свой» узел, чтобы не перескочить на чужой
  std::string saved; uint8_t t;
  if (!savedTarget(saved, t)) {
    prefs.putString("addr", addr.toString().c_str());
    prefs.putUChar("type", addr.getType());
    Serial.printf("[BLE] узел %s запомнен. Сброс: удерживать BOOT при включении\n", addr.toString().c_str());
  }
  Serial.printf("[BLE] подключено к %s\n", name.c_str());
  linkState = LINK_READY;
  return true;
}

static void sendHealth(const PulseReading &r) {
  uint32_t id = esp_random() & 0x7FFFFFFF;
  if (!id) id = 1;
  bool ok = writeToRadio(pbw::healthToRadio(id, r.bpm, r.spo2, HOP_LIMIT));
  if (r.bpm) Serial.printf("[MESH] -> пульс %u, SpO2 %u%% %s\n", r.bpm, r.spo2, ok ? "OK" : "ОШИБКА");
  else Serial.printf("[MESH] -> нет контакта с кожей %s\n", ok ? "OK" : "ОШИБКА");
}

static void bleTask(void *) {
  uint32_t failures = 0, lastSend = 0, lastHeartbeat = 0;
  bool lastFinger = false, lastUrgent = false, firstSend = true;
  for (;;) {
    if (!client || !client->isConnected() || disconnected) {
      linkState = LINK_SCANNING;
      if (!connectNode()) {
        vTaskDelay(pdMS_TO_TICKS(min<uint32_t>(2000 * ++failures, 15000)));
        continue;
      }
      failures = 0; firstSend = true;
    }
    uint32_t now = millis();
    if (fromNumPending) { fromNumPending = false; drainFromRadio(50); }

    PulseReading r = pulseRead();
    bool urgent = r.bpm && (r.bpm < URGENT_HR_LOW || r.bpm > URGENT_HR_HIGH);
    bool changed = r.finger != lastFinger || urgent != lastUrgent;
    bool due = firstSend || now - lastSend >= SEND_INTERVAL_S * 1000UL;
    // при смене состояния отправляем раньше, но не чаще MIN_SEND_GAP_S
    if (sensorOk && (due || (changed && now - lastSend >= MIN_SEND_GAP_S * 1000UL))) {
      // сразу после касания пульс ещё не определён — даём ему 8 с, если это не плановая отправка
      if (!due && r.finger && !r.bpm) { vTaskDelay(pdMS_TO_TICKS(200)); continue; }
      sendHealth(r);
      lastSend = now; firstSend = false; lastFinger = r.finger; lastUrgent = urgent;
    }
    if (now - lastHeartbeat > 60000) { writeToRadio(pbw::heartbeat()); lastHeartbeat = now; }
    vTaskDelay(pdMS_TO_TICKS(200));
  }
}

void setup() {
  Serial.begin(115200);
  pinMode(PIN_LED, OUTPUT);
  pinMode(PIN_BUTTON, INPUT_PULLUP);
  led(false);
  delay(1500);
  Serial.println("\n=== Mesh Pulse: ESP32-C3 + MAX30102 ===");

  prefs.begin("meshpulse", false);
  uint32_t t0 = millis();
  while (digitalRead(PIN_BUTTON) == LOW && millis() - t0 < 3000) delay(10);
  if (millis() - t0 >= 3000) {
    prefs.clear();
    NimBLEDevice::init("");
    NimBLEDevice::deleteAllBonds();
    Serial.println("Сохранённый узел и сопряжения удалены");
    for (int i = 0; i < 10; i++) { led(i % 2); delay(100); }
  }

  sensorOk = pulseBegin();
  if (!sensorOk) {
    Serial.println("MAX30102 не найден! Проверьте SDA/SCL (config.h) и питание 3.3 В");
    for (int i = 0; i < 3; i++) { for (int j = 0; j < 6; j++) { led(j % 2 == 0); delay(j < 2 ? 150 : 400); } }
  } else {
    Serial.println("MAX30102 OK");
  }

  NimBLEDevice::init("MeshPulse-C3");
  NimBLEDevice::setPower(9);                              // dBm, максимум для C3
  NimBLEDevice::setMTU(512);                              // пакеты FromRadio до 512 байт
  NimBLEDevice::setSecurityAuth(true, true, true);        // bonding + MITM + secure connections
  NimBLEDevice::setSecurityIOCap(BLE_HS_IO_KEYBOARD_ONLY); // «вводим» PIN узла

  xTaskCreate(ledTask, "led", 2048, nullptr, 1, nullptr);
  xTaskCreate(bleTask, "ble", 8192, nullptr, 2, nullptr);
}

void loop() {
  if (sensorOk) pulseLoop();
  // команда в мониторе порта: forget — забыть узел
  if (Serial.available()) {
    String cmd = Serial.readStringUntil('\n');
    cmd.trim();
    if (cmd == "forget") { prefs.clear(); NimBLEDevice::deleteAllBonds(); Serial.println("Узел забыт, перезагрузка"); delay(200); ESP.restart(); }
    if (cmd == "status") { PulseReading r = pulseRead(); Serial.printf("link=%d finger=%d bpm=%u spo2=%u\n", linkState, r.finger, r.bpm, r.spo2); }
  }
  delay(2);
}
