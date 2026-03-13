// Meshtastic Monitor - Frontend Application

const API_BASE = '';
let map;
let markers = {};
let devices = [];
let metrics = [];
let alerts = [];
let messages = [];
let ws = null;
let refreshInterval = null;

// Инициализация
document.addEventListener('DOMContentLoaded', () => {
    initMap();
    initTabs();
    initWebSocket();
    loadData();
    startAutoRefresh();
    checkSimulatorStatus();
});

// Карта
function initMap() {
    map = L.map('map').setView([55.7558, 37.6173], 10);
    
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '© OpenStreetMap contributors'
    }).addTo(map);
    
    // Тёмная тема карты
    fetch('https://basemaps.cartocdn.com/rastertiles/voyager_dark/{z}/{x}/{y}{r}.png')
        .then(response => {
            if (response.ok) {
                L.tileLayer('https://basemaps.cartocdn.com/rastertiles/voyager_dark/{z}/{x}/{y}{r}.png', {
                    attribution: '© OpenStreetMap © CARTO'
                }).addTo(map);
            }
        })
        .catch(() => {});
}

// Вкладки
function initTabs() {
    document.querySelectorAll('#sidebarTabs .nav-link').forEach(link => {
        link.addEventListener('click', (e) => {
            e.preventDefault();
            const tab = e.target.closest('.nav-link').dataset.tab;
            
            document.querySelectorAll('#sidebarTabs .nav-link').forEach(l => l.classList.remove('active'));
            document.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('active'));
            
            e.target.closest('.nav-link').classList.add('active');
            document.getElementById(`${tab}-tab`).classList.add('active');
        });
    });
}

// WebSocket
function initWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${protocol}//${window.location.host}/ws`);
    
    ws.onopen = () => {
        document.getElementById('wsStatus').className = 'ws-status ws-connected';
        console.log('WebSocket connected');
    };
    
    ws.onclose = () => {
        document.getElementById('wsStatus').className = 'ws-status ws-disconnected';
        console.log('WebSocket disconnected, reconnecting...');
        setTimeout(initWebSocket, 3000);
    };
    
    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
    };
    
    ws.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            handleWebSocketMessage(data);
        } catch (e) {
            console.error('Failed to parse WebSocket message:', e);
        }
    };
}

function handleWebSocketMessage(data) {
    console.log('WebSocket message:', data);

    if (data.type === 'metrics') {
        updateMetrics(data.payload);
    } else if (data.type === 'alert') {
        addAlert(data.payload);
    } else if (data.type === 'device') {
        updateDevice(data.payload);
    } else if (data.type === 'new_message') {
        // Новое сообщение - обновляем список
        console.log('New message received:', data.message);
        loadData();
    }

    loadData();
}

// Загрузка данных
async function loadData() {
    try {
        await Promise.all([
            loadDevices(),
            loadMetrics(),
            loadAlerts(),
            loadMessages()
        ]);
        updateLastUpdate();
    } catch (error) {
        console.error('Failed to load data:', error);
    }
}

async function loadDevices() {
    const response = await fetch(`${API_BASE}/api/devices`);
    devices = await response.json();
    updateMapMarkers();
    updateDeviceList();
    updateStats();
}

async function loadMetrics() {
    const response = await fetch(`${API_BASE}/api/metrics`);
    metrics = await response.json();
    updateMetricsDisplay();
}

async function loadAlerts() {
    const response = await fetch(`${API_BASE}/api/alerts`);
    alerts = await response.json();
    updateAlertsDisplay();
    updateAlertBadge();
}

async function loadMessages() {
    // Загружаем все сообщения (и входящие и исходящие)
    const response = await fetch(`${API_BASE}/api/messages/device?device_id=1&limit=50`);
    messages = await response.json();
    updateMessagesDisplay();
}

// Обновление карты
function updateMapMarkers() {
    devices.forEach(device => {
        if (device.latitude && device.longitude) {
            if (!markers[device.id]) {
                const marker = L.marker([device.latitude, device.longitude]).addTo(map);
                marker.bindPopup(createDevicePopup(device));
                markers[device.id] = marker;
            } else {
                markers[device.id].setLatLng([device.latitude, device.longitude]);
                markers[device.id].setPopupContent(createDevicePopup(device));
            }
        }
    });
    
    // Удаляем маркеры для удалённых устройств
    Object.keys(markers).forEach(id => {
        if (!devices.find(d => d.id == id)) {
            map.removeLayer(markers[id]);
            delete markers[id];
        }
    });
    
    // Центрируем карту если есть устройства
    if (devices.length > 0) {
        const validDevices = devices.filter(d => d.latitude && d.longitude);
        if (validDevices.length > 0) {
            const bounds = validDevices.map(d => [d.latitude, d.longitude]);
            map.fitBounds(bounds, { padding: [50, 50] });
        }
    }
}

function createDevicePopup(device) {
    const lastSeen = device.last_seen ? new Date(device.last_seen).toLocaleString('ru-RU') : 'Никогда';
    const isOnline = device.last_seen && new Date(device.last_seen) > new Date(Date.now() - 5 * 60 * 1000);
    
    return `
        <div style="min-width: 200px;">
            <h6><i class="bi bi-broadcast"></i> ${escapeHtml(device.name || device.node_id)}</h6>
            <p class="mb-1"><small><strong>ID:</strong> ${escapeHtml(device.node_id)}</small></p>
            <p class="mb-1"><small><strong>Координаты:</strong> ${device.latitude?.toFixed(4) || 'N/A'}, ${device.longitude?.toFixed(4) || 'N/A'}</small></p>
            <p class="mb-1"><small><strong>Высота:</strong> ${device.altitude || 0} м</small></p>
            <p class="mb-0"><small><strong>В сети:</strong> 
                <span class="${isOnline ? 'status-online' : 'status-offline'}">
                    ${isOnline ? '● Онлайн' : '○ Офлайн'}
                </span>
            </small></p>
            <p><small><strong>Последний раз:</strong> ${lastSeen}</small></p>
        </div>
    `;
}

function updateDeviceList() {
    const container = document.getElementById('deviceList');
    container.innerHTML = devices.map(device => {
        const isOnline = device.last_seen && new Date(device.last_seen) > new Date(Date.now() - 5 * 60 * 1000);
        return `
            <div class="d-flex justify-content-between align-items-center p-2 border-bottom border-secondary">
                <div>
                    <strong>${escapeHtml(device.name || device.node_id)}</strong>
                    <br><small class="text-muted">${escapeHtml(device.node_id)}</small>
                </div>
                <span class="${isOnline ? 'status-online' : 'status-offline'}">
                    ${isOnline ? '●' : '○'}
                </span>
            </div>
        `;
    }).join('') || '<p class="text-muted text-center">Нет устройств</p>';
}

// Метрики
function updateMetricsDisplay() {
    const container = document.getElementById('metricsList');
    
    if (metrics.length === 0) {
        container.innerHTML = '<p class="text-muted text-center">Нет метрик</p>';
        return;
    }
    
    const deviceMetrics = {};
    metrics.forEach(m => {
        if (!deviceMetrics[m.device_id]) {
            deviceMetrics[m.device_id] = [];
        }
        deviceMetrics[m.device_id].push(m);
    });
    
    container.innerHTML = Object.entries(deviceMetrics).map(([deviceId, deviceMetrics]) => {
        const device = devices.find(d => d.id == deviceId);
        const latest = deviceMetrics[0];
        const timestamp = latest.timestamp ? new Date(latest.timestamp).toLocaleString('ru-RU') : '';
        
        return `
            <div class="card">
                <div class="card-header">
                    <i class="bi bi-activity"></i> ${escapeHtml(device?.name || device?.node_id || `Device ${deviceId}`)}
                    <small class="text-muted float-end">${timestamp}</small>
                </div>
                <div class="card-body">
                    <div class="row text-center">
                        ${latest.heart_rate ? `
                            <div class="col-6 mb-2">
                                <div class="metric-value"><i class="bi bi-heart-pulse"></i> ${latest.heart_rate}</div>
                                <div class="metric-label">Пульс (bpm)</div>
                            </div>
                        ` : ''}
                        ${latest.co2 ? `
                            <div class="col-6 mb-2">
                                <div class="metric-value"><i class="bi bi-wind"></i> ${latest.co2}</div>
                                <div class="metric-label">CO₂ (ppm)</div>
                            </div>
                        ` : ''}
                        ${latest.temp ? `
                            <div class="col-6 mb-2">
                                <div class="metric-value"><i class="bi bi-thermometer-half"></i> ${latest.temp.toFixed(1)}°C</div>
                                <div class="metric-label">Температура</div>
                            </div>
                        ` : ''}
                        ${latest.humidity ? `
                            <div class="col-6 mb-2">
                                <div class="metric-value"><i class="bi bi-droplet"></i> ${latest.humidity.toFixed(0)}%</div>
                                <div class="metric-label">Влажность</div>
                            </div>
                        ` : ''}
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

// Алерты
function updateAlertsDisplay() {
    const container = document.getElementById('alertsList');
    
    if (alerts.length === 0) {
        container.innerHTML = '<p class="text-muted text-center">Нет уведомлений</p>';
        return;
    }
    
    container.innerHTML = alerts.map(alert => {
        const device = devices.find(d => d.id == alert.device_id);
        const time = alert.created_at ? new Date(alert.created_at).toLocaleString('ru-RU') : '';
        
        return `
            <div class="alert-item alert-${alert.severity}">
                <div class="d-flex justify-content-between">
                    <strong>${getAlertIcon(alert.type)} ${getAlertTitle(alert.type)}</strong>
                    <small class="text-muted">${time}</small>
                </div>
                <p class="mb-1">${escapeHtml(alert.message)}</p>
                <small class="text-muted">
                    <i class="bi bi-broadcast"></i> ${escapeHtml(device?.name || device?.node_id || 'Unknown')}
                    ${!alert.is_read ? '<span class="badge bg-danger ms-2">Новый</span>' : ''}
                </small>
            </div>
        `;
    }).join('');
}

function updateAlertBadge() {
    const badge = document.getElementById('alertBadge');
    const unreadCount = alerts.filter(a => !a.is_read).length;
    
    if (unreadCount > 0) {
        badge.textContent = unreadCount;
        badge.classList.remove('d-none');
    } else {
        badge.classList.add('d-none');
    }
    
    document.getElementById('alertCount').textContent = alerts.length;
}

function getAlertIcon(type) {
    const icons = {
        'out_of_zone': '<i class="bi bi-geo-alt-fill"></i>',
        'low_battery': '<i class="bi bi-battery-low"></i>',
        'signal_lost': '<i class="bi bi-wifi-off"></i>'
    };
    return icons[type] || '<i class="bi bi-bell-fill"></i>';
}

function getAlertTitle(type) {
    const titles = {
        'out_of_zone': 'Вне зоны',
        'low_battery': 'Низкий заряд',
        'signal_lost': 'Потеря сигнала'
    };
    return titles[type] || type;
}

async function markAllAlertsRead() {
    await fetch(`${API_BASE}/api/alerts/read-all`, { method: 'PUT' });
    await loadAlerts();
}

// Сообщения
function updateMessagesDisplay() {
    const container = document.getElementById('messagesList');

    if (messages.length === 0) {
        container.innerHTML = '<p class="text-muted text-center">Нет сообщений</p>';
        return;
    }

    container.innerHTML = messages.map(msg => {
        const time = msg.sent_at ? new Date(msg.sent_at).toLocaleString('ru-RU') : '';
        const isInbound = msg.direction === 'inbound';

        return `
            <div class="message-item message-${msg.direction}" style="border-radius: 8px; margin-bottom: 8px; padding: 10px;">
                <div class="d-flex justify-content-between align-items-center">
                    <div>
                        <span class="badge bg-${isInbound ? 'info' : 'success'} mb-1">
                            <i class="bi bi-${isInbound ? 'arrow-down' : 'arrow-up'}"></i>
                            ${isInbound ? 'Входящее' : 'Исходящее'}
                        </span>
                        <p class="mb-1" style="font-size: 1rem;">${escapeHtml(msg.text)}</p>
                        <small class="text-muted">
                            <i class="bi bi-person-circle"></i> ${escapeHtml(msg.from_node)} 
                            <i class="bi bi-arrow-right"></i> 
                            ${escapeHtml(msg.to_node || 'Все')}
                        </small>
                    </div>
                    <small class="text-muted">${time}</small>
                </div>
            </div>
        `;
    }).join('');
    
    // Прокрутка к последнему сообщению
    container.scrollTop = container.scrollHeight;
}

document.getElementById('messageForm').addEventListener('submit', async (e) => {
    e.preventDefault();

    let fromNode = document.getElementById('msgFrom').value;
    const toNode = document.getElementById('msgTo').value;
    const text = document.getElementById('msgText').value;

    // Если from_node не указан, используем устройство по умолчанию
    if (!fromNode) {
        // Ищем устройство по умолчанию или первое доступное
        const defaultDevice = devices.find(d => d.node_id === '!default');
        if (defaultDevice) {
            fromNode = defaultDevice.node_id;
        } else if (devices.length > 0) {
            fromNode = devices[0].node_id;
        } else {
            fromNode = '!default';
        }
        document.getElementById('msgFrom').value = fromNode;
    }

    const device = devices.find(d => d.node_id === fromNode);

    const payload = {
        device_id: device?.id || 1,
        from_node: fromNode,
        to_node: toNode,
        text: text,
        direction: 'outbound'
    };

    try {
        const response = await fetch(`${API_BASE}/api/messages`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        if (response.ok) {
            document.getElementById('msgText').value = '';
            await loadMessages();
        } else {
            const error = await response.text();
            alert('Ошибка отправки сообщения: ' + error);
        }
    } catch (error) {
        alert('Ошибка отправки сообщения: ' + error.message);
    }
});

// Статистика
function updateStats() {
    document.getElementById('deviceCount').textContent = devices.length;
}

function updateLastUpdate() {
    document.getElementById('lastUpdate').textContent = 
        'Обновлено: ' + new Date().toLocaleTimeString('ru-RU');
}

// Автообновление
function startAutoRefresh() {
    refreshInterval = setInterval(loadData, 10000); // Каждые 10 секунд
}

// Утилиты
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Генерация тестовых событий симулятора
async function generateEvent(eventType) {
    try {
        await fetch(`${API_BASE}/api/simulator/event`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ type: eventType })
        });

        // Обновляем данные через 1 секунду
        setTimeout(loadData, 1000);
    } catch (error) {
        alert('Ошибка генерации события: ' + error.message);
    }
}

// Проверка статуса симулятора
async function checkSimulatorStatus() {
    try {
        const response = await fetch(`${API_BASE}/api/simulator/status`);
        const status = await response.json();

        const badge = document.getElementById('simStatus');
        if (status.running) {
            badge.className = 'badge bg-success';
            badge.textContent = '🟢 Симулятор активен';
        } else {
            badge.className = 'badge bg-secondary';
            badge.textContent = '⚪ Симулятор остановлен';
        }
    } catch (error) {
        console.error('Failed to check simulator status:', error);
    }
}

// Сканирование ESP32/Meshtastic устройств
async function scanESP32() {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');
    
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Сканирование WiFi и Bluetooth...';
    connectBtn.disabled = true;

    try {
        // Сканируем WiFi и Bluetooth параллельно
        const [wifiResponse, bleResponse] = await Promise.all([
            fetch(`${API_BASE}/api/esp32/scan`),
            fetch(`${API_BASE}/api/esp32/bluetooth`)
        ]);

        const wifiResult = await wifiResponse.json();
        const bleResult = await bleResponse.json();

        let html = '';
        let found = false;
        let firstDevice = null;

        // WiFi устройства
        if (wifiResult.count > 0) {
            found = true;
            firstDevice = wifiResult.auto_connect;
            html += '<div class="mb-3"><strong>📡 WiFi устройства:</strong><br>';
            wifiResult.wifi_devices.forEach(ip => {
                html += `<span class="badge bg-success me-1">${ip}</span>`;
            });
            html += '</div>';
        }

        // Bluetooth устройства
        if (bleResult.count > 0) {
            found = true;
            if (!firstDevice && bleResult.esp32_found) {
                firstDevice = bleResult.esp32_mac;
            }
            html += '<div class="mb-3"><strong>📶 Bluetooth устройства:</strong><br>';
            bleResult.devices.forEach(device => {
                const badge = device.isESP32 ? 'bg-primary' : 'bg-secondary';
                const icon = device.isESP32 ? '📱' : '🔵';
                html += `<span class="badge ${badge} me-1">${icon} ${device.name} (${device.address}) RSSI: ${device.rssi}</span>`;
            });
            html += '</div>';
        }

        if (found) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = '<i class="bi bi-check-circle"></i> <strong>Найдено устройств:</strong> ' + (wifiResult.count + bleResult.count) + '<br>' + html;
            
            // Активируем кнопку быстрого подключения
            if (firstDevice) {
                connectBtn.disabled = false;
                connectBtn.onclick = () => {
                    if (firstDevice.includes('.')) {
                        // Это IP адрес
                        connectToESP32(firstDevice);
                    } else {
                        // Это MAC адрес
                        connectToESP32BLE(firstDevice);
                    }
                };
            }
        } else {
            statusDiv.className = 'alert alert-warning';
            statusDiv.innerHTML = `
                <i class="bi bi-exclamation-triangle"></i> Устройства не найдены.<br>
                <small>Убедитесь что ESP32 включен и находится в той же сети или в радиусе Bluetooth</small>
            `;
            connectBtn.disabled = true;
        }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка сканирования: ${error.message}`;
        connectBtn.disabled = true;
    }
}

// Быстрое подключение
function quickConnect() {
    const ip = document.getElementById('esp32IP').value;
    if (ip) {
        connectToESP32(ip);
    }
}

// Подключение к ESP32 по WiFi
async function connectToESP32(ip) {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');
    
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к ' + ip + '...';
    
    try {
        const response = await fetch(`${API_BASE}/api/esp32/connect`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ip: ip })
        });

        const result = await response.json();

        if (response.ok) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `
                <i class="bi bi-check-circle"></i> <strong>Подключено к ${ip}</strong><br>
                <small>Теперь можно отправлять сообщения через LoRa/Bluetooth</small>
            `;
            connectBtn.className = 'btn btn-sm btn-success';
            connectBtn.innerHTML = '<i class="bi bi-check"></i> Подключено';
            connectBtn.disabled = true;
        } else {
            throw new Error(result.message || 'Ошибка подключения');
        }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        connectBtn.disabled = false;
    }
}

// Подключение к ESP32 по Bluetooth
async function connectToESP32BLE(macAddress) {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');
    
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к Bluetooth устройству ' + macAddress + '...';
    
    try {
        const response = await fetch(`${API_BASE}/api/esp32/connect`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac: macAddress })
        });

        const result = await response.json();

        if (response.ok) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `
                <i class="bi bi-bluetooth"></i> <strong>Подключено к ${macAddress}</strong><br>
                <small>Теперь можно отправлять сообщения</small>
            `;
            connectBtn.className = 'btn btn-sm btn-success';
            connectBtn.innerHTML = '<i class="bi bi-check"></i> Подключено';
            connectBtn.disabled = true;
        } else {
            throw new Error(result.message || 'Ошибка подключения');
        }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        connectBtn.disabled = false;
    }
}

// Для отладки
window.app = {
    devices: () => devices,
    metrics: () => metrics,
    alerts: () => alerts,
    messages: () => messages,
    refresh: loadData,
    scanESP32: scanESP32,
    connectToESP32: connectToESP32
};
