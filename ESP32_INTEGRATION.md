# Интеграция с ESP32 Meshtastic

## Обзор

Этот документ описывает как использовать mesh-server вместе с ESP32 и телефоном для переписки через LoRa mesh-сеть.

## Архитектура

```
┌─────────────┐      Bluetooth      ┌─────────────┐      WiFi/USB     ┌─────────────┐
│   Телефон   │ ◄─────────────────► │    ESP32    │ ◄───────────────► │ Mesh Server │
│ Meshtastic  │                     │  Meshtastic │                     │   :8080     │
└─────────────┘                     └─────────────┘                     └─────────────┘
       │                                    │                                    │
       │                                    │                                    │
       └────────────────────────────────────┴────────────────────────────────────┘
                                    LoRa Mesh
```

## Компоненты

### 1. ESP32 с Meshtastic

**Вариант A: Официальная прошивка Meshtastic**
- Загрузите с https://meshtastic.org/docs/getting-started/flashing-firmware
- Настройте через веб-интерфейс или приложение
- Включите HTTP API в настройках

**Вариант B: Кастомная прошивка**
- Используйте `esp32/firmware.ino` как основу
- Реализует HTTP API для получения/отправки сообщений

### 2. Настройка ESP32 URL

Добавьте в `.env`:

```bash
# URL вашего ESP32 устройства
ESP32_URL=http://192.168.1.100
```

Или используйте переменную окружения при запуске:

```bash
ESP32_URL=http://192.168.1.100 ./mesh-server.exe
```

## API ESP32

### Отправка сообщения на ESP32

**Запрос:**
```http
POST http://<ESP32_IP>/api/message
Content-Type: application/json

{
    "to_node": "!87654321",
    "text": "Привет из mesh-сети!"
}
```

**Ответ:**
```json
{
    "success": true,
    "message": "Message sent",
    "node_id": "!12345678"
}
```

### Проверка состояния

```http
GET http://<ESP32_IP>/health
```

## Как это работает

### Отправка сообщения с сервера на телефон

1. Откройте веб-интерфейс `http://localhost:8080`
2. Перейдите на вкладку "Сообщения"
3. Заполните форму:
   - **От кого**: `!default` (устройство по умолчанию) или Node ID вашего устройства
   - **Кому**: Node ID телефона (например, `!87654321`)
   - **Сообщение**: Текст сообщения
4. Нажмите "Отправить"

**По умолчанию:**
- При первом запуске сервера автоматически создается устройство с `node_id: !default`
- Это устройство используется для отправки сообщений если не указано другое

**Что происходит:**
- Сервер сохраняет сообщение в БД
- Отправляет на ESP32 через HTTP API
- ESP32 передает сообщение по LoRa
- Телефон получает сообщение через Bluetooth

### Получение сообщения с телефона

1. Телефон отправляет сообщение через приложение Meshtastic
2. Сообщение передается по Bluetooth на ESP32
3. ESP32 отправляет на сервер:

```http
POST http://<SERVER_IP>:8080/api/messages
Content-Type: application/json

{
    "device_id": 1,
    "from_node": "!87654321",
    "to_node": "!12345678",
    "text": "Ответ с телефона",
    "direction": "inbound"
}
```

4. Сервер сохраняет и показывает в веб-интерфейсе
5. WebSocket уведомление отправляется подключенным клиентам

## Настройка

### Шаг 1: Установка Meshtastic на ESP32

1. Подключите ESP32 по USB
2. Откройте https://meshtastic.org/docs/getting-started/flashing-firmware
3. Выберите вашу плату ESP32
4. Загрузите последнюю версию
5. Настройте через веб-интерфейс (192.168.4.1)

### Шаг 2: Настройка WiFi на ESP32

1. Подключитесь к точке доступа ESP32
2. Откройте веб-интерфейс
3. Введите credentials вашей WiFi сети
4. ESP32 подключится и получит IP адрес

### Шаг 3: Настройка mesh-server

1. Отредактируйте `.env`:
```bash
ESP32_URL=http://<IP_ESP32>
MESHTASTIC_IP=<IP_ESP32>  # опционально для discovery
```

2. Запустите сервер:
```bash
./mesh-server.exe
```

### Шаг 4: Подключение телефона

1. Установите приложение Meshtastic (Android/iOS)
2. Включите Bluetooth на телефоне
3. Подключитесь к ESP32 через приложение
4. Настройте Node ID вашего устройства

### Шаг 5: Проверка работы

1. Откройте `http://localhost:8080`
2. Проверьте вкладку "Сообщения"
3. Отправьте тестовое сообщение
4. Проверьте получение на телефоне

## API Endpoints

### ESP32 Discovery API

```http
# Сканировать ESP32 устройства в сети
GET /api/esp32/scan

# Подключиться к ESP32 по IP
POST /api/esp32/connect
{
    "ip": "192.168.4.1"
}

# Подключиться к ESP32 по MAC адресу (Bluetooth)
POST /api/esp32/connect
{
    "mac": "AA:BB:CC:DD:EE:FF"
}

# Отключиться от ESP32
POST /api/esp32/disconnect

# Получить статус подключения
GET /api/esp32/status

# Получить обнаруженные Bluetooth устройства
GET /api/esp32/bluetooth

# Принять входящее сообщение от ESP32
POST /api/esp32/message
{
    "from_node": "!87654321",
    "to_node": "!12345678",
    "text": "Привет с телефона!"
}
```

### Сообщения

```http
# Отправить сообщение
POST /api/messages
{
    "device_id": 1,
    "from_node": "!12345678",
    "to_node": "!87654321",
    "text": "Привет!",
    "direction": "outbound"
}

# Получить сообщения устройства
GET /api/messages/device?device_id=1&limit=50

# Получить исходящие сообщения
GET /api/messages
```

### WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'new_message') {
        console.log('Новое сообщение:', data.message);
    }
};
```

## Примеры использования

### curl

```bash
# Отправить сообщение
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": 1,
    "from_node": "!12345678",
    "to_node": "!87654321",
    "text": "Тестовое сообщение",
    "direction": "outbound"
  }'

# Получить сообщения
curl http://localhost:8080/api/messages/device?device_id=1
```

### Python

```python
import requests

# Отправка сообщения
response = requests.post('http://localhost:8080/api/messages', json={
    'device_id': 1,
    'from_node': '!12345678',
    'to_node': '!87654321',
    'text': 'Привет!',
    'direction': 'outbound'
})

print(response.json())

# Получение сообщений
response = requests.get('http://localhost:8080/api/messages/device?device_id=1')
messages = response.json()
for msg in messages:
    print(f"{msg['from_node']} -> {msg['to_node']}: {msg['text']}")
```

## Решение проблем

### ESP32 не отвечает

1. Проверьте подключение к WiFi
2. Убедитесь что IP адрес правильный
3. Проверьте логи сервера:
```bash
log.Printf("Failed to send message to ESP32: %v", err)
```

### Сообщения не отправляются

1. Проверьте что `from_node` соответствует Node ID устройства
2. Убедитесь что ESP32 включен
3. Проверьте настройки LoRa частоты

### Телефон не подключается

1. Перезапустите Bluetooth на телефоне
2. Удалите старое сопряжение с ESP32
3. Подключитесь заново через приложение

## Дополнительные возможности

### Прямое подключение к Meshtastic

Если ESP32 имеет прямой IP, можно использовать discovery:

```bash
MESHTASTIC_IP=192.168.1.100
```

Сервер будет автоматически опрашивать устройство и получать:
- Информацию об узлах
- Метрики (пульс, температура, etc.)
- Позицию GPS

### WebSocket уведомления

Все новые сообщения транслируются через WebSocket:

```json
{
    "type": "new_message",
    "message": {
        "id": 1,
        "device_id": 1,
        "from_node": "!12345678",
        "to_node": "!87654321",
        "text": "Привет!",
        "direction": "outbound",
        "sent_at": "2026-03-12T14:30:00Z"
    }
}
```

## Безопасность

### Рекомендации

1. Используйте HTTPS для продакшена
2. Настройте аутентификацию API
3. Ограничьте доступ к WiFi сети ESP32
4. Используйте шифрование LoRa (AES256)

## Ресурсы

- [Meshtastic Documentation](https://meshtastic.org/docs/)
- [ESP32 Arduino Core](https://github.com/espressif/arduino-esp32)
- [Meshtastic GitHub](https://github.com/meshtastic/Meshtastic)
