/*
  MaxPulse — Arduino Pro Micro + MAX30102, обмен с ПК по USB (COM-порт).

  Подключение:  MAX30102 SDA → пин 2 (SDA)   SCL → пин 3 (SCL)   VIN → VCC   GND → GND

  Протокол — текстовые строки, оканчиваются \n (можно смотреть в мониторе порта Arduino IDE).

  Arduino → ПК:
    HELLO,MAXPULSE,<версия>                        приветствие (при старте и на PING)
    D,<мс>,<палец 0/1>,<пульс>,<SpO2>,<IR>,<RED>   сводка раз в RATE мс (0 = нет значения)
    B,<мс>,<мгновенный пульс>                      каждый удар сердца
    R,<мс>,<IR>,<RED>                              сырой сигнал 100 Гц (если RAW ON)
    S,<run>,<raw>,<rate>,<led>,<temp*10>           состояние (на STATUS)
    OK,<команда> / ERR,<причина>

  ПК → Arduino:
    PING | STATUS | START | STOP | RAW ON | RAW OFF | RATE <200..10000> | LED <0..255>
*/
#include <Arduino.h>
#include <Wire.h>
#include <MAX30105.h>
#include <heartRate.h>

#define FW_VERSION "1.0"
#define FINGER_IR 50000UL   // ниже — датчик не касается кожи
#define BEATS 8

MAX30105 sensor;
bool sensorOk = false, running = true, rawOn = false;
uint16_t rateMs = 1000;
uint8_t ledPower = 0x1F;

uint8_t beatBpm[BEATS];
uint8_t beatCount = 0, beatPos = 0;
uint32_t lastBeat = 0, lastSummary = 0;
uint32_t lastIr = 0, lastRed = 0;
bool finger = false;

// окно между ударами для оценки SpO2
uint32_t irMin, irMax, redMin, redMax;
uint32_t irSum, redSum;  // суммы /16, чтобы не переполнить 32 бита
uint16_t winN;
float spo2Avg = 0;

char line[24];
uint8_t lineLen = 0;

void resetWindow() { irMin = redMin = 0xFFFFFFFF; irMax = redMax = 0; irSum = redSum = 0; winN = 0; }

void configureSensor() {
  // яркость, усреднение 4, Red+IR, 400 Гц/4 = 100 отсчётов/с, 411 мкс, диапазон 16384
  sensor.setup(ledPower, 4, 2, 400, 411, 16384);
}

void hello() { Serial.print(F("HELLO,MAXPULSE," FW_VERSION "\n")); }

uint8_t medianBpm() {
  uint8_t t[BEATS], n = beatCount;
  for (uint8_t i = 0; i < n; i++) t[i] = beatBpm[i];
  for (uint8_t i = 1; i < n; i++)
    for (uint8_t j = i; j > 0 && t[j - 1] > t[j]; j--) { uint8_t x = t[j]; t[j] = t[j - 1]; t[j - 1] = x; }
  return n ? t[n / 2] : 0;
}

bool stable() { return beatCount >= 4 && lastBeat && millis() - lastBeat < 3000; }

void status() {
  int16_t temp = sensorOk ? (int16_t)(sensor.readTemperature() * 10) : 0;
  Serial.print(F("S,")); Serial.print(running); Serial.print(','); Serial.print(rawOn); Serial.print(',');
  Serial.print(rateMs); Serial.print(','); Serial.print(ledPower); Serial.print(','); Serial.print(temp); Serial.print('\n');
}

void command(char *c) {
  for (char *p = c; *p; p++) *p = toupper(*p);
  if (!strcmp(c, "PING")) { hello(); return; }
  if (!strcmp(c, "STATUS")) { status(); return; }
  if (!strcmp(c, "START")) running = true;
  else if (!strcmp(c, "STOP")) running = false;
  else if (!strcmp(c, "RAW ON")) rawOn = true;
  else if (!strcmp(c, "RAW OFF")) rawOn = false;
  else if (!strncmp(c, "RATE ", 5)) {
    long v = atol(c + 5);
    if (v < 200 || v > 10000) { Serial.print(F("ERR,RATE 200..10000\n")); return; }
    rateMs = v;
  } else if (!strncmp(c, "LED ", 4)) {
    long v = atol(c + 4);
    if (v < 0 || v > 255) { Serial.print(F("ERR,LED 0..255\n")); return; }
    ledPower = v;
    if (sensorOk) configureSensor();
  } else { Serial.print(F("ERR,UNKNOWN ")); Serial.print(c); Serial.print('\n'); return; }
  Serial.print(F("OK,")); Serial.print(c); Serial.print('\n');
}

void readCommands() {
  while (Serial.available()) {
    char ch = Serial.read();
    if (ch == '\r') continue;
    if (ch == '\n') { line[lineLen] = 0; if (lineLen) command(line); lineLen = 0; }
    else if (lineLen < sizeof(line) - 1) line[lineLen++] = ch;
  }
}

void processSample(uint32_t ir, uint32_t red) {
  uint32_t now = millis();
  lastIr = ir; lastRed = red;
  if (rawOn && running) {
    Serial.print(F("R,")); Serial.print(now); Serial.print(','); Serial.print(ir); Serial.print(','); Serial.print(red); Serial.print('\n');
  }
  if (ir < FINGER_IR) {
    finger = false; beatCount = 0; beatPos = 0; lastBeat = 0; spo2Avg = 0; resetWindow();
    return;
  }
  finger = true;
  if (ir < irMin) irMin = ir;
  if (ir > irMax) irMax = ir;
  if (red < redMin) redMin = red;
  if (red > redMax) redMax = red;
  irSum += ir >> 4; redSum += red >> 4; winN++;

  if (checkForBeat(ir)) {
    if (lastBeat) {
      float bpm = 60000.0 / (now - lastBeat);
      if (bpm >= 30 && bpm <= 220) {
        beatBpm[beatPos] = (uint8_t)bpm; beatPos = (beatPos + 1) % BEATS;
        if (beatCount < BEATS) beatCount++;
        if (running) { Serial.print(F("B,")); Serial.print(now); Serial.print(','); Serial.print((int)bpm); Serial.print('\n'); }
        if (winN > 10 && irMax > irMin && redMax > redMin) {
          float irDC = (float)irSum * 16 / winN, redDC = (float)redSum * 16 / winN;
          float R = ((redMax - redMin) / redDC) / ((irMax - irMin) / irDC);
          float s = 110.0 - 25.0 * R;
          if (s >= 70 && s <= 100) spo2Avg = spo2Avg == 0 ? s : spo2Avg * 0.8 + s * 0.2;
        }
      }
    }
    lastBeat = now;
    resetWindow();
  }
}

void summary() {
  bool st = finger && stable();
  Serial.print(F("D,")); Serial.print(millis()); Serial.print(','); Serial.print(finger); Serial.print(',');
  Serial.print(st ? medianBpm() : 0); Serial.print(','); Serial.print(st && spo2Avg > 0 ? (int)(spo2Avg + 0.5) : 0);
  Serial.print(','); Serial.print(lastIr); Serial.print(','); Serial.print(lastRed); Serial.print('\n');
}

void setup() {
  Serial.begin(115200);
  uint32_t t0 = millis();
  while (!Serial && millis() - t0 < 3000) {}  // ждём открытия порта, но не вечно
  Wire.begin();
  Wire.setClock(400000);
  sensorOk = sensor.begin(Wire, I2C_SPEED_FAST);
  if (sensorOk) { configureSensor(); sensor.enableDIETEMPRDY(); }
  resetWindow();
  hello();
  if (!sensorOk) Serial.print(F("ERR,SENSOR_NOT_FOUND\n"));
}

void loop() {
  readCommands();
  if (!sensorOk) {
    static uint32_t lastTry = 0;
    if (millis() - lastTry > 3000) {   // пробуем переподключить датчик
      lastTry = millis();
      sensorOk = sensor.begin(Wire, I2C_SPEED_FAST);
      if (sensorOk) { configureSensor(); Serial.print(F("OK,SENSOR_FOUND\n")); }
      else Serial.print(F("ERR,SENSOR_NOT_FOUND\n"));
    }
    return;
  }
  sensor.check();
  while (sensor.available()) {
    processSample(sensor.getFIFOIR(), sensor.getFIFORed());
    sensor.nextSample();
  }
  if (running && millis() - lastSummary >= rateMs) { lastSummary = millis(); summary(); }
}
