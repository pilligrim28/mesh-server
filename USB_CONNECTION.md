# USB (COM-порт) подключение к Meshtastic

## Обзор

Mesh-server поддерживает прямое подключение к устройствам Meshtastic через USB (COM-порт). Это позволяет общаться с ESP32 без необходимости настройки WiFi или Bluetooth.

## Требования

- ESP32 с прошивкой Meshtastic
- USB кабель для подключения к компьютеру
- Установленные драйверы для ESP32 (CP210x или CH340)

## Шаг 1: Подключите ESP32

1. Подключите ESP32 к компьютеру через USB кабель
2. Определите COM-порт в Диспетчере устройств Windows:
   - Нажмите `Win + X` → **Диспетчер устройств**
   - Раскройте **Порты (COM и LPT)**
   - Найдите устройство типа `Silicon Labs CP210x USB to UART Bridge (COMX)`
   - Запомните номер порта (например, `COM3`)

## Шаг 2: Настройте сервер

### Вариант A: Через .env файл

Откройте `.env` и укажите COM-порт:

```bash
ESP32_COM_PORT=COM3
```

### Вариант B: Через переменную окружения

```bash
set ESP32_COM_PORT=COM3
./mesh-server.exe
```

## Шаг 3: Запустите сервер

```bash
./mesh-server.exe
```

В логе вы должны увидеть:
```
Meshtastic Serial: подключен к COM3
SerialService запущен на COM3
```

## API для работы с COM-портом

### Сканировать доступные COM-порты

```bash
GET /api/serial/scan
```

**Ответ:**
```json
{
    "success": true,
    "ports": ["COM3", "COM4"],
    "count": 2
}
```

### Подключиться к COM-порту

```bash
POST /api/serial/connect
Content-Type: application/json

{
    "port": "COM3"
}
```

**Ответ:**
```json
{
    "success": true,
    "port": "COM3",
    "message": "Подключено к COM3"
}
```

### Проверить статус подключения

```bash
GET /api/serial/status
```

**Ответ:**
```json
{
    "connected": true,
    "port": "COM3"
}
```

### Отправить сообщение

```bash
POST /api/serial/message
Content-Type: application/json

{
    "to_node": "!87654321",
    "text": "Привет через USB!"
}
```

**Ответ:**
```json
{
    "success": true,
    "to_node": "!87654321",
    "text": "Привет через USB!",
    "message": "Сообщение отправлено"
}
```

### Отключиться от COM-порта

```bash
POST /api/serial/disconnect
```

**Ответ:**
```json
{
    "success": true,
    "message": "Отключено от COM-порта"
}
```

## Как это работает

### Входящие сообщения

1. Телефон отправляет сообщение через приложение Meshtastic
2. Сообщение передается по Bluetooth на ESP32
3. ESP32 отправляет сообщение через USB на сервер
4. Сервер сохраняет сообщение в базу данных
5. Сообщение отображается в веб-интерфейсе

### Исходящие сообщения

1. Вы отправляете сообщение через веб-интерфейс
2. Сервер отправляет сообщение через COM-порт на ESP32
3. ESP32 передает сообщение по LoRa
4. Телефон получает сообщение через Bluetooth

## Решение проблем

### Сервер не видит COM-порт

1. Проверьте что ESP32 подключен и включен
2. Проверьте драйверы USB-UART конвертера:
   - CP210x: https://www.silabs.com/developers/usb-to-uart-bridge-vcp-drivers
   - CH340: http://www.wch.cn/downloads/CH341SER_ZIP.html
3. Попробуйте другой USB кабель (некоторые кабели только для зарядки)

### Сообщение не отправляется

1. Проверьте статус подключения: `GET /api/serial/status`
2. Убедитесь что ESP32 с прошивкой Meshtastic
3. Проверьте логи сервера

### Неправильный COM-порт

1. Откройте Диспетчер устройств
2. Найдите правильный порт
3. Обновите `.env` или перезапустите сервер с правильной переменной окружения

## Примеры использования

### curl

```bash
# Сканировать порты
curl http://localhost:8080/api/serial/scan

# Подключиться
curl -X POST http://localhost:8080/api/serial/connect ^
  -H "Content-Type: application/json" ^
  -d "{\"port\": \"COM3\"}"

# Проверить статус
curl http://localhost:8080/api/serial/status

# Отправить сообщение
curl -X POST http://localhost:8080/api/serial/message ^
  -H "Content-Type: application/json" ^
  -d "{\"to_node\": \"!87654321\", \"text\": \"Привет!\"}"
```

### PowerShell

```powershell
# Сканировать порты
Invoke-RestMethod -Uri http://localhost:8080/api/serial/scan

# Подключиться
Invoke-RestMethod -Method POST -Uri http://localhost:8080/api/serial/connect `
  -ContentType "application/json" `
  -Body '{"port": "COM3"}'

# Отправить сообщение
Invoke-RestMethod -Method POST -Uri http://localhost:8080/api/serial/message `
  -ContentType "application/json" `
  -Body '{"to_node": "!87654321", "text": "Привет!"}'
```

## Преимущества USB подключения

| Преимущество | Описание |
|--------------|----------|
| 🔌 Надежность | Проводное соединение стабильнее беспроводного |
| ⚡ Скорость | Выше скорость передачи данных |
| 🔋 Энергия | ESP32 получает питание от USB |
| 🔒 Безопасность | Нет беспроводного излучения |
| 🛠️ Отладка | Удобно для разработки и тестирования |

## Сравнение с другими способами

| Способ | Скорость | Надежность | Удобство |
|--------|----------|------------|----------|
| USB (COM) | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| WiFi | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Bluetooth | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |

## Ресурсы

- [Meshtastic Documentation](https://meshtastic.org/docs/)
- [CP210x Драйверы](https://www.silabs.com/developers/usb-to-uart-bridge-vcp-drivers)
- [CH340 Драйверы](http://www.wch.cn/downloads/CH341SER_ZIP.html)
