// MAX30102: пульс по ИК-каналу (детектор ударов SparkFun) и оценка SpO2 по отношению R = (AC/DC)red / (AC/DC)ir.
#include "pulse.h"
#include "config.h"
#include <Arduino.h>
#include <Wire.h>
#include <MAX30105.h>
#include <heartRate.h>

static MAX30105 sensor;
static portMUX_TYPE mux = portMUX_INITIALIZER_UNLOCKED;
static PulseReading current = {false, 0, 0};

static const int BEATS = 8;
static uint16_t beatBpm[BEATS];
static int beatCount = 0, beatPos = 0;
static uint32_t lastBeatMs = 0;

// окно между ударами для SpO2
static uint32_t irMin = UINT32_MAX, irMax = 0, redMin = UINT32_MAX, redMax = 0;
static uint64_t irSum = 0, redSum = 0;
static uint32_t winN = 0;
static float spo2Avg = 0;

static void resetWindow() { irMin = redMin = UINT32_MAX; irMax = redMax = 0; irSum = redSum = 0; winN = 0; }

bool pulseBegin() {
  Wire.begin(PIN_I2C_SDA, PIN_I2C_SCL);
  if (!sensor.begin(Wire, I2C_SPEED_FAST)) return false;
  // яркость, усреднение 4, режим Red+IR, 400 Гц / 4 = 100 отсчётов/с, ширина импульса 411 мкс, диапазон АЦП 16384
  sensor.setup(LED_BRIGHTNESS, 4, 2, 400, 411, 16384);
  return true;
}

static uint16_t medianBpm() {
  uint16_t tmp[BEATS];
  int n = beatCount < BEATS ? beatCount : BEATS;
  for (int i = 0; i < n; i++) tmp[i] = beatBpm[i];
  for (int i = 1; i < n; i++)
    for (int j = i; j > 0 && tmp[j - 1] > tmp[j]; j--) { uint16_t t = tmp[j]; tmp[j] = tmp[j - 1]; tmp[j - 1] = t; }
  return n ? tmp[n / 2] : 0;
}

void pulseLoop() {
  sensor.check();
  while (sensor.available()) {
    uint32_t ir = sensor.getFIFOIR(), red = sensor.getFIFORed();
    sensor.nextSample();
    uint32_t now = millis();

    if (ir < FINGER_IR_THRESHOLD) {  // нет контакта
      beatCount = 0; lastBeatMs = 0; spo2Avg = 0; resetWindow();
      portENTER_CRITICAL(&mux); current = {false, 0, 0}; portEXIT_CRITICAL(&mux);
      continue;
    }
    irMin = min(irMin, ir); irMax = max(irMax, ir); redMin = min(redMin, red); redMax = max(redMax, red);
    irSum += ir; redSum += red; winN++;

    if (checkForBeat(ir)) {
      if (lastBeatMs) {
        uint32_t dt = now - lastBeatMs;
        float bpm = 60000.0f / dt;
        if (bpm >= 30 && bpm <= 220) {
          beatBpm[beatPos] = (uint16_t)bpm; beatPos = (beatPos + 1) % BEATS;
          if (beatCount < BEATS) beatCount++;
          // SpO2 по одному периоду пульса
          if (winN > 10 && irMax > irMin && redMax > redMin) {
            float irDC = (float)irSum / winN, redDC = (float)redSum / winN;
            float R = ((redMax - redMin) / redDC) / ((irMax - irMin) / irDC);
            float s = 110.0f - 25.0f * R;
            if (s >= 70 && s <= 100) spo2Avg = spo2Avg == 0 ? s : spo2Avg * 0.8f + s * 0.2f;
          }
        }
      }
      lastBeatMs = now;
      resetWindow();
    }
    // если ударов нет больше 3 с — сигнал потерян (палец сдвинулся)
    bool stable = beatCount >= 4 && lastBeatMs && now - lastBeatMs < 3000;
    portENTER_CRITICAL(&mux);
    current = {true, stable ? medianBpm() : (uint16_t)0, (uint8_t)(stable && spo2Avg > 0 ? spo2Avg + 0.5f : 0)};
    portEXIT_CRITICAL(&mux);
  }
}

PulseReading pulseRead() {
  portENTER_CRITICAL(&mux);
  PulseReading r = current;
  portEXIT_CRITICAL(&mux);
  return r;
}
