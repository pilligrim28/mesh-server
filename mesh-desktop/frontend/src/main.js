// Import Wails runtime
import {EventsOn, EventsOff} from '../wailsjs/runtime/runtime';
import * as App from '../wailsjs/go/main/App';

// Глобальные переменные
let map;
let markers = {};
let refreshInterval;
let serverConnected = false;

// Инициализация приложения
document.addEventListener('DOMContentLoaded', () => {
    initMap();
    initTabs();
    initMessageForm();
    loadServerUrl();
    checkConnection();
    
    // Автообновление каждые 5 секунд
    refreshInterval = setInterval(loadData, 5000);
    
    console.log('Meshtastic Monitor initialized');
});

// Очистка при закрытии
window.addEventListener('beforeunload', () => {
    if (refreshInterval) {
        clearInterval(refreshInterval);
    }
    EventsOff('update');
});

// Инициализация карты
function initMap() {
    map = L.map('map').setView([55.7558, 37.6173], 10);
    
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '© OpenStreetMap contributors'
    }).addTo(map);
}

// Инициализация табов
function initTabs() {
    document.querySelectorAll('.nav-link').forEach(link => {
        link.addEventListener('click', (e) => {
            e.preventDefault();
            const tab = e.target.closest('.nav-link').dataset.tab;
            
            // Переключение активных классов
            document.querySelectorAll('.nav-link').forEach(l => l.classList.remove('active'));
            document.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('active'));
            
            e.target.closest('.nav-link').classList.add('active');
            document.getElementById(`${tab}-tab`).classList.add('active');
        });
    });
}

// Инициализация формы сообщений
function initMessageForm() {
    document.getElementById('messageForm').addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const deviceID = parseInt(document.getElementById('msgDeviceID').value);
        const fromNode = document.getElementById('msgFrom').value;
        const toNode = document.getElementById('msgTo').value;
        const text = document.getElementById('msgText').value;
        
        try {
            const success = await App.SendMessage(deviceID, fromNode, toNode, text);
            if (success) {
                alert('Сообщение отправлено!');
                document.getElementById('msgText').value = '';
                loadMessages(deviceID);
            } else {
                alert('Ошибка отправки сообщения');
            }
        } catch (error) {
            console.error('Error sending message:', error);
            alert('Ошибка: ' + error.message);
        }
    });
}

// Загрузка URL сервера
async function loadServerUrl() {
    try {
        const url = await App.GetServerURL();
        document.getElementById('serverUrlInput').value = url;
        document.getElementById('serverUrlDisplay').textContent = url.replace('http://', '').replace('https://', '');
    } catch (error) {
        console.error('Error loading server URL:', error);
    }
}

// Сохранение URL сервера
async function saveServerUrl() {
    const url = document.getElementById('serverUrlInput').value;
    try {
        await App.SetServerURL(url);
        document.getElementById('serverUrlDisplay').textContent = url.replace('http://', '').replace('https://', '');
        alert('URL сервера сохранен: ' + url);
        checkConnection();
    } catch (error) {
        console.error('Error saving server URL:', error);
        alert('Ошибка: ' + error.message);
    }
}

// Проверка подключения
async function checkConnection() {
    const statusDiv = document.getElementById('connectionStatus');
    statusDiv.innerHTML = '<div class="text-info"><i class="bi bi-hourglass-split"></i> Проверка...</div>';
    
    try {
        const connected = await App.CheckServerConnection();
        serverConnected = connected;
        
        if (connected) {
            statusDiv.innerHTML = '<div class="text-success"><i class="bi bi-check-circle"></i> Подключено к серверу!</div>';
            updateWSStatus(true);
            loadData();
        } else {
            statusDiv.innerHTML = '<div class="text-danger"><i class="bi bi-x-circle"></i> Сервер недоступен. Проверьте URL и запустите mesh-server.</div>';
            updateWSStatus(false);
        }
    } catch (error) {
        statusDiv.innerHTML = '<div class="text-danger"><i class="bi bi-x-circle"></i> Ошибка: ' + error.message + '</div>';
        updateWSStatus(false);
    }
}

// Загрузка всех данных
async function loadData() {
    if (!serverConnected) {
        return;
    }
    
    try {
        await loadDevices();
        await loadMetrics();
        await loadAlerts();
        await loadStats();
        
        document.getElementById('lastUpdate').textContent = 
            new Date().toLocaleTimeString('ru-RU');
    } catch (error) {
        console.error('Error loading data:', error);
    }
}

// Загрузка устройств
async function loadDevices() {
    try {
        const devices = await App.GetDevices();
        
        // Обновление счетчика
        document.getElementById('deviceCount').textContent = devices.length;
        
        // Обновление списка устройств
        const deviceList = document.getElementById('deviceList');
        deviceList.innerHTML = '';
        
        devices.forEach(device => {
            const div = document.createElement('div');
            div.className = 'mb-2 p-2 border-bottom border-secondary';
            div.innerHTML = `
                <div class="d-flex justify-content-between align-items-center">
                    <strong>${device.name || device.node_id}</strong>
                    <span class="badge ${device.is_online ? 'bg-success' : 'bg-danger'}">
                        ${device.is_online ? 'Online' : 'Offline'}
                    </span>
                </div>
                <small class="text-muted">
                    📍 ${device.latitude.toFixed(4)}, ${device.longitude.toFixed(4)}
                </small>
            `;
            deviceList.appendChild(div);
            
            // Обновление маркеров на карте
            updateMapMarker(device);
        });
    } catch (error) {
        console.error('Error loading devices:', error);
    }
}

// Обновление маркера на карте
function updateMapMarker(device) {
    const latLng = [device.latitude, device.longitude];
    
    if (markers[device.id]) {
        markers[device.id].setLatLng(latLng);
    } else {
        const marker = L.marker(latLng).addTo(map);
        marker.bindPopup(`
            <strong>${device.name || device.node_id}</strong><br>
            Altitude: ${device.altitude}m<br>
            Status: ${device.is_online ? 'Online' : 'Offline'}
        `);
        markers[device.id] = marker;
    }
}

// Загрузка метрик
async function loadMetrics() {
    try {
        const metrics = await App.GetMetrics();
        
        const metricsList = document.getElementById('metricsList');
        metricsList.innerHTML = '';
        
        if (metrics.length === 0) {
            metricsList.innerHTML = '<div class="loading">Нет данных метрик</div>';
            return;
        }
        
        metrics.forEach(metric => {
            const card = document.createElement('div');
            card.className = 'card mb-2';
            card.innerHTML = `
                <div class="card-header">
                    <i class="bi bi-device"></i> Device #${metric.device_id}
                    <small class="float-end text-muted">${metric.timestamp}</small>
                </div>
                <div class="card-body">
                    <div class="row text-center">
                        <div class="col-3">
                            <div class="metric-value">${metric.heart_rate || '-'}</div>
                            <div class="metric-label">❤️ Пульс</div>
                        </div>
                        <div class="col-3">
                            <div class="metric-value">${metric.co2 || '-'}</div>
                            <div class="metric-label">💨 CO₂</div>
                        </div>
                        <div class="col-3">
                            <div class="metric-value">${metric.temp || '-'}</div>
                            <div class="metric-label">🌡️ Температура</div>
                        </div>
                        <div class="col-3">
                            <div class="metric-value">${metric.humidity || '-'}</div>
                            <div class="metric-label">💧 Влажность</div>
                        </div>
                    </div>
                </div>
            `;
            metricsList.appendChild(card);
        });
    } catch (error) {
        console.error('Error loading metrics:', error);
    }
}

// Загрузка алертов
async function loadAlerts() {
    try {
        const alerts = await App.GetUnreadAlerts();
        
        // Обновление бейджа
        const badge = document.getElementById('alertBadge');
        if (alerts.length > 0) {
            badge.textContent = alerts.length;
            badge.classList.remove('d-none');
        } else {
            badge.classList.add('d-none');
        }
        
        // Обновление счетчика
        document.getElementById('alertCount').textContent = alerts.length;
        
        // Обновление списка
        const alertsList = document.getElementById('alertsList');
        alertsList.innerHTML = '';
        
        if (alerts.length === 0) {
            alertsList.innerHTML = '<div class="loading">Нет непрочитанных уведомлений</div>';
            return;
        }
        
        alerts.forEach(alert => {
            const div = document.createElement('div');
            div.className = `alert-item alert-${alert.severity}`;
            div.innerHTML = `
                <div class="d-flex justify-content-between align-items-start">
                    <div>
                        <strong>${alert.type}</strong>
                        <p class="mb-1 small">${alert.message}</p>
                        <small class="text-muted">${alert.timestamp}</small>
                    </div>
                    <button class="btn btn-sm btn-outline-light" onclick="markAlertRead(${alert.id})">
                        <i class="bi bi-check"></i>
                    </button>
                </div>
            `;
            alertsList.appendChild(div);
        });
    } catch (error) {
        console.error('Error loading alerts:', error);
    }
}

// Загрузка статистики
async function loadStats() {
    try {
        const stats = await App.GetStats();
        console.log('Stats:', stats);
    } catch (error) {
        console.error('Error loading stats:', error);
    }
}

// Загрузка сообщений
async function loadMessages(deviceID = 1) {
    try {
        const messages = await App.GetMessages(deviceID, 50);
        
        const messagesList = document.getElementById('messagesList');
        messagesList.innerHTML = '';
        
        if (messages.length === 0) {
            messagesList.innerHTML = '<div class="loading">Нет сообщений</div>';
            return;
        }
        
        messages.forEach(msg => {
            const div = document.createElement('div');
            div.className = `message-item ${msg.direction === 'outbound' ? 'message-outbound' : 'message-inbound'}`;
            div.innerHTML = `
                <div class="d-flex justify-content-between">
                    <small class="text-info">${msg.from_node}</small>
                    <small class="text-muted">${msg.timestamp}</small>
                </div>
                <div class="text-muted small">→ ${msg.to_node}</div>
                <div class="mt-1">${msg.text}</div>
            `;
            messagesList.appendChild(div);
        });
    } catch (error) {
        console.error('Error loading messages:', error);
    }
}

// Пометить алерт как прочитанный
async function markAlertRead(alertID) {
    try {
        const success = await App.MarkAlertAsRead(alertID);
        if (success) {
            loadAlerts();
        }
    } catch (error) {
        console.error('Error marking alert as read:', error);
    }
}

// Пометить все алерты как прочитанные
async function markAllAlertsRead() {
    try {
        const success = await App.MarkAllAlertsAsRead(0);
        if (success) {
            loadAlerts();
        }
    } catch (error) {
        console.error('Error marking all alerts as read:', error);
    }
}

// Bluetooth сканирование
async function startBluetoothScan() {
    try {
        const status = await App.GetDiscoveryStatus();
        if (status.bluetooth_scanning) {
            await App.StopBluetoothScan();
            document.getElementById('discoveryStatus').textContent = 'Bluetooth scan stopped';
        } else {
            await App.StartBluetoothScan();
            document.getElementById('discoveryStatus').textContent = 'Bluetooth scanning...';
        }
    } catch (error) {
        console.error('Error with Bluetooth scan:', error);
        alert('Ошибка: ' + error.message);
    }
}

// WiFi сканирование
async function startWiFiScan() {
    try {
        const status = await App.GetDiscoveryStatus();
        if (status.wifi_scanning) {
            await App.StopWiFiScan();
            document.getElementById('discoveryStatus').textContent = 'WiFi scan stopped';
        } else {
            await App.StartWiFiScan();
            document.getElementById('discoveryStatus').textContent = 'WiFi scanning...';
        }
    } catch (error) {
        console.error('Error with WiFi scan:', error);
        alert('Ошибка: ' + error.message);
    }
}

// Загрузка обнаруженных устройств
async function loadDiscoveredDevices() {
    try {
        const devices = await App.GetDiscoveredDevices();
        
        if (devices.length === 0) {
            alert('Обнаруженные устройства не найдены. Запустите сканирование.');
            return;
        }
        
        let message = 'Обнаруженные устройства:\n\n';
        devices.forEach(d => {
            message += `${d.name} (${d.address}) - ${d.type} - RSSI: ${d.rssi || 'N/A'}\n`;
        });
        
        alert(message);
    } catch (error) {
        console.error('Error loading discovered devices:', error);
        alert('Ошибка: ' + error.message);
    }
}

// Обновление статуса WebSocket
function updateWSStatus(connected) {
    const status = document.getElementById('wsStatus');
    if (connected) {
        status.classList.remove('ws-disconnected');
        status.classList.add('ws-connected');
    } else {
        status.classList.remove('ws-connected');
        status.classList.add('ws-disconnected');
    }
}

// Экспорт функций для глобального доступа
window.startBluetoothScan = startBluetoothScan;
window.startWiFiScan = startWiFiScan;
window.loadDiscoveredDevices = loadDiscoveredDevices;
window.markAlertRead = markAlertRead;
window.markAllAlertsRead = markAllAlertsRead;
window.saveServerUrl = saveServerUrl;
window.checkConnection = checkConnection;
