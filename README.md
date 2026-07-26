# MESH SERVER — Сервер мониторинга Meshtastic

## 1. ОБЩЕЕ ОПИСАНИЕ

**Mesh Server** — серверная платформа на языке Go для мониторинга и управления сетью устройств Meshtastic (LoRa mesh-сети). Представляет собой единое решение для отслеживания местоположения, сбора биометрических данных с носимых устройств, обмена сообщениями через LoRa и интеграции с глобальной MQTT сетью Meshtastic.

**Целевая аудитория:** Организации, которым нужен мониторинг людей в полевых условиях (спасатели, охрана, логистика, исследования).

**Решаемые задачи:**
- Отслеживание местоположения группы людей через LoRa mesh
- Мониторинг здоровья (пульс, температура, стресс)
- Автоматические алерты при выходе за зону/потере сигнала
- Обмен сообщениями без интернета
- Визуализация данных в реальном времени

---

## 2. ТЕХНИЧЕСКАЯ АРХИТЕКТУРА

### 2.1 Стек технологий

| Компонент | Технология |
|-----------|------------|
| Язык сервера | Go 1.25 |
| База данных | SQLite (pure-Go, без CGO) |
| HTTP | net/http (стандартная библиотека) |
| WebSocket | gorilla/websocket |
| MQTT | eclipse/paho.mqtt.golang |
| Bluetooth | go-ble/ble |
| Serial/USB | go.bug.st/serial |
| Protobuf | buf.build/gen/go/meshtastic |
| Десктоп | Wails (Go + Web UI) |

### 2.2 Структура проекта

```
mesh-server/
├── main.go              # Точка входа, роутинг, запуск сервисов
├── config/              # Конфигурация через переменные окружения
├── models/              # Модели данных (Device, Metrics, Alert, Message, Route, User)
├── database/            # Подключение к SQLite, миграции
├── repository/          # Репозитории (9 штук: Device, Metrics, Alert, Message, Discovery, Route, Healbe, User, Session)
├── handler/             # HTTP обработчики (20 файлов)
├── service/             # Бизнес-логика (MQTT, Serial, ESP32 Hub)
├── client/              # Клиенты для внешних сервисов (ESP32, Meshtastic, Healbe)
├── discovery/           # Сканеры Bluetooth и WiFi
├── ml/                  # Machine Learning (аномалии, предсказания)
├── simulator/           # Симуляторы (8 штук + движение людей)
├── meshtastic/          # HTTP API сервер для приложения Meshtastic + BLE/mDNS серверы
├── esp32/               # Прошивка ESP32 (PlatformIO)
├── static/              # Веб-интерфейс (HTML, JS)
└── mesh-server.exe      # Готовый бинарник
```

---

## 3. МОДУЛИ И ФУНКЦИОНАЛЬНОСТЬ

### 3.1 Устройства (Devices)

- CRUD API для управления устройствами Meshtastic
- Хранение: ID, NodeID, Name, GPS координаты, время последнего обнаружения
- Автоматическое обновление при получении данных от устройства

### 3.2 Метрики здоровья (Metrics)

- Пульс (heart_rate) — 50-120 bpm
- Уровень стресса (stress_level) — 0-4
- CO2 (co2) — 350-800 ppm
- Температура (temp) — 35.5-38.0°C
- Влажность (humidity) — 20-80%
- Заряд батареи (battery) — 0-100%

### 3.3 Система алертов (Alerts)

**Типы алертов:**
- `out_of_zone` — выход за разрешенную зону
- `low_battery` — низкий заряд батареи
- `signal_lost` — потеря сигнала

**Уровни важности:**
- `info` — информация
- `warning` — предупреждение
- `critical` — критично

**Функции:**
- Создание алертов вручную или автоматически
- Пометка как прочитанные (отдельные и все)
- Фильтрация по устройству

### 3.4 Обмен сообщениями (Messages)

- Отправка/получение сообщений через LoRa mesh
- Направление: `inbound` (входящее) / `outbound` (исходящее)
- Привязка к устройствам (from_node, to_node)
- Интеграция с MQTT для глобальной доставки

### 3.5 Карта расположений (Map)

- Отображение всех устройств на карте
- Данные: координаты, имя, время последнего обнаружения
- WebSocket обновления в реальном времени

### 3.6 Discovery — обнаружение устройств

**Bluetooth сканер:**
- Сканирование BLE устройств
- Определение RSSI (уровень сигнала)
- Идентификация устройств Meshtastic

**WiFi сканер:**
- Сканирование локальной сети
- Обнаружение устройств по IP
- Определение Meshtastic устройств

**API:**
- `/api/discovery` — все обнаруженные устройства
- `/api/discovery/bluetooth` — только Bluetooth
- `/api/discovery/wifi` — только WiFi
- `/api/discovery/meshtastic` — только Meshtastic
- `/api/discovery/status` — статус сканирования
- `/api/discovery/scan` — запуск сканирования
- `/api/discovery/network` — информация о сетевых интерфейсах

### 3.7 MQTT интеграция

- Подключение к публичному MQTT серверу Meshtastic (`mqtt.meshtastic.org`)
- Прием сообщений из глобальной сети (`msh/RU/json/#`)
- Отправка сообщений в глобальную сеть
- Map Reporting — отправка позиции на карту Meshtastic
- Управление через API: connect, disconnect, send message

### 3.8 WebSocket

- Endpoint: `/ws`
- Realtime обновления метрик, алертов, сообщений
- Автоматическая рассылка всем подключенным клиентам

### 3.9 Machine Learning (ML)

**AnomalyDetector (Z-score):**
- Скользящее окно из 60 последних замеров
- Автоматическое определение порогов для каждого типа метрик
- Вычисление среднего и стандартного отклонения
- Детектирование выбросов (Z-score > 2.0-2.5)

**Predictor (экспоненциальное сглаживание):**
- Предсказание следующей позиции на основе истории
- Коэффициент сглаживания α = 0.3
- Расчет направления и скорости движения
- Уверенность предсказания (confidence)

### 3.10 Симуляторы

**8 штук симуляторов носимых устройств:**
- Случайное перемещение по predefined routes (Санкт-Петербург)
- Генерация пульса, CO2, температуры, влажности
- Случайные алерты (вне зоны, низкий заряд)
- Настраиваемая скорость и интервалы
- Предустановленные маршруты: Невский проспект, Эрмитаж, Петропавловка, Забег, Вне зоны

**Симулятор передвижения людей:**
- Настройка центра, радиуса, количества людей
- Генерация случайных маршрутов в заданной зоне
- Интеграция с метриками здоровья

**Healbe Demo:**
- Симуляция данных часов GoBe (пульс, стресс, батарея)
- Используется в демо-режиме

### 3.11 Интеграция с ESP32

**ESP32 Hub Service:**
- Мост между mesh-сетями и сервером
- Опрос устройства по HTTP
- Синхронизация устройств и сообщений

**ESP32 Bluetooth Client:**
- Подключение к ESP32 через BLE
- Обмен сообщениями

**Serial/USB:**
- Подключение к ESP32 через COM-порт
- Прямая передача данных

### 3.12 Healbe (часы GoBe)

- Подключение к часам через ESP32 мост
- Получение пульса, уровня стресса, заряда батареи
- Автоподключение по MAC адресу
- Пересылка данных в mesh-сеть (опционально)

### 3.13 Маршруты (Routes)

- Создание маршрутов сотрудников
- Запись точек маршрута
- История перемещений
- Визуализация на карте

### 3.14 Аутентификация

- Регистрация и вход пользователей
- Сессии через cookies
- Middleware для защищенных API
- Профиль пользователя, смена пароля

### 3.15 Meshtastic HTTP API

- Сервер на порту 4403 для приложения Meshtastic
- mDNS объявление (`_meshtastic._tcp`)
- BLE сервер для обнаружения через Bluetooth
- Совместимость с приложением Meshtastic

---

## 4. API ENDPOINTS (Полный список)

### Авторизация
- `POST /api/auth/login` — вход
- `POST /api/auth/logout` — выход
- `GET /api/auth/me` — текущий пользователь
- `PUT /api/auth/profile` — обновление профиля
- `PUT /api/auth/password` — смена пароля

### Устройства
- `GET /api/devices` — список устройств
- `POST /api/devices` — создание устройства

### Метрики
- `GET /api/metrics` — последние метрики
- `GET /api/metrics/device?device_id=N` — метрики устройства
- `POST /api/metrics` — создание метрики

### Алерты
- `GET /api/alerts` — непрочитанные алерты
- `GET /api/alerts/device?device_id=N` — алерты устройства
- `POST /api/alerts` — создание алерта
- `PUT /api/alerts/read?id=N` — пометить как прочитанный
- `PUT /api/alerts/read-all?device_id=N` — пометить все как прочитанные

### Сообщения
- `GET /api/messages` — исходящие сообщения
- `GET /api/messages/device?device_id=N&limit=M` — сообщения устройства
- `POST /api/messages` — отправка сообщения

### Карта
- `GET /api/map` — данные для карты

### Discovery
- `GET /api/discovery` — все обнаруженные устройства
- `GET /api/discovery/bluetooth` — Bluetooth устройства
- `GET /api/discovery/wifi` — WiFi устройства
- `GET /api/discovery/meshtastic` — Meshtastic устройства
- `GET /api/discovery/status` — статус сканирования
- `GET /api/discovery/network` — сетевые интерфейсы
- `POST /api/discovery/scan` — запуск сканирования
- `POST /api/discovery/bluetooth/start` — старт Bluetooth
- `POST /api/discovery/bluetooth/stop` — стоп Bluetooth
- `POST /api/discovery/wifi/start` — старт WiFi
- `POST /api/discovery/wifi/stop` — стоп WiFi
- `DELETE /api/discovery/clear` — очистка списка

### ESP32
- `GET /api/esp32/scan` — сканирование ESP32
- `POST /api/esp32/connect` — подключение
- `POST /api/esp32/disconnect` — отключение
- `GET /api/esp32/status` — статус
- `POST /api/esp32/message` — получение сообщения
- `GET /api/esp32/bluetooth` — Bluetooth устройства
- `GET /api/esp32/hub/status` — статус хаба

### Serial (USB)
- `GET /api/serial/scan` — сканирование портов
- `POST /api/serial/connect` — подключение
- `POST /api/serial/disconnect` — отключение
- `GET /api/serial/status` — статус
- `POST /api/serial/message` — отправка сообщения

### MQTT
- `GET /api/mqtt/status` — статус
- `POST /api/mqtt/connect` — подключение
- `POST /api/mqtt/disconnect` — отключение
- `POST /api/mqtt/message` — отправка сообщения

### Симулятор
- `GET /api/simulator/status` — статус симулятора
- `GET /api/simulator/status/all` — статус всех
- `POST /api/simulator/start` — запуск
- `POST /api/simulator/start/all` — запуск всех
- `POST /api/simulator/stop` — остановка
- `POST /api/simulator/stop/all` — остановка всех
- `POST /api/simulator/event` — генерация события
- `POST /api/simulator/route` — смена маршрута
- `POST /api/simulator/speed` — установка скорости
- `GET /api/simulator/locations` — локации
- `GET /api/simulator/routes` — маршруты
- `GET /api/simulator/history` — история маршрута
- `GET /api/simulator/history/all` — история всех
- `DELETE /api/simulator/clear-history` — очистка истории
- `DELETE /api/simulator/clear-history/all` — очистка истории всех

### People Simulation
- `GET /api/people-sim/status` — статус
- `POST /api/people-sim/zone` — настройка зоны
- `POST /api/people-sim/start` — запуск
- `POST /api/people-sim/stop` — остановка

### ML
- `GET /api/ml/anomalies` — список аномалий
- `GET /api/ml/predict?device_id=N` — предсказание позиции
- `GET /api/ml/analyze?device_id=N` — анализ устройства

### Healbe
- `GET /api/healbe/scan` — сканирование
- `POST /api/healbe/connect` — подключение
- `POST /api/healbe/disconnect` — отключение
- `GET /api/healbe/status` — статус
- `GET /api/healbe/data` — данные
- `POST /api/healbe/forward` — пересылка в mesh
- `POST /api/healbe/ingest` — прием данных
- `GET /api/healbe/esp32/config` — конфигурация ESP32

### Маршруты
- `GET /api/routes` — список маршрутов
- `POST /api/routes` — создание маршрута
- `POST /api/routes/record` — запись точки
- `GET /api/routes/points` — точки маршрута
- `DELETE /api/routes/delete` — удаление маршрута

### Система
- `GET /api/system/status` — статус системы (демо-режим)
- `GET /api/ble/status` — статус BLE сервера
- `GET /api/mdns/status` — статус mDNS
- `GET /health` — проверка работоспособности

### WebSocket
- `WS /ws` — realtime обновления

---

## 5. КОНФИГУРАЦИЯ

Все настройки через переменные окружения:

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `SERVER_PORT` | `8080` | Порт HTTP сервера |
| `DATABASE_PATH` | `mesh-server.db` | Путь к SQLite |
| `ENABLE_BLUETOOTH` | `true` | Bluetooth сканирование |
| `ENABLE_WIFI` | `true` | WiFi сканирование |
| `DISCOVERY_INTERVAL` | `30` | Интервал сканирования (сек) |
| `MESHTASTIC_IP` | — | IP устройства Meshtastic |
| `ESP32_URL` | — | URL ESP32 |
| `ESP32_COM_PORT` | — | COM-порт для USB |
| `MQTT_ENABLED` | `false` | MQTT интеграция |
| `MQTT_SERVER` | `mqtt.meshtastic.org` | MQTT сервер |
| `MQTT_USERNAME` | `meshdev` | MQTT логин |
| `MQTT_PASSWORD` | `large4cats` | MQTT пароль |
| `MQTT_ROOT_TOPIC` | `msh/RU` | Корневой топик |
| `MQTT_MAP_REPORTING` | `false` | Отправка позиции |
| `ESP32_HUB_ENABLED` | `true` | ESP32 Hub |
| `ESP32_HUB_POLL_INTERVAL` | `5` | Интервал опроса (сек) |
| `HEALBE_MAC` | — | MAC адрес часов |
| `HEALBE_MODE` | `auto` | Режим Healbe |
| `HEALBE_FORWARD_MESH` | `false` | Пересылка в mesh |
| `HEALBE_BRIDGE_URL` | — | URL моста |
| `DEMO_MODE` | `false` | Демо-режим |
| `PEOPLE_SIM_ENABLED` | `false` | Симулятор людей |
| `PEOPLE_SIM_COUNT` | `8` | Количество людей |
| `PEOPLE_SIM_RADIUS_KM` | `2` | Радиус зоны (км) |
| `PEOPLE_SIM_CENTER_LAT` | `59.9343` | Ширина центра |
| `PEOPLE_SIM_CENTER_LON` | `30.3351` | Долгота центра |
| `PEOPLE_SIM_INTERVAL` | `5` | Интервал (сек) |

---

## 6. ДЕМО-РЕЖИМ

При `DEMO_MODE=true` автоматически:
- Отключается Bluetooth и WiFi сканирование
- Запускаются 8 симуляторов
- Запускается симулятор передвижения людей
- Запускается симулятор Healbe
- Настраиваются тестовые MAC адреса
- Включается пересылка данных в mesh

Идеально для презентации и тестирования без реального оборудования.

---

## 7. ВЕБ-ИНТЕРФЕЙС

Файлы в `static/`:
- `index.html` — главная страница (карта, метрики, алерты, сообщения)
- `login.html` — страница входа
- `profile.html` — профиль пользователя
- `app.js` — логика приложения (WebSocket, API вызовы)

**Функции:**
- Карта с устройствами в реальном времени
- Панель метрик (пульс, CO2, температура, влажность)
- Уведомления с индикацией непрочитанных
- Отправка и просмотр сообщений
- WebSocket для мгновенных обновлений

---

## 8. ДЕСКТОПНОЕ ПРИЛОЖЕНИЕ

**Meshtastic Monitor Desktop** — нативное приложение на Wails (Go + Web UI).

**Преимущества:**
- Работает без браузера
- Компактный размер (~15-20 МБ)
- Быстрый запуск
- Полная интеграция с системными уведомлениями
- Автономная работа с SQLite

**Сборка:**
```bash
cd mesh-desktop
wails dev      # режим разработки
wails build    # сборка
```

---

## 9. ПРОШИВКА ESP32

В папке `esp32/` находится прошивка для ESP32:
- `firmware/` — исходный код прошивки
- `platformio.ini` — конфигурация PlatformIO
- Поддержка LoRa, Bluetooth, WiFi
- Интеграция с Meshtastic

---

## 10. СТАДИЯ ГОТОВНОСТИ

| Компонент | Статус |
|-----------|--------|
| Go сервер | Готов |
| SQLite БД | Готов |
| REST API | Готов (60+ endpoints) |
| WebSocket | Готов |
| MQTT интеграция | Готова |
| Bluetooth/WiFi Discovery | Готов |
| ML (аномалии, предсказания) | Готов |
| 8 симуляторов | Готовы |
| Симулятор людей | Готов |
| Healbe интеграция | Готова |
| ESP32 Hub | Готов |
| Serial/USB | Готов |
| Веб-интерфейс | Готов |
| Десктопное приложение | Готово |
| Аутентификация | Готова |
| Маршруты | Готовы |
| Демо-режим | Готов |
| Документация | Готова |
| Прошивка ESP32 | Готова |

**Статус: Готов к демонстрации, тестированию и эксплуатации.**

---

## 11. БЫСТРЫЙ СТАРТ

```bash
# Сборка (не требует CGO)
go build -o mesh-server.exe

# Запуск
./mesh-server.exe

# С переменными окружения
SERVER_PORT=8080 DATABASE_PATH=mesh.db ./mesh-server.exe

# С интеграцией ESP32
ESP32_URL=http://192.168.1.100 ./mesh-server.exe

# Демо-режим (для презентации)
DEMO_MODE=true ./mesh-server.exe

# С MQTT
MQTT_ENABLED=true ./mesh-server.exe
```

---

## 12. ЛИЦЕНЗИЯ

MIT License — бесплатное использование, модификация и распространение.
