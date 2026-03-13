# Подключение к ESP32 через Bluetooth

## Обзор

Mesh-server поддерживает обнаружение и подключение к ESP32 устройствам через Bluetooth LE (Bluetooth Low Energy) на компьютере с Windows.

## Требования

- Windows 10/11 с Bluetooth 4.0+ (поддержка BLE)
- ESP32 с прошивкой Meshtastic
- Включенный Bluetooth на компьютере

## Шаг 1: Сканирование ESP32 устройств

### Через веб-интерфейс:
1. Откройте `http://localhost:8080`
2. Перейдите на вкладку "Сообщения"
3. Нажмите кнопку "Найти ESP32" (если доступна)

### Через API:
```bash
# Сканировать сеть на наличие ESP32
curl http://localhost:8080/api/esp32/scan

# Ответ:
{
    "devices": ["192.168.4.1"],
    "count": 1
}
```

### Через PowerShell (Bluetooth LE):
```powershell
# Запустить сканирование BLE устройств
$watcher = [Windows.Devices.Bluetooth.Advertisement.BluetoothLEAdvertisementWatcher]::new()
$watcher.Start()
Start-Sleep -Seconds 5
$watcher.Stop()
```

## Шаг 2: Подключение к ESP32

### Вариант A: WiFi подключение (рекомендуется)

ESP32 Meshtastic обычно доступен по одному из этих адресов:
- `192.168.4.1` - режим точки доступа
- `192.168.1.100` - режим клиента в вашей сети

```bash
# Подключиться по IP
curl -X POST http://localhost:8080/api/esp32/connect \
  -H "Content-Type: application/json" \
  -d '{"ip": "192.168.4.1"}'
```

### Вариант B: Bluetooth подключение

1. Узнайте MAC адрес ESP32 через сканирование:
```bash
curl http://localhost:8080/api/esp32/bluetooth
```

2. Подключитесь по MAC адресу:
```bash
curl -X POST http://localhost:8080/api/esp32/connect \
  -H "Content-Type: application/json" \
  -d '{"mac": "AA:BB:CC:DD:EE:FF"}'
```

## Шаг 3: Проверка подключения

```bash
# Проверить статус
curl http://localhost:8080/api/esp32/status

# Ответ:
{
    "connected": true,
    "ip": "192.168.4.1",
    "reachable": true
}
```

## Шаг 4: Отправка сообщения

```bash
# Отправить сообщение на телефон через ESP32
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": 1,
    "from_node": "!default",
    "to_node": "!87654321",
    "text": "Привет через Bluetooth!",
    "direction": "outbound"
  }'
```

## Шаг 5: Получение сообщений с телефона

ESP32 автоматически отправляет входящие сообщения на сервер:

```bash
# ESP32 отправляет POST запрос на сервер
POST http://<SERVER_IP>:8080/api/esp32/message
{
    "from_node": "!87654321",
    "to_node": "!12345678",
    "text": "Ответ с телефона",
    "direction": "inbound"
}
```

## Адреса ESP32 Meshtastic по умолчанию

| Режим | IP адрес | Описание |
|-------|----------|----------|
| AP | 192.168.4.1 | Точка доступа ESP32 |
| Station | 192.168.1.100 | Клиент в вашей WiFi сети |
| WiFi | 192.168.1.x | DHCP от роутера |

## Поиск ESP32 в сети

### Автоматическое сканирование:
```bash
curl http://localhost:8080/api/esp32/scan
```

### Ручное сканирование сети:
```bash
# Windows PowerShell
1..254 | ForEach-Object {
    $ip = "192.168.4.$_"
    if (Test-Connection -ComputerName $ip -Count 1 -Quiet) {
        try {
            $response = Invoke-WebRequest -Uri "http://$ip/health" -TimeoutSec 2 -UseBasicParsing
            Write-Host "Found ESP32 at $ip"
        } catch {}
    }
}
```

## Решение проблем

### ESP32 не найден при сканировании

1. Убедитесь что ESP32 включен
2. Проверьте что WiFi/Bluetooth включен на ESP32
3. Перезагрузите ESP32

### Не удается подключиться по Bluetooth

1. Проверьте что Bluetooth включен на компьютере
2. Удалите старое сопряжение с ESP32
3. Попробуйте подключиться через WiFi вместо Bluetooth

### Сообщения не отправляются

1. Проверьте статус подключения: `GET /api/esp32/status`
2. Убедитесь что ESP32 доступен: `ping 192.168.4.1`
3. Проверьте логи сервера

## Пример полного цикла

```bash
# 1. Сканирование
curl http://localhost:8080/api/esp32/scan
# Ответ: {"devices": ["192.168.4.1"], "count": 1}

# 2. Подключение
curl -X POST http://localhost:8080/api/esp32/connect \
  -H "Content-Type: application/json" \
  -d '{"ip": "192.168.4.1"}'

# 3. Проверка
curl http://localhost:8080/api/esp32/status
# Ответ: {"connected": true, "ip": "192.168.4.1", "reachable": true}

# 4. Отправка сообщения
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": 1,
    "from_node": "!default",
    "to_node": "!87654321",
    "text": "Тест",
    "direction": "outbound"
  }'
```

## Bluetooth LE на Windows

Для работы Bluetooth LE на Windows требуется:
- Windows 10 версии 1709 или новее
- Bluetooth адаптер с поддержкой BLE (Bluetooth 4.0+)
- Драйверы Bluetooth установлены

### Проверка поддержки BLE:
```powershell
Get-PnpDevice | Where-Object {$_.FriendlyName -like "*Bluetooth*"}
```

## Альтернативные варианты

Если Bluetooth не работает, используйте WiFi:

1. Подключите ESP32 к той же WiFi сети что и компьютер
2. Узнайте IP адрес ESP32 через приложение Meshtastic
3. Используйте `ESP32_URL=http://<IP>` при запуске сервера

```bash
ESP32_URL=http://192.168.1.100 ./mesh-server.exe
```
