# Meshtastic Monitor Desktop

Десктопное приложение для мониторинга устройств Meshtastic, построенное на базе **Wails** (Go + Web).

## Особенности

- 🖥️ **Нативное приложение** для Windows, macOS и Linux
- 📦 **Компактный размер** (~15-20 МБ)
- ⚡ **Быстрая работа** благодаря Go бэкенду
- 🎨 **Современный UI** на Bootstrap 5 + Leaflet карты
- 🔌 **Подключение к серверу** по HTTP API
- 📡 **Bluetooth/WiFi сканирование** через сервер
- 💬 **Отправка сообщений** через Meshtastic сеть
- 📊 **Мониторинг метрик** в реальном времени

## Архитектура

```
┌─────────────────┐         HTTP API         ┌─────────────────┐
│   Desktop App   │ ◄──────────────────────► │   Mesh Server   │
│   (Wails Go)    │                          │   (Go + SQLite) │
│                 │                          │                 │
│  - UI (Web)     │                          │  - БД SQLite    │
│  - Wails Go     │                          │  - Bluetooth    │
│                 │                          │  - WiFi         │
└─────────────────┘                          │  - MQTT         │
                                             └─────────────────┘
```

**Десктопное приложение:**
- Не требует базы данных
- Подключается к серверу по HTTP
- Все данные хранятся на сервере
- Работает как тонкий клиент

## Быстрый старт

### Требования

- Go 1.23+
- Node.js 18+
- Wails CLI v2.11+

```bash
# Установка Wails
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Запуск сервера

Сначала запустите mesh-server:

```bash
cd ../mesh-server
go run main.go
```

### Разработка десктопного приложения

```bash
cd mesh-desktop

# Запуск в режиме разработки (с горячей перезагрузкой)
wails dev

# Или с флагом отладки
wails dev -tags dev
```

### Сборка

```bash
# Сборка для текущей ОС
wails build

# Сборка для всех платформ
wails build -platform windows/darwin/linux
```

## Структура проекта

```
mesh-desktop/
├── main.go              # Точка входа Wails
├── app.go               # Основной App struct с методами для frontend
├── mesh_service.go      # Сервис для работы с Meshtastic
├── wails.json           # Конфигурация Wails
├── go.mod               # Go зависимости
├── frontend/
│   ├── index.html       # HTML шаблон
│   ├── src/
│   │   └── main.js      # JavaScript логика
│   ├── package.json     # Node.js зависимости
│   └── wailsjs/         # Авто-сгенерированные bindings
└── build/               # Собранные бинарники
```

## Доступные API методы

### Устройства
- `GetDevices()` - Получить все устройства
- `GetMapData()` - Получить данные для карты

### Метрики
- `GetMetrics()` - Получить последние метрики

### Алерты
- `GetUnreadAlerts()` - Получить непрочитанные уведомления
- `MarkAlertAsRead(id)` - Пометить алерт как прочитанный
- `MarkAllAlertsAsRead(deviceId)` - Пометить все алерты как прочитанные

### Сообщения
- `GetMessages(deviceId, limit)` - Получить сообщения
- `SendMessage(deviceId, fromNode, toNode, text)` - Отправить сообщение

### Discovery
- `GetDiscoveredDevices()` - Получить обнаруженные устройства
- `StartBluetoothScan()` - Запустить Bluetooth сканирование
- `StopBluetoothScan()` - Остановить Bluetooth сканирование
- `StartWiFiScan()` - Запустить WiFi сканирование
- `StopWiFiScan()` - Остановить WiFi сканирование
- `GetDiscoveryStatus()` - Получить статус сканирования

### Статистика
- `GetStats()` - Получить статистику приложения

## Интеграция с основным сервером

Приложение использует общие пакеты из основного проекта `mesh-server`:

- `models` - модели данных
- `database` - работа с SQLite
- `repository` - доступ к данным
- `discovery` - обнаружение устройств

Это обеспечивает полную совместимость данных между веб-сервером и десктопным приложением.

## База данных

Приложение создает SQLite базу данных `mesh-server.db` в директории пользователя.

Структура БД:
- `devices` - устройства
- `metrics` - метрики (пульс, CO2, температура, влажность)
- `alerts` - уведомления
- `messages` - сообщения
- `discovered_devices` - обнаруженные устройства

## Особенности платформы

### Windows
- Требуется WebView2 (устанавливается автоматически)
- Минимальная версия: Windows 10

### macOS
- Требуется macOS 10.15+
- Поддержка темной темы

### Linux
- Требуется libwebkit2gtk-4.0
- Установка зависимостей:
  ```bash
  # Ubuntu/Debian
  sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev
    
  # Fedora
  sudo dnf install gtk3-devel webkit2gtk3-devel
  ```

## Отладка

В режиме разработки (`wails dev`):
- Горячая перезагрузка frontend
- DevTools доступны через Ctrl+Shift+J (Windows/Linux) или Cmd+Option+J (macOS)
- Логи Go выводятся в консоль

## Сборка релиза

```bash
# production сборка
wails build -production

# Сборка с конкретным названием
wails build -o MeshtasticMonitor

# Кроссплатформенная сборка
wails build -platform windows/amd64 darwin/amd64 linux/amd64
```

## Лицензия

MIT
