#pragma once
// ===================== Настройки датчика =====================

// --- Пины (ESP32-C3 SuperMini; для других плат поменяйте) ---
#define PIN_I2C_SDA 6
#define PIN_I2C_SCL 7
#define PIN_LED 8            // встроенный синий светодиод SuperMini
#define LED_ACTIVE_LOW 1     // на SuperMini светодиод горит при LOW
#define PIN_BUTTON 9         // кнопка BOOT: удерживать 3 с при включении = забыть узел

// --- Связь с узлом Meshtastic по BLE ---
// PIN узла: тот же, что USERPREFS_FIXED_BLUETOOTH в прошивке носимого Heltec.
// Для узла в режиме NO_PIN значение не используется.
#define MESH_BLE_PIN 123456

// Пусто — при первом запуске подключиться к БЛИЖАЙШЕМУ узлу Meshtastic и запомнить его.
// Либо укажите BLE-имя своего узла, например "Meshtastic_1a2b".
#define MESH_TARGET_NAME ""

// --- Отправка данных ---
// Интервал отправки в mesh, сек. Не меньше 15: прошивка Meshtastic принимает
// телеметрию от клиента не чаще раза в 10 с, а LoRa-эфир общий для всех.
#define SEND_INTERVAL_S 30
// Отправить сразу (не дожидаясь интервала), если пульс вышел за эти пороги
#define URGENT_HR_LOW 45
#define URGENT_HR_HIGH 150
#define MIN_SEND_GAP_S 12    // защита от лимита прошивки Meshtastic (10 с)
#define HOP_LIMIT 3          // сколько ретрансляций в mesh

// --- MAX30102 ---
#define FINGER_IR_THRESHOLD 50000  // ниже — датчик не касается кожи
#define LED_BRIGHTNESS 0x1F        // 0..255, ~6 мА; увеличьте при слабом сигнале
