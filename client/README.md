# Mesh Server Client - Desktop приложение

Desktop клиент для mesh-server на Python + PySide6.

## Быстрый старт (Windows)

```bash
# Запуск через скрипт
run.bat
```

## Ручная установка

```bash
# Создание виртуального окружения
python -m venv venv

# Активация
venv\Scripts\activate  # Windows
# source venv/bin/activate  # Linux/Mac

# Установка зависимостей
pip install PySide6 requests websocket-client
```

## Запуск

```bash
# Через скрипт
run.bat

# Или напрямую
python main.py
python main.py http://192.168.1.100:8080  # Свой URL сервера
```

## Возможности

- 🗺️ Карта устройств в реальном времени
- 📊 Мониторинг метрик (пульс, CO2, температура, влажность)
- 🔔 Просмотр и управление алертами
- 💬 Отправка/получение сообщений
- 📡 Discovery устройств (Bluetooth, WiFi)
- 🔌 WebSocket для realtime обновлений
- 🧪 Управление симулятором

## Структура проекта

```
client/
├── main.py           # Точка входа, главное окно
├── api_client.py     # REST API клиент
├── ws_client.py      # WebSocket клиент
├── widgets.py        # UI виджеты (вкладки)
├── requirements.txt  # Зависимости Python
├── run.bat           # Скрипт запуска (Windows)
└── README.md         # Документация
```

## Зависимости

- Python 3.8+
- PySide6 (Qt6)
- requests
- websocket-client

## Скриншоты

### Вкладки приложения:
1. **Устройства** - список устройств с координатами
2. **Метрики** - пульс, CO2, температура, влажность
3. **Алерты** - уведомления с цветовой индикацией
4. **Сообщения** - отправка/получение через Meshtastic
5. **Карта** - таблица координат (для полноценной карты интегрируйте Яндекс.Карты)
6. **Discovery** - сканирование Bluetooth/WiFi устройств
7. **Симулятор** - управление генерацией тестовых данных
8. **Настройки** - подключение к серверу, WebSocket
