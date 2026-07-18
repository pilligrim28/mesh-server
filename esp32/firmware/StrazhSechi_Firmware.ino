// ============================================================
// СтражСети — Прошивка ESP32 для mesh-мониторинга
// Версия: 1.0.0
// ============================================================
//
// Возможности:
// - Meshtastic-совместимый HTTP API (поддержка приложения Meshtastic)
// - Подключение к mesh-server через WiFi или Serial (USB)
// - GPS-трекинг (через встроенный GPS или smartphone tethering)
// - LoRa mesh (SX1276/SX1262) — опционально
// - BLE сканирование и подключение к Healbe GoBe
// - OTA-обновления через WiFi
// - Веб-интерфейс для настройки
// - Автономный AP-режим для первоначальной настройки
//
// Совместимость:
// - Meshtastic Android/iOS приложение (через HTTP API на порту 4403)
// - mesh-server API (через WiFi)
// - Serial USB (115200 baud) — прямое подключение к ПК
//
// Платформы:
// - ESP32 (основная)
// - ESP32-S3 (с поддержкой BLE 5.0)
// - ESP32 + SX1276/SX1262 (LoRa)
// ============================================================

#include <WiFi.h>
#include <HTTPClient.h>
#include <ArduinoJson.h>
#include <WebServer.h>
#include <Update.h>

// BLE
#include <BLEDevice.h>
#include <BLEUtils.h>
#include <BLEScan.h>
#include <BLEAdvertisedDevice.h>
#include <BLEClient.h>
#include <BLERemoteCharacteristic.h>

// Опционально: LoRa (раскомментировать для LoRa-плат)
// #include <SPI.h>
// #include <LoRa.h>

// ============================================================
// КОНФИГУРАЦИЯ
// ============================================================

// Идентификация устройства
#define DEVICE_NAME       "СтражСети Hub"
#define DEVICE_SHORT_NAME "СС"
#define FIRMWARE_VERSION  "1.0.0"
#define HARDWARE_MODEL    "ESP32-S3"
#define REGION            "RU"

// WiFi (AP режим для первоначальной настройки)
#define AP_SSID           "СтражСети-Setup"
#define AP_PASSWORD       "stражсеть2024"
#define AP_CHANNEL        1

// Serial
#define SERIAL_BAUD       115200
#define SERIAL_TIMEOUT    100

// Таймеры (мс)
#define WIFI_RETRY_INTERVAL    30000
#define SERVER_POLL_INTERVAL   5000
#define BLE_SCAN_INTERVAL      10000
#define GPS_READ_INTERVAL      1000
#define HEALBE_SYNC_INTERVAL   10000
#define STATUS_LED_INTERVAL    1000
#define OTA_CHECK_INTERVAL     3600000  // 1 час

// GPIO пины (ESP32-S3)
#define PIN_LED_STATUS    48   // Встроенный RGB LED (ESP32-S3)
#define PIN_LED_RED       48
#define PIN_LED_GREEN     47
#define PIN_LED_BLUE      21
#define PIN_LORA_CS       10   // LoRa chip select
#define PIN_LORA_RST      9    // LoRa reset
#define PIN_LORA_DIO0     4    // LoRa DIO0
#define PIN_GPS_RX        16   // GPS TX → ESP RX
#define PIN_GPS_TX        17   // ESP TX → GPS RX
#define PIN_BUTTON        0    // Кнопка настройки

// ============================================================
// СТРУКТУРЫ ДАННЫХ
// ============================================================

struct DeviceConfig {
    char wifiSSID[64];
    char wifiPassword[64];
    char serverURL[128];
    char nodeName[32];
    uint32_t nodeNum;
    bool loraEnabled;
    uint32_t loraFrequency;  // Гц
    int8_t loraTxPower;      // дБм
    bool gpsEnabled;
    bool bleEnabled;
    bool mqttEnabled;
    char mqttServer[128];
    char mqttTopic[64];
    float gpsLatitude;
    float gpsLongitude;
    int32_t gpsAltitude;
};

struct SensorData {
    float latitude;
    float longitude;
    int32_t altitude;
    float speed;
    float hdop;
    uint8_t satellites;
    int heartRate;
    int stressLevel;
    int batteryPercent;
    float temperature;
    float humidity;
    int co2;
    uint32_t timestamp;
    bool gpsValid;
};

struct MeshMessage {
    uint32_t from;
    uint32_t to;
    uint32_t id;
    uint8_t channel;
    uint8_t portnum;
    String payload;
    float snr;
    int16_t rssi;
    uint32_t rxTime;
    bool wantsAck;
};

struct NodeInfo {
    uint32_t num;
    String longName;
    String shortName;
    float latitude;
    float longitude;
    int32_t altitude;
    uint32_t lastHeard;
    float snr;
    int16_t rssi;
    bool hasPosition;
    bool isFavorite;
};

// ============================================================
// ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ
// ============================================================

DeviceConfig config;
SensorData sensors;
WebServer httpServer(80);
WebServer meshtasticServer(4403);  // Meshtastic HTTP API port

// BLE
BLEScan* bleScan;
bool bleClientConnected = false;
BLEClient* bleClient = nullptr;
BLERemoteCharacteristic* heartRateChar = nullptr;
BLERemoteCharacteristic* stressChar = nullptr;

// Таймеры
unsigned long lastWifiRetry = 0;
unsigned long lastServerPoll = 0;
unsigned long lastBleScan = 0;
unsigned long lastGpsRead = 0;
unsigned long lastHealbeSync = 0;
unsigned long lastStatusLed = 0;
unsigned long lastOtaCheck = 0;
unsigned long lastPositionReport = 0;

// Состояние
bool wifiConnected = false;
bool apMode = false;
bool serialConnected = false;
String serialBuffer = "";
std::vector<NodeInfo> meshNodes;
std::vector<String> discoveredDevices;

// Meshtastic HTTP API стейт
String meshtasticConfigJson = "";
String meshtasticNodesJson = "";

// ============================================================
// НАСТРОЙКА
// ============================================================

void setup() {
    Serial.begin(SERIAL_BAUD);
    delay(1000);

    printBanner();

    // Инициализация GPIO
    initGPIO();

    // Загрузка конфигурации из EEPROM
    loadConfig();

    // Если имя WiFi не задано — запускаем AP
    if (strlen(config.wifiSSID) == 0) {
        startAP();
    } else {
        connectWiFi();
    }

    // Инициализация BLE
    if (config.bleEnabled) {
        initBLE();
    }

    // Инициализация GPS
    if (config.gpsEnabled) {
        initGPS();
    }

    // Инициализация LoRa
    if (config.loraEnabled) {
        initLoRa();
    }

    // Веб-сервер mesh-server
    setupMeshServerAPI();

    // Meshtastic HTTP API (совместимость с приложением)
    setupMeshtasticAPI();

    // HTTP сервер для настройки
    setupConfigWebServer();

    // Запуск серверов
    httpServer.begin();
    meshtasticServer.begin();

    log("✓ СтражСети Hub запущен (v" + String(FIRMWARE_VERSION) + ")");
    log("  IP: " + WiFi.localIP().toString());
    log("  Meshtastic API: порт 4403");
    log("  Настройка: http://" + WiFi.localIP().toString());

    blinkLed(3);  // 3 мигания = готов к работе
}

// ============================================================
// ОСНОВНОЙ ЦИКЛ
// ============================================================

void loop() {
    unsigned long now = millis();

    // Обработка HTTP запросов
    httpServer.handleClient();
    meshtasticServer.handleClient();

    // Serial USB (прямое подключение к mesh-server)
    handleSerial();

    // WiFi проверка
    if (now - lastWifiRetry > WIFI_RETRY_INTERVAL) {
        lastWifiRetry = now;
        checkWiFi();
    }

    // GPS чтение
    if (config.gpsEnabled && now - lastGpsRead > GPS_READ_INTERVAL) {
        lastGpsRead = now;
        readGPS();
    }

    // BLE сканирование
    if (config.bleEnabled && now - lastBleScan > BLE_SCAN_INTERVAL) {
        lastBleScan = now;
        scanBLE();
    }

    // Отправка позиции на сервер
    if (now - lastPositionReport > SERVER_POLL_INTERVAL) {
        lastPositionReport = now;
        reportPosition();
    }

    // Получение сообщений от сервера
    if (now - lastServerPoll > SERVER_POLL_INTERVAL) {
        lastServerPoll = now;
        pollServer();
    }

    // Healbe синхронизация
    if (now - lastHealbeSync > HEALBE_SYNC_INTERVAL) {
        lastHealbeSync = now;
        syncHealbe();
    }

    // LED статус
    if (now - lastStatusLed > STATUS_LED_INTERVAL) {
        lastStatusLed = now;
        updateStatusLed();
    }

    // LoRa обработка
    if (config.loraEnabled) {
        handleLoRa();
    }

    delay(10);
}

// ============================================================
// СЕТЕВЫЕ ФУНКЦИИ
// ============================================================

void connectWiFi() {
    log("Подключение к WiFi: " + String(config.wifiSSID));

    WiFi.mode(WIFI_STA);
    WiFi.setHostname(config.nodeName);
    WiFi.begin(config.wifiSSID, config.wifiPassword);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
        delay(500);
        Serial.print(".");
        attempts++;
    }

    if (WiFi.status() == WL_CONNECTED) {
        wifiConnected = true;
        apMode = false;
        log("\n✓ WiFi подключен: " + WiFi.localIP().toString());
    } else {
        log("\n✗ WiFi не подключен, запуск AP");
        startAP();
    }
}

void startAP() {
    WiFi.mode(WIFI_AP_STA);
    WiFi.softAP(AP_SSID, AP_PASSWORD, AP_CHANNEL);
    apMode = true;
    wifiConnected = false;
    log("✓ AP создан: " + String(AP_SSID));
    log("  IP: " + WiFi.softAPIP().toString());
}

void checkWiFi() {
    if (apMode) return;

    if (WiFi.status() != WL_CONNECTED) {
        wifiConnected = false;
        log("WiFi отключен, переподключение...");
        WiFi.reconnect();
    } else {
        wifiConnected = true;
    }
}

// ============================================================
// MESHTASTIC-СОВМЕСТИМЫЙ HTTP API
// ============================================================

void setupMeshtasticAPI() {
    // GET /json/device — информация об устройстве
    meshtasticServer.on("/json/device", HTTP_GET, []() {
        StaticJsonDocument<512> doc;
        doc["status"] = "ok";
        JsonObject data = doc.createNestedObject("data");
        data["version"] = FIRMWARE_VERSION;
        data["firmware"] = "СтражСети";
        data["hardware"] = HARDWARE_MODEL;
        data["region"] = REGION;
        data["has_wifi"] = true;
        data["has_bluetooth"] = config.bleEnabled;
        data["has_lora"] = config.loraEnabled;
        data["has_gps"] = config.gpsEnabled;

        String json;
        serializeJson(doc, json);
        meshtasticServer.send(200, "application/json", json);
    });

    // GET /json/nodes — список узлов mesh-сети
    meshtasticServer.on("/json/nodes", HTTP_GET, []() {
        String json = buildNodesJson();
        meshtasticServer.send(200, "application/json", json);
    });

    // GET /json/report — отчёт об устройстве
    meshtasticServer.on("/json/report", HTTP_GET, []() {
        StaticJsonDocument<512> doc;
        doc["status"] = "ok";
        JsonObject data = doc.createNestedObject("data");

        JsonObject wifi = data.createNestedObject("wifi");
        wifi["rssi"] = WiFi.RSSI();
        wifi["ip"] = WiFi.localIP().toString();

        JsonObject mem = data.createNestedObject("memory");
        mem["heap_total"] = ESP.getHeapSize();
        mem["heap_free"] = ESP.getFreeHeap();

        JsonObject power = data.createNestedObject("power");
        power["battery_percent"] = sensors.batteryPercent;
        power["has_battery"] = true;

        JsonObject radio = data.createNestedObject("radio");
        radio["has_lora"] = config.loraEnabled;

        String json;
        serializeJson(doc, json);
        meshtasticServer.send(200, "application/json", json);
    });

    // GET /api/v1/fromradio — protobuf API (заглушка)
    meshtasticServer.on("/api/v1/fromradio", HTTP_GET, []() {
        meshtasticServer.sendHeader("Content-Type", "application/x-protobuf");
        meshtasticServer.send(200, "application/x-protobuf", "");
    });

    // PUT /api/v1/toradio — protobuf API (заглушка)
    meshtasticServer.on("/api/v1/toradio", HTTP_PUT, []() {
        meshtasticServer.send(200, "application/x-protobuf", "");
    });

    // GET / — главная страница (совместимость с Meshtastic)
    meshtasticServer.on("/", HTTP_GET, []() {
        String html = buildMeshtasticHomePage();
        meshtasticServer.send(200, "text/html", html);
    });

    // GET /admin
    meshtasticServer.on("/admin", HTTP_GET, []() {
        String html = buildAdminPage();
        meshtasticServer.send(200, "text/html", html);
    });

    // GET /restart
    meshtasticServer.on("/restart", HTTP_GET, []() {
        meshtasticServer.send(200, "text/html",
            "<html><body><h1>СтражСети</h1><p>Перезагрузка...</p></body></html>");
        delay(1000);
        ESP.restart();
    });

    // Captive portal
    meshtasticServer.on("/hotspot-detect.html", HTTP_GET, []() {
        meshtasticServer.sendHeader("Connection", "close");
        meshtasticServer.send(302, "text/html", "");
    });
}

// ============================================================
// API ДЛЯ MESH-SERVER
// ============================================================

void setupMeshServerAPI() {
    // Health check
    httpServer.on("/health", HTTP_GET, []() {
        httpServer.send(200, "text/plain", "OK");
    });

    // Статус устройства
    httpServer.on("/api/status", HTTP_GET, []() {
        StaticJsonDocument<512> doc;
        doc["device_name"] = DEVICE_NAME;
        doc["firmware"] = FIRMWARE_VERSION;
        doc["wifi_connected"] = wifiConnected;
        doc["wifi_ip"] = WiFi.localIP().toString();
        doc["wifi_rssi"] = WiFi.RSSI();
        doc["gps_valid"] = sensors.gpsValid;
        doc["latitude"] = sensors.latitude;
        doc["longitude"] = sensors.longitude;
        doc["altitude"] = sensors.altitude;
        doc["battery"] = sensors.batteryPercent;
        doc["uptime"] = millis() / 1000;
        doc["free_heap"] = ESP.getFreeHeap();
        doc["lora_enabled"] = config.loraEnabled;
        doc["ble_enabled"] = config.bleEnabled;
        doc["mesh_nodes"] = (int)meshNodes.size();

        String json;
        serializeJson(doc, json);
        httpServer.send(200, "application/json", json);
    });

    // Healbe bridge API
    httpServer.on("/api/healbe/connect", HTTP_POST, handleHealbeConnect);
    httpServer.on("/api/healbe/disconnect", HTTP_POST, handleHealbeDisconnect);
    httpServer.on("/api/healbe/status", HTTP_GET, handleHealbeStatus);
    httpServer.on("/api/healbe/data", HTTP_GET, handleHealbeData);

    // Получение сообщений от сервера
    httpServer.on("/api/message", HTTP_POST, handleIncomingMessage);

    // Обновление конфигурации
    httpServer.on("/api/config", HTTP_POST, handleConfigUpdate);
    httpServer.on("/api/config", HTTP_GET, handleConfigGet);

    // OTA обновление
    httpServer.on("/api/ota", HTTP_POST, handleOtaStart);
    httpServer.on("/api/ota/upload", HTTP_POST, handleOtaUpload);
    httpServer.on("/api/ota/complete", HTTP_POST, handleOtaComplete);
}

// ============================================================
// MESHTASTIC HOME PAGE
// ============================================================

String buildMeshtasticHomePage() {
    String html = "<!DOCTYPE html><html><head>";
    html += "<meta charset='UTF-8'>";
    html += "<meta name='viewport' content='width=device-width,initial-scale=1'>";
    html += "<title>" + String(DEVICE_NAME) + "</title>";
    html += "<style>";
    html += "body{font-family:system-ui;background:#0a1628;color:#e8edf5;margin:0;padding:20px;}";
    html += ".card{background:#121d33;border:1px solid #1e3456;border-radius:12px;padding:16px;margin:12px 0;}";
    html += "h1{color:#22d3a7;margin-bottom:4px;}";
    html += "h2{color:#8fa3c4;font-size:14px;margin-top:16px;}";
    html += ".stat{display:inline-block;text-align:center;padding:12px 16px;background:#0b1a30;border-radius:8px;margin:4px;min-width:80px;}";
    html += ".stat .val{font-size:24px;font-weight:bold;color:#22d3a7;}";
    html += ".stat .label{font-size:11px;color:#5e7a9e;margin-top:4px;}";
    html += ".online{color:#22d3a7;} .offline{color:#ef4444;}";
    html += "a{color:#22d3a7;text-decoration:none;}";
    html += "</style></head><body>";

    html += "<h1>🛡️ " + String(DEVICE_NAME) + "</h1>";
    html += "<p style='color:#5e7a9e;'>Node: !" + String(config.nodeNum, HEX) + " · v" + FIRMWARE_VERSION + "</p>";

    // Статус
    html += "<div class='card'>";
    html += "<h2>📡 Статус</h2>";
    html += "<div class='stat'><div class='val'>" + String(WiFi.RSSI()) + "</div><div class='label'>RSSI</div></div>";
    html += "<div class='stat'><div class='val'>" + String(sensors.gpsValid ? "✓" : "✗") + "</div><div class='label'>GPS</div></div>";
    html += "<div class='stat'><div class='val'>" + String(sensors.batteryPercent) + "%</div><div class='label'>Батарея</div></div>";
    html += "<div class='stat'><div class='val'>" + String(meshNodes.size()) + "</div><div class='label'>Узлов</div></div>";
    html += "</div>";

    // Позиция
    if (sensors.gpsValid) {
        html += "<div class='card'>";
        html += "<h2>📍 Позиция</h2>";
        html += "<p>Широта: " + String(sensors.latitude, 6) + "</p>";
        html += "<p>Долгота: " + String(sensors.longitude, 6) + "</p>";
        html += "<p>Высота: " + String(sensors.altitude) + " м</p>";
        html += "<p><a href='https://maps.google.com/?q=" + String(sensors.latitude, 6) + "," + String(sensors.longitude, 6) + "' target='_blank'>Открыть на карте</a></p>";
        html += "</div>";
    }

    // Mesh-узлы
    if (meshNodes.size() > 0) {
        html += "<div class='card'>";
        html += "<h2>🔗 Mesh-узлы (" + String(meshNodes.size()) + ")</h2>";
        for (const auto& node : meshNodes) {
            html += "<p class='" + String(node.lastHeard > millis()-60000 ? "online" : "offline") + "'>";
            html += "● " + node.longName + " (!" + String(node.num, HEX) + ")";
            html += " — " + String(node.snr, 1) + " dB";
            if (node.hasPosition) {
                html += " · 📍";
            }
            html += "</p>";
        }
        html += "</div>";
    }

    // Ссылки
    html += "<div class='card'>";
    html += "<h2>⚙️ Навигация</h2>";
    html += "<p><a href='/admin'>⚙️ Настройка</a></p>";
    html += "<p><a href='/json/device'>📄 JSON: устройство</a></p>";
    html += "<p><a href='/json/nodes'>📄 JSON: узлы</a></p>";
    html += "<p><a href='/api/status'>📊 API статус</a></p>";
    html += "</div>";

    html += "</body></html>";
    return html;
}

String buildAdminPage() {
    String html = "<!DOCTYPE html><html><head>";
    html += "<meta charset='UTF-8'><meta name='viewport' content='width=device-width,initial-scale=1'>";
    html += "<title>" + String(DEVICE_NAME) + " — Настройка</title>";
    html += "<style>";
    html += "body{font-family:system-ui;background:#0a1628;color:#e8edf5;margin:0;padding:20px;}";
    html += "input,select,textarea{background:#0b1120;border:1px solid #1e3456;color:#fff;padding:8px;border-radius:6px;width:100%;margin:4px 0 12px;box-sizing:border-box;}";
    html += "input:focus{border-color:#22d3a7;outline:none;}";
    html += "button{background:#22d3a7;color:#0a1628;border:none;padding:10px 20px;border-radius:6px;font-weight:bold;cursor:pointer;width:100%;margin:4px 0;}";
    html += "button:hover{background:#1aab8c;}";
    html += "button.danger{background:#ef4444;}";
    html += ".card{background:#121d33;border:1px solid #1e3456;border-radius:12px;padding:16px;margin:12px 0;}";
    html += "h1{color:#22d3a7;} h2{color:#8fa3c4;font-size:16px;}";
    html += "label{color:#8fa3c4;font-size:13px;display:block;margin-top:8px;}";
    html += "</style></head><body>";

    html += "<h1>🛡️ " + String(DEVICE_NAME) + " — Настройка</h1>";

    html += "<div class='card'>";
    html += "<h2>📶 WiFi</h2>";
    html += "<label>SSID:</label>";
    html += "<input id='ssid' value='" + String(config.wifiSSID) + "'>";
    html += "<label>Пароль:</label>";
    html += "<input id='pass' type='password' value='" + String(config.wifiPassword) + "'>";
    html += "</div>";

    html += "<div class='card'>";
    html += "<h2>🖥️ Mesh-сервер</h2>";
    html += "<label>URL сервера:</label>";
    html += "<input id='server' value='" + String(config.serverURL) + "'>";
    html += "</div>";

    html += "<div class='card'>";
    html += "<h2>📡 Устройство</h2>";
    html += "<label>Имя узла:</label>";
    html += "<input id='nodename' value='" + String(config.nodeName) + "'>";
    html += "</div>";

    html += "<div class='card'>";
    html += "<h2>📡 LoRa (опционально)</h2>";
    html += "<label><input type='checkbox' id='lora'" + String(config.loraEnabled ? " checked" : "") + "> Включить LoRa</label>";
    html += "<label>Частота (Гц):</label>";
    html += "<input id='freq' type='number' value='" + String(config.loraFrequency) + "'>";
    html += "</div>";

    html += "<button onclick='saveConfig()'>💾 Сохранить и перезагрузить</button>";
    html += "<button class='danger' onclick='if(confirm(\"Перезагрузить?\"))location.reload()'>🔄 Перезагрузить</button>";

    html += "<script>";
    html += "function saveConfig(){";
    html += "  const c={";
    html += "    wifi_ssid:document.getElementById('ssid').value,";
    html += "    wifi_password:document.getElementById('pass').value,";
    html += "    server_url:document.getElementById('server').value,";
    html += "    node_name:document.getElementById('nodename').value,";
    html += "    lora_enabled:document.getElementById('lora').checked,";
    html += "    lora_frequency:parseInt(document.getElementById('freq').value)";
    html += "  };";
    html += "  fetch('/api/config',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(c)})";
    html += "    .then(r=>r.json()).then(d=>{alert('Сохранено! Перезагрузка...');setTimeout(()=>location.reload(),2000);});";
    html += "}";
    html += "</script>";

    html += "</body></html>";
    return html;
}

// ============================================================
// GPS
// ============================================================

void initGPS() {
    log("Инициализация GPS...");
    // HardwareSerial для GPS
    Serial1.begin(9600, SERIAL_8N1, PIN_GPS_RX, PIN_GPS_TX);
    log("GPS инициализирован (UART1)");
}

void readGPS() {
    // Простой парсинг NMEA
    while (Serial1.available()) {
        char c = Serial1.read();
        serialBuffer += c;
        if (c == '\n') {
            parseNMEA(serialBuffer);
            serialBuffer = "";
        }
    }
}

void parseNMEA(String sentence) {
    if (!sentence.startsWith("$GPGGA") && !sentence.startsWith("$GNGGA")) return;

    // Парсинг GGA (позиция + количество спутников)
    int commas[15];
    int idx = 0;
    for (int i = 0; i < sentence.length() && idx < 15; i++) {
        if (sentence[i] == ',') commas[idx++] = i;
    }

    if (idx < 10) return;

    // Время
    String timeStr = sentence.substring(commas[0] + 1, commas[1]);

    // Широта
    String latStr = sentence.substring(commas[1] + 1, commas[2]);
    String latDir = sentence.substring(commas[2] + 1, commas[3]);

    // Долгота
    String lonStr = sentence.substring(commas[3] + 1, commas[4]);
    String lonDir = sentence.substring(commas[4] + 1, commas[5]);

    // Качество GPS
    String quality = sentence.substring(commas[5] + 1, commas[6]);

    // Количество спутников
    String sats = sentence.substring(commas[6] + 1, commas[7]);

    // HDOP
    String hdop = sentence.substring(commas[7] + 1, commas[8]);

    // Высота
    String alt = sentence.substring(commas[8] + 1, commas[9]);

    if (latStr.length() > 0 && lonStr.length() > 0) {
        sensors.latitude = parseNMEACoord(latStr, latDir == "N");
        sensors.longitude = parseNMEACoord(lonStr, lonDir == "E");
        sensors.altitude = alt.toFloat();
        sensors.satellites = sats.toInt();
        sensors.hdop = hdop.toFloat();
        sensors.gpsValid = quality.toInt() > 0;
        sensors.timestamp = millis();
    }
}

float parseNMEACoord(String coord, bool positive) {
    if (coord.length() < 4) return 0;
    float raw = coord.toFloat();
    int deg = (int)(raw / 100);
    float minutes = raw - deg * 100;
    float decimal = deg + minutes / 60.0;
    return positive ? decimal : -decimal;
}

// ============================================================
// BLE
// ============================================================

void initBLE() {
    log("Инициализация BLE...");
    BLEDevice::init(String(DEVICE_NAME));
    bleScan = BLEDevice::getScan();
    bleScan->setAdvertisedDeviceCallbacks(new BLEScanCallback());
    bleScan->setInterval(100);
    bleScan->setWindow(99);
    bleScan->setActiveScan(true);
    log("BLE инициализирован");
}

void scanBLE() {
    if (!config.bleEnabled) return;

    BLEScanResults results = bleScan->start(3, false);
    discoveredDevices.clear();

    for (int i = 0; i < results.getCount(); i++) {
        BLEAdvertisedDevice dev = results.getDevice(i);
        String info = String(dev.getAddress().toString().c_str()) + "|" +
                      String(dev.getName().c_str()) + "|" +
                      String(dev.getRSSI());
        discoveredDevices.push_back(info);

        // Проверяем Healbe GoBe
        String name = String(dev.getName().c_str());
        name.toLowerCase();
        if (name.indexOf("healbe") >= 0 || name.indexOf("gobe") >= 0) {
            // Нашли часы — пытаемся подключиться
            connectToHealbe(dev);
        }
    }

    bleScan->clearResults();
}

// ============================================================
// HEALBE GOBE BRIDGE
// ============================================================

void connectToHealbe(BLEAdvertisedDevice& device) {
    log("Healbe найден: " + String(device.getAddress().toString().c_str()));

    // Подключение к Healbe через GATT
    if (bleClient && bleClient->isConnected()) {
        bleClient->disconnect();
    }

    bleClient = BLEDevice::createClient();
    if (!bleClient->connect(&device)) {
        log("Не удалось подключиться к Healbe");
        return;
    }

    log("Healbe подключен");

    // Heart Rate Service (0x180D)
    BLERemoteService* hrService = bleClient->getService("180d");
    if (hrService) {
        heartRateChar = hrService->getCharacteristic("2a37");
        if (heartRateChar) {
            heartRateChar->registerForNotify([](BLERemoteCharacteristic* c, uint8_t* data, size_t len, bool) {
                if (len >= 2) {
                    sensors.heartRate = data[1];
                    if (data[0] & 0x01 && len >= 3) {
                        sensors.heartRate = data[1] | (data[2] << 8);
                    }
                }
            });
            heartRateChar->getDescriptor(BLEUUID((uint16_t)0x2902))->writeValue((uint8_t*)"\x01\x00", 2);
        }
    }

    bleClientConnected = true;
}

void disconnectHealbe() {
    if (bleClient) {
        if (bleClient->isConnected()) {
            bleClient->disconnect();
        }
        delete bleClient;
        bleClient = nullptr;
    }
    bleClientConnected = false;
    sensors.heartRate = 0;
    sensors.stressLevel = 0;
}

// ============================================================
// LoRa MESH (заглушки — нужна библиотека RadioLib или LoRa)
// ============================================================

void initLoRa() {
    log("Инициализация LoRa...");
    // LoRa.begin(PIN_LORA_CS, PIN_LORA_RST, PIN_LORA_DIO0);
    // LoRa.setFrequency(config.loraFrequency);
    // LoRa.setTxPower(config.loraTxPower);
    // LoRa.setSpreadingFactor(10);
    // LoRa.setSignalBandwidth(125E3);
    // LoRa.setCodingRate4(5);
    log("LoRa: заглушка (добавьте библиотеку RadioLib)");
}

void handleLoRa() {
    // int packetSize = LoRa.parsePacket();
    // if (packetSize) {
    //     String received = "";
    //     while (LoRa.available()) {
    //         received += (char)LoRa.read();
    //     }
    //     float snr = LoRa.packetSnr();
    //     int16_t rssi = LoRa.packetRssi();
    //     processLoRaMessage(received, snr, rssi);
    // }
}

void sendLoRaMessage(uint32_t to, const String& payload) {
    // LoRa.beginPacket();
    // LoRa.write(0xFF);  // Sync word
    // LoRa.write((uint8_t*)&to, 4);
    // LoRa.write((uint8_t*)&config.nodeNum, 4);
    // LoRa.print(payload);
    // LoRa.endPacket();
    log("LoRa TX: " + payload);
}

// ============================================================
// SERIAL USB (прямое подключение к mesh-server)
// ============================================================

void handleSerial() {
    while (Serial.available()) {
        char c = Serial.read();
        if (c == '\n') {
            processSerialMessage(serialBuffer);
            serialBuffer = "";
        } else {
            serialBuffer += c;
        }
    }
}

void processSerialMessage(String msg) {
    // Формат: COMMAND:PARAMS
    if (msg.startsWith("STATUS")) {
        sendSerialStatus();
    } else if (msg.startsWith("POS:")) {
        // Парсинг позиции от mesh-server
        parsePosition(msg.substring(4));
    } else if (msg.startsWith("MSG:")) {
        // Сообщение для отправки в mesh
        processOutboundMessage(msg.substring(4));
    } else if (msg.startsWith("CONFIG:")) {
        // Обновление конфигурации
        processConfigCommand(msg.substring(7));
    }
}

void sendSerialStatus() {
    StaticJsonDocument<256> doc;
    doc["type"] = "status";
    doc["node_id"] = "!" + String(config.nodeNum, HEX);
    doc["firmware"] = FIRMWARE_VERSION;
    doc["wifi"] = wifiConnected;
    doc["gps"] = sensors.gpsValid;
    doc["lat"] = sensors.latitude;
    doc["lon"] = sensors.longitude;
    doc["alt"] = sensors.altitude;
    doc["battery"] = sensors.batteryPercent;
    doc["uptime"] = millis() / 1000;

    String json;
    serializeJson(doc, json);
    Serial.println(json);
}

// ============================================================
// ОТПРАВКА ДАННЫХ НА СЕРВЕР
// ============================================================

void reportPosition() {
    if (!wifiConnected || strlen(config.serverURL) == 0) return;

    HTTPClient http;
    String url = String(config.serverURL) + "/api/esp32/hub/status";
    http.begin(url);
    http.addHeader("Content-Type", "application/json");

    StaticJsonDocument<512> doc;
    doc["node_id"] = "!" + String(config.nodeNum, HEX);
    doc["latitude"] = sensors.latitude;
    doc["longitude"] = sensors.longitude;
    doc["altitude"] = sensors.altitude;
    doc["gps_valid"] = sensors.gpsValid;
    doc["battery"] = sensors.batteryPercent;
    doc["free_heap"] = ESP.getFreeHeap();
    doc["uptime"] = millis() / 1000;
    doc["wifi_rssi"] = WiFi.RSSI();
    doc["mesh_nodes"] = (int)meshNodes.size();

    String json;
    serializeJson(doc, json);

    int code = http.POST(json);
    http.end();
}

void pollServer() {
    if (!wifiConnected || strlen(config.serverURL) == 0) return;

    HTTPClient http;
    String url = String(config.serverURL) + "/api/messages/device?device_id=1&limit=5";
    http.begin(url);

    int code = http.GET();
    if (code == 200) {
        String payload = http.getString();
        // Парсим и обрабатываем входящие сообщения
        StaticJsonDocument<2048> doc;
        if (!deserializeJson(doc, payload)) {
            JsonArray msgs = doc.as<JsonArray>();
            for (JsonObject msg : msgs) {
                if (msg.containsKey("text")) {
                    String text = msg["text"].as<String>();
                    String from = msg.containsKey("from_node") ? msg["from_node"].as<String>() : "";
                    // Отправляем через LoRa если нужно
                    if (config.loraEnabled && from.length() > 0) {
                        processOutboundMessage(text);
                    }
                }
            }
        }
    }
    http.end();
}

// ============================================================
// HEALBE BRIDGE
// ============================================================

void syncHealbe() {
    if (!wifiConnected || !healbeActive) return;

    // Если есть реальные данные от BLE — отправляем
    if (sensors.heartRate > 0) {
        HTTPClient http;
        String url = String(config.serverURL) + "/api/healbe/ingest";
        http.begin(url);
        http.addHeader("Content-Type", "application/json");

        StaticJsonDocument<256> doc;
        doc["device_id"] = healbeMac;
        doc["heart_rate"] = sensors.heartRate;
        doc["stress_level"] = sensors.stressLevel;
        doc["battery"] = sensors.batteryPercent;

        String json;
        serializeJson(doc, json);
        http.POST(json);
        http.end();
    }
}

// ============================================================
// OTA ОБНОВЛЕНИЕ
// ============================================================

void handleOtaStart() {
    if (!httpServer.hasArg("plain")) {
        httpServer.send(400, "application/json", "{\"error\":\"no body\"}");
        return;
    }
    httpServer.send(200, "application/json", "{\"success\":true}");
}

void handleOtaUpload() {
    HTTPUpload& upload = httpServer.upload();
    if (upload.status == UPLOAD_FILE_START) {
        Serial.printf("OTA: начало загрузки %s\n", upload.filename.c_str());
        Update.begin(UPDATE_SIZE_UNKNOWN);
    } else if (upload.status == UPLOAD_FILE_WRITE) {
        Update.write(upload.buf, upload.currentSize);
    } else if (upload.status == UPLOAD_FILE_END) {
        if (Update.end(true)) {
            Serial.printf("OTA: загружено %u байт\n", upload.totalSize);
        }
    }
}

void handleOtaComplete() {
    httpServer.send(200, "application/json", "{\"success\":true}");
    delay(1000);
    ESP.restart();
}

// ============================================================
// КОНФИГУРАЦИЯ
// ============================================================

void loadConfig() {
    // Значения по умолчанию
    memset(&config, 0, sizeof(config));
    strcpy(config.wifiSSID, "");
    strcpy(config.wifiPassword, "");
    strcpy(config.serverURL, "http://192.168.1.100:8080");
    strcpy(config.nodeName, "СтражСети-Hub");
    config.nodeNum = 0xdeadbeef;
    config.loraEnabled = false;
    config.loraFrequency = 868000000;  // EU 868 MHz
    config.loraTxPower = 14;
    config.gpsEnabled = false;
    config.bleEnabled = true;
    config.mqttEnabled = false;

    // TODO: Загрузка из EEPROM / Preferences
    log("Конфигурация загружена");
}

void saveConfig() {
    // TODO: Сохранение в EEPROM / Preferences
    log("Конфигурация сохранена");
}

void processConfigCommand(String jsonStr) {
    StaticJsonDocument<256> doc;
    if (deserializeJson(doc, jsonStr)) return;

    if (doc.containsKey("wifi_ssid")) strcpy(config.wifiSSID, doc["wifi_ssid"]);
    if (doc.containsKey("wifi_password")) strcpy(config.wifiPassword, doc["wifi_password"]);
    if (doc.containsKey("server_url")) strcpy(config.serverURL, doc["server_url"]);
    if (doc.containsKey("node_name")) strcpy(config.nodeName, doc["node_name"]);
    if (doc.containsKey("lora_enabled")) config.loraEnabled = doc["lora_enabled"];
    if (doc.containsKey("lora_frequency")) config.loraFrequency = doc["lora_frequency"];

    saveConfig();
    log("Конфигурация обновлена");
}

// ============================================================
// CALLBACK HANDLERS
// ============================================================

void handleHealbeConnect() {
    if (!httpServer.hasArg("plain")) {
        httpServer.send(400, "application/json", "{\"error\":\"no body\"}");
        return;
    }
    StaticJsonDocument<128> doc;
    deserializeJson(doc, httpServer.arg("plain"));
    healbeMac = doc["mac"] | "";
    healbeActive = healbeMac.length() > 0;

    httpServer.send(200, "application/json",
        "{\"success\":" + String(healbeActive ? "true" : "false") + "}");
}

void handleHealbeDisconnect() {
    healbeActive = false;
    healbeMac = "";
    disconnectHealbe();
    httpServer.send(200, "application/json", "{\"success\":true}");
}

void handleHealbeStatus() {
    StaticJsonDocument<256> doc;
    doc["connected"] = bleClientConnected;
    doc["mac"] = healbeMac;
    doc["heart_rate"] = sensors.heartRate;
    doc["stress_level"] = sensors.stressLevel;
    doc["battery"] = sensors.batteryPercent;

    String json;
    serializeJson(doc, json);
    httpServer.send(200, "application/json", json);
}

void handleHealbeData() {
    handleHealbeStatus();
}

void handleIncomingMessage() {
    if (!httpServer.hasArg("plain")) {
        httpServer.send(400, "application/json", "{\"error\":\"no body\"}");
        return;
    }
    StaticJsonDocument<512> doc;
    deserializeJson(doc, httpServer.arg("plain"));

    String toNode = doc["to_node"] | "";
    String text = doc["text"] | "";

    log("Входящее сообщение: " + text);

    // Отправляем через LoRa если подключен
    if (config.loraEnabled) {
        sendLoRaMessage(0xFFFFFFFF, text);  // Broadcast
    }

    httpServer.send(200, "application/json", "{\"success\":true}");
}

void handleConfigUpdate() {
    if (!httpServer.hasArg("plain")) {
        httpServer.send(400, "application/json", "{\"error\":\"no body\"}");
        return;
    }
    processConfigCommand(httpServer.arg("plain"));
    httpServer.send(200, "application/json", "{\"success\":true}");
}

void handleConfigGet() {
    StaticJsonDocument<256> doc;
    doc["wifi_ssid"] = config.wifiSSID;
    doc["wifi_password"] = config.wifiPassword;
    doc["server_url"] = config.serverURL;
    doc["node_name"] = config.nodeName;
    doc["lora_enabled"] = config.loraEnabled;
    doc["lora_frequency"] = config.loraFrequency;

    String json;
    serializeJson(doc, json);
    httpServer.send(200, "application/json", json);
}

// ============================================================
// УТИЛИТЫ
// ============================================================

void initGPIO() {
    pinMode(PIN_LED_STATUS, OUTPUT);
    digitalWrite(PIN_LED_STATUS, LOW);
}

void blinkLed(int times) {
    for (int i = 0; i < times; i++) {
        digitalWrite(PIN_LED_STATUS, HIGH);
        delay(100);
        digitalWrite(PIN_LED_STATUS, LOW);
        delay(100);
    }
}

void updateStatusLed() {
    static bool ledState = false;

    if (wifiConnected) {
        if (sensors.gpsValid) {
            // Зелёный = всё ОК
            digitalWrite(PIN_LED_STATUS, HIGH);
        } else {
            // Мигание = WiFi OK, нет GPS
            ledState = !ledState;
            digitalWrite(PIN_LED_STATUS, ledState ? HIGH : LOW);
        }
    } else {
        // Выключен = нет WiFi
        digitalWrite(PIN_LED_STATUS, LOW);
    }
}

String buildNodesJson() {
    StaticJsonDocument<2048> doc;
    doc["status"] = "ok";
    JsonObject data = doc.createNestedObject("data");
    JsonArray nodes = data.createNestedArray("nodes");

    // Добавляем себя
    JsonObject self = nodes.createNestedObject();
    self["id"] = "!" + String(config.nodeNum, HEX);
    self["long_name"] = DEVICE_NAME;
    self["short_name"] = DEVICE_SHORT_NAME;
    self["hw_model"] = HARDWARE_MODEL;
    self["last_heard"] = millis() / 1000;
    if (sensors.gpsValid) {
        JsonObject pos = self.createNestedObject("position");
        pos["latitude"] = sensors.latitude;
        pos["longitude"] = sensors.longitude;
        pos["altitude"] = sensors.altitude;
    }

    // Добавляем mesh-узлы
    for (const auto& node : meshNodes) {
        JsonObject n = nodes.createNestedObject();
        n["id"] = "!" + String(node.num, HEX);
        n["long_name"] = node.longName;
        n["short_name"] = node.shortName;
        n["snr"] = node.snr;
        n["last_heard"] = node.lastHeard / 1000;
        if (node.hasPosition) {
            JsonObject pos = n.createNestedObject("position");
            pos["latitude"] = node.latitude;
            pos["longitude"] = node.longitude;
            pos["altitude"] = node.altitude;
        }
    }

    String json;
    serializeJson(doc, json);
    return json;
}

void log(String msg) {
    Serial.println("[СтражСети] " + msg);
}

void printBanner() {
    Serial.println();
    Serial.println("╔══════════════════════════════════════╗");
    Serial.println("║       🛡️ СтражСети Hub v" + String(FIRMWARE_VERSION) + "       ║");
    Serial.println("║  Mesh-мониторинг и трекинг          ║");
    Serial.println("╚══════════════════════════════════════╝");
    Serial.println();
}

void processOutboundMessage(String data) {
    // Формат: TO_NODE|TEXT
    int pipeIdx = data.indexOf('|');
    if (pipeIdx < 0) return;

    String toNode = data.substring(0, pipeIdx);
    String text = data.substring(pipeIdx + 1);

    // Отправляем через LoRa
    if (config.loraEnabled) {
        uint32_t toNum = 0xFFFFFFFF;
        if (toNode != "broadcast" && toNode != "^all") {
            toNum = strtoul(toNode.c_str(), nullptr, 16);
        }
        sendLoRaMessage(toNum, text);
    }

    // Отправляем на сервер
    if (wifiConnected && strlen(config.serverURL) > 0) {
        HTTPClient http;
        String url = String(config.serverURL) + "/api/messages";
        http.begin(url);
        http.addHeader("Content-Type", "application/json");

        StaticJsonDocument<256> doc;
        doc["device_id"] = 1;
        doc["from_node"] = "!" + String(config.nodeNum, HEX);
        doc["to_node"] = toNode;
        doc["text"] = text;
        doc["direction"] = "outbound";

        String json;
        serializeJson(doc, json);
        http.POST(json);
        http.end();
    }
}

void parsePosition(String data) {
    // Формат: LAT,LON,ALT
    int comma1 = data.indexOf(',');
    int comma2 = data.indexOf(',', comma1 + 1);
    if (comma1 < 0 || comma2 < 0) return;

    sensors.latitude = data.substring(0, comma1).toFloat();
    sensors.longitude = data.substring(comma1 + 1, comma2).toFloat();
    sensors.altitude = data.substring(comma2 + 1).toInt();
    sensors.gpsValid = true;
}
