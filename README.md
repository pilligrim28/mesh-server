# Mesh Pulse — мониторинг пульса через Meshtastic

Сервер принимает пульс и SpO₂ с носимых датчиков **ESP32-C3 + MAX30102**, которые передают данные по LoRa-сети **Meshtastic** без интернета и сотовой связи.

```
 ┌───────────────────┐  BLE   ┌──────────────┐   LoRa mesh   ┌──────────────┐  BLE / USB  ┌─────────────┐
 │ ESP32-C3+MAX30102 │ ─────▶ │ Heltec V3    │ ~~~~~~~~~~~~▶ │ Heltec V3    │ ──────────▶ │ mesh-server │──▶ браузер
 │ (датчик на руке)  │  PIN   │ носимый узел │  до ~км, с    │ шлюз «GW_…»  │   без PIN   │ Win / Linux │
 └───────────────────┘        └──────────────┘  ретрансляцией└──────────────┘             └─────────────┘
```

* ESP32-C3 подключается к своему Heltec как обычный клиент Meshtastic (как телефон) и отправляет стандартную телеметрию `HealthMetrics` (`heart_bpm`, `spO2`).
* Носимые узлы ретранслируют пакеты друг через друга, пока они не дойдут до шлюза.
* Сервер **сам находит шлюз по Bluetooth**, подключается и переподключается при обрыве. Запасной вариант — USB-кабель.
* Веб-интерфейс встроен в бинарник и **работает без интернета** (нет CDN).

> ⚠️ Это не медицинское изделие. SpO₂ с MAX30102 — оценочное значение.

## Состав репозитория

| Путь | Что это |
|---|---|
| `cmd/mesh-server` | точка входа сервера |
| `internal/mesh` | протокол Meshtastic: декодирование `FromRadio`, служебные `ToRadio` |
| `internal/transport` | BLE (автопоиск/переподключение), USB-serial, симулятор |
| `internal/core` | обработка показаний, алерты, дедупликация между шлюзами, live-события |
| `internal/store` | SQLite (pure Go, без CGO) |
| `internal/web`, `web/` | REST API, WebSocket, веб-интерфейс |
| `firmware/esp32c3-pulse` | прошивка датчика (PlatformIO) |
| `firmware/meshtastic-heltec-v3` | сборка прошивок Meshtastic для шлюза и носимых узлов |
| `firmware/promicro-max30102`, `cmd/pulse-com` | Pro Micro + MAX30102 по USB: скетч и приложение записи в CSV |

---

## 1. Быстрый старт без железа

```bash
go run ./cmd/mesh-server        # Go 1.25+
# или с симулятором 4 узлов:
TRANSPORT=sim go run ./cmd/mesh-server          # Linux/macOS
set TRANSPORT=sim && go run ./cmd/mesh-server   # Windows cmd
```

Откройте <http://localhost:8080>, логин `admin` / `admin`. Смените пароль кнопкой 🔑.

Симулятор формирует настоящие protobuf-пакеты Meshtastic и прогоняет их через тот же декодер, что и данные с железа. Он показывает и алерты: высокий пульс, снятый датчик.

## 2. Железо

| Кол-во | Устройство |
|---|---|
| 1 | Heltec WiFi LoRa 32 **V3** — шлюз у сервера |
| N | Heltec WiFi LoRa 32 **V3** — по одному на человека (носимый узел) |
| N | ESP32-C3 (например, SuperMini) + модуль **MAX30102** |
| — | антенны 868 МГц на каждый Heltec (**не включайте Heltec без антенны**) |

### Подключение MAX30102 к ESP32-C3 SuperMini

| MAX30102 | ESP32-C3 |
|---|---|
| VIN | 3V3 |
| GND | GND |
| SDA | GPIO 6 |
| SCL | GPIO 7 |
| INT | не нужен |

Пины меняются в `firmware/esp32c3-pulse/include/config.h`. Красные «китайские» модули MAX30102 иногда имеют подтяжки I²C к 1.8 В. Если датчик не находится, проверьте модуль по схеме или поставьте подтяжки 4.7 кОм к 3.3 В.

> Сейчас поддерживается только **MAX30102**. MAX30100 — другой чип с другим протоколом.

## 3. Прошивка Heltec V3 (Meshtastic)

Готовые прошивки лежат в `firmware/meshtastic-heltec-v3/out/` (или в архиве релиза). Чтобы собрать их самостоятельно:

```bash
pip install platformio
cd firmware/meshtastic-heltec-v3
python build.py            # gateway + node, ~10 мин на каждую при первой сборке
```

| Профиль | BLE | Имя по BLE | Для чего |
|---|---|---|---|
| `gateway` | **без PIN** | `GW_xxxx` | у сервера: Windows и Linux подключаются автоматически, без сопряжения |
| `node` | PIN `123456` | `Meshtastic_xxxx` | носимый узел: к нему подключается ESP32-C3 |

В обе прошивки зашиты регион **RU** (868.7–869.2 МГц), пресет `LONG_FAST`, приватный канал `MeshPulse` и **общий 256-битный ключ шифрования**. Узлы сразу видят друг друга, а посторонние Meshtastic-устройства ваши данные прочитать не могут.

Параметры задаются в `settings.json`: регион, пресет, имя канала, PIN, интервал отчёта о батарее.
**Ключ сети хранится в `settings.json`.** Не теряйте и не публикуйте этот файл: прошивки для новых узлов собирайте с ним же. Файл добавлен в `.gitignore`.

### Как прошить

Настройки применяются только к «чистому» устройству, поэтому прошивайте **factory.bin с полной очисткой памяти**:

```bash
pip install esptool
esptool.py --chip esp32s3 erase_flash
esptool.py --chip esp32s3 --baud 921600 write_flash 0x0 meshpulse-gateway-heltec-v3-<версия>.factory.bin
```

Без командной строки: откройте в Chrome <https://espressif.github.io/esptool-js/>, нажмите Connect → **Erase Flash** → загрузите файл по адресу `0x0` → Program.

Если плата не определяется: удерживайте **PRG**, нажмите **RST**, отпустите PRG.

## 4. Прошивка ESP32-C3

```bash
cd firmware/esp32c3-pulse
pio run -t upload
pio device monitor        # лог; команды: status, forget
```

Готовый файл `pulse-esp32c3.factory.bin` можно залить через тот же esptool-js по адресу `0x0`.

**Как датчик находит «свой» узел.** При первом включении ESP32-C3 подключается к **ближайшему** узлу Meshtastic, сопрягается по PIN и запоминает его. Шлюзы `GW_…` она пропускает.
Поэтому при первом включении держите датчик рядом со своим Heltec, а остальные узлы — подальше.
Сброс привязки: удерживайте **BOOT** 3 секунды при включении либо отправьте `forget` в мониторе порта.
Привязать датчик к конкретному узлу можно и явно: `MESH_TARGET_NAME "Meshtastic_1a2b"` в `config.h`.

| Светодиод | Значение |
|---|---|
| частое мигание | ищет узел |
| двойные вспышки | подключается |
| вспышка на каждый удар | подключено, пульс есть |
| короткая вспышка раз в 3 с | подключено, нет контакта с кожей |

Данные уходят в сеть раз в **30 с** (`SEND_INTERVAL_S`). При выходе пульса за пороги и при снятии датчика — раньше, но не чаще раза в 12 с: этого требуют лимит прошивки Meshtastic и общий LoRa-эфир.

## 5. Запуск сервера

### Windows 10/11
1. Включите Bluetooth в «Параметрах». Сопрягать шлюз в Windows **не нужно**.
2. Положите рядом `mesh-server-windows-amd64.exe`, `.env.example` и `run-windows.bat`, затем запустите `run-windows.bat`.

### Linux / Raspberry Pi
```bash
sudo apt install bluez         # нужен bluetoothd (BlueZ ≥ 5.50)
cp .env.example .env
./mesh-server-linux-arm64      # Raspberry Pi 4/5 (64-бит); amd64 — ПК
```
Права: пользователь должен иметь доступ к BlueZ через D-Bus (обычно достаточно группы `bluetooth`).
Для автозапуска есть `deploy/mesh-server.service` (systemd).

### Через USB вместо Bluetooth
Подключите шлюз кабелем и задайте `TRANSPORT=serial` и `SERIAL_PORT=COM4` (Windows) или `/dev/ttyUSB0` (Linux).
Можно использовать оба способа сразу: `TRANSPORT=ble,serial`. Повторы пакетов отбрасываются.

### Сборка сервера
```bash
make test     # go vet + тесты
make dist     # windows-amd64, linux-amd64, linux-arm64, linux-armv7 → dist/
```

## 6. Настройки (`.env`)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `TRANSPORT` | `ble` | `ble`, `serial`, `sim` или комбинация через запятую |
| `BLE_NAME_FILTER` | `GW_` | подстрока BLE-имени шлюза |
| `BLE_ADDRESSES` | — | жёсткий список адресов шлюзов |
| `BLE_MAX_GATEWAYS` | `1` | сколько шлюзов держать подключёнными |
| `SERIAL_PORT` | — | порт для `serial` |
| `HR_LOW` / `HR_HIGH` | `45` / `150` | пороги алертов по пульсу |
| `SPO2_LOW` | `90` | порог SpO₂ |
| `OFFLINE_AFTER_SECONDS` | `180` | алерт «нет данных» |
| `RETENTION_DAYS` | `30` | хранение показаний |
| `ADMIN_USER` / `ADMIN_PASSWORD` | `admin` / `admin` | создаётся при первом запуске |

Сброс пароля: `mesh-server -reset-password НовыйПароль`.

## 7. Алерты

| Тип | Когда |
|---|---|
| `hr_high` / `hr_low` | пульс вне порогов |
| `spo2_low` | SpO₂ ниже порога |
| `no_contact` | датчик подключён, но не касается кожи |
| `offline` | от датчика нет пакетов дольше `OFFLINE_AFTER_SECONDS` |

Алерт закрывается автоматически, когда показатель возвращается в норму. Кнопка «Принять» отмечает, что алерт увидели.

## 8. Диагностика

| Симптом | Что проверить |
|---|---|
| Шлюз «поиск…» | шлюз прошит профилем `gateway` (имя `GW_…` видно в nRF Connect); к нему не подключён телефон — у узла одно BLE-подключение |
| Шлюз подключён, данных нет | на всех узлах одинаковые регион и ключ (собраны с одним `settings.json`); у Heltec есть антенна |
| ESP32-C3 мигает часто | её узел далеко или занят телефоном; удерживайте BOOT для сброса привязки |
| `запись отклонена — неверный PIN?` | `MESH_BLE_PIN` ≠ `node_ble_pin` из `settings.json` |
| «нет контакта» при пальце на датчике | слабо прижат палец/запястье; увеличьте `LED_BRIGHTNESS` |
| Linux: `Bluetooth-адаптер недоступен` | `systemctl status bluetooth`, `rfkill unblock bluetooth` |

## 9. API

Все методы, кроме `/api/health` и `/api/login`, требуют cookie сессии.

| Метод | Путь | |
|---|---|---|
| POST | `/api/login` | `{"username","password"}` |
| GET | `/api/nodes` | узлы с последними показаниями |
| GET | `/api/nodes/{num}/readings?minutes=60` | история (`num` — число или `!a1b2c3d4`) |
| PATCH | `/api/nodes/{num}` | `{"alias":"Иванов"}` |
| DELETE | `/api/nodes/{num}` | удалить узел и историю |
| GET | `/api/alerts?active=1` | алерты |
| POST | `/api/alerts/{id}/ack` | принять алерт |
| GET | `/api/gateways` | состояние подключений |
| WS | `/ws` | события `reading`, `alert`, `gateway`, `battery`, … |

## 10. Вариант на Arduino Pro Micro по USB (без радио)

Если пока есть только Pro Micro с MAX30102 (SDA → 2, SCL → 3): скетч и приложение `pulse-com`
для чтения по COM-порту с записью каждой сессии в отдельные CSV —
см. [firmware/promicro-max30102/README.md](firmware/promicro-max30102/README.md).

```
go build -o pulse-com.exe ./cmd/pulse-com     # или go run ./cmd/pulse-com -sim
```
