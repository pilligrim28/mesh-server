# Mesh Server - Мониторинг Meshtastic

Сервер для мониторинга устройств Meshtastic с поддержкой отслеживания местоположения, физических показателей датчиков, алертов и обмена сообщениями.

## Возможности

- 🗺️ **Карта расположений** - отслеживание координат устройств LoRa
- ❤️ **Физические показатели** - пульс, CO2, температура, влажность
- 🚨 **Алерты** - уведомления о событиях (вне зоны, низкий заряд, потеря сигнала)
- 💬 **Сообщения** - отправка и получение сообщений через Meshtastic
- 📱 **Интеграция с ESP32** - переписка с телефоном через LoRa mesh-сеть
- 📡 **Bluetooth LE** - обнаружение ESP32 через Bluetooth компьютера
- 🌐 **MQTT** - интеграция с публичной MQTT сетью Meshtastic для глобального обмена сообщениями
- 🔌 **WebSocket** - realtime обновления метрик
- 🖥️ **Веб-интерфейс** - готовая панель мониторинга
- 🧪 **Симулятор** - генерация тестовых данных носимого устройства человека
- 📡 **Discovery** - обнаружение реальных устройств Meshtastic через Bluetooth и WiFi
- 🖥️ **Десктопное приложение** - нативное приложение на Wails (Windows, macOS, Linux)

## Быстрый старт

```bash
# Сборка (не требует CGO, используется pure-Go SQLite драйвер)
go build -o mesh-server.exe

# Запуск
./mesh-server.exe

# Или с переменными окружения
SERVER_PORT=8080 DATABASE_PATH=mesh.db ./mesh-server.exe

# С интеграцией ESP32
ESP32_URL=http://192.168.1.100 ./mesh-server.exe
```

## Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `SERVER_PORT` | `8080` | Порт HTTP сервера |
| `DATABASE_PATH` | `mesh-server.db` | Путь к SQLite базе данных |
| `ENABLE_BLUETOOTH` | `true` | Включить Bluetooth сканирование |
| `ENABLE_WIFI` | `true` | Включить WiFi сканирование |
| `DISCOVERY_INTERVAL` | `30` | Интервал сканирования (секунды) |
| `MESHTASTIC_IP` | `` | Прямой IP адрес устройства Meshtastic |
| `ESP32_URL` | `` | URL для подключения к ESP32 |
| `MQTT_ENABLED` | `false` | Включить MQTT интеграцию |
| `MQTT_SERVER` | `mqtt.meshtastic.org` | MQTT сервер |
| `MQTT_USERNAME` | `meshdev` | MQTT пользователь |
| `MQTT_PASSWORD` | `large4cats` | MQTT пароль |
| `MQTT_ROOT_TOPIC` | `msh/RU` | Корневой MQTT топик |

## API Endpoints

### Устройства (Devices)

```
GET    /api/devices          # Получить все устройства
POST   /api/devices          # Создать устройство
```

**Пример создания устройства:**
```json
POST /api/devices
{
  "node_id": "!12345678",
  "name": "Device 1",
  "latitude": 55.7558,
  "longitude": 37.6173,
  "altitude": 150
}
```

### Метрики (Metrics)

```
GET    /api/metrics                # Получить последние метрики всех устройств
GET    /api/metrics/device?device_id=1  # Получить метрики устройства
POST   /api/metrics                # Создать метрику
```

**Пример создания метрики:**
```json
POST /api/metrics
{
  "device_id": 1,
  "heart_rate": 72,
  "co2": 450,
  "temp": 23.5,
  "humidity": 45
}
```

### Алерты (Alerts)

```
GET    /api/alerts                      # Получить непрочитанные алерты
GET    /api/alerts/device?device_id=1   # Получить алерты устройства
POST   /api/alerts                      # Создать алерт
PUT    /api/alerts/read?id=1            # Пометить алерт как прочитанный
PUT    /api/alerts/read-all?device_id=1 # Пометить все алерты как прочитанные
```

**Пример создания алерта (вне зоны):**
```json
POST /api/alerts
{
  "device_id": 1,
  "type": "out_of_zone",
  "message": "Пользователь покинул разрешенную зону",
  "severity": "critical"
}
```

**Типы алертов:**
- `out_of_zone` - вне зоны
- `low_battery` - низкий заряд
- `signal_lost` - потеря сигнала

**Уровни важности:**
- `info` - информация
- `warning` - предупреждение
- `critical` - критично

### Сообщения (Messages)

```
GET    /api/messages                # Получить исходящие сообщения
GET    /api/messages/device?device_id=1&limit=50  # Получить сообщения устройства
POST   /api/messages                # Отправить сообщение
```

**Пример отправки сообщения:**
```json
POST /api/messages
{
  "device_id": 1,
  "from_node": "!12345678",
  "to_node": "!87654321",
  "text": "Привет!",
  "direction": "outbound"
}
```

### Карта (Map)

```
GET    /api/map          # Получить данные для карты расположений
```

**Ответ:**
```json
[
  {
    "node_id": "!12345678",
    "name": "Device 1",
    "latitude": 55.7558,
    "longitude": 37.6173,
    "altitude": 150,
    "last_seen": "2026-03-04 12:30:00"
  }
]
```

### WebSocket

```
WS     /ws               # WebSocket для realtime обновлений
```

**Подключение:**
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Получены данные:', data);
};
```

### Health Check

```
GET    /health           # Проверка работоспособности сервера
```

### Discovery (Обнаружение устройств)

API для обнаружения реальных устройств Meshtastic (ESP32) через Bluetooth и WiFi.

```
GET    /api/discovery                    # Получить все обнаруженные устройства
GET    /api/discovery/bluetooth          # Получить устройства через Bluetooth
GET    /api/discovery/wifi               # Получить устройства через WiFi
GET    /api/discovery/meshtastic         # Получить только Meshtastic устройства
GET    /api/discovery/status             # Получить статус сканирования
GET    /api/discovery/network            # Получить информацию о сетевых интерфейсах
POST   /api/discovery/scan               # Запустить сканирование (type: bluetooth|wifi|all)
POST   /api/discovery/bluetooth/start    # Запустить Bluetooth сканирование
POST   /api/discovery/bluetooth/stop     # Остановить Bluetooth сканирование
POST   /api/discovery/wifi/start         # Запустить WiFi сканирование
POST   /api/discovery/wifi/stop          # Остановить WiFi сканирование
DELETE /api/discovery/clear              # Очистить список обнаруженных устройств
```

**Пример ответа обнаруженных устройств:**
```json
[
  {
    "id": 1,
    "address": "192.168.1.100",
    "name": "Meshtastic-192.168.1.100",
    "type": "wifi",
    "meshtastic": true,
    "last_seen": "2026-03-05T12:30:00Z",
    "discovered_at": "2026-03-05T12:25:00Z"
  },
  {
    "id": 2,
    "address": "AA:BB:CC:DD:EE:FF",
    "name": "Meshtastic-AA:BB:CC:DD:EE:FF",
    "type": "bluetooth",
    "rssi": -65,
    "meshtastic": true,
    "last_seen": "2026-03-05T12:30:00Z",
    "discovered_at": "2026-03-05T12:28:00Z"
  }
]
```

**Пример запуска сканирования:**
```json
POST /api/discovery/scan
{
  "type": "all"
}
```

**Пример статуса сканирования:**
```json
{
  "bluetooth_scanning": true,
  "wifi_scanning": true,
  "devices_found": 5
}
```

## Симулятор носимого устройства

Сервер автоматически запускает симулятор носимого устройства человека при старте.

**Симуляция включает:**
- 📍 Перемещение по району (Москва, Красная площадь)
- ❤️ Пульс с учётом циркадных ритмов и активности
- 🌡️ Температура тела с суточными колебаниями
- 💨 CO2 в зависимости от физической активности
- 💧 Влажность
- 🚨 Случайные алерты (вне зоны, низкий заряд)

**API симулятора:**

```
GET    /api/simulator/status          # Статус симулятора
POST   /api/simulator/start           # Запустить
POST   /api/simulator/stop            # Остановить
POST   /api/simulator/event           # Генерация события
```

**Пример генерации тестового события:**
```json
POST /api/simulator/event
{
  "type": "high_heart_rate"
}
```

**Типы событий:**
- `high_heart_rate` - высокий пульс (140+ bpm)
- `low_battery` - низкий заряд батареи
- `signal_lost` - потеря сигнала

## Веб-интерфейс

Откройте в браузере `http://localhost:8080` для доступа к панели мониторинга.

**Функции веб-интерфейса:**
- 🗺️ Карта с устройствами в реальном времени
- 📊 Панель метрик (пульс, CO2, температура, влажность)
- 🔔 Уведомления с индикацией непрочитанных
- 💬 Отправка и просмотр сообщений
- 🔄 Автообновление каждые 10 секунд
- 🔌 WebSocket для мгновенных обновлений

## Структура проекта

```
mesh-server/
├── config/           # Конфигурация
├── database/         # Подключение к БД и миграции
├── discovery/        # Обнаружение устройств (Bluetooth, WiFi)
├── models/           # Модели данных
├── repository/       # Репозитории для работы с БД
├── handler/          # HTTP обработчики
├── service/          # Сервисный слой
├── simulator/        # Симулятор носимого устройства
├── static/           # Веб-интерфейс (HTML, CSS, JS)
│   ├── index.html    # Главная страница
│   └── app.js        # Frontend логика
├── main.go           # Точка входа
└── README.md         # Документация
```

## Примеры использования

### curl

```bash
# Получить все устройства
curl http://localhost:8080/api/devices

# Создать метрику
curl -X POST http://localhost:8080/api/metrics \
  -H "Content-Type: application/json" \
  -d '{"device_id":1,"heart_rate":75,"co2":400,"temp":22.5}'

# Создать алерт
curl -X POST http://localhost:8080/api/alerts \
  -H "Content-Type: application/json" \
  -d '{"device_id":1,"type":"out_of_zone","message":"Вне зоны","severity":"warning"}'

# Отправить сообщение
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{"device_id":1,"from_node":"!12345678","to_node":"!87654321","text":"Тест"}'

# Получить данные для карты
curl http://localhost:8080/api/map

# Discovery API - получить все обнаруженные устройства
curl http://localhost:8080/api/discovery

# Получить устройства через Bluetooth
curl http://localhost:8080/api/discovery/bluetooth

# Получить устройства через WiFi
curl http://localhost:8080/api/discovery/wifi

# Получить только Meshtastic устройства
curl http://localhost:8080/api/discovery/meshtastic

# Получить статус сканирования
curl http://localhost:8080/api/discovery/status

# Запустить сканирование всех интерфейсов
curl -X POST http://localhost:8080/api/discovery/scan \
  -H "Content-Type: application/json" \
  -d '{"type": "all"}'

# Запустить Bluetooth сканирование
curl -X POST http://localhost:8080/api/discovery/bluetooth/start

# Остановить WiFi сканирование
curl -X POST http://localhost:8080/api/discovery/wifi/stop

# Очистить список обнаруженных устройств
curl -X DELETE http://localhost:8080/api/discovery/clear

# Получить информацию о сетевых интерфейсах
curl http://localhost:8080/api/discovery/network
```

## Интеграция с ESP32

### Подключение ESP32 для переписки с телефоном

1. **Настройте ESP32 с Meshtastic:**
   - Загрузите прошивку с https://meshtastic.org
   - Настройте WiFi подключение
   - Запомните IP адрес устройства

2. **Запустите сервер с интеграцией ESP32:**
```bash
ESP32_URL=http://192.168.1.100 ./mesh-server.exe
```

3. **Найдите ESP32 через Bluetooth или WiFi:**
```bash
# Сканировать ESP32 в сети
curl http://localhost:8080/api/esp32/scan

# Подключиться к ESP32
curl -X POST http://localhost:8080/api/esp32/connect \
  -H "Content-Type: application/json" \
  -d '{"ip": "192.168.4.1"}'
```

4. **Отправьте сообщение через веб-интерфейс:**
   - Откройте `http://localhost:8080`
   - Перейдите на вкладку "Сообщения"
   - Заполните форму и нажмите "Отправить"

5. **Сообщение будет доставлено на телефон через:**
   - Сервер → ESP32 (WiFi/Bluetooth)
   - ESP32 → Телефон (Bluetooth/LoRa)

📖 **Подробная документация:**
- [ESP32_INTEGRATION.md](ESP32_INTEGRATION.md) - общая интеграция
- [ESP32_BLUETOOTH.md](ESP32_BLUETOOTH.md) - подключение через Bluetooth

### Пример отправки сообщения

```bash
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": 1,
    "from_node": "!12345678",
    "to_node": "!87654321",
    "text": "Привет через LoRa!",
    "direction": "outbound"
  }'
```

## MQTT Интеграция (Meshtastic)

### Включение MQTT:

```bash
# В .env файле:
MQTT_ENABLED=true
MQTT_SERVER=mqtt.meshtastic.org
MQTT_USERNAME=meshdev
MQTT_PASSWORD=large4cats
MQTT_ROOT_TOPIC=msh/RU

# Запуск
./mesh-server.exe
```

### MQTT API Endpoints:

```bash
# Статус MQTT
GET /api/mqtt/status

# Подключиться к MQTT
POST /api/mqtt/connect

# Отключиться от MQTT
POST /api/mqtt/disconnect

# Отправить сообщение через MQTT
POST /api/mqtt/message
{
  "from_node": "!12345678",
  "to_node": "!87654321",
  "text": "Привет!"
}
```

### Что делает MQTT интеграция:

1. **Прием сообщений из глобальной сети Meshtastic**
   - Все сообщения из MQTT топика `msh/RU/2/json/#` сохраняются в БД
   - Уведомления отправляются через WebSocket в веб-интерфейс

2. **Отправка сообщений в глобальную сеть**
   - Сообщения из веб-интерфейса публикуются в MQTT
   - Доступны всем устройствам Meshtastic в мире

3. **Map Reporting** (опционально)
   - Периодическая отправка позиции сервера на карту Meshtastic
   - Включается: `MQTT_MAP_REPORTING=true`

### Публичный MQTT сервер Meshtastic:

- **Сервер:** `mqtt.meshtastic.org:1883`
- **Логин:** `meshdev`
- **Пароль:** `large4cats`
- **Топик:** `msh/RU` (Россия) или `msh/US` (США)

⚠️ **Важно:** Все сообщения в публичном MQTT не шифруются!

## Десктопное приложение

🖥️ **Meshtastic Monitor Desktop** — нативное приложение для Windows, macOS и Linux.

### Преимущества десктопного приложения:

- ✅ Работает без браузера
- ✅ Компактный размер (~15-20 МБ)
- ✅ Быстрый запуск
- ✅ Полная интеграция с системными уведомлениями
- ✅ Автономная работа с SQLite БД

### Установка и запуск:

```bash
cd mesh-desktop

# Режим разработки
wails dev

# Сборка приложения
wails build

# Готовый бинарник в build/bin/
```

📖 **Подробная документация:** [mesh-desktop/README.md](mesh-desktop/README.md)

## Лицензия

MIT
