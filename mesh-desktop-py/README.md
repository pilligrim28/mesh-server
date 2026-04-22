# Meshtastic Monitor Desktop (PySide6)

Десктопное приложение для мониторинга устройств Meshtastic на Python + PySide6.

## Особенности

- 🖥️ **Нативное приложение** для Linux, Windows, macOS
- 🐍 **Python + PySide6** - никаких проблем с webkit
- 🔌 **Подключение к серверу** по HTTP API
- 📊 **Мониторинг в реальном времени** - автообновление каждые 5 секунд
- 📡 **Bluetooth/WiFi сканирование** через сервер
- 💬 **Отправка сообщений** через Meshtastic сеть

## Требования

- Python 3.10+
- mesh-server (запущен на localhost:8080 или другом адресе)

## Установка

```bash
# Перейти в директорию
cd mesh-desktop-py

# Установить зависимости
pip install -r requirements.txt

# Или через pipx (рекомендуется)
pipx install -r requirements.txt
```

## Запуск

```bash
# Убедитесь, что сервер запущен
# В другом терминале:
cd ../mesh-server
go run main.go

# Запуск приложения
python main.py
```

## Структура проекта

```
mesh-desktop-py/
├── main.py              # Основной GUI
├── api_client.py        # API клиент для mesh-server
├── requirements.txt     # Python зависимости
└── README.md           # Документация
```

## Функционал

### Обзор
- Счетчик устройств и алертов
- Список устройств с онлайн статусом
- Кнопки Bluetooth/WiFi сканирования

### Метрики
- Пульс (bpm)
- CO2 (ppm)
- Температура (°C)
- Влажность (%)

### Алерты
- Просмотр непрочитанных уведомлений
- Пометка как прочитанные
- Цветовая индикация важности

### Сообщения
- Отправка сообщений через сервер
- История сообщений
- Настройка Node ID

### Настройки
- URL сервера
- Проверка подключения
- Информация о приложении

## Архитектура

```
┌─────────────────┐         HTTP API         ┌─────────────────┐
│  Desktop (Qt)   │ ◄──────────────────────► │   Mesh Server   │
│  PySide6        │                          │   (Go + SQLite) │
│                 │                          │                 │
│  - Главный окно │                          │  - БД SQLite    │
│  - API клиент   │                          │  - Bluetooth    │
│  - Таймер       │                          │  - WiFi         │
└─────────────────┘                          │  - MQTT         │
                                             └─────────────────┘
```

## Зависимости

- **PySide6** - Qt для Python
- **requests** - HTTP запросы к серверу
- **folium** - (опционально) для карты
- **websocket-client** - (опционально) для WebSocket

## Сборка исполняемого файла

```bash
# Установить PyInstaller
pip install pyinstaller

# Собрать для Linux
pyinstaller --onefile --windowed --name="MeshtasticMonitor" main.py

# Исполняемый файл в dist/
```

## Лицензия

MIT
