# 🛡️ СтражСети — Прошивка ESP32

Прошивка для ESP32, совместимая с Meshtastic и mesh-server.

## Возможности

### Основные
- **Meshtastic HTTP API** — совместимость с приложением Meshtastic Android/iOS
- **GPS трекинг** — отслеживание позиции через UART GPS модуль
- **WiFi** — подключение к сети и передача данных на сервер
- **Serial USB** — прямое подключение к ПК через USB
- **OTA обновления** — обновление прошивки по WiFi

### Mesh
- **LoRa mesh** (SX1276/SX1262) — mesh-сеть через LoRa (опционально)
- **Позиционирование** — передача координат через mesh
- **Сообщения** — обмен текстовыми сообщениями

### Сенсоры
- **BLE** — сканирование Bluetooth Low Energy устройств
- **Healbe GoBe** — чтение пульса, стресса и батареи с часов
- **Температура/влажность** — через встроенные датчики

### Интерфейсы
- **Веб-панель** — настройка устройства через браузер
- **REST API** — управление через HTTP
- **Serial Monitor** — отладка через COM-порт

## Совместимость

| Приложение | Протокол | Порт | Статус |
|-----------|----------|------|--------|
| Meshtastic Android | HTTP API | 4403 | ✅ |
| Meshtastic iOS | HTTP API | 4403 | ✅ |
| mesh-server | HTTP/Serial | 8080/COM | ✅ |
| Serial Monitor | UART | 115200 | ✅ |

## Установка

### Через Arduino IDE

1. Установите Arduino IDE 2.x
2. Добавьте ESP32 в менеджер плат:
   - Файл → Настройки → URLs дополнительных менеджеров:
     `https://raw.githubusercontent.com/espressif/arduino-esp32/gh-pages/package_esp32_index.json`
3. Инструменты → Плата → ESP32 Arduino → ESP32S3 Dev Module
4. Установите библиотеку: Sketch → Include Library → Manage Libraries → ArduinoJson
5. Откройте `StrazhSechi_Firmware.ino`
6. Скомпилируйте и загрузите

### Через PlatformIO

```bash
cd esp32/
pio run                    # Сборка
pio run -t upload          # Загрузка
pio device monitor         # Мониторинг
```

### Профили плат

```bash
pio run -e esp32s3         # ESP32-S3 (рекомендуется)
pio run -e esp32dev        # ESP32 (базовый)
pio run -e esp32_lora      # ESP32 + LoRa модуль
```

## Настройка

### Через WiFi (AP режим)

1. Включите ESP32
2. Подключитесь к WiFi: **СтражСети-Setup** (пароль: `stражсеть2024`)
3. Откройте `http://192.168.4.1`
4. Введите настройки WiFi и сервера
5. Сохраните и перезагрузите

### Через Serial

```bash
# Настройка WiFi
CONFIG:{"wifi_ssid":"MyWiFi","wifi_password":"12345678","server_url":"http://192.168.1.100:8080"}

# Статус устройства
STATUS
```

### Через HTTP API

```bash
# Получить конфигурацию
curl http://ESP32_IP/api/config

# Обновить конфигурацию
curl -X POST http://ESP32_IP/api/config \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyWiFi","server_url":"http://192.168.1.100:8080"}'

# Получить статус
curl http://ESP32_IP/api/status
```

## API

### Mesh-сервер API (порт 80)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/health` | Health check |
| GET | `/api/status` | Статус устройства |
| GET | `/api/config` | Получить конфигурацию |
| POST | `/api/config` | Обновить конфигурацию |
| POST | `/api/message` | Получить сообщение |
| GET | `/api/healbe/status` | Статус Healbe |
| POST | `/api/healbe/connect` | Подключить Healbe |

### Meshtastic API (порт 4403)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/json/device` | Информация об устройстве |
| GET | `/json/nodes` | Список узлов mesh |
| GET | `/json/report` | Отчёт об устройстве |
| GET | `/api/v1/fromradio` | Protobuf API (заглушка) |
| PUT | `/api/v1/toradio` | Protobuf API (заглушка) |

## Схема подключения

### ESP32-S3 (базовая)

```
ESP32-S3
├── USB ──────────── PC (Serial)
├── GPIO 16/17 ───── GPS модуль (UART)
└── Встроенный WiFi ─ Роутер ─ mesh-server
```

### ESP32 + LoRa

```
ESP32
├── USB ──────────── PC (Serial)
├── SPI ──────────── SX1276/SX1262 (LoRa)
│   ├── GPIO 10 ──── CS
│   ├── GPIO 9 ───── RST
│   └── GPIO 4 ───── DIO0
├── GPIO 16/17 ───── GPS модуль (UART)
└── WiFi ─────────── Роутер ─ mesh-server
```

## Архитектура

```
┌─────────────────────────────────────────────┐
│              СтражСети Hub                   │
│                                             │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐    │
│  │  GPS    │  │   BLE   │  │  LoRa   │    │
│  │ Module  │  │ Scanner │  │  Mesh   │    │
│  └────┬────┘  └────┬────┘  └────┬────┘    │
│       │            │            │           │
│       └──────┬─────┘────────────┘           │
│              │                              │
│        ┌─────┴─────┐                        │
│        │  Core     │                        │
│        │  Logic    │                        │
│        └─────┬─────┘                        │
│              │                              │
│  ┌───────────┼───────────┐                  │
│  │           │           │                  │
│  ▼           ▼           ▼                  │
│ WiFi       Serial     HTTP                  │
│ Client     USB        Server                │
│  │           │           │                  │
└──┼───────────┼───────────┼──────────────────┘
   │           │           │
   ▼           ▼           ▼
mesh-server  PC/Monitor  Meshtastic App
```

## Режимы работы

### 1. WiFi Hub (по умолчанию)
- Подключается к WiFi
- Отправляет позицию на mesh-server
- Принимает сообщения от сервера
- Работает с Healbe GoBe

### 2. Serial Bridge
- Подключается к ПК по USB
- Принимает команды от mesh-server
- Отправляет данные в реальном времени

### 3. LoRa Mesh
- Mesh-сеть через LoRa
- Пересылка сообщений между узлами
- Позиционирование через mesh

### 4. Combo
- WiFi + LoRa одновременно
- Мост между mesh-сетью и сервером

## Troubleshooting

### ESP32 не подключается к WiFi
1. Проверьте SSID и пароль
2. Убедитесь, что роутер на 2.4 ГГц (не 5 ГГц)
3. Переведите в AP режим (удерживайте кнопку 5 сек)

### GPS не выдает координаты
1. Убедитесь, что GPS модуль подключен к GPIO 16/17
2. Выходите на улицу для первого.fix
3. Подождите 1-3 минуты для холодного старта

### Healbe не подключается
1. Убедитесь, что часы включены
2. Проверьте MAC адрес
3. Подойдите ближе к ESP32 (<3м)
