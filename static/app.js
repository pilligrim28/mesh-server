// Meshtastic Monitor - Frontend Application

const API_BASE = '';
let map;
let markers = {};
let routeLines = {}; // {simId: L.polyline}
let simMarkers = {}; // {simId: L.marker} для маркеров симуляторов
let showRouteLine = true;
let devices = [];
let metrics = [];
let alerts = [];
let messages = [];
let ws = null;
let refreshInterval = null;

// Цветовая схема
const COLORS = {
    primary: '#4ecca3',
    danger: '#dc3545',
    warning: '#ffc107',
    info: '#0dcaf0',
    online: '#4ecca3',
    offline: '#dc3545'
};

// Инициализация
document.addEventListener('DOMContentLoaded', () => {
    initMap();
    initTabs();
    initWebSocket();
    loadData();
    startAutoRefresh();
    checkSimulatorStatus();
    loadRouteHistory();
    checkBLEStatus();
    checkMDNSStatus();
    addStPetersburgPoints();
});

// Карта
function initMap() {
    map = L.map('map').setView([59.9343, 30.3351], 13);

    L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a> &copy; <a href="https://carto.com/attributions">CARTO</a>',
        subdomains: 'abcd',
        maxZoom: 19,
        minZoom: 1
    }).addTo(map);
}

// Функция для создания цветного маркера
function getDeviceMarker(device) {
    const isOnline = device.last_seen && new Date(device.last_seen) > new Date(Date.now() - 5 * 60 * 1000);
    const color = isOnline ? COLORS.online : COLORS.offline;
    
    const html = `
        <div style="
            background: ${color};
            width: 32px;
            height: 32px;
            border-radius: 50%;
            border: 3px solid white;
            box-shadow: 0 0 10px rgba(0,0,0,0.5);
            display: flex;
            align-items: center;
            justify-content: center;
            transition: transform 0.2s ease;
            cursor: pointer;
        ">
            <i class="bi bi-broadcast" style="color: white; font-size: 16px;"></i>
        </div>
    `;
    
    return L.divIcon({
        html: html,
        className: 'custom-device-marker',
        iconSize: [32, 32],
        iconAnchor: [16, 16],
        popupAnchor: [0, -16]
    });
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
        loadData();
    } else if (data.type === 'healbe_data') {
        handleHealbeWebSocket(data);
    }

    loadData();
}

function updateMetrics(metricData) {
    const existingIndex = metrics.findIndex(m => m.device_id === metricData.device_id);
    if (existingIndex !== -1) {
        metrics[existingIndex] = metricData;
    } else {
        metrics.unshift(metricData);
    }
    if (metrics.length > 100) metrics.pop();
    updateMetricsDisplay();
}

function updateDevice(deviceData) {
    const existingIndex = devices.findIndex(d => d.id === deviceData.id);
    if (existingIndex !== -1) {
        devices[existingIndex] = { ...devices[existingIndex], ...deviceData };
    } else {
        devices.push(deviceData);
    }
    updateMapMarkers();
    updateDeviceList();
    updateStats();
}

function addAlert(alertData) {
    alerts.unshift(alertData);
    if (alerts.length > 100) alerts.pop();
    updateAlertsDisplay();
    updateAlertBadge();
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
    const response = await fetch(`${API_BASE}/api/messages/device?device_id=1&limit=50`);
    messages = await response.json();
    updateMessagesDisplay();
}

// Обновление карты
function updateMapMarkers() {
    devices.forEach(device => {
        if (device.latitude && device.longitude) {
            if (!markers[device.id]) {
                const marker = L.marker([device.latitude, device.longitude], {
                    icon: getDeviceMarker(device)
                }).addTo(map);
                marker.bindPopup(createDevicePopup(device));
                markers[device.id] = marker;
            } else {
                markers[device.id].setLatLng([device.latitude, device.longitude]);
                markers[device.id].setIcon(getDeviceMarker(device));
                markers[device.id].setPopupContent(createDevicePopup(device));
            }
        }
    });

    Object.keys(markers).forEach(id => {
        if (!devices.find(d => d.id == id)) {
            map.removeLayer(markers[id]);
            delete markers[id];
        }
    });
}

function createDevicePopup(device) {
    const lastSeen = device.last_seen ? new Date(device.last_seen).toLocaleString('ru-RU') : 'Никогда';
    const isOnline = device.last_seen && new Date(device.last_seen) > new Date(Date.now() - 5 * 60 * 1000);
    
    const statusColor = isOnline ? COLORS.online : COLORS.offline;
    const statusText = isOnline ? '● Онлайн' : '○ Офлайн';

    let deviceName = device.name && device.name !== 'Unknown' ? device.name : `Устройство #${device.id}`;

    return `
        <div style="min-width: 260px; background: #1a1a2e; color: #eee; padding: 12px; border-radius: 12px; box-shadow: 0 5px 15px rgba(0,0,0,0.3);">
            <h6 style="color: ${COLORS.primary}; margin-bottom: 10px; border-bottom: 1px solid #0f3460; padding-bottom: 5px;">
                <i class="bi bi-broadcast"></i> ${escapeHtml(deviceName)}
            </h6>
            <p class="mb-1"><small><strong style="color: #aaa;">🆔 ID:</strong> <span style="color: #fff;">${escapeHtml(device.node_id)}</span></small></p>
            <p class="mb-1"><small><strong style="color: #aaa;">📍 Координаты:</strong> <span style="color: #fff;">${device.latitude?.toFixed(6) || 'N/A'}, ${device.longitude?.toFixed(6) || 'N/A'}</span></small></p>
            <p class="mb-1"><small><strong style="color: #aaa;">📊 Высота:</strong> <span style="color: #fff;">${device.altitude || 0} м</span></small></p>
            <p class="mb-0"><small><strong style="color: #aaa;">📡 Статус:</strong>
                <span style="color: ${statusColor}; font-weight: bold;"> ${statusText}</span>
            </small></p>
            <p class="mb-0"><small><strong style="color: #aaa;">🕐 Последний раз:</strong> <span style="color: #fff;">${lastSeen}</span></small></p>
            ${device.battery ? `<p class="mb-0"><small><strong style="color: #aaa;">🔋 Батарея:</strong> <span style="color: #fff;">${device.battery}%</span></small></p>` : ''}
        </div>
    `;
}

function updateDeviceList() {
    const container = document.getElementById('deviceList');
    
    if (!devices || devices.length === 0) {
        container.innerHTML = '<p class="text-muted text-center" style="color: #aaa !important; padding: 20px;">📡 Нет устройств</p>';
        return;
    }
    
    const onlineCount = devices.filter(d => d.last_seen && new Date(d.last_seen) > new Date(Date.now() - 5 * 60 * 1000)).length;
    const onlineSpan = document.getElementById('onlineCount');
    if (onlineSpan) {
        onlineSpan.innerHTML = `<span class="badge bg-success">🟢 ${onlineCount}/${devices.length} онлайн</span>`;
    }
    
    container.innerHTML = devices.map(device => {
        const isOnline = device.last_seen && new Date(device.last_seen) > new Date(Date.now() - 5 * 60 * 1000);
        const statusColor = isOnline ? COLORS.online : COLORS.offline;
        const statusIcon = isOnline ? '🟢' : '🔴';
        
        let displayName = device.name && device.name !== 'Unknown' && device.name !== 'undefined' 
            ? device.name 
            : `Устройство #${device.id}`;
        
        let nodeDisplay = device.node_id && device.node_id !== '!default' 
            ? device.node_id.substring(0, 16) + (device.node_id.length > 16 ? '...' : '')
            : 'Активно';
        
        return `
            <div class="device-item d-flex justify-content-between align-items-center">
                <div>
                    <strong class="device-name">
                        <i class="bi bi-broadcast me-1"></i> 
                        ${escapeHtml(displayName)}
                    </strong>
                    <br>
                    <span class="device-id">📡 ID: ${escapeHtml(nodeDisplay)}</span>
                </div>
                <div style="text-align: right;">
                    <span class="device-status" style="color: ${statusColor}; font-weight: bold;">
                        ${statusIcon} ${isOnline ? 'Online' : 'Offline'}
                    </span>
                    ${device.battery ? `<br><span class="device-id">🔋 ${device.battery}%</span>` : ''}
                </div>
            </div>
        `;
    }).join('');
}

// Метрики
function updateMetricsDisplay() {
    const container = document.getElementById('metricsList');

    if (metrics.length === 0) {
        container.innerHTML = '<p class="text-muted text-center">📊 Нет метрик</p>';
        return;
    }

    const deviceMetrics = {};
    metrics.forEach(m => {
        if (!deviceMetrics[m.device_id]) {
            deviceMetrics[m.device_id] = [];
        }
        deviceMetrics[m.device_id].push(m);
    });

    container.innerHTML = Object.entries(deviceMetrics).map(([deviceId, deviceMetricsList]) => {
        const device = devices.find(d => d.id == deviceId);
        const latest = deviceMetricsList[0];
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

    const alertCountEl = document.getElementById('alertCount');
    if (alertCountEl) alertCountEl.textContent = alerts.length;
}

function getAlertIcon(type) {
    const icons = {
        'out_of_zone': '<i class="bi bi-geo-alt-fill"></i>',
        'low_battery': '<i class="bi bi-battery-low"></i>',
        'signal_lost': '<i class="bi bi-wifi-off"></i>',
        'high_heart_rate': '<i class="bi bi-heart-pulse"></i>'
    };
    return icons[type] || '<i class="bi bi-bell-fill"></i>';
}

function getAlertTitle(type) {
    const titles = {
        'out_of_zone': 'Вне зоны',
        'low_battery': 'Низкий заряд',
        'signal_lost': 'Потеря сигнала',
        'high_heart_rate': 'Высокий пульс'
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

    if (container) container.scrollTop = container.scrollHeight;
}

document.getElementById('messageForm')?.addEventListener('submit', async (e) => {
    e.preventDefault();

    let fromNode = document.getElementById('msgFrom').value;
    const toNode = document.getElementById('msgTo').value;
    const text = document.getElementById('msgText').value;

    if (!fromNode) {
        const defaultDevice = devices.find(d => d.node_id === '!default');
        if (defaultDevice) {
            fromNode = defaultDevice.node_id;
        } else if (devices.length > 0) {
            fromNode = devices[0].node_id;
        } else {
            fromNode = '!default';
        }
        const fromInput = document.getElementById('msgFrom');
        if (fromInput) fromInput.value = fromNode;
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
            const msgText = document.getElementById('msgText');
            if (msgText) msgText.value = '';
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
    const deviceCountEl = document.getElementById('deviceCount');
    if (deviceCountEl) deviceCountEl.textContent = devices.length;
}

function updateLastUpdate() {
    const lastUpdateEl = document.getElementById('lastUpdate');
    if (lastUpdateEl) {
        lastUpdateEl.textContent = 'Обновлено: ' + new Date().toLocaleTimeString('ru-RU');
    }
}

// Автообновление
function startAutoRefresh() {
    refreshInterval = setInterval(() => {
        loadData();
        checkSimulatorStatus();
        loadRouteHistory();
        checkBLEStatus();
        checkMDNSStatus();
    }, 10000);
}

// Проверка статуса BLE
async function checkBLEStatus() {
    try {
        const response = await fetch(`${API_BASE}/api/ble/status`);
        const info = await response.json();

        const badge = document.getElementById('bleStatus');
        if (badge) {
            if (info.running) {
                badge.className = 'badge bg-success';
                badge.textContent = '🔵 BLE: ' + (info.device_name || 'ON');
            } else if (info.supported) {
                badge.className = 'badge bg-warning text-dark';
                badge.textContent = '🔵 BLE: OFF';
            } else {
                badge.className = 'badge bg-secondary';
                badge.textContent = '🔵 BLE: N/A';
            }
        }
    } catch (error) {
        const badge = document.getElementById('bleStatus');
        if (badge) {
            badge.className = 'badge bg-secondary';
            badge.textContent = '🔵 BLE: ?';
        }
    }
}

// Проверка статуса mDNS
async function checkMDNSStatus() {
    try {
        const response = await fetch(`${API_BASE}/api/mdns/status`);
        const info = await response.json();

        const badge = document.getElementById('mdnsStatus');
        if (badge) {
            if (info.running) {
                badge.className = 'badge bg-success';
                badge.textContent = '📡 mDNS: ON';
            } else {
                badge.className = 'badge bg-warning text-dark';
                badge.textContent = '📡 mDNS: OFF';
            }
        }
    } catch (error) {
        const badge = document.getElementById('mdnsStatus');
        if (badge) {
            badge.className = 'badge bg-secondary';
            badge.textContent = '📡 mDNS: ?';
        }
    }
}

// Добавление точек Санкт-Петербурга
function addStPetersburgPoints() {
    const points = [
        { name: 'Эрмитаж', lat: 59.9398, lon: 30.3146, type: 'landmark' },
        { name: 'Исаакиевский собор', lat: 59.9341, lon: 30.3062, type: 'landmark' },
        { name: 'Петропавловская крепость', lat: 59.9497, lon: 30.3162, type: 'landmark' },
        { name: 'Невский проспект', lat: 59.9343, lon: 30.3351, type: 'landmark' },
        { name: 'Казанский собор', lat: 59.9343, lon: 30.3242, type: 'landmark' },
        { name: 'Спас на Крови', lat: 59.9400, lon: 30.3289, type: 'landmark' },
        { name: 'Мариинский театр', lat: 59.9258, lon: 30.2956, type: 'landmark' },
        { name: 'Смольный собор', lat: 59.9490, lon: 30.3951, type: 'landmark' },
        { name: 'Площадь Восстания', lat: 59.9316, lon: 30.3604, type: 'landmark' },
        { name: 'Московский вокзал', lat: 59.9302, lon: 30.3618, type: 'landmark' }
    ];

    const landmarkIcon = L.divIcon({
        className: 'custom-landmark-marker',
        html: '<div style="background: #ff6b6b; width: 12px; height: 12px; border-radius: 50%; border: 2px solid white; box-shadow: 0 0 4px rgba(0,0,0,0.5);"></div>',
        iconSize: [12, 12],
        iconAnchor: [6, 6]
    });

    points.forEach(point => {
        const marker = L.marker([point.lat, point.lon], { icon: landmarkIcon }).addTo(map);
        marker.bindPopup(`
            <div style="min-width: 150px;">
                <h6 style="margin: 0 0 5px 0; color: #ff6b6b;">
                    <i class="bi bi-geo-alt-fill"></i> ${point.name}
                </h6>
                <small class="text-muted">Достопримечательность СПб</small>
            </div>
        `);
    });

    console.log('Added', points.length, 'St. Petersburg landmarks');
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
        setTimeout(loadData, 1000);
    } catch (error) {
        alert('Ошибка генерации события: ' + error.message);
    }
}

// Проверка статуса симуляторов
async function checkSimulatorStatus() {
    try {
        const response = await fetch(`${API_BASE}/api/simulator/status/all`);
        const sims = await response.json();

        const simList = document.getElementById('simList');
        if (!simList || !sims) return;

        simList.innerHTML = sims.map(sim => {
            const statusColor = sim.running ? sim.color : '#666';
            const statusIcon = sim.running ? '🟢' : '⚪';
            const loc = sim.current_location || '—';
            const route = sim.route_name || '—';
            return `
                <div class="device-item d-flex justify-content-between align-items-center" style="border-left: 3px solid ${statusColor};">
                    <div>
                        <strong style="color: ${statusColor};">
                            <i class="bi bi-person-fill"></i> ${sim.name}
                        </strong>
                        <br>
                        <small class="text-muted">
                            <i class="bi bi-geo-alt"></i> ${loc}
                            <span class="ms-1">(${route})</span>
                        </small>
                    </div>
                    <div style="text-align: right;">
                        <span style="color: ${statusColor}; font-weight: bold;">${statusIcon}</span>
                        <br>
                        <small class="text-muted">HR: ${sim.base_heart_rate}</small>
                    </div>
                </div>
            `;
        }).join('');
    } catch (error) {
        console.error('Failed to check simulator status:', error);
    }
}

// Запуск всех симуляторов
async function startAllSims() {
    try {
        await fetch(`${API_BASE}/api/simulator/start/all`, { method: 'POST' });
        setTimeout(checkSimulatorStatus, 500);
    } catch (error) {
        console.error('Failed to start simulators:', error);
    }
}

// Остановка всех симуляторов
async function stopAllSims() {
    try {
        await fetch(`${API_BASE}/api/simulator/stop/all`, { method: 'POST' });
        setTimeout(checkSimulatorStatus, 500);
    } catch (error) {
        console.error('Failed to stop simulators:', error);
    }
}

// Очистка истории всех симуляторов
async function clearRouteHistory() {
    try {
        await fetch(`${API_BASE}/api/simulator/clear-history/all`, { method: 'POST' });
        // Удаляем все polyline
        Object.values(routeLines).forEach(line => map.removeLayer(line));
        routeLines = {};
    } catch (error) {
        console.error('Failed to clear history:', error);
    }
}

// Показать/скрыть маршруты
function toggleRouteLine() {
    showRouteLine = !showRouteLine;
    const btn = document.getElementById('routeToggleText');
    Object.values(routeLines).forEach(line => {
        if (showRouteLine) {
            line.addTo(map);
        } else {
            map.removeLayer(line);
        }
    });
    if (btn) btn.textContent = showRouteLine ? 'Скрыть маршруты' : 'Показать маршруты';
}

// Загрузка и отрисовка маршрутов всех симуляторов
async function loadRouteHistory() {
    try {
        const response = await fetch(`${API_BASE}/api/simulator/history/all`);
        const sims = await response.json();

        if (!sims) return;

        sims.forEach(sim => {
            if (!sim.history || sim.history.length < 2) return;

            // Удаляем старую линию
            if (routeLines[sim.id]) {
                map.removeLayer(routeLines[sim.id]);
            }

            // Удаляем старый маркер симулятора
            if (simMarkers[sim.id]) {
                map.removeLayer(simMarkers[sim.id]);
            }

            const latlngs = sim.history.map(p => [p.lat, p.lon]);

            // Рисуем polyline
            routeLines[sim.id] = L.polyline(latlngs, {
                color: sim.color,
                weight: 2,
                opacity: 0.7,
                dashArray: '6, 4',
                lineCap: 'round'
            });

            if (showRouteLine) {
                routeLines[sim.id].addTo(map);
            }

            // Маркер текущей позиции (конец маршрута)
            const lastPoint = latlngs[latlngs.length - 1];
            const lastHistory = sim.history[sim.history.length - 1];

            const markerHtml = `
                <div style="
                    background: ${sim.color};
                    width: 28px;
                    height: 28px;
                    border-radius: 50%;
                    border: 3px solid white;
                    box-shadow: 0 0 8px rgba(0,0,0,0.5);
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    font-size: 11px;
                    font-weight: bold;
                    color: #1a1a2e;
                ">${sim.id + 1}</div>
            `;

            const icon = L.divIcon({
                html: markerHtml,
                className: `sim-marker-${sim.id}`,
                iconSize: [28, 28],
                iconAnchor: [14, 14]
            });

            simMarkers[sim.id] = L.marker(lastPoint, { icon })
                .addTo(map)
                .bindPopup(`
                    <div style="min-width: 180px;">
                        <h6 style="color: ${sim.color}; margin: 0 0 5px 0;">
                            ${sim.name}
                        </h6>
                        <p class="mb-1"><small><b>Локация:</b> ${lastHistory.name || '—'}</small></p>
                        <p class="mb-0"><small><b>Точек:</b> ${sim.history.length}</small></p>
                    </div>
                `);
        });
    } catch (error) {
        console.error('Failed to load route history:', error);
    }
}

// Сканирование ESP32
async function scanESP32() {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');

    if (statusDiv) {
        statusDiv.className = 'alert alert-warning';
        statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Сканирование WiFi и Bluetooth...';
    }
    if (connectBtn) connectBtn.disabled = true;

    try {
        const [wifiResponse, bleResponse] = await Promise.all([
            fetch(`${API_BASE}/api/esp32/scan`),
            fetch(`${API_BASE}/api/esp32/bluetooth`)
        ]);

        const wifiResult = await wifiResponse.json();
        const bleResult = await bleResponse.json();

        let html = '';
        let found = false;
        let firstDevice = null;

        if (wifiResult.count > 0) {
            found = true;
            firstDevice = wifiResult.auto_connect;
            html += '<div class="mb-3"><strong>📡 WiFi устройства:</strong><br>';
            wifiResult.wifi_devices.forEach(ip => {
                html += `<span class="badge bg-success me-1">${ip}</span>`;
            });
            html += '</div>';
        }

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

        if (statusDiv) {
            if (found) {
                statusDiv.className = 'alert alert-success';
                statusDiv.innerHTML = '<i class="bi bi-check-circle"></i> <strong>Найдено устройств:</strong> ' + (wifiResult.count + bleResult.count) + '<br>' + html;
                if (connectBtn && firstDevice) {
                    connectBtn.disabled = false;
                    connectBtn.onclick = () => {
                        if (firstDevice.includes('.')) {
                            connectToESP32(firstDevice);
                        } else {
                            connectToESP32BLE(firstDevice);
                        }
                    };
                }
            } else {
                statusDiv.className = 'alert alert-warning';
                statusDiv.innerHTML = `<i class="bi bi-exclamation-triangle"></i> Устройства не найдены.<br><small>Убедитесь что ESP32 включен и находится в той же сети или в радиусе Bluetooth</small>`;
                if (connectBtn) connectBtn.disabled = true;
            }
        }
    } catch (error) {
        if (statusDiv) {
            statusDiv.className = 'alert alert-danger';
            statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка сканирования: ${error.message}`;
        }
        if (connectBtn) connectBtn.disabled = true;
    }
}

function quickConnect() {
    const ipInput = document.getElementById('esp32IP');
    if (ipInput && ipInput.value) {
        connectToESP32(ipInput.value);
    }
}

async function connectToESP32(ip) {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');

    if (statusDiv) {
        statusDiv.className = 'alert alert-warning';
        statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к ' + ip + '...';
    }

    try {
        const response = await fetch(`${API_BASE}/api/esp32/connect`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ip: ip })
        });

        const result = await response.json();

        if (response.ok && statusDiv) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `<i class="bi bi-check-circle"></i> <strong>Подключено к ${ip}</strong><br><small>Теперь можно отправлять сообщения через LoRa/Bluetooth</small>`;
            if (connectBtn) {
                connectBtn.className = 'btn btn-sm btn-success';
                connectBtn.innerHTML = '<i class="bi bi-check"></i> Подключено';
                connectBtn.disabled = true;
            }
        } else {
            throw new Error(result.message || 'Ошибка подключения');
        }
    } catch (error) {
        if (statusDiv) {
            statusDiv.className = 'alert alert-danger';
            statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        }
        if (connectBtn) connectBtn.disabled = false;
    }
}

async function connectToESP32BLE(macAddress) {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');

    if (statusDiv) {
        statusDiv.className = 'alert alert-warning';
        statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к Bluetooth устройству ' + macAddress + '...';
    }

    try {
        const response = await fetch(`${API_BASE}/api/esp32/connect`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac: macAddress })
        });

        const result = await response.json();

        if (response.ok && statusDiv) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `<i class="bi bi-bluetooth"></i> <strong>Подключено к ${macAddress}</strong><br><small>Теперь можно отправлять сообщения</small>`;
            if (connectBtn) {
                connectBtn.className = 'btn btn-sm btn-success';
                connectBtn.innerHTML = '<i class="bi bi-check"></i> Подключено';
                connectBtn.disabled = true;
            }
        } else {
            throw new Error(result.message || 'Ошибка подключения');
        }
    } catch (error) {
        if (statusDiv) {
            statusDiv.className = 'alert alert-danger';
            statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        }
        if (connectBtn) connectBtn.disabled = false;
    }
}

// Healbe Functions
async function scanHealbe() {
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');

    if (statusDiv) {
        statusDiv.className = 'alert alert-warning';
        statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Сканирование Bluetooth...';
    }
    if (connectBtn) connectBtn.disabled = true;

    try {
        const response = await fetch(`${API_BASE}/api/healbe/scan`);
        const result = await response.json();

        if (result.count > 0 && result.devices.length > 0 && statusDiv) {
            const device = result.devices[0];
            const macInput = document.getElementById('healbeMAC');
            if (macInput) macInput.value = device.address;

            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `<i class="bi bi-check-circle"></i> <strong>Найдено:</strong> ${device.name}<br><small>MAC: ${device.address} (RSSI: ${device.rssi} dBm)</small>`;
            if (connectBtn) connectBtn.disabled = false;
        } else if (statusDiv) {
            statusDiv.className = 'alert alert-warning';
            statusDiv.innerHTML = `<i class="bi bi-exclamation-triangle"></i> Часы Healbe не найдены.<br><small>Убедитесь что часы включены и находятся в радиусе Bluetooth</small>`;
        }
    } catch (error) {
        if (statusDiv) {
            statusDiv.className = 'alert alert-danger';
            statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка сканирования: ${error.message}`;
        }
    }
}

async function connectHealbe() {
    const macInput = document.getElementById('healbeMAC');
    const mac = macInput ? macInput.value : '';
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');

    if (!mac) {
        alert('Введите MAC адрес часов');
        return;
    }

    if (statusDiv) {
        statusDiv.className = 'alert alert-warning';
        statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к ' + mac + '...';
    }
    if (connectBtn) connectBtn.disabled = true;

    try {
        const response = await fetch(`${API_BASE}/api/healbe/connect`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac: mac })
        });

        const result = await response.json();

        if (response.ok && statusDiv) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `<i class="bi bi-bluetooth"></i> <strong>Подключено к ${mac}</strong><br><small>Получение данных о пульсе и стрессе...</small>`;
            const connStatus = document.getElementById('healbeConnectionStatus');
            if (connStatus) {
                connStatus.className = 'badge bg-success float-end';
                connStatus.textContent = 'Подключено';
            }

            const forwardEnabled = document.getElementById('healbeForwardMeshtastic');
            if (forwardEnabled && forwardEnabled.checked) {
                await fetch(`${API_BASE}/api/healbe/forward`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ enabled: true })
                });
            }

            startHealbeDataPolling();
        } else {
            throw new Error(result.message || 'Ошибка подключения');
        }
    } catch (error) {
        if (statusDiv) {
            statusDiv.className = 'alert alert-danger';
            statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        }
        if (connectBtn) connectBtn.disabled = false;
    }
}

async function disconnectHealbe() {
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');

    try {
        await fetch(`${API_BASE}/api/healbe/disconnect`, { method: 'POST' });

        if (statusDiv) {
            statusDiv.className = 'alert alert-info';
            statusDiv.innerHTML = '<i class="bi bi-info-circle"></i> Отключено от часов Healbe';
        }
        const connStatus = document.getElementById('healbeConnectionStatus');
        if (connStatus) {
            connStatus.className = 'badge bg-secondary float-end';
            connStatus.textContent = 'Не подключено';
        }
        if (connectBtn) connectBtn.disabled = false;

        stopHealbeDataPolling();

        const heartRateEl = document.getElementById('healbeHeartRate');
        const stressEl = document.getElementById('healbeStress');
        const batteryEl = document.getElementById('healbeBattery');
        const lastUpdateEl = document.getElementById('healbeLastUpdate');
        
        if (heartRateEl) heartRateEl.textContent = '--';
        if (stressEl) stressEl.textContent = '--';
        if (batteryEl) batteryEl.textContent = '--';
        if (lastUpdateEl) lastUpdateEl.textContent = '--';
    } catch (error) {
        if (statusDiv) {
            statusDiv.className = 'alert alert-danger';
            statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка отключения: ${error.message}`;
        }
    }
}

let healbePollingInterval = null;

function startHealbeDataPolling() {
    loadHealbeData();
    healbePollingInterval = setInterval(loadHealbeData, 5000);
}

function stopHealbeDataPolling() {
    if (healbePollingInterval) {
        clearInterval(healbePollingInterval);
        healbePollingInterval = null;
    }
}

async function loadHealbeData() {
    try {
        const response = await fetch(`${API_BASE}/api/healbe/data`);
        if (!response.ok) return;

        const data = await response.json();
        if (data && data.length > 0) {
            const latest = data[0];
            updateHealbeDisplay(latest);
        }
    } catch (error) {
        console.error('Failed to load Healbe data:', error);
    }
}

function updateHealbeDisplay(data) {
    const heartRateEl = document.getElementById('healbeHeartRate');
    const stressEl = document.getElementById('healbeStress');
    const batteryEl = document.getElementById('healbeBattery');
    const lastUpdateEl = document.getElementById('healbeLastUpdate');
    
    if (heartRateEl && data.heart_rate) heartRateEl.textContent = data.heart_rate;
    if (stressEl && data.stress_level !== undefined) {
        const stressLabels = ['Нет', 'Низкий', 'Средний', 'Высокий', 'Критический'];
        stressEl.textContent = stressLabels[data.stress_level] || data.stress_level;
    }
    if (batteryEl && data.battery) batteryEl.textContent = data.battery + '%';
    if (lastUpdateEl && data.timestamp) {
        const time = new Date(data.timestamp).toLocaleTimeString('ru-RU');
        lastUpdateEl.textContent = time;
    }
}

function handleHealbeWebSocket(data) {
    if (data.type === 'healbe_data') {
        updateHealbeDisplay(data.payload);
    }
}

// Экспорт для отладки
window.app = {
    devices: () => devices,
    metrics: () => metrics,
    alerts: () => alerts,
    messages: () => messages,
    refresh: loadData,
    scanESP32: scanESP32,
    connectToESP32: connectToESP32,
    scanHealbe: scanHealbe,
    connectHealbe: connectHealbe,
    disconnectHealbe: disconnectHealbe
};