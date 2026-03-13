// Meshtastic ESP32 Firmware
// Прошивка для ESP32 с поддержкой Meshtastic
// 
// Используемые библиотеки:
// - Meshtastic (официальная прошивка)
// - Или кастомная прошивка с HTTP API
//
// Инструкция по установке:
// 1. Установите Arduino IDE или PlatformIO
// 2. Установите библиотеку Meshtastic
// 3. Загрузите прошивку на ESP32
// 4. Настройте WiFi подключение
// 5. Подключите телефон через Bluetooth

#include <WiFi.h>
#include <HTTPClient.h>
#include <ArduinoJson.h>
#include <Meshtastic.h>

// ==================== НАСТРОЙКИ WiFi ====================
const char* ssid = "YOUR_WIFI_SSID";
const char* password = "YOUR_WIFI_PASSWORD";

// ==================== НАСТРОЙКИ СЕРВЕРА ====================
const char* serverUrl = "http://192.168.1.100:8080"; // Адрес вашего mesh-server

// ==================== НАСТРОЙКИ MESHTASTIC ====================
#define LORA_FREQUENCY 868.0  // Частота LoRa (868 MHz для Европы)
#define LORA_TX_POWER 17      // Мощность передачи (dBm)

// ==================== ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ ====================
WiFiClient client;
unsigned long lastMessageCheck = 0;
const unsigned long messageCheckInterval = 5000; // Проверка сообщений каждые 5 секунд

// ==================== ФУНКЦИИ ====================

void setup() {
    Serial.begin(115200);
    delay(1000);
    
    Serial.println("\n\nMeshtastic ESP32 Bridge");
    Serial.println("======================");
    
    // Инициализация WiFi
    connectToWiFi();
    
    // Инициализация Meshtastic
    initMeshtastic();
    
    Serial.println("\nГотов к работе!");
    Serial.println("Откройте http://<IP-ESP32>/api/message для отправки сообщений");
}

void loop() {
    // Проверка новых сообщений с сервера
    if (millis() - lastMessageCheck > messageCheckInterval) {
        lastMessageCheck = millis();
        checkNewMessages();
    }
    
    // Обработка входящих LoRa сообщений
    handleLoRaMessages();
    
    delay(100);
}

void connectToWiFi() {
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
    } else {
        Serial.println("\nНе удалось подключиться к WiFi");
    }
}

void initMeshtastic() {
    Serial.println("Инициализация Meshtastic...");
    // Здесь должна быть инициализация Meshtastic
    // В зависимости от используемой библиотеки
}

void handleLoRaMessages() {
    // Обработка входящих LoRa сообщений
    // При получении сообщения - отправка на сервер
}

void checkNewMessages() {
    // Проверка новых сообщений на сервере
    // Отправка через LoRa
}

// HTTP сервер для получения сообщений от mesh-server
void sendMessageToLoRa(String toNode, String text) {
    Serial.print("Отправка сообщения на ");
    Serial.print(toNode);
    Serial.print(": ");
    Serial.println(text);
    
    // Здесь код отправки через LoRa
    // В зависимости от библиотеки Meshtastic
}

// Отправка входящего сообщения на сервер
void sendInboundMessageToServer(String fromNode, String toNode, String text) {
    if (WiFi.status() != WL_CONNECTED) {
        Serial.println("WiFi не подключен");
        return;
    }
    
    HTTPClient http;
    http.begin(serverUrl + String("/api/messages"));
    http.addHeader("Content-Type", "application/json");
    
    // Формирование JSON payload
    StaticJsonDocument<256> doc;
    doc["device_id"] = 1;
    doc["from_node"] = fromNode;
    doc["to_node"] = toNode;
    doc["text"] = text;
    doc["direction"] = "inbound";
    
    String jsonPayload;
    serializeJson(doc, jsonPayload);
    
    int httpResponseCode = http.POST(jsonPayload);
    
    if (httpResponseCode > 0) {
        Serial.print("Ответ сервера: ");
        Serial.println(httpResponseCode);
    } else {
        Serial.print("Ошибка отправки: ");
        Serial.println(httpResponseCode);
    }
    
    http.end();
}

// Health check endpoint
bool healthCheck() {
    return true;
}
