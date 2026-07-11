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
let selectedDeviceId = null;
let mapUserFocused = false;
let mapInitialFitDone = false;
let peopleSimZoneLayer = null;
let zonePickMode = false;
let zonePickMarker = null;

// Инициализация
document.addEventListener('DOMContentLoaded', () => {
    initMap();
    initTabs();
    initDeviceListClicks();
    initWebSocket();
    loadData();
    startAutoRefresh();
    checkSimulatorStatus();
    addStPetersburgPoints();
    loadPeopleSimZone();

    const fitAllBtn = document.getElementById('fitAllDevicesBtn');
    if (fitAllBtn) {
        fitAllBtn.addEventListener('click', fitAllDevicesOnMap);
    }

    initHealbeTab();
    initPeopleSimZoneUI();
    loadSystemStatus();
    setInterval(loadSystemStatus, 10000);
});

// Карта
function initMap() {
    map = L.map('map').setView([59.9343, 30.3351], 12);

    // CARTO Voyager — цветная карта (dark_all был ч/б для тёмной темы UI)
    L.tileLayer('https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', {
        attribution: '&copy; OpenStreetMap &copy; CARTO',
        subdomains: 'abcd',
        maxZoom: 20
    }).addTo(map);
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
    } else if (data.type === 'healbe_data') {
        // Данные от часов Healbe
        handleHealbeWebSocket(data);
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
            const isSimPerson = (device.node_id || '').startsWith('!SIM_P');
            if (!markers[device.id]) {
                const markerOptions = isSimPerson
                    ? { icon: L.divIcon({
                        className: 'sim-person-marker',
                        html: '<div style="background:#4ecca3;width:14px;height:14px;border-radius:50%;border:2px solid #fff;box-shadow:0 0 6px rgba(78,204,163,0.8);"></div>',
                        iconSize: [14, 14],
                        iconAnchor: [7, 7]
                    }) }
                    : {};
                const marker = L.marker([device.latitude, device.longitude], markerOptions).addTo(map);
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

    // Центрируем карту только при первой загрузке
    if (!mapUserFocused) {
        const validDevices = devices.filter(d => d.latitude && d.longitude);
        if (validDevices.length > 0 && !mapInitialFitDone) {
            const bounds = validDevices.map(d => [d.latitude, d.longitude]);
            map.fitBounds(bounds, { padding: [50, 50] });
            mapInitialFitDone = true;
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

function initDeviceListClicks() {
    document.addEventListener('click', (e) => {
        const item = e.target.closest('[data-device-id]');
        if (!item) {
            return;
        }

        const deviceId = Number(item.dataset.deviceId);
        focusDeviceOnMap(deviceId);
    });
}

function isDeviceOnline(device) {
    return device.last_seen && new Date(device.last_seen) > new Date(Date.now() - 5 * 60 * 1000);
}

function buildDeviceListHTML() {
    if (devices.length === 0) {
        return '<p class="text-muted text-center py-3">Нет клиентов в сети</p>';
    }

    const sortedDevices = [...devices].sort((a, b) => {
        const aOnline = isDeviceOnline(a) ? 1 : 0;
        const bOnline = isDeviceOnline(b) ? 1 : 0;
        if (aOnline !== bOnline) {
            return bOnline - aOnline;
        }
        return (a.name || a.node_id).localeCompare(b.name || b.node_id, 'ru');
    });

    return sortedDevices.map(device => {
        const isOnline = isDeviceOnline(device);
        const hasLocation = Boolean(device.latitude && device.longitude);
        const isActive = selectedDeviceId === device.id;
        const lastSeen = device.last_seen ? new Date(device.last_seen).toLocaleString('ru-RU') : 'Никогда';
        const coordsText = hasLocation
            ? `${device.latitude.toFixed(5)}, ${device.longitude.toFixed(5)}`
            : 'Координаты неизвестны';

        return `
            <div class="device-list-item ${isActive ? 'active' : ''} ${hasLocation ? '' : 'no-location'}"
                 data-device-id="${device.id}"
                 title="${hasLocation ? 'Показать на карте' : 'Нет координат для отображения'}">
                <div class="flex-grow-1">
                    <div class="device-list-name">${escapeHtml(device.name || device.node_id)}</div>
                    <div class="device-list-meta">${escapeHtml(device.node_id)}</div>
                    <div class="device-list-coords">
                        <i class="bi bi-geo-alt"></i> ${coordsText}
                    </div>
                </div>
                <div class="device-list-status">
                    <span class="${isOnline ? 'status-online' : 'status-offline'}">
                        ${isOnline ? '● Онлайн' : '○ Офлайн'}
                    </span>
                    <span class="status-label">${lastSeen}</span>
                </div>
            </div>
        `;
    }).join('');
}

function updateDeviceList() {
    const html = buildDeviceListHTML();
    const overviewList = document.getElementById('deviceList');
    const clientsList = document.getElementById('clientsList');

    if (overviewList) {
        overviewList.innerHTML = html;
    }
    if (clientsList) {
        clientsList.innerHTML = html;
    }
}

function focusDeviceOnMap(deviceId) {
    const device = devices.find(d => d.id === deviceId);
    if (!device) {
        return;
    }

    selectedDeviceId = deviceId;
    updateDeviceList();

    if (!device.latitude || !device.longitude) {
        return;
    }

    mapUserFocused = true;
    map.flyTo([device.latitude, device.longitude], 17, {
        animate: true,
        duration: 0.8
    });

    window.setTimeout(() => {
        if (markers[deviceId]) {
            markers[deviceId].openPopup();
        }
    }, 700);
}

function fitAllDevicesOnMap() {
    selectedDeviceId = null;
    mapUserFocused = false;

    const validDevices = devices.filter(d => d.latitude && d.longitude);
    if (validDevices.length > 0) {
        const bounds = validDevices.map(d => [d.latitude, d.longitude]);
        map.fitBounds(bounds, { padding: [50, 50] });
    }

    updateDeviceList();
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
                    <small class="float-end message-meta">${timestamp}</small>
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
                    <small class="message-meta">${time}</small>
                </div>
                <p class="mb-1">${escapeHtml(alert.message)}</p>
                <small class="message-meta">
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
            <div class="message-item message-${msg.direction}">
                <div class="d-flex justify-content-between align-items-start gap-2">
                    <div class="flex-grow-1">
                        <span class="badge bg-${isInbound ? 'info' : 'success'} mb-2">
                            <i class="bi bi-${isInbound ? 'arrow-down' : 'arrow-up'}"></i>
                            ${isInbound ? 'Входящее' : 'Исходящее'}
                        </span>
                        <p class="mb-2 fw-semibold">${escapeHtml(msg.text)}</p>
                        <div class="message-meta">
                            <i class="bi bi-person-circle"></i> ${escapeHtml(msg.from_node)}
                            <i class="bi bi-arrow-right mx-1"></i>
                            ${escapeHtml(msg.to_node || 'Все')}
                        </div>
                    </div>
                    <small class="message-meta text-nowrap">${time}</small>
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
    refreshInterval = setInterval(() => {
        loadData();
        loadPeopleSimZone();
    }, 10000);
}

// Зона симуляции людей
async function loadPeopleSimZone() {
    try {
        const response = await fetch(`${API_BASE}/api/people-sim/zone`);
        const zone = await response.json();
        if (!zone.center || !zone.radius_m) {
            return;
        }

        fillZoneForm(zone);
        drawPeopleSimZone(zone);

        const statusEl = document.getElementById('zoneStatus');
        if (statusEl) {
            const outside = zone.outside_count ?? 0;
            statusEl.textContent = `Радиус ${zone.radius_km} км · вне зоны: ${outside} · алерт через ${zone.alert_delay_minutes || 5} мин`;
        }
    } catch (error) {
        console.warn('People sim zone not available:', error);
    }
}

function fillZoneForm(zone) {
    const latInput = document.getElementById('zoneLat');
    const lonInput = document.getElementById('zoneLon');
    const radiusInput = document.getElementById('zoneRadius');
    if (latInput && zone.center) latInput.value = zone.center.lat;
    if (lonInput && zone.center) lonInput.value = zone.center.lon;
    if (radiusInput && zone.radius_km) radiusInput.value = zone.radius_km;
}

function drawPeopleSimZone(zone) {
    if (peopleSimZoneLayer) {
        map.removeLayer(peopleSimZoneLayer);
    }

    peopleSimZoneLayer = L.circle([zone.center.lat, zone.center.lon], {
        radius: zone.radius_m,
        color: '#ff6b6b',
        weight: 2,
        fillColor: '#4ecca3',
        fillOpacity: 0.05,
        dashArray: '10 8'
    }).addTo(map);

    peopleSimZoneLayer.bindPopup(
        `<strong>Геозона</strong><br>Радиус: ${zone.radius_km} км<br>Алерт: через ${zone.alert_delay_minutes || 5} мин вне зоны`
    );

    if (zonePickMarker) {
        zonePickMarker.setLatLng([zone.center.lat, zone.center.lon]);
    }
}

function initPeopleSimZoneUI() {
    const pickBtn = document.getElementById('pickZoneCenterBtn');
    const applyBtn = document.getElementById('applyZoneBtn');

    if (pickBtn) {
        pickBtn.addEventListener('click', () => {
            zonePickMode = !zonePickMode;
            pickBtn.classList.toggle('btn-warning', zonePickMode);
            pickBtn.classList.toggle('btn-outline-secondary', !zonePickMode);
            const statusEl = document.getElementById('zoneStatus');
            if (statusEl) {
                statusEl.textContent = zonePickMode
                    ? 'Кликните на карте для выбора центра зоны'
                    : '';
            }
        });
    }

    if (applyBtn) {
        applyBtn.addEventListener('click', applyPeopleSimZone);
    }

    map.on('click', (e) => {
        if (!zonePickMode) return;
        zonePickMode = false;
        if (pickBtn) {
            pickBtn.classList.remove('btn-warning');
            pickBtn.classList.add('btn-outline-secondary');
        }
        document.getElementById('zoneLat').value = e.latlng.lat.toFixed(6);
        document.getElementById('zoneLon').value = e.latlng.lng.toFixed(6);
        if (!zonePickMarker) {
            zonePickMarker = L.marker(e.latlng, {
                icon: L.divIcon({
                    className: 'zone-center-marker',
                    html: '<div style="background:#ff6b6b;width:12px;height:12px;border-radius:50%;border:2px solid #fff;"></div>',
                    iconSize: [12, 12],
                    iconAnchor: [6, 6]
                })
            }).addTo(map);
        } else {
            zonePickMarker.setLatLng(e.latlng);
        }
        const statusEl = document.getElementById('zoneStatus');
        if (statusEl) statusEl.textContent = 'Центр выбран — нажмите «Применить границу»';
    });
}

async function applyPeopleSimZone() {
    const lat = parseFloat(document.getElementById('zoneLat')?.value);
    const lon = parseFloat(document.getElementById('zoneLon')?.value);
    const radiusKm = parseFloat(document.getElementById('zoneRadius')?.value);
    const statusEl = document.getElementById('zoneStatus');

    if (!lat || !lon || !radiusKm || radiusKm <= 0) {
        if (statusEl) statusEl.textContent = 'Заполните координаты и радиус';
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/api/people-sim/zone`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ lat, lon, radius_km: radiusKm })
        });
        const result = await response.json();
        if (!response.ok) {
            throw new Error(result.message || 'Ошибка сохранения');
        }
        if (result.zone) {
            drawPeopleSimZone(result.zone);
            fillZoneForm(result.zone);
        }
        if (statusEl) statusEl.textContent = result.message || 'Граница обновлена';
        loadData();
    } catch (error) {
        if (statusEl) statusEl.textContent = 'Ошибка: ' + error.message;
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
    connectToESP32: connectToESP32,
    scanHealbe: scanHealbe,
    connectHealbe: connectHealbe,
    disconnectHealbe: disconnectHealbe
};

// ===== System / Demo =====

async function loadSystemStatus() {
    try {
        const response = await fetch(`${API_BASE}/api/system/status`);
        const status = await response.json();

        const demoBadge = document.getElementById('demoBadge');
        const esp32Badge = document.getElementById('esp32Badge');
        const demoBanner = document.getElementById('demoBanner');

        if (status.demo_mode) {
            if (demoBadge) demoBadge.classList.remove('d-none');
            if (demoBanner) {
                demoBanner.classList.remove('d-none');
                demoBanner.textContent = status.demo_message || 'Демо-режим активен';
            }
            const modeSelect = document.getElementById('healbeViaMode');
            if (modeSelect) {
                modeSelect.value = 'demo';
            }
            startHealbeDataPolling();
            startHealbeStatusPolling();
        } else if (demoBadge) {
            demoBadge.classList.add('d-none');
        }

        if (esp32Badge) {
            if (status.serial_connected) {
                esp32Badge.classList.remove('d-none');
                esp32Badge.textContent = 'ESP32 ' + (status.serial_port || 'USB');
                esp32Badge.className = 'badge bg-info text-dark';
            } else {
                esp32Badge.classList.remove('d-none');
                esp32Badge.textContent = 'ESP32 не подключён';
                esp32Badge.className = 'badge bg-secondary';
            }
        }
    } catch (error) {
        console.error('Failed to load system status:', error);
    }
}

// ===== Healbe GoBe Functions =====

function normalizeHealbeMAC(mac) {
    const cleaned = mac.trim().replace(/[^0-9a-fA-F]/g, '').toUpperCase();
    if (cleaned.length !== 12) {
        return mac.trim().toUpperCase();
    }
    return cleaned.match(/.{1,2}/g).join(':');
}

function isValidHealbeMAC(mac) {
    return /^([0-9A-F]{2}:){5}[0-9A-F]{2}$/.test(normalizeHealbeMAC(mac));
}

function updateHealbeConnectButton() {
    const macInput = document.getElementById('healbeMAC');
    const connectBtn = document.getElementById('healbeConnectBtn');
    if (!macInput || !connectBtn) {
        return;
    }
    connectBtn.disabled = !isValidHealbeMAC(macInput.value);
}

function getHealbeViaMode() {
    return document.getElementById('healbeViaMode')?.value || 'auto';
}

function healbeViaLabel(mode) {
    switch (mode) {
        case 'demo': return 'Демо';
        case 'pc': return 'Bluetooth ПК';
        case 'esp32': return 'ESP32 bridge';
        default: return 'Авто';
    }
}

function initHealbeTab() {
    const macInput = document.getElementById('healbeMAC');
    if (!macInput) {
        return;
    }

    const savedMAC = localStorage.getItem('healbeMAC');
    const savedMode = localStorage.getItem('healbeViaMode');
    if (savedMAC) {
        macInput.value = savedMAC;
    } else if (!macInput.value.trim()) {
        macInput.value = '8B:20:91:8E:F5:CB';
    }
    if (savedMode) {
        const modeSelect = document.getElementById('healbeViaMode');
        if (modeSelect) {
            modeSelect.value = savedMode;
        }
    }

    macInput.addEventListener('input', () => {
        macInput.value = normalizeHealbeMAC(macInput.value);
        updateHealbeConnectButton();
    });

    const modeSelect = document.getElementById('healbeViaMode');
    if (modeSelect) {
        modeSelect.addEventListener('change', () => {
            localStorage.setItem('healbeViaMode', modeSelect.value);
        });
    }

    updateHealbeConnectButton();
    loadHealbeStatus();
    startHealbeStatusPolling();
}

// Сканирование часов Healbe
async function scanHealbe() {
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');
    const macInput = document.getElementById('healbeMAC');
    const viaMode = getHealbeViaMode();
    const mac = normalizeHealbeMAC(macInput?.value || '');

    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = viaMode === 'esp32'
        ? '<i class="bi bi-hourglass-split"></i> Сканирование Bluetooth на ПК (для ESP32 bridge не обязательно)...'
        : '<i class="bi bi-hourglass-split"></i> Сканирование Bluetooth на ПК...';

    try {
        const scanURL = mac
            ? `${API_BASE}/api/healbe/scan?mac=${encodeURIComponent(mac)}`
            : `${API_BASE}/api/healbe/scan`;
        const response = await fetch(scanURL);
        const result = await response.json();

        if (result.count > 0 && result.devices.length > 0) {
            const device = result.devices[0];
            macInput.value = device.address;
            localStorage.setItem('healbeMAC', device.address);

            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `
                <i class="bi bi-check-circle"></i> <strong>Найдено:</strong> ${device.name}<br>
                <small>MAC: ${device.address} (RSSI: ${device.rssi} dBm)</small>
            `;
        } else if ((viaMode === 'auto' || viaMode === 'esp32') && isValidHealbeMAC(mac)) {
            statusDiv.className = 'alert alert-info';
            statusDiv.innerHTML = `
                <i class="bi bi-info-circle"></i> Часы не видны с ПК, но MAC указан.<br>
                <small>${result.message || 'Нажмите «Подключить» — в режиме Авто будет использован доступный канал.'}</small>
            `;
        } else {
            statusDiv.className = 'alert alert-warning';
            statusDiv.innerHTML = `
                <i class="bi bi-exclamation-triangle"></i> ${result.message || 'Часы Healbe не найдены.'}<br>
                <small>${viaMode === 'pc' ? 'Убедитесь что часы включены и находятся рядом с ПК' : 'Введите MAC вручную и нажмите «Подключить»'}</small>
            `;
        }
        updateHealbeConnectButton();
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка сканирования: ${error.message}`;
        updateHealbeConnectButton();
    }
}

// Подключение к часам Healbe
async function connectHealbe() {
    const mac = normalizeHealbeMAC(document.getElementById('healbeMAC').value);
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');

    if (!isValidHealbeMAC(mac)) {
        alert('Введите корректный MAC адрес часов (AA:BB:CC:DD:EE:FF)');
        return;
    }

    localStorage.setItem('healbeMAC', mac);
    localStorage.setItem('healbeViaMode', getHealbeViaMode());
    document.getElementById('healbeMAC').value = mac;

    const viaMode = getHealbeViaMode();

    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к ' + mac + ' (' + healbeViaLabel(viaMode) + ')...';
    connectBtn.disabled = true;

    try {
        const response = await fetch(`${API_BASE}/api/healbe/connect`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac: mac, via: viaMode })
        });

        const result = await response.json();

        if (response.ok) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = `
                <i class="bi bi-bluetooth"></i> <strong>${result.message || 'Подключено'}</strong><br>
                <small>MAC: ${mac}${result.via ? ' · канал: ' + result.via : ''}${result.resolved ? ' · ' + result.resolved : ''}</small>
            `;
            document.getElementById('healbeConnectionStatus').className = 'badge bg-success float-end';
            document.getElementById('healbeConnectionStatus').textContent = 'Подключено';

            // Включаем пересылку в Meshtastic если checkbox отмечен
            const forwardEnabled = document.getElementById('healbeForwardMeshtastic').checked;
            if (forwardEnabled) {
                await fetch(`${API_BASE}/api/healbe/forward`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ enabled: true })
                });
            }

            // Начинаем polling данных
            startHealbeDataPolling();
            loadHealbeStatus();
        } else {
            throw new Error(result.message || 'Ошибка подключения');
        }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        connectBtn.disabled = false;
    }
}

// Отключение от часов Healbe
async function disconnectHealbe() {
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');

    try {
        await fetch(`${API_BASE}/api/healbe/disconnect`, { method: 'POST' });

        statusDiv.className = 'alert alert-info';
        statusDiv.innerHTML = '<i class="bi bi-info-circle"></i> Отключено от часов Healbe';
        document.getElementById('healbeConnectionStatus').className = 'badge bg-secondary float-end';
        document.getElementById('healbeConnectionStatus').textContent = 'Не подключено';
        connectBtn.disabled = false;

        // Останавливаем polling
        stopHealbeDataPolling();
        stopHealbeStatusPolling();

        // Сбрасываем отображение данных
        document.getElementById('healbeHeartRate').textContent = '--';
        document.getElementById('healbeStress').textContent = '--';
        document.getElementById('healbeBattery').textContent = '--';
        document.getElementById('healbeLastUpdate').textContent = '--';
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка отключения: ${error.message}`;
    }
}

// Polling данных Healbe
let healbePollingInterval = null;
let healbeStatusInterval = null;

function startHealbeStatusPolling() {
    if (healbeStatusInterval) {
        return;
    }
    healbeStatusInterval = setInterval(loadHealbeStatus, 5000);
}

function stopHealbeStatusPolling() {
    if (healbeStatusInterval) {
        clearInterval(healbeStatusInterval);
        healbeStatusInterval = null;
    }
}

async function loadHealbeStatus() {
    const diagnosticsDiv = document.getElementById('healbeDiagnostics');
    if (!diagnosticsDiv) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/api/healbe/status`);
        const status = await response.json();
        updateHealbeDiagnostics(status);

        const badge = document.getElementById('healbeConnectionStatus');
        if (badge) {
            if (status.connected) {
                badge.className = 'badge bg-success float-end';
                badge.textContent = status.transport_ready ? 'Данные идут' : 'Подключено';
            } else {
                badge.className = 'badge bg-secondary float-end';
                badge.textContent = 'Не подключено';
            }
        }

        const forwardCheckbox = document.getElementById('healbeForwardMeshtastic');
        if (forwardCheckbox && typeof status.forward_enabled === 'boolean') {
            forwardCheckbox.checked = status.forward_enabled;
        }
    } catch (error) {
        diagnosticsDiv.innerHTML = `<div class="text-danger">Ошибка загрузки статуса: ${error.message}</div>`;
    }
}

function updateHealbeDiagnostics(status) {
    const diagnosticsDiv = document.getElementById('healbeDiagnostics');
    if (!diagnosticsDiv) {
        return;
    }

    const staleClass = status.data_stale ? 'text-warning' : 'text-muted';
    const readyClass = status.transport_ready ? 'text-success' : staleClass;

    diagnosticsDiv.innerHTML = `
        <div class="mb-1"><strong>Состояние:</strong> <span class="${readyClass}">${status.reason_text || '—'}</span></div>
        <div class="mb-1"><strong>Канал:</strong> ${status.via || '—'} (режим: ${status.requested_mode || 'auto'})</div>
        <div class="mb-1"><strong>MAC:</strong> ${status.mac || '—'}</div>
        <div class="mb-1"><strong>Mesh:</strong> ${status.mesh_sender || '—'}${status.forward_enabled ? ' · пересылка вкл.' : ''}</div>
        <div class="mb-1"><strong>Bridge URL:</strong> ${status.bridge_url || 'не задан (ingest/ПК)'}</div>
        <div class="mb-0"><strong>Последние данные:</strong> ${status.last_data_at ? new Date(status.last_data_at).toLocaleString('ru-RU') : 'нет'}</div>
    `;
}

function startHealbeDataPolling() {
    loadHealbeData();
    loadHealbeStatus();

    healbePollingInterval = setInterval(() => {
        loadHealbeData();
        loadHealbeStatus();
    }, 5000);
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
    if (data.heart_rate) {
        document.getElementById('healbeHeartRate').textContent = data.heart_rate;
    }
    if (data.stress_level !== undefined) {
        const stressLabels = ['Нет', 'Низкий', 'Средний', 'Высокий', 'Критический'];
        document.getElementById('healbeStress').textContent = stressLabels[data.stress_level] || data.stress_level;
    }
    if (data.battery) {
        document.getElementById('healbeBattery').textContent = data.battery + '%';
    }
    if (data.timestamp) {
        const time = new Date(data.timestamp).toLocaleTimeString('ru-RU');
        document.getElementById('healbeLastUpdate').textContent = time;
    }
}

// Обработка WebSocket сообщений от Healbe
function handleHealbeWebSocket(data) {
    if (data.type === 'healbe_data') {
        updateHealbeDisplay(data.payload);
    }
}
