// Mesh Server ESP32 Firmware
// Прошивка для ESP32 с поддержкой сканирования WiFi/BLE и подключения к mesh-server
//
// Возможности:
// - Подключение к WiFi сети
// - Сканирование WiFi сетей и отправка на сервер
// - Сканирование BLE устройств и отправка на сервер
// - Обмен сообщениями с сервером
// - Health check endpoint

#include <WiFi.h>
#include <HTTPClient.h>
#include <ArduinoJson.h>
#include <WebServer.h>
#include <BLEDevice.h>
#include <BLEUtils.h>
#include <BLEScan.h>
#include <BLEAdvertisedDevice.h>

// ==================== НАСТРОЙКИ WiFi ====================
// Можно настроить через web-интерфейс или оставить по умолчанию
const char* DEFAULT_SSID = "YOUR_WIFI_SSID";
const char* DEFAULT_PASSWORD = "YOUR_WIFI_PASSWORD";

// ==================== НАСТРОЙКИ СЕРВЕРА ====================
const char* DEFAULT_SERVER_URL = "http://192.168.1.100:8080";

// ==================== ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ ====================
String currentSSID = "";
String currentPassword = "";
String serverUrl = DEFAULT_SERVER_URL;

WebServer server(80);
BLEScan* pBLEScan;

unsigned long lastWiFiScan = 0;
unsigned long lastBLEScan = 0;
unsigned long lastServerCheck = 0;

const unsigned long wifiScanInterval = 30000;    // Сканирование WiFi каждые 30 секунд
const unsigned long bleScanInterval = 10000;     // Сканирование BLE каждые 10 секунд
const unsigned long serverCheckInterval = 5000;  // Проверка сервера каждые 5 секунд

bool wifiScanning = false;
bool bleScanning = false;

// Результаты сканирования
std::vector<String> wifiNetworks;
std::vector<String> bleDevices;

// ==================== BLE SCAN CALLBACK ====================
class BLEAdvertisedDeviceCallbacks : public BLEAdvertisedDeviceCallbacks {
    void onResult(BLEAdvertisedDevice advertisedDevice) {
      String deviceInfo = String(advertisedDevice.getAddress().toString().c_str()) + "|" + 
                         String(advertisedDevice.getName().c_str()) + "|" + 
                         String(advertisedDevice.getRSSI());
      
      // Проверяем, не дубликат ли это
      bool duplicate = false;
      for (const auto& dev : bleDevices) {
        if (dev.startsWith(advertisedDevice.getAddress().toString().c_str())) {
          duplicate = true;
          break;
        }
      }
      
      if (!duplicate) {
        bleDevices.push_back(deviceInfo);
        Serial.printf("Найдено BLE устройство: %s, RSSI: %d\n", 
                     advertisedDevice.getName().c_str(), 
                     advertisedDevice.getRSSI());
      }
    }
};

// ==================== ФУНКЦИИ ====================

void setup() {
    Serial.begin(115200);
    delay(1000);

    Serial.println("\n\nMesh Server ESP32 Bridge");
    Serial.println("========================");

    // Инициализация WiFi
    connectToWiFi(DEFAULT_SSID, DEFAULT_PASSWORD);

    // Инициализация BLE
    initBLE();

    // Настройка HTTP сервера
    setupWebServer();

    // Запуск сервера
    server.begin();

    Serial.println("\nГотов к работе!");
    Serial.print("HTTP сервер: http://");
    Serial.println(WiFi.localIP());
    Serial.print("API сервер: ");
    Serial.println(serverUrl);
}

void loop() {
    // Обработка HTTP запросов
    server.handleClient();

    // Проверка подключения к WiFi
    if (WiFi.status() != WL_CONNECTED) {
        Serial.println("WiFi отключен, переподключение...");
        connectToWiFi(currentSSID.length() > 0 ? currentSSID.c_str() : DEFAULT_SSID, 
                     currentPassword.length() > 0 ? currentPassword.c_str() : DEFAULT_PASSWORD);
    }

    // Сканирование WiFi
    if (millis() - lastWiFiScan > wifiScanInterval && !wifiScanning) {
        lastWiFiScan = millis();
        scanWiFiNetworks();
    }

    // Сканирование BLE
    if (millis() - lastBLEScan > bleScanInterval && !bleScanning) {
        lastBLEScan = millis();
        scanBLEDevices();
    }

    // Отправка данных на сервер
    if (millis() - lastServerCheck > serverCheckInterval) {
        lastServerCheck = millis();
        sendScanResultsToServer();
    }

    delay(100);
}

// ==================== WiFi ФУНКЦИИ ====================

void connectToWiFi(const char* ssid, const char* password) {
    currentSSID = String(ssid);
    currentPassword = String(password);

    Serial.print("Подключение к WiFi: ");
    Serial.println(ssid);

    WiFi.begin(ssid, password);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 30) {
        delay(500);
        Serial.print(".");
        attempts++;
    }

    if (WiFi.status() == WL_CONNECTED) {
        Serial.println("\nWiFi подключен!");
        Serial.print("IP адрес: ");
        Serial.println(WiFi.localIP());
        Serial.print("Шлюз: ");
        Serial.println(WiFi.gatewayIP());
        Serial.print("Маска: ");
        Serial.println(WiFi.subnetMask());
    } else {
        Serial.println("\nНе удалось подключиться к WiFi");
        // Создаем точку доступа для настройки
        setupAccessPoint();
    }
}

void setupAccessPoint() {
    Serial.println("Создание точки доступа для настройки...");
    WiFi.softAP("ESP32-MeshServer", "meshserver");
    Serial.print("Точка доступа создана: http://");
    Serial.println(WiFi.softAPIP());
}

void scanWiFiNetworks() {
    if (wifiScanning) return;
    
    wifiScanning = true;
    wifiNetworks.clear();
    
    Serial.println("\n=== Сканирование WiFi сетей ===");
    
    int n = WiFi.scanNetworks();
    
    if (n == 0) {
        Serial.println("WiFi сети не найдены");
    } else {
        Serial.printf("Найдено WiFi сетей: %d\n", n);
        
        for (int i = 0; i < n; ++i) {
            String networkInfo = String(WiFi.SSID(i)) + "|" + 
                                String(WiFi.BSSIDstr(i)) + "|" + 
                                String(WiFi.RSSI(i)) + "|" + 
                                String(WiFi.channel(i)) + "|" +
                                (WiFi.encryptionType(i) == WIFI_AUTH_OPEN ? "open" : "secured");
            wifiNetworks.push_back(networkInfo);
            
            Serial.printf("  %d: %s (%s) RSSI: %d, Channel: %d, Security: %s\n", 
                         i + 1,
                         WiFi.SSID(i).c_str(),
                         WiFi.BSSIDstr(i).c_str(),
                         WiFi.RSSI(i),
                         WiFi.channel(i),
                         WiFi.encryptionType(i) == WIFI_AUTH_OPEN ? "Open" : "Secured");
        }
    }
    
    wifiScanning = false;
}

// ==================== BLE ФУНКЦИИ ====================

void initBLE() {
    Serial.println("Инициализация BLE...");
    BLEDevice::init("ESP32-MeshServer");
    pBLEScan = BLEDevice::getScan();
    pBLEScan->setAdvertisedDeviceCallbacks(new BLEAdvertisedDeviceCallbacks());
    pBLEScan->setInterval(100);
    pBLEScan->setWindow(99);
    pBLEScan->setActiveScan(true);
    Serial.println("BLE инициализирован");
}

void scanBLEDevices() {
    if (bleScanning) return;
    
    bleScanning = true;
    bleDevices.clear();
    
    Serial.println("\n=== Сканирование BLE устройств ===");
    
    BLEScanResults found = pBLEScan->start(5, false);
    
    Serial.printf("Найдено BLE устройств: %d\n", found.getCount());
    
    for (int i = 0; i < found.getCount(); i++) {
        BLEAdvertisedDevice device = found.getDevice(i);
        String deviceInfo = String(device.getAddress().toString().c_str()) + "|" + 
                           String(device.getName().c_str()) + "|" + 
                           String(device.getRSSI());
        bleDevices.push_back(deviceInfo);
        
        Serial.printf("  %d: %s (%s) RSSI: %d\n", 
                     i + 1,
                     device.getName().c_str(),
                     device.getAddress().toString().c_str(),
                     device.getRSSI());
    }
    
    bleScanning = false;
}

// ==================== HTTP СЕРВЕР (API для mesh-server) ====================

void setupWebServer() {
    // Health check
    server.on("/health", HTTP_GET, []() {
        server.send(200, "text/plain", "OK");
    });

    // Статус устройства
    server.on("/api/status", HTTP_GET, []() {
        StaticJsonDocument<512> doc;
        doc["wifi_connected"] = WiFi.status() == WL_CONNECTED;
        doc["wifi_ssid"] = currentSSID.length() > 0 ? currentSSID : String(DEFAULT_SSID);
        doc["ip_address"] = WiFi.status() == WL_CONNECTED ? WiFi.localIP().toString() : "N/A";
        doc["server_url"] = serverUrl;
        doc["wifi_scanning"] = wifiScanning;
        doc["ble_scanning"] = bleScanning;
        doc["wifi_networks_count"] = (int)wifiNetworks.size();
        doc["ble_devices_count"] = (int)bleDevices.size();
        doc["uptime"] = millis() / 1000;
        
        String json;
        serializeJson(doc, json);
        server.send(200, "application/json", json);
    });

    // Сканирование WiFi
    server.on("/api/wifi/scan", HTTP_GET, []() {
        if (!wifiScanning) {
            scanWiFiNetworks();
        }
        
        StaticJsonDocument<1024> doc;
        JsonArray networks = doc.createNestedArray("networks");
        
        for (const auto& net : wifiNetworks) {
            String ssid, bssid, rssi, channel, security;
            parseWiFiNetwork(net, ssid, bssid, rssi, channel, security);
            
            JsonObject networkObj = networks.createNestedObject();
            networkObj["ssid"] = ssid;
            networkObj["bssid"] = bssid;
            networkObj["rssi"] = rssi.toInt();
            networkObj["channel"] = channel.toInt();
            networkObj["security"] = security;
        }
        doc["count"] = (int)wifiNetworks.size();
        
        String json;
        serializeJson(doc, json);
        server.send(200, "application/json", json);
    });

    // Сканирование BLE
    server.on("/api/ble/scan", HTTP_GET, []() {
        if (!bleScanning) {
            scanBLEDevices();
        }
        
        StaticJsonDocument<1024> doc;
        JsonArray devices = doc.createNestedArray("devices");
        
        for (const auto& dev : bleDevices) {
            String address, name, rssi;
            parseBLEDevice(dev, address, name, rssi);
            
            JsonObject deviceObj = devices.createNestedObject();
            deviceObj["address"] = address;
            deviceObj["name"] = name;
            deviceObj["rssi"] = rssi.toInt();
            deviceObj["is_esp32"] = name.indexOf("ESP32") >= 0 || name.indexOf("Meshtastic") >= 0;
        }
        doc["count"] = (int)bleDevices.size();
        
        String json;
        serializeJson(doc, json);
        server.send(200, "application/json", json);
    });

    // Получение всех результатов сканирования
    server.on("/api/scan", HTTP_GET, []() {
        StaticJsonDocument<2048> doc;
        
        // WiFi сети
        JsonArray wifiArray = doc.createNestedArray("wifi");
        for (const auto& net : wifiNetworks) {
            String ssid, bssid, rssi, channel, security;
            parseWiFiNetwork(net, ssid, bssid, rssi, channel, security);
            
            JsonObject netObj = wifiArray.createNestedObject();
            netObj["ssid"] = ssid;
            netObj["bssid"] = bssid;
            netObj["rssi"] = rssi.toInt();
            netObj["channel"] = channel.toInt();
            netObj["security"] = security;
        }
        
        // BLE устройства
        JsonArray bleArray = doc.createNestedArray("ble");
        for (const auto& dev : bleDevices) {
            String address, name, rssi;
            parseBLEDevice(dev, address, name, rssi);
            
            JsonObject devObj = bleArray.createNestedObject();
            devObj["address"] = address;
            devObj["name"] = name;
            devObj["rssi"] = rssi.toInt();
            devObj["is_esp32"] = name.indexOf("ESP32") >= 0 || name.indexOf("Meshtastic") >= 0;
        }
        
        String json;
        serializeJson(doc, json);
        server.send(200, "application/json", json);
    });

    // Обновление настроек сервера
    server.on("/api/config/server", HTTP_POST, []() {
        if (server.hasArg("plain")) {
            StaticJsonDocument<256> doc;
            DeserializationError error = deserializeJson(doc, server.arg("plain"));
            
            if (error) {
                server.send(400, "application/json", "{\"error\":\"Invalid JSON\"}");
                return;
            }
            
            if (doc.containsKey("server_url")) {
                serverUrl = String(doc["server_url"].as<const char*>());
                Serial.print("Server URL обновлен: ");
                Serial.println(serverUrl);
            }
            
            server.send(200, "application/json", "{\"success\":true}");
        }
    });

    // Подключение к новой WiFi сети
    server.on("/api/wifi/connect", HTTP_POST, []() {
        if (server.hasArg("plain")) {
            StaticJsonDocument<256> doc;
            DeserializationError error = deserializeJson(doc, server.arg("plain"));
            
            if (error) {
                server.send(400, "application/json", "{\"error\":\"Invalid JSON\"}");
                return;
            }
            
            String ssid = doc.containsKey("ssid") ? String(doc["ssid"].as<const char*>()) : String(DEFAULT_SSID);
            String password = doc.containsKey("password") ? String(doc["password"].as<const char*>()) : String(DEFAULT_PASSWORD);
            
            Serial.print("Подключение к новой сети: ");
            Serial.println(ssid);
            
            WiFi.disconnect();
            delay(100);
            connectToWiFi(ssid.c_str(), password.c_str());
            
            StaticJsonDocument<256> response;
            response["success"] = WiFi.status() == WL_CONNECTED;
            response["ip"] = WiFi.status() == WL_CONNECTED ? WiFi.localIP().toString() : "N/A";
            
            String json;
            serializeJson(response, json);
            server.send(200, "application/json", json);
        }
    });

    // Обработка сообщений от сервера
    server.on("/api/message", HTTP_POST, []() {
        if (server.hasArg("plain")) {
            StaticJsonDocument<512> doc;
            DeserializationError error = deserializeJson(doc, server.arg("plain"));
            
            if (error) {
                server.send(400, "application/json", "{\"error\":\"Invalid JSON\"}");
                return;
            }
            
            String toNode = doc.containsKey("to_node") ? String(doc["to_node"].as<const char*>()) : "";
            String text = doc.containsKey("text") ? String(doc["text"].as<const char*>()) : "";
            
            Serial.print("Получено сообщение для ");
            Serial.print(toNode);
            Serial.print(": ");
            Serial.println(text);
            
            // Здесь должна быть логика отправки через LoRa
            // Для sekarang просто подтверждаем получение
            
            StaticJsonDocument<256> response;
            response["success"] = true;
            response["message"] = "Message received";
            
            String json;
            serializeJson(response, json);
            server.send(200, "application/json", json);
        }
    });

    // Главная страница
    server.on("/", HTTP_GET, []() {
        String html = "<!DOCTYPE html><html><head><title>ESP32 Mesh Server</title>";
        html += "<meta http-equiv='refresh' content='10'>";
        html += "<style>body{font-family:Arial;margin:20px;} h1{color:#333;} .status{padding:10px;margin:10px 0;border-radius:5px;} .ok{background:#d4edda;} .error{background:#f8d7da;} table{border-collapse:collapse;width:100%;} th,td{border:1px solid #ddd;padding:8px;} th{background:#333;color:white;}</style>";
        html += "</head><body>";
        html += "<h1>ESP32 Mesh Server Bridge</h1>";
        
        html += "<div class='status ";
        html += WiFi.status() == WL_CONNECTED ? "ok'>WiFi: Подключен" : "error'>WiFi: Отключен";
        html += "</div>";
        
        if (WiFi.status() == WL_CONNECTED) {
            html += "<p><strong>IP:</strong> " + WiFi.localIP().toString() + "</p>";
            html += "<p><strong>SSID:</strong> " + (currentSSID.length() > 0 ? currentSSID : String(DEFAULT_SSID)) + "</p>";
            html += "<p><strong>Server:</strong> " + serverUrl + "</p>";
        }
        
        html += "<h2>Результаты сканирования</h2>";
        html += "<h3>WiFi сети: " + String(wifiNetworks.size()) + "</h3>";
        html += "<table><tr><th>SSID</th><th>BSSID</th><th>RSSI</th><th>Channel</th><th>Security</th></tr>";
        for (const auto& net : wifiNetworks) {
            String ssid, bssid, rssi, channel, security;
            parseWiFiNetwork(net, ssid, bssid, rssi, channel, security);
            html += "<tr><td>" + ssid + "</td><td>" + bssid + "</td><td>" + rssi + "</td><td>" + channel + "</td><td>" + security + "</td></tr>";
        }
        html += "</table>";
        
        html += "<h3>BLE устройства: " + String(bleDevices.size()) + "</h3>";
        html += "<table><tr><th>Name</th><th>Address</th><th>RSSI</th></tr>";
        for (const auto& dev : bleDevices) {
            String address, name, rssi;
            parseBLEDevice(dev, address, name, rssi);
            html += "<tr><td>" + name + "</td><td>" + address + "</td><td>" + rssi + "</td></tr>";
        }
        html += "</table>";
        
        html += "<h2>Управление</h2>";
        html += "<p><a href='/api/wifi/scan'>Сканировать WiFi</a> | <a href='/api/ble/scan'>Сканировать BLE</a> | <a href='/api/status'>Статус API</a></p>";
        
        html += "</body></html>";
        
        server.send(200, "text/html", html);
    });

    // 404
    server.onNotFound([]() {
        server.send(404, "text/plain", "Not Found");
    });
}

// ==================== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ====================

void parseWiFiNetwork(const String& data, String& ssid, String& bssid, String& rssi, String& channel, String& security) {
    int firstPipe = data.indexOf('|');
    int secondPipe = data.indexOf('|', firstPipe + 1);
    int thirdPipe = data.indexOf('|', secondPipe + 1);
    int fourthPipe = data.indexOf('|', thirdPipe + 1);
    
    ssid = data.substring(0, firstPipe);
    bssid = data.substring(firstPipe + 1, secondPipe);
    rssi = data.substring(secondPipe + 1, thirdPipe);
    channel = data.substring(thirdPipe + 1, fourthPipe);
    security = data.substring(fourthPipe + 1);
}

void parseBLEDevice(const String& data, String& address, String& name, String& rssi) {
    int firstPipe = data.indexOf('|');
    int secondPipe = data.indexOf('|', firstPipe + 1);
    
    address = data.substring(0, firstPipe);
    name = data.substring(firstPipe + 1, secondPipe);
    rssi = data.substring(secondPipe + 1);
}

// ==================== ОТПРАВКА ДАННЫХ НА СЕРВЕР ====================

void sendScanResultsToServer() {
    if (WiFi.status() != WL_CONNECTED) {
        return;
    }

    if (serverUrl.length() == 0 || serverUrl == "") {
        return;
    }

    HTTPClient http;
    
    // Отправляем WiFi сети
    if (wifiNetworks.size() > 0) {
        StaticJsonDocument<2048> doc;
        JsonArray networks = doc.createNestedArray("networks");
        
        for (const auto& net : wifiNetworks) {
            String ssid, bssid, rssi, channel, security;
            parseWiFiNetwork(net, ssid, bssid, rssi, channel, security);
            
            JsonObject netObj = networks.createNestedObject();
            netObj["ssid"] = ssid;
            netObj["bssid"] = bssid;
            netObj["rssi"] = rssi.toInt();
            netObj["channel"] = channel.toInt();
            netObj["security"] = security;
            netObj["type"] = "wifi";
        }
        doc["count"] = (int)wifiNetworks.size();
        doc["device_type"] = "esp32";
        
        String jsonPayload;
        serializeJson(doc, jsonPayload);
        
        String url = serverUrl + "/api/discovery/wifi";
        http.begin(url);
        http.addHeader("Content-Type", "application/json");
        
        int httpResponseCode = http.POST(jsonPayload);
        if (httpResponseCode > 0) {
            Serial.printf("WiFi сети отправлены: %d\n", httpResponseCode);
        } else {
            Serial.printf("Ошибка отправки WiFi: %d\n", httpResponseCode);
        }
        http.end();
    }
    
    // Отправляем BLE устройства
    if (bleDevices.size() > 0) {
        StaticJsonDocument<2048> doc;
        JsonArray devices = doc.createNestedArray("devices");
        
        for (const auto& dev : bleDevices) {
            String address, name, rssi;
            parseBLEDevice(dev, address, name, rssi);
            
            JsonObject devObj = devices.createNestedObject();
            devObj["address"] = address;
            devObj["name"] = name;
            devObj["rssi"] = rssi.toInt();
            devObj["type"] = "bluetooth";
            devObj["is_esp32"] = name.indexOf("ESP32") >= 0 || name.indexOf("Meshtastic") >= 0;
        }
        doc["count"] = (int)bleDevices.size();
        doc["device_type"] = "esp32";
        
        String jsonPayload;
        serializeJson(doc, jsonPayload);
        
        String url = serverUrl + "/api/discovery/bluetooth";
        http.begin(url);
        http.addHeader("Content-Type", "application/json");
        
        int httpResponseCode = http.POST(jsonPayload);
        if (httpResponseCode > 0) {
            Serial.printf("BLE устройства отправлены: %d\n", httpResponseCode);
        } else {
            Serial.printf("Ошибка отправки BLE: %d\n", httpResponseCode);
        }
        http.end();
    }
}

// ==================== LoRa ФУНКЦИИ (заглушки для будущей реализации) ====================

void initMeshtastic() {
    Serial.println("Инициализация Meshtastic (LoRa)...");
    // Здесь будет инициализация LoRa модуля
    // Для ESP32 с SX1276/RFM95:
    // - SPI интерфейс
    // - Частота 868 MHz (EU) или 915 MHz (US)
    // - Мощность передачи
}

void handleLoRaMessages() {
    // Обработка входящих LoRa сообщений
    // При получении - отправка на сервер
}

void sendMessageToLoRa(String toNode, String text) {
    Serial.print("Отправка сообщения LoRa на ");
    Serial.print(toNode);
    Serial.print(": ");
    Serial.println(text);
    
    // Здесь код отправки через LoRa
}
