/*
 * Meshtastic Firmware для Raspberry Pi Pico W
 * 
 * Эта прошивка превращает Pico W в Meshtastic устройство
 * с поддержкой WiFi подключения к mesh-server
 * 
 * Автор: mesh-server project
 * Лицензия: MIT
 */

#include <WiFi.h>
#include <HTTPClient.h>
#include <ArduinoJson.h>
#include <Wire.h>

// ==================== НАСТРОЙКИ WiFi ====================
const char* ssid = "Sasha+Nastya";        // Измените на ваш SSID
const char* password = "NastyaForever"; // Измените на ваш пароль

// ==================== НАСТРОЙКИ СЕРВЕРА ====================
const char* serverUrl = "http://192.168.1.100:8080"; // Адрес mesh-server
const int deviceId = 1;
const char* nodeId = "!12345678";  // Node ID вашего устройства

// ==================== НАСТРОЙКИ LoRa (опционально) ====================
#define LORA_CS_PIN 17
#define LORA_RST_PIN 20
#define LORA_DIO0_PIN 21
#define LORA_FREQUENCY 868.0  // MHz (Европа)
#define LORA_TX_POWER 17      // dBm

// ==================== НАСТРОЙКИ OLED (опционально) ====================
#define OLED_SDA_PIN 4
#define OLED_SCL_PIN 5
#define OLED_ADDRESS 0x3C

// ==================== ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ ====================
WiFiClient client;
unsigned long lastMessageCheck = 0;
const unsigned long messageCheckInterval = 5000; // Проверка каждые 5 секунд
bool wifiConnected = false;
bool oledInitialized = false;

// ==================== ФУНКЦИИ ====================

void setup() {
    Serial.begin(115200);
    delay(2000);
    
    Serial.println("\n\nMeshtastic Pico W");
    Serial.println("=================");
    
    // Инициализация I2C для OLED
    Wire.setSDA(OLED_SDA_PIN);
    Wire.setSCL(OLED_SCL_PIN);
    Wire.begin();
    
    // Инициализация OLED дисплея
    if (initOLED()) {
        oledInitialized = true;
        displayMessage("Meshtastic", "Pico W");
    }
    
    // Подключение к WiFi
    connectToWiFi();
    
    // Инициализация LoRa (если подключен модуль)
    // initLoRa();
    
    Serial.println("\nГотов к работе!");
    
    if (oledInitialized) {
        displayMessage("WiFi:", wifiConnected ? "OK" : "FAIL");
    }
}

void loop() {
    // Обработка OTA обновлений (если включено)
    // ArduinoOTA.handle();
    
    // Проверка WiFi подключения
    if (WiFi.status() != WL_CONNECTED && wifiConnected) {
        Serial.println("WiFi disconnected, reconnecting...");
        connectToWiFi();
    }
    
    // Проверка новых сообщений с сервера
    if (millis() - lastMessageCheck > messageCheckInterval) {
        lastMessageCheck = millis();
        checkNewMessages();
    }
    
    // Обработка входящих LoRa сообщений
    // handleLoRaMessages();
    
    delay(100);
}

// ==================== WiFi ФУНКЦИИ ====================

void connectToWiFi() {
    Serial.print("Подключение к WiFi: ");
    Serial.println(ssid);
    
    if (oledInitialized) {
        displayMessage("Connecting...", ssid);
    }
    
    WiFi.begin(ssid, password);
    
    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 30) {
        delay(500);
        Serial.print(".");
        attempts++;
    }
    
    if (WiFi.status() == WL_CONNECTED) {
        wifiConnected = true;
        Serial.println("\nWiFi подключен!");
        Serial.print("IP адрес: ");
        Serial.println(WiFi.localIP());

        if (oledInitialized) {
            displayMessage("IP:", WiFi.localIP().toString().c_str());
        }
    } else {
        wifiConnected = false;
        Serial.println("\nНе удалось подключиться к WiFi");
        
        if (oledInitialized) {
            displayMessage("WiFi", "Failed!");
        }
    }
}

// ==================== HTTP ФУНКЦИИ ====================

// Отправка сообщения на сервер
bool sendMessageToServer(const char* toNode, const char* text) {
    if (!wifiConnected) {
        Serial.println("WiFi не подключен");
        return false;
    }
    
    HTTPClient http;
    String url = String(serverUrl) + "/api/messages";
    http.begin(url);
    http.addHeader("Content-Type", "application/json");
    
    // Формирование JSON payload
    JsonDocument doc;
    doc["device_id"] = deviceId;
    doc["from_node"] = nodeId;
    doc["to_node"] = toNode;
    doc["text"] = text;
    doc["direction"] = "outbound";
    
    String jsonPayload;
    serializeJson(doc, jsonPayload);
    
    Serial.print("Отправка сообщения: ");
    Serial.println(jsonPayload);
    
    int httpResponseCode = http.POST(jsonPayload);
    
    if (httpResponseCode > 0) {
        Serial.print("Ответ сервера: ");
        Serial.println(httpResponseCode);
        
        if (httpResponseCode == 200 || httpResponseCode == 201) {
            if (oledInitialized) {
                displayMessage("Sent!", toNode);
            }
            http.end();
            return true;
        }
    } else {
        Serial.print("Ошибка отправки: ");
        Serial.println(httpResponseCode);
    }
    
    http.end();
    return false;
}

// Получение новых сообщений с сервера
void checkNewMessages() {
    if (!wifiConnected) {
        return;
    }
    
    HTTPClient http;
    String url = String(serverUrl) + "/api/messages/device?device_id=" + String(deviceId) + "&limit=10";
    http.begin(url);
    
    int httpResponseCode = http.GET();
    
    if (httpResponseCode > 0) {
        String payload = http.getString();
        
        if (httpResponseCode == 200) {
            parseMessages(payload);
        }
    } else {
        Serial.print("Ошибка получения сообщений: ");
        Serial.println(httpResponseCode);
    }
    
    http.end();
}

// Парсинг полученных сообщений
void parseMessages(const String& json) {
    JsonDocument doc;
    DeserializationError error = deserializeJson(doc, json);
    
    if (error) {
        Serial.print("Ошибка парсинга JSON: ");
        Serial.println(error.c_str());
        return;
    }
    
    JsonArray messages = doc.as<JsonArray>();
    
    for (JsonObject msg : messages) {
        const char* fromNode = msg["from_node"];
        const char* text = msg["text"];
        const char* direction = msg["direction"];
        
        // Обрабатываем только входящие сообщения
        if (strcmp(direction, "inbound") == 0) {
            Serial.print("Входящее от ");
            Serial.print(fromNode);
            Serial.print(": ");
            Serial.println(text);
            
            if (oledInitialized) {
                displayMessage(fromNode, text);
            }
            
            // Здесь можно добавить обработку команд
            processCommand(fromNode, text);
        }
    }
}

// Проверка подключения к серверу
bool checkServerConnection() {
    if (!wifiConnected) {
        return false;
    }
    
    HTTPClient http;
    String url = String(serverUrl) + "/health";
    http.begin(url);
    
    int httpResponseCode = http.GET();
    bool reachable = (httpResponseCode == 200);
    
    http.end();
    return reachable;
}

// ==================== OLED ФУНКЦИИ ====================

bool initOLED() {
    // Простая инициализация OLED
    // Для полноценной работы используйте библиотеку Adafruit_SSD1306
    
    Wire.beginTransmission(OLED_ADDRESS);
    bool success = (Wire.endTransmission() == 0);
    
    if (success) {
        Serial.println("OLED найден");
        // Здесь должна быть полноценная инициализация дисплея
    } else {
        Serial.println("OLED не найден");
    }
    
    return success;
}

void displayMessage(const char* line1, const char* line2) {
    if (!oledInitialized) {
        return;
    }
    
    // Упрощенная реализация
    // Для полноценной работы используйте библиотеку дисплея
    
    Serial.print("[OLED] ");
    Serial.print(line1);
    Serial.print(": ");
    Serial.println(line2);
}

// ==================== LoRa ФУНКЦИИ (опционально) ====================

/*
void initLoRa() {
    // Инициализация LoRa модуля
    // Используйте библиотеку RadioLib или аналогичную
    
    Serial.println("Инициализация LoRa...");
    
    // Здесь код инициализации LoRa
    // В зависимости от используемой библиотеки
}

void handleLoRaMessages() {
    // Обработка входящих LoRa сообщений
    
    if (LoRa.available()) {
        String message = LoRa.readString();
        Serial.print("LoRa сообщение: ");
        Serial.println(message);
        
        // Отправка на сервер
        sendMessageToServer("!broadcast", message.c_str());
    }
}

void sendViaLoRa(const char* toNode, const char* text) {
    // Отправка сообщения через LoRa
    
    Serial.print("LoRa отправка: ");
    Serial.println(text);
    
    // Здесь код отправки через LoRa
}
*/

// ==================== УТИЛИТЫ ====================

void processCommand(const char* fromNode, const char* text) {
    // Обработка команд от других устройств
    
    if (strcmp(text, "ping") == 0) {
        sendMessageToServer(fromNode, "pong");
    }
    
    if (strcmp(text, "status") == 0) {
        String status = "WiFi: " + String(wifiConnected ? "OK" : "FAIL");
        sendMessageToServer(fromNode, status.c_str());
    }
}

// Перезагрузка устройства
void rebootDevice() {
    Serial.println("Перезагрузка...");
    rp2040.reboot();  // Для Raspberry Pi Pico
}

// Глубокий сон
void enterDeepSleep(uint32_t seconds) {
    Serial.println("Переход в глубокий сон...");
    
    // Отключение WiFi
    WiFi.disconnect(true);
    
    // Для Pico W:
    // pico_enter_deep_sleep(seconds * 1000);
    
    delay(seconds * 1000);
}
