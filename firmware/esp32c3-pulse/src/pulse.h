#pragma once
#include <stdint.h>

// Результат измерения. bpm == 0 — нет контакта или сигнал нестабилен.
struct PulseReading {
  bool finger;
  uint16_t bpm;
  uint8_t spo2;  // 0 — нет оценки
};

bool pulseBegin();
void pulseLoop();            // вызывать как можно чаще
PulseReading pulseRead();    // потокобезопасный снимок последних значений
