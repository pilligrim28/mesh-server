# Прошивка для Raspberry Pi Pico W

## Обзор

Эта прошивка превращает Raspberry Pi Pico W в Meshtastic устройство с поддержкой:
- WiFi подключения к mesh-server
- Отправки/получения сообщений через HTTP API
- Подключения по Bluetooth LE (опционально)

## Требования

### Железо:
- Raspberry Pi Pico W (с WiFi)
- Опционально: RFM95/RFM96 LoRa модуль (для связи по LoRa)
- Опционально: OLED дисплей 0.96" I2C

### ПО:
- Raspberry Pi Pico SDK
- Arduino IDE или PlatformIO
- Python 3.x (для скриптов)

## Установка

### Вариант 1: Arduino IDE

1. Установите Arduino IDE
2. Добавьте поддержку Raspberry Pi Pico:
   - File → Preferences
   - Добавьте URL: `https://github.com/earlephilhower/arduino-pico/releases/download/global/package_rp2040_index.json`
   - Tools → Board → Boards Manager
   - Найдите "Raspberry Pi Pico/RP2040" и установите

3. Установите библиотеки:
   - WiFiLibrary (встроена)
   - ArduinoJson
   - HTTPClient
   - Meshtastic (опционально)

### Вариант 2: PlatformIO

```ini
; platformio.ini
[env:pico]
platform = raspberrypi
board = pico
framework = arduino
lib_deps = 
    bblanchon/ArduinoJson@^6.21.3
    meshtastic/Meshtastic@^2.0
```

## Настройка WiFi

Откройте `pico_firmware.ino` и измените настройки:

```cpp
const char* ssid = "YOUR_WIFI_SSID";
const char* password = "YOUR_WIFI_PASSWORD";
const char* serverUrl = "http://192.168.1.100:8080";  // Адрес mesh-server
```

## Прошивка

### Arduino IDE:
1. Откройте `pico_firmware.ino`
2. Выберите плату: Tools → Board → Raspberry Pi Pico W
3. Нажмите Upload

### PlatformIO:
```bash
pio run -t upload
```

### Manual (UF2):
1. Соберите прошивку: `pio run`
2. Скопируйте `.uf2` файл на Pico в режиме загрузчика

## API

### Отправка сообщения на сервер

```cpp
// Формирование сообщения
StaticJsonDocument<256> doc;
doc["device_id"] = 1;
doc["from_node"] = "!12345678";
doc["to_node"] = "!87654321";
doc["text"] = "Привет!";
doc["direction"] = "outbound";

// Отправка на сервер
sendToServer("/api/messages", doc);
```

### Получение сообщений

```cpp
// Запрос новых сообщений
GET /api/messages/device?device_id=1&limit=10

// Обработка ответа
for (auto msg : messages) {
    if (msg.direction == "inbound") {
        displayMessage(msg.from_node, msg.text);
    }
}
```

## Структура проекта

```
firmware/pico/
├── pico_firmware.ino      # Основная прошивка (Arduino)
├── src/
│   ├── wifi_manager.cpp   # Управление WiFi
│   ├── message_handler.cpp # Обработка сообщений
│   ├── display.cpp        # OLED дисплей
│   └── lora_driver.cpp    # LoRa драйвер
├── include/
│   ├── config.h           # Конфигурация
│   └── types.h            # Типы данных
├── lib/
│   └── meshtastic/        # Meshtastic библиотека
├── platformio.ini         # PlatformIO конфиг
└── README.md              # Эта документация
```

## Подключение компонентов

### LoRa модуль (RFM95/RFM96):

```
Pico W      RFM95
--------    -----
3.3V        VCC
GND         GND
GPIO 17     NSS
GPIO 18     SCK
GPIO 19     MOSI
GPIO 16     MISO
GPIO 20     RST
GPIO 21     DIO0
```

### OLED дисплей (0.96" I2C):

```
Pico W      OLED
--------    ----
3.3V        VCC
GND         GND
GPIO 4      SDA
GPIO 5      SCL
```

## Конфигурация

### Настройки WiFi:
```cpp
#define WIFI_SSID "YourSSID"
#define WIFI_PASSWORD "YourPassword"
```

### Настройки сервера:
```cpp
#define SERVER_URL "http://192.168.1.100:8080"
#define DEVICE_ID 1
```

### Настройки LoRa:
```cpp
#define LORA_FREQUENCY 868.0  // MHz (Европа)
#define LORA_TX_POWER 17      // dBm
#define LORA_BANDWIDTH 125    // kHz
```

## Тестирование

### Проверка WiFi:
```cpp
if (WiFi.status() == WL_CONNECTED) {
    Serial.println("WiFi connected");
    Serial.println(WiFi.localIP());
}
```

### Проверка подключения к серверу:
```cpp
if (checkServerConnection()) {
    Serial.println("Server reachable");
}
```

### Отправка тестового сообщения:
```cpp
sendMessage("!87654321", "Тест");
```

## Режимы работы

### 1. WiFi мост
Pico W подключается к WiFi и передает сообщения между сервером и телефоном через Bluetooth.

### 2. LoRa ретранслятор
Pico W с LoRa модулем ретранслирует сообщения между mesh-сетью и сервером.

### 3. Автономное устройство
Pico W работает как независимый узел с OLED дисплеем для отображения сообщений.

## Энергосбережение

```cpp
// Глубокий сон
enterDeepSleep(60);  // 60 секунд

// Легкий сон
enterLightSleep(10);  // 10 секунд
```

## Обновление по воздуху (OTA)

```cpp
#include <ArduinoOTA.h>

void setupOTA() {
    ArduinoOTA.setHostname("pico-meshtastic");
    ArduinoOTA.onStart([]() {
        Serial.println("OTA Start");
    });
    ArduinoOTA.onEnd([]() {
        Serial.println("OTA End");
    });
    ArduinoOTA.begin();
}

void loop() {
    ArduinoOTA.handle();
}
```

## Отладка

### Serial вывод:
```cpp
Serial.begin(115200);
while (!Serial) {
    delay(10);
}
Serial.println("Debug info");
```

### Логирование:
```cpp
#define LOG_LEVEL DEBUG  // DEBUG, INFO, WARNING, ERROR

void log(const char* level, const char* message) {
    Serial.printf("[%s] %s\n", level, message);
}
```

## Решение проблем

### Не подключается к WiFi:
1. Проверьте SSID и пароль
2. Убедитесь что WiFi 2.4GHz (Pico W не поддерживает 5GHz)
3. Проверьте уровень сигнала

### Не подключается к серверу:
1. Проверьте что сервер доступен: `ping 192.168.1.100`
2. Проверьте порт: `telnet 192.168.1.100 8080`
3. Проверьте firewall на сервере

### LoRa не работает:
1. Проверьте подключение SPI
2. Убедитесь что частота правильная для вашего региона
3. Проверьте питание модуля

## Примеры использования

### Пример 1: Отправка сообщения
```cpp
void loop() {
    if (Serial.available()) {
        String text = Serial.readStringUntil('\n');
        sendMessage("!87654321", text);
    }
}
```

### Пример 2: Получение и отображение
```cpp
void loop() {
    auto messages = getNewMessages();
    for (auto& msg : messages) {
        display.print(msg.from_node);
        display.println(msg.text);
    }
}
```

### Пример 3: GPS трекинг
```cpp
#include <TinyGPS++.h>

void loop() {
    if (gps.location.isUpdated()) {
        sendPosition(gps.location.lat(), gps.location.lng());
    }
}
```

## Ресурсы

- [Raspberry Pi Pico W Documentation](https://www.raspberrypi.com/documentation/microcontrollers/pico-series.html)
- [Meshtastic Project](https://meshtastic.org/)
- [ArduinoJson Library](https://arduinojson.org/)
- [PlatformIO](https://platformio.org/)

## Лицензия

MIT License
