// СтражСети - Mesh-мониторинг и трекинг

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

// ===== Route Drawing State =====
let drawMode = false;
let drawWaypoints = [];
let drawMarkers = [];
let drawPolyline = null;
let historyLayer = null;
let predictMarker = null;
let predictLine = null;
let savedRoutesLayers = {};

// ===== Movement Trails =====
let deviceTrails = {};        // device_id -> { points: [{lat,lon}], polyline: L.polyline }
let trailEnabled = true;
const MAX_TRAIL_POINTS = 20;  // keep last 20 positions per device
const TRAIL_POLL_INTERVAL = 30000;

// ===== Auth: global fetch wrapper for 401 redirect =====
const _origFetch = window.fetch;
window.fetch = async function(...args) {
    const resp = await _origFetch.apply(this, args);
    if (resp.status === 401 && !args[0].toString().includes('/api/auth/')) {
        window.location.href = '/';
        return resp;
    }
    return resp;
};

// ===== Auth: load current user info =====
let currentUser = null;
async function loadCurrentUser() {
    try {
        const resp = await _origFetch('/api/auth/me');
        if (!resp.ok) {
            window.location.href = '/';
            return;
        }
        currentUser = await resp.json();
        const nameEl = document.getElementById('userNameText');
        if (nameEl) {
            nameEl.textContent = currentUser.name || currentUser.username;
        }
    } catch (e) {
        window.location.href = '/';
    }
}

// Инициализация
document.addEventListener('DOMContentLoaded', () => {
    loadCurrentUser();
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

    // Initialize new tabs
    initRouteTab();
    initMLTab();
    initRouteSelects();
    initOverviewMetrics();
});

// Карта
function initMap() {
    map = L.map('map').setView([59.9343, 30.3351], 12);

    // CARTO Voyager — цветная карта
    L.tileLayer('https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', {
        attribution: '&copy; OpenStreetMap &copy; CARTO',
        subdomains: 'abcd',
        maxZoom: 20
    }).addTo(map);
}

// Accordion sections
function initTabs() {
    // Accordion sections are handled by toggleAccordion onclick
    // Lazy load on first open
    const observer = new MutationObserver(() => {
        document.querySelectorAll('.accordion-section.open').forEach(section => {
            if (section.dataset.loaded) return;
            const id = section.id;
            if (id === 'routes-section') { loadRoutes(); section.dataset.loaded = '1'; }
            if (id === 'ml-section') { loadMLData(); section.dataset.loaded = '1'; }
        });
    });
    document.querySelectorAll('.accordion-section').forEach(section => {
        observer.observe(section, { attributes: true, attributeFilter: ['class'] });
    });
}

function toggleAccordion(header) {
    const section = header.closest('.accordion-section');
    if (!section) return;
    const wasOpen = section.classList.contains('open');
    // Close all sections
    document.querySelectorAll('.accordion-section').forEach(s => s.classList.remove('open'));
    // Toggle this one
    if (!wasOpen) section.classList.add('open');
}

// ===== WebSocket =====
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
        console.log('New message received:', data.message);
        loadData();
    } else if (data.type === 'healbe_data') {
        handleHealbeWebSocket(data);
    } else if (data.type === 'device_position') {
        // Real-time position update from ESP32 serial or MQTT
        handleDevicePosition(data.position, data.source);
        return; // Don't reload all data, just update position
    }

    loadData();
}

function handleDevicePosition(position, source) {
    if (!position || !position.node_id) return;
    console.log('Position update:', position.node_id, position.latitude, position.longitude, 'via', source);

    // Find or create device in local array
    let device = devices.find(d => d.node_id === position.node_id);
    if (device) {
        device.latitude = position.latitude;
        device.longitude = position.longitude;
        device.altitude = position.altitude || 0;
        device.last_seen = new Date().toISOString();
    }

    // Update or create marker with animation
    const existingMarker = Object.values(markers).find(m => {
        const popup = m.getPopup?.();
        return popup && popup.getContent?.().includes(position.node_id);
    });

    // Use device.id if we found the device
    if (device && markers[device.id]) {
        const oldLatLng = markers[device.id].getLatLng();
        const newLatLng = L.latLng(position.latitude, position.longitude);
        const dist = oldLatLng.distanceTo(newLatLng);
        if (dist > 2) {
            addTrailPoint(device.id, position.latitude, position.longitude);
        }
        if (dist > 5) {
            animateMarker(markers[device.id], oldLatLng, newLatLng, Math.min(dist * 15, 3000));
        } else {
            markers[device.id].setLatLng(newLatLng);
        }
    }
}

// ===== Data Loading =====
async function loadData() {
    try {
        await Promise.all([
            loadDevices(),
            loadMetrics(),
            loadAlerts(),
            loadMessages()
        ]);
        renderOverviewMetrics();
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
    // Update route/ML/overview selects with new devices
    updateRouteSelects();
    updateOverviewDeviceSelect();
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

// ===== Map Markers & Movement Trails =====
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
                // Init trail for sim persons
                if (isSimPerson) {
                    initDeviceTrail(device.id, device.latitude, device.longitude);
                }
            } else {
                const oldLatLng = markers[device.id].getLatLng();
                const newLatLng = L.latLng(device.latitude, device.longitude);
                const dist = oldLatLng.distanceTo(newLatLng);
                // Update trail for sim persons
                if (isSimPerson && dist > 2) {
                    addTrailPoint(device.id, device.latitude, device.longitude);
                }
                // Smooth animation for 30-second polling interval
                if (dist > 5) {
                    const animDuration = Math.min(dist * 15, 3000);
                    animateMarker(markers[device.id], oldLatLng, newLatLng, Math.max(animDuration, 1500));
                } else {
                    markers[device.id].setLatLng(newLatLng);
                }
                markers[device.id].setPopupContent(createDevicePopup(device));
            }
        }
    });

    // Remove markers and trails for removed devices
    Object.keys(markers).forEach(id => {
        if (!devices.find(d => d.id == id)) {
            map.removeLayer(markers[id]);
            delete markers[id];
            removeDeviceTrail(id);
        }
    });

    // Ensure trail toggle button exists on the map
    ensureTrailToggle();

    // Center map only on first load
    if (!mapUserFocused) {
        const validDevices = devices.filter(d => d.latitude && d.longitude);
        if (validDevices.length > 0 && !mapInitialFitDone) {
            const bounds = validDevices.map(d => [d.latitude, d.longitude]);
            map.fitBounds(bounds, { padding: [50, 50] });
            mapInitialFitDone = true;
        }
    }
}

// ===== Smooth Marker Animation =====
let animationFrames = {};

function animateMarker(marker, from, to, durationMs) {
    const markerId = Object.keys(markers).find(k => markers[k] === marker) || 'anim';
    if (animationFrames[markerId]) {
        cancelAnimationFrame(animationFrames[markerId]);
    }
    const start = performance.now();
    function frame(time) {
        const t = Math.min((time - start) / durationMs, 1);
        // Ease-out cubic for smoother movement
        const ease = 1 - Math.pow(1 - t, 3);
        const lat = from.lat + (to.lat - from.lat) * ease;
        const lng = from.lng + (to.lng - from.lng) * ease;
        marker.setLatLng([lat, lng]);
        if (t < 1) {
            animationFrames[markerId] = requestAnimationFrame(frame);
        } else {
            delete animationFrames[markerId];
        }
    }
    animationFrames[markerId] = requestAnimationFrame(frame);
}

function animateMarkerAlongPath(marker, points, durationMs) {
    if (!points || points.length < 2) return;
    const markerId = Object.keys(markers).find(k => markers[k] === marker) || 'path';
    if (animationFrames[markerId]) {
        cancelAnimationFrame(animationFrames[markerId]);
    }
    const start = performance.now();
    const totalTime = durationMs || 2000;
    function frame(time) {
        const elapsed = time - start;
        const t = Math.min(elapsed / totalTime, 1);
        const idx = t * (points.length - 1);
        const i0 = Math.floor(idx);
        const i1 = Math.min(i0 + 1, points.length - 1);
        const frac = idx - i0;
        if (points[i0] && points[i1]) {
            const lat = points[i0].lat + (points[i1].lat - points[i0].lat) * frac;
            const lng = points[i0].lon + (points[i1].lon - points[i0].lon) * frac;
            marker.setLatLng([lat, lng]);
        }
        if (t < 1) {
            animationFrames[markerId] = requestAnimationFrame(frame);
        } else if (points[points.length-1]) {
            marker.setLatLng([points[points.length-1].lat, points[points.length-1].lon]);
            delete animationFrames[markerId];
        }
    }
    animationFrames[markerId] = requestAnimationFrame(frame);
}


// ===== Movement Trail Management =====
function initDeviceTrail(deviceId, lat, lon) {
    if (deviceTrails[deviceId]) {
        removeDeviceTrail(deviceId);
    }
    const polyline = L.polyline([[lat, lon]], {
        color: '#4ecca3',
        weight: 2,
        opacity: 0.35,
        dashArray: '6 4',
        lineCap: 'round',
        lineJoin: 'round'
    }).addTo(map);
    deviceTrails[deviceId] = {
        points: [{ lat, lon, ts: Date.now() }],
        polyline: polyline
    };
}

function addTrailPoint(deviceId, lat, lon) {
    let trail = deviceTrails[deviceId];
    if (!trail) {
        initDeviceTrail(deviceId, lat, lon);
        trail = deviceTrails[deviceId];
    }
    trail.points.push({ lat, lon, ts: Date.now() });
    if (trail.points.length > MAX_TRAIL_POINTS) {
        trail.points = trail.points.slice(-MAX_TRAIL_POINTS);
    }
    const latlngs = trail.points.map(p => [p.lat, p.lon]);
    trail.polyline.setLatLngs(latlngs);
    const count = trail.points.length;
    const opacity = Math.min(0.5, 0.15 + (count / MAX_TRAIL_POINTS) * 0.35);
    trail.polyline.setStyle({ opacity: opacity });
}

function removeDeviceTrail(deviceId) {
    if (deviceTrails[deviceId]) {
        if (deviceTrails[deviceId].polyline) {
            map.removeLayer(deviceTrails[deviceId].polyline);
        }
        delete deviceTrails[deviceId];
    }
}

function clearAllTrails() {
    Object.keys(deviceTrails).forEach(id => removeDeviceTrail(id));
}

function ensureTrailToggle() {
    if (document.getElementById('trailToggleBtn')) return;
    const control = L.control({ position: 'bottomleft' });
    control.onAdd = function() {
        const div = L.DomUtil.create('div', 'leaflet-bar leaflet-control');
        div.innerHTML = '<button id="trailToggleBtn" title="Вкл/Выкл следы перемещения" style="width:36px;height:36px;background:#1a1a2e;border:1px solid #2d4a7a;border-radius:4px;cursor:pointer;display:flex;align-items:center;justify-content:center;color:#4ecca3;font-size:18px;line-height:1;">▶▶</button>';
        return div;
    };
    control.addTo(map);
    setTimeout(() => {
        const btn = document.getElementById('trailToggleBtn');
        if (btn) {
            btn.addEventListener('click', () => {
                trailEnabled = !trailEnabled;
                Object.values(deviceTrails).forEach(t => {
                    if (t.polyline) {
                        if (trailEnabled) map.addLayer(t.polyline);
                        else map.removeLayer(t.polyline);
                    }
                });
                btn.style.color = trailEnabled ? '#4ecca3' : '#666';
                btn.title = trailEnabled ? 'Скрыть следы' : 'Показать следы';
            });
        }
    }, 100);
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
        if (!item) return;
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
        if (aOnline !== bOnline) return bOnline - aOnline;
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
    const clientsList = document.getElementById('clientsList');
    if (clientsList) clientsList.innerHTML = html;

    // Render combined device list with metrics for overview
    renderOverviewDeviceList();
}

function focusDeviceOnMap(deviceId) {
    const device = devices.find(d => d.id === deviceId);
    if (!device) return;
    selectedDeviceId = deviceId;
    updateDeviceList();
    if (!device.latitude || !device.longitude) return;
    mapUserFocused = true;
    map.flyTo([device.latitude, device.longitude], 17, { animate: true, duration: 0.8 });
    window.setTimeout(() => {
        if (markers[deviceId]) markers[deviceId].openPopup();
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

// ===== Metrics =====
function updateMetricsDisplay() {
    // Re-render the overview metrics panel for the selected device
    renderOverviewMetrics();
}

function initOverviewMetrics() {
    const select = document.getElementById('overviewDeviceSelect');
    if (select) {
        select.addEventListener('change', () => {
            renderOverviewMetrics();
        });
    }
}

function updateOverviewDeviceSelect() {
    const select = document.getElementById('overviewDeviceSelect');
    if (!select) return;
    const currentVal = select.value;
    select.innerHTML = '<option value="">— Все устройства (обзор) —</option>';
    devices.forEach(d => {
        const latest = metrics.find(m => m.device_id == d.id);
        const hasMetrics = latest && (latest.heart_rate || latest.co2 || latest.temp);
        const icon = hasMetrics ? '🟢' : '⚪';
        select.innerHTML += `<option value="${d.id}">${icon} ${escapeHtml(d.name || d.node_id)}</option>`;
    });
    if (currentVal) select.value = currentVal;
}

function renderOverviewMetrics() {
    const container = document.getElementById('overviewMetricsContent');
    if (!container) return;

    const select = document.getElementById('overviewDeviceSelect');
    const selectedId = select?.value;

    if (!selectedId) {
        container.innerHTML = '';
        renderOverviewDeviceList();
        return;
    }

    renderDeviceDetailMetrics(container, parseInt(selectedId));
    renderOverviewDeviceList(parseInt(selectedId));
}

function initOverviewMetrics() {
    const select = document.getElementById('overviewDeviceSelect');
    if (select) {
        select.addEventListener('change', () => {
            renderOverviewMetrics();
        });
    }
}

function updateOverviewDeviceSelect() {
    const select = document.getElementById('overviewDeviceSelect');
    if (!select) return;
    const currentVal = select.value;
    select.innerHTML = '<option value="">— Все устройства —</option>';
    devices.forEach(d => {
        const latest = metrics.find(m => m.device_id == d.id);
        const hasMetrics = latest && (latest.heart_rate || latest.co2 || latest.temp);
        const icon = hasMetrics ? '🟢' : '⚪';
        select.innerHTML += `<option value="${d.id}">${icon} ${escapeHtml(d.name || d.node_id)}</option>`;
    });
    if (currentVal) select.value = currentVal;
}

function renderOverviewDeviceList(highlightId) {
    const container = document.getElementById('overviewDeviceList');
    if (!container) return;

    const sorted = [...devices].sort((a, b) => {
        const aOnline = isDeviceOnline(a) ? 1 : 0;
        const bOnline = isDeviceOnline(b) ? 1 : 0;
        if (aOnline !== bOnline) return bOnline - aOnline;
        return (a.name || a.node_id).localeCompare(b.name || b.node_id, 'ru');
    });

    container.innerHTML = sorted.map(device => {
        const isOnline = isDeviceOnline(device);
        const isActive = selectedDeviceId === device.id;
        const isHL = highlightId === device.id;
        const dm = metrics.find(m => m.device_id == device.id);
        const ls = device.last_seen ? new Date(device.last_seen).toLocaleString('ru-RU') : '';

        return `
            <div class="overview-device-item"
                 data-device-id="${device.id}"
                 style="padding:0.55rem 0.65rem;margin-bottom:0.3rem;border:1px solid ${isHL ? 'var(--accent)' : 'var(--border-color)'};
                        border-radius:0.45rem;background:${isHL ? 'rgba(34,211,167,0.08)' : 'rgba(11,26,48,0.4)'};
                        cursor:pointer;transition:all 0.15s ease;"
                 onclick="focusDeviceOnMap(${device.id});document.getElementById('overviewDeviceSelect').value='${device.id}';renderOverviewMetrics();">
                <div class="d-flex justify-content-between align-items-center">
                    <div class="flex-grow-1">
                        <span style="font-weight:600;font-size:0.85rem;color:var(--text-primary);">
                            ${isOnline ? '<span style="color:var(--accent);">●</span>' : '<span style="color:#ef4444;">○</span>'}
                            ${escapeHtml(device.name || device.node_id)}
                        </span>
                    </div>
                    ${dm ? `
                    <div class="d-flex gap-2" style="font-size:0.7rem;">
                        ${dm.heart_rate ? `<span style="color:#ef4444;"><i class="bi bi-heart-pulse"></i> ${dm.heart_rate}</span>` : ''}
                        ${dm.co2 ? `<span style="color:#38bdf8;"><i class="bi bi-wind"></i> ${dm.co2}</span>` : ''}
                        ${dm.temp ? `<span style="color:#fbbf24;"><i class="bi bi-thermometer-half"></i> ${dm.temp.toFixed(1)}°</span>` : ''}
                        ${dm.humidity ? `<span style="color:#a855f7;"><i class="bi bi-droplet"></i> ${dm.humidity.toFixed(0)}%</span>` : ''}
                    </div>
                    ` : ''}
                </div>
                <div style="font-size:0.68rem;color:var(--text-muted);margin-top:1px;">
                    ${escapeHtml(device.node_id)}${ls ? ' · ' + ls : ''}
                </div>
            </div>
        `;
    }).join('');
}

function renderDeviceDetailMetrics(container, deviceId) {
    const device = devices.find(d => d.id === deviceId);
    const deviceMetrics = metrics.filter(m => m.device_id == deviceId);

    if (!device) {
        container.innerHTML = '<p class="small text-muted text-center mb-0">Устройство не найдено</p>';
        return;
    }

    if (deviceMetrics.length === 0) {
        container.innerHTML = `
            <div class="text-center py-3">
                <div style="font-size:1.5rem;color:var(--text-muted);"><i class="bi bi-activity"></i></div>
                <p class="small text-muted mb-0">Нет метрик для ${escapeHtml(device.name || device.node_id)}</p>
            </div>`;
        return;
    }

    const latest = deviceMetrics[0];
    const timestamp = latest.timestamp ? new Date(latest.timestamp).toLocaleString('ru-RU') : '';
    const isOnline = device.last_seen && new Date(device.last_seen) > new Date(Date.now() - 5 * 60 * 1000);

    // Build history sparkline data for last 10 readings
    const heartRates = deviceMetrics.slice(0, 10).reverse().map(m => m.heart_rate || 0);
    const temps = deviceMetrics.slice(0, 10).reverse().map(m => m.temp || 0);
    const co2s = deviceMetrics.slice(0, 10).reverse().map(m => m.co2 || 0);

    container.innerHTML = `
        <div class="d-flex align-items-center justify-content-between mb-2">
            <div>
                <strong style="font-size:0.95rem;">${escapeHtml(device.name || device.node_id)}</strong>
                <small class="text-muted ms-2">${escapeHtml(device.node_id)}</small>
            </div>
            <span class="${isOnline ? 'status-online' : 'status-offline'}" style="font-size:0.8rem;">
                ${isOnline ? '● Онлайн' : '○ Офлайн'}
            </span>
        </div>
        <small class="text-muted mb-3 d-block">Обновлено: ${timestamp}</small>

        <!-- Основные показатели -->
        <div class="row text-center g-2 mb-3">
            <div class="col-3">
                <div style="background:rgba(34,211,167,0.08);border-radius:8px;padding:8px 4px;">
                    <div style="font-size:1.3rem;font-weight:700;color:var(--accent);"><i class="bi bi-heart-pulse"></i></div>
                    <div style="font-size:1.2rem;font-weight:700;color:var(--text-primary);">${latest.heart_rate || '—'}</div>
                    <div style="font-size:0.65rem;color:var(--text-secondary);">Пульс</div>
                </div>
            </div>
            <div class="col-3">
                <div style="background:rgba(56,189,248,0.08);border-radius:8px;padding:8px 4px;">
                    <div style="font-size:1.3rem;font-weight:700;color:#38bdf8;"><i class="bi bi-wind"></i></div>
                    <div style="font-size:1.2rem;font-weight:700;color:var(--text-primary);">${latest.co2 || '—'}</div>
                    <div style="font-size:0.65rem;color:var(--text-secondary);">CO₂</div>
                </div>
            </div>
            <div class="col-3">
                <div style="background:rgba(251,191,36,0.08);border-radius:8px;padding:8px 4px;">
                    <div style="font-size:1.3rem;font-weight:700;color:#fbbf24;"><i class="bi bi-thermometer-half"></i></div>
                    <div style="font-size:1.2rem;font-weight:700;color:var(--text-primary);">${latest.temp ? latest.temp.toFixed(1) : '—'}</div>
                    <div style="font-size:0.65rem;color:var(--text-secondary);">Темп.</div>
                </div>
            </div>
            <div class="col-3">
                <div style="background:rgba(168,85,247,0.08);border-radius:8px;padding:8px 4px;">
                    <div style="font-size:1.3rem;font-weight:700;color:#a855f7;"><i class="bi bi-droplet"></i></div>
                    <div style="font-size:1.2rem;font-weight:700;color:var(--text-primary);">${latest.humidity ? latest.humidity.toFixed(0) + '%' : '—'}</div>
                    <div style="font-size:0.65rem;color:var(--text-secondary);">Влажн.</div>
                </div>
            </div>
        </div>

        <!-- Мини-графики (sparklines) -->
        ${heartRates.some(v => v > 0) ? `
        <div class="mb-2">
            <div class="d-flex justify-content-between align-items-center mb-1">
                <small class="text-muted"><i class="bi bi-graph-up"></i> Пульс (последние ${deviceMetrics.length} замеров)</small>
                <small class="text-muted">${Math.min(...heartRates.filter(v=>v>0))}–${Math.max(...heartRates)} bpm</small>
            </div>
            ${renderSparkline(heartRates, 'var(--accent)')}
        </div>
        ` : ''}

        ${co2s.some(v => v > 0) ? `
        <div class="mb-2">
            <div class="d-flex justify-content-between align-items-center mb-1">
                <small class="text-muted"><i class="bi bi-graph-up"></i> CO₂</small>
                <small class="text-muted">${Math.min(...co2s.filter(v=>v>0))}–${Math.max(...co2s)} ppm</small>
            </div>
            ${renderSparkline(co2s, '#38bdf8')}
        </div>
        ` : ''}

        ${temps.some(v => v > 0) ? `
        <div class="mb-2">
            <div class="d-flex justify-content-between align-items-center mb-1">
                <small class="text-muted"><i class="bi bi-graph-up"></i> Температура</small>
                <small class="text-muted">${Math.min(...temps.filter(v=>v>0)).toFixed(1)}–${Math.max(...temps).toFixed(1)} °C</small>
            </div>
            ${renderSparkline(temps, '#fbbf24')}
        </div>
        ` : ''}

        <!-- Координаты -->
        ${device.latitude || device.longitude ? `
        <div class="mt-2 pt-2" style="border-top:1px solid var(--border-color);">
            <small class="text-muted"><i class="bi bi-geo-alt"></i> ${device.latitude?.toFixed(5)}, ${device.longitude?.toFixed(5)}</small>
        </div>
        ` : ''}
    `;
}

function renderSparkline(values, color) {
    if (!values || values.length < 2) return '<div style="height:30px;"></div>';
    const max = Math.max(...values.filter(v => v > 0));
    const min = Math.min(...values.filter(v => v > 0));
    const range = max - min || 1;
    const h = 30;
    const w = 100;
    const step = w / (values.length - 1);

    const points = values.map((v, i) => {
        const x = i * step;
        const y = v > 0 ? h - ((v - min) / range) * h : h;
        return `${x},${y}`;
    }).join(' ');

    return `<svg viewBox="0 0 ${w} ${h}" style="width:100%;height:30px;" preserveAspectRatio="none">
        <polyline points="${points}" fill="none" stroke="${color}" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round"/>
    </svg>`;
}

// ===== Alerts =====
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
    const icons = { 'out_of_zone': '<i class="bi bi-geo-alt-fill"></i>', 'low_battery': '<i class="bi bi-battery-low"></i>', 'signal_lost': '<i class="bi bi-wifi-off"></i>' };
    return icons[type] || '<i class="bi bi-bell-fill"></i>';
}

function getAlertTitle(type) {
    const titles = { 'out_of_zone': 'Вне зоны', 'low_battery': 'Низкий заряд', 'signal_lost': 'Потеря сигнала' };
    return titles[type] || type;
}

async function markAllAlertsRead() {
    await fetch(`${API_BASE}/api/alerts/read-all`, { method: 'PUT' });
    await loadAlerts();
}

// ===== Messages =====
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
    container.scrollTop = container.scrollHeight;
}

document.getElementById('messageForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    let fromNode = document.getElementById('msgFrom').value;
    const toNode = document.getElementById('msgTo').value;
    const text = document.getElementById('msgText').value;
    if (!fromNode) {
        const defaultDevice = devices.find(d => d.node_id === '!default');
        if (defaultDevice) fromNode = defaultDevice.node_id;
        else if (devices.length > 0) fromNode = devices[0].node_id;
        else fromNode = '!default';
        document.getElementById('msgFrom').value = fromNode;
    }
    const device = devices.find(d => d.node_id === fromNode);
    const payload = { device_id: device?.id || 1, from_node: fromNode, to_node: toNode, text: text, direction: 'outbound' };
    try {
        const response = await fetch(`${API_BASE}/api/messages`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
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

// ===== Stats =====
function updateStats() { document.getElementById('deviceCount').textContent = devices.length; }

function updateLastUpdate() {
    document.getElementById('lastUpdate').textContent = 'Обновлено: ' + new Date().toLocaleTimeString('ru-RU');
}

// ===== Auto Refresh =====
function startAutoRefresh() {
    refreshInterval = setInterval(() => { loadData(); loadPeopleSimZone(); }, 30000);
}

// ===== People Sim Zone =====
async function loadPeopleSimZone() {
    try {
        const response = await fetch(`${API_BASE}/api/people-sim/zone`);
        const zone = await response.json();
        if (!zone.center || !zone.radius_m) return;
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
    if (peopleSimZoneLayer) map.removeLayer(peopleSimZoneLayer);
    peopleSimZoneLayer = L.circle([zone.center.lat, zone.center.lon], {
        radius: zone.radius_m, color: '#ff6b6b', weight: 2,
        fillColor: '#4ecca3', fillOpacity: 0.05, dashArray: '10 8'
    }).addTo(map);
    peopleSimZoneLayer.bindPopup(`<strong>Геозона</strong><br>Радиус: ${zone.radius_km} км<br>Алерт: через ${zone.alert_delay_minutes || 5} мин вне зоны`);
    if (zonePickMarker) zonePickMarker.setLatLng([zone.center.lat, zone.center.lon]);
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
            if (statusEl) statusEl.textContent = zonePickMode ? 'Кликните на карте для выбора центра зоны' : '';
        });
    }
    if (applyBtn) applyBtn.addEventListener('click', applyPeopleSimZone);
    map.on('click', (e) => {
        if (!zonePickMode) return;
        zonePickMode = false;
        if (pickBtn) { pickBtn.classList.remove('btn-warning'); pickBtn.classList.add('btn-outline-secondary'); }
        document.getElementById('zoneLat').value = e.latlng.lat.toFixed(6);
        document.getElementById('zoneLon').value = e.latlng.lng.toFixed(6);
        if (!zonePickMarker) {
            zonePickMarker = L.marker(e.latlng, {
                icon: L.divIcon({
                    className: 'zone-center-marker',
                    html: '<div style="background:#ff6b6b;width:12px;height:12px;border-radius:50%;border:2px solid #fff;"></div>',
                    iconSize: [12, 12], iconAnchor: [6, 6]
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
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ lat, lon, radius_km: radiusKm })
        });
        const result = await response.json();
        if (!response.ok) throw new Error(result.message || 'Ошибка сохранения');
        if (result.zone) { drawPeopleSimZone(result.zone); fillZoneForm(result.zone); }
        if (statusEl) statusEl.textContent = result.message || 'Граница обновлена';
        loadData();
    } catch (error) {
        if (statusEl) statusEl.textContent = 'Ошибка: ' + error.message;
    }
}

// ===== St. Petersburg Points =====
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
        iconSize: [12, 12], iconAnchor: [6, 6]
    });
    points.forEach(point => {
        const marker = L.marker([point.lat, point.lon], { icon: landmarkIcon }).addTo(map);
        marker.bindPopup(`<div style="min-width:150px;"><h6 style="margin:0 0 5px 0;color:#ff6b6b;"><i class="bi bi-geo-alt-fill"></i> ${point.name}</h6><small class="text-muted">Достопримечательность СПб</small></div>`);
    });
    console.log('Added', points.length, 'St. Petersburg landmarks');
}

// ===== Utilities =====
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// ===== Simulator Events =====
async function generateEvent(eventType) {
    try {
        await fetch(`${API_BASE}/api/simulator/event`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ type: eventType })
        });
        setTimeout(loadData, 1000);
    } catch (error) { alert('Ошибка генерации события: ' + error.message); }
}

async function checkSimulatorStatus() {
    try {
        const response = await fetch(`${API_BASE}/api/simulator/status`);
        const status = await response.json();
        const badge = document.getElementById('simStatus');
        if (status.running) { badge.className = 'badge bg-success'; badge.textContent = '🟢 Симулятор активен'; }
        else { badge.className = 'badge bg-secondary'; badge.textContent = '⚪ Симулятор остановлен'; }
    } catch (error) { console.error('Failed to check simulator status:', error); }
}

// ===== ESP32 =====
async function scanESP32() {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Сканирование WiFi и Bluetooth...';
    connectBtn.disabled = true;
    try {
        const [wifiResponse, bleResponse] = await Promise.all([
            fetch(`${API_BASE}/api/esp32/scan`),
            fetch(`${API_BASE}/api/esp32/bluetooth`)
        ]);
        const wifiResult = await wifiResponse.json();
        const bleResult = await bleResponse.json();
        let html = ''; let found = false; let firstDevice = null;
        if (wifiResult.count > 0) {
            found = true; firstDevice = wifiResult.auto_connect;
            html += '<div class="mb-3"><strong>📡 WiFi устройства:</strong><br>';
            wifiResult.wifi_devices.forEach(ip => { html += `<span class="badge bg-success me-1">${ip}</span>`; });
            html += '</div>';
        }
        if (bleResult.count > 0) {
            found = true;
            if (!firstDevice && bleResult.esp32_found) firstDevice = bleResult.esp32_mac;
            html += '<div class="mb-3"><strong>📶 Bluetooth устройства:</strong><br>';
            bleResult.devices.forEach(device => {
                const badge = device.isESP32 ? 'bg-primary' : 'bg-secondary';
                html += `<span class="badge ${badge} me-1">${device.isESP32 ? '📱' : '🔵'} ${device.name} (${device.address}) RSSI: ${device.rssi}</span>`;
            });
            html += '</div>';
        }
        if (found) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = '<i class="bi bi-check-circle"></i> <strong>Найдено устройств:</strong> ' + (wifiResult.count + bleResult.count) + '<br>' + html;
            if (firstDevice) {
                connectBtn.disabled = false;
                connectBtn.onclick = () => { if (firstDevice.includes('.')) connectToESP32(firstDevice); else connectToESP32BLE(firstDevice); };
            }
        } else {
            statusDiv.className = 'alert alert-warning';
            statusDiv.innerHTML = '<i class="bi bi-exclamation-triangle"></i> Устройства не найдены.<br><small>Убедитесь что ESP32 включен и находится в той же сети или в радиусе Bluetooth</small>';
            connectBtn.disabled = true;
        }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка сканирования: ${error.message}`;
        connectBtn.disabled = true;
    }
}

function quickConnect() {
    const ip = document.getElementById('esp32IP').value;
    if (ip) connectToESP32(ip);
}

async function connectToESP32(ip) {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к ' + ip + '...';
    try {
        const response = await fetch(`${API_BASE}/api/esp32/connect`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ip: ip })
        });
        const result = await response.json();
        if (response.ok) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = '<i class="bi bi-check-circle"></i> <strong>Подключено к ' + ip + '</strong><br><small>Теперь можно отправлять сообщения через LoRa/Bluetooth</small>';
            connectBtn.className = 'btn btn-sm btn-success';
            connectBtn.innerHTML = '<i class="bi bi-check"></i> Подключено';
            connectBtn.disabled = true;
        } else { throw new Error(result.message || 'Ошибка подключения'); }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        connectBtn.disabled = false;
    }
}

async function connectToESP32BLE(macAddress) {
    const statusDiv = document.getElementById('esp32Status');
    const connectBtn = document.getElementById('quickConnectBtn');
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к Bluetooth устройству ' + macAddress + '...';
    try {
        const response = await fetch(`${API_BASE}/api/esp32/connect`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac: macAddress })
        });
        const result = await response.json();
        if (response.ok) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = '<i class="bi bi-bluetooth"></i> <strong>Подключено к ' + macAddress + '</strong><br><small>Теперь можно отправлять сообщения</small>';
            connectBtn.className = 'btn btn-sm btn-success';
            connectBtn.innerHTML = '<i class="bi bi-check"></i> Подключено';
            connectBtn.disabled = true;
        } else { throw new Error(result.message || 'Ошибка подключения'); }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = `<i class="bi bi-x-circle"></i> Ошибка подключения: ${error.message}`;
        connectBtn.disabled = false;
    }
}

// Debug
window.app = {
    devices: () => devices, metrics: () => metrics, alerts: () => alerts, messages: () => messages,
    refresh: loadData, scanESP32: scanESP32, connectToESP32: connectToESP32,
    scanHealbe: scanHealbe, connectHealbe: connectHealbe, disconnectHealbe: disconnectHealbe, loadRoutes: loadRoutes
};

// ===== System =====
async function loadSystemStatus() {
    try {
        const response = await fetch(`${API_BASE}/api/system/status`);
        const status = await response.json();
        const demoBadge = document.getElementById('demoBadge');
        const esp32Badge = document.getElementById('esp32Badge');
        const demoBanner = document.getElementById('demoBanner');
        if (status.demo_mode) {
            if (demoBadge) demoBadge.classList.remove('d-none');
            if (demoBanner) { demoBanner.classList.remove('d-none'); demoBanner.textContent = status.demo_message || 'Демо-режим активен'; }
            const modeSelect = document.getElementById('healbeViaMode');
            if (modeSelect) modeSelect.value = 'demo';
            startHealbeDataPolling();
            startHealbeStatusPolling();
        } else if (demoBadge) demoBadge.classList.add('d-none');
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
    } catch (error) { console.error('Failed to load system status:', error); }
}

// ===== Healbe =====
function normalizeHealbeMAC(mac) { /* ... same as before ... */ return mac; }
function isValidHealbeMAC(mac) { return /^([0-9A-F]{2}:){5}[0-9A-F]{2}$/.test(normalizeHealbeMAC(mac)); }
function updateHealbeConnectButton() {
    const macInput = document.getElementById('healbeMAC');
    const connectBtn = document.getElementById('healbeConnectBtn');
    if (!macInput || !connectBtn) return;
    connectBtn.disabled = !isValidHealbeMAC(macInput.value);
}
function getHealbeViaMode() { return document.getElementById('healbeViaMode')?.value || 'auto'; }
function healbeViaLabel(mode) { const l = {demo:'Демо', pc:'Bluetooth ПК', esp32:'ESP32 bridge'}; return l[mode]||'Авто'; }

function initHealbeTab() {
    const macInput = document.getElementById('healbeMAC');
    if (!macInput) return;
    const savedMAC = localStorage.getItem('healbeMAC');
    const savedMode = localStorage.getItem('healbeViaMode');
    if (savedMAC) macInput.value = savedMAC;
    else if (!macInput.value.trim()) macInput.value = '8B:20:91:8E:F5:CB';
    if (savedMode) { const m = document.getElementById('healbeViaMode'); if (m) m.value = savedMode; }
    macInput.addEventListener('input', () => { macInput.value = normalizeHealbeMAC(macInput.value); updateHealbeConnectButton(); });
    const modeSelect = document.getElementById('healbeViaMode');
    if (modeSelect) modeSelect.addEventListener('change', () => { localStorage.setItem('healbeViaMode', modeSelect.value); });
    updateHealbeConnectButton();
    loadHealbeStatus();
    startHealbeStatusPolling();
}

async function scanHealbe() {
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');
    const macInput = document.getElementById('healbeMAC');
    const viaMode = getHealbeViaMode();
    const mac = normalizeHealbeMAC(macInput?.value || '');
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = viaMode === 'esp32' ? '<i class="bi bi-hourglass-split"></i> Сканирование Bluetooth на ПК (для ESP32 bridge не обязательно)...' : '<i class="bi bi-hourglass-split"></i> Сканирование Bluetooth на ПК...';
    try {
        const scanURL = mac ? `${API_BASE}/api/healbe/scan?mac=${encodeURIComponent(mac)}` : `${API_BASE}/api/healbe/scan`;
        const response = await fetch(scanURL);
        const result = await response.json();
        if (result.count > 0 && result.devices.length > 0) {
            const device = result.devices[0];
            macInput.value = device.address;
            localStorage.setItem('healbeMAC', device.address);
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = '<i class="bi bi-check-circle"></i> <strong>Найдено:</strong> ' + device.name + '<br><small>MAC: ' + device.address + ' (RSSI: ' + device.rssi + ' dBm)</small>';
        } else if ((viaMode === 'auto' || viaMode === 'esp32') && isValidHealbeMAC(mac)) {
            statusDiv.className = 'alert alert-info';
            statusDiv.innerHTML = '<i class="bi bi-info-circle"></i> Часы не видны с ПК, но MAC указан.<br><small>' + (result.message || 'Нажмите «Подключить» — в режиме Авто будет использован доступный канал.') + '</small>';
        } else {
            statusDiv.className = 'alert alert-warning';
            statusDiv.innerHTML = '<i class="bi bi-exclamation-triangle"></i> ' + (result.message || 'Часы Healbe не найдены.') + '<br><small>' + (viaMode === 'pc' ? 'Убедитесь что часы включены и находятся рядом с ПК' : 'Введите MAC вручную и нажмите «Подключить»') + '</small>';
        }
        updateHealbeConnectButton();
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = '<i class="bi bi-x-circle"></i> Ошибка сканирования: ' + error.message;
        updateHealbeConnectButton();
    }
}

async function connectHealbe() {
    const mac = normalizeHealbeMAC(document.getElementById('healbeMAC').value);
    const statusDiv = document.getElementById('healbeStatus');
    const connectBtn = document.getElementById('healbeConnectBtn');
    if (!isValidHealbeMAC(mac)) { alert('Введите корректный MAC адрес часов (AA:BB:CC:DD:EE:FF)'); return; }
    localStorage.setItem('healbeMAC', mac);
    localStorage.setItem('healbeViaMode', getHealbeViaMode());
    document.getElementById('healbeMAC').value = mac;
    const viaMode = getHealbeViaMode();
    statusDiv.className = 'alert alert-warning';
    statusDiv.innerHTML = '<i class="bi bi-hourglass-split"></i> Подключение к ' + mac + ' (' + healbeViaLabel(viaMode) + ')...';
    connectBtn.disabled = true;
    try {
        const response = await fetch(`${API_BASE}/api/healbe/connect`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac: mac, via: viaMode })
        });
        const result = await response.json();
        if (response.ok) {
            statusDiv.className = 'alert alert-success';
            statusDiv.innerHTML = '<i class="bi bi-bluetooth"></i> <strong>' + (result.message || 'Подключено') + '</strong><br><small>MAC: ' + mac + (result.via ? ' · канал: ' + result.via : '') + (result.resolved ? ' · ' + result.resolved : '') + '</small>';
            document.getElementById('healbeConnectionStatus').className = 'badge bg-success float-end';
            document.getElementById('healbeConnectionStatus').textContent = 'Подключено';
            const forwardEnabled = document.getElementById('healbeForwardMeshtastic').checked;
            if (forwardEnabled) { await fetch(`${API_BASE}/api/healbe/forward`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled: true }) }); }
            startHealbeDataPolling();
            loadHealbeStatus();
        } else { throw new Error(result.message || 'Ошибка подключения'); }
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = '<i class="bi bi-x-circle"></i> Ошибка подключения: ' + error.message;
        connectBtn.disabled = false;
    }
}

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
        stopHealbeDataPolling();
        stopHealbeStatusPolling();
        document.getElementById('healbeHeartRate').textContent = '--';
        document.getElementById('healbeStress').textContent = '--';
        document.getElementById('healbeBattery').textContent = '--';
        document.getElementById('healbeLastUpdate').textContent = '--';
    } catch (error) {
        statusDiv.className = 'alert alert-danger';
        statusDiv.innerHTML = '<i class="bi bi-x-circle"></i> Ошибка отключения: ' + error.message;
    }
}

let healbePollingInterval = null;
let healbeStatusInterval = null;
function startHealbeStatusPolling() { if (!healbeStatusInterval) healbeStatusInterval = setInterval(loadHealbeStatus, 5000); }
function stopHealbeStatusPolling() { if (healbeStatusInterval) { clearInterval(healbeStatusInterval); healbeStatusInterval = null; } }
async function loadHealbeStatus() {
    const diagnosticsDiv = document.getElementById('healbeDiagnostics');
    if (!diagnosticsDiv) return;
    try {
        const response = await fetch(`${API_BASE}/api/healbe/status`);
        const status = await response.json();
        updateHealbeDiagnostics(status);
        const badge = document.getElementById('healbeConnectionStatus');
        if (badge) { badge.className = 'badge ' + (status.connected ? 'bg-success' : 'bg-secondary') + ' float-end'; badge.textContent = status.connected ? (status.transport_ready ? 'Данные идут' : 'Подключено') : 'Не подключено'; }
        const fwd = document.getElementById('healbeForwardMeshtastic');
        if (fwd && typeof status.forward_enabled === 'boolean') fwd.checked = status.forward_enabled;
    } catch (error) { diagnosticsDiv.innerHTML = '<div class="text-danger">Ошибка загрузки статуса: ' + error.message + '</div>'; }
}
function updateHealbeDiagnostics(status) {
    const d = document.getElementById('healbeDiagnostics');
    if (!d) return;
    d.innerHTML = '<div class="mb-1"><strong>Состояние:</strong> <span class="' + (status.transport_ready ? 'text-success' : (status.data_stale ? 'text-warning' : 'text-muted')) + '">' + (status.reason_text || '—') + '</span></div>' +
        '<div class="mb-1"><strong>Канал:</strong> ' + (status.via || '—') + ' (режим: ' + (status.requested_mode || 'auto') + ')</div>' +
        '<div class="mb-1"><strong>MAC:</strong> ' + (status.mac || '—') + '</div>' +
        '<div class="mb-1"><strong>Mesh:</strong> ' + (status.mesh_sender || '—') + (status.forward_enabled ? ' · пересылка вкл.' : '') + '</div>' +
        '<div class="mb-1"><strong>Bridge URL:</strong> ' + (status.bridge_url || 'не задан (ingest/ПК)') + '</div>' +
        '<div class="mb-0"><strong>Последние данные:</strong> ' + (status.last_data_at ? new Date(status.last_data_at).toLocaleString('ru-RU') : 'нет') + '</div>';
}
function startHealbeDataPolling() { loadHealbeData(); loadHealbeStatus(); healbePollingInterval = setInterval(() => { loadHealbeData(); loadHealbeStatus(); }, 5000); }
function stopHealbeDataPolling() { if (healbePollingInterval) { clearInterval(healbePollingInterval); healbePollingInterval = null; } }
async function loadHealbeData() {
    try {
        const response = await fetch(`${API_BASE}/api/healbe/data`);
        if (!response.ok) return;
        const data = await response.json();
        if (data && data.length > 0) updateHealbeDisplay(data[0]);
    } catch (error) { console.error('Failed to load Healbe data:', error); }
}
function updateHealbeDisplay(data) {
    if (data.heart_rate) document.getElementById('healbeHeartRate').textContent = data.heart_rate;
    if (data.stress_level !== undefined) {
        const labels = ['Нет', 'Низкий', 'Средний', 'Высокий', 'Критический'];
        document.getElementById('healbeStress').textContent = labels[data.stress_level] || data.stress_level;
    }
    if (data.battery) document.getElementById('healbeBattery').textContent = data.battery + '%';
    if (data.timestamp) document.getElementById('healbeLastUpdate').textContent = new Date(data.timestamp).toLocaleTimeString('ru-RU');
}
function handleHealbeWebSocket(data) { if (data.type === 'healbe_data') updateHealbeDisplay(data.payload); }

// ===================================================================
// ===== ROUTE TAB =====
// ===================================================================

let drawModeActive = false;
let drawWaypointsList = [];
let drawMarkersList = [];
let drawPolylineLayer = null;
let historyLayerObj = null;
let predictMarkerObj = null;
let predictLineObj = null;

function updateRouteSelects() {
    const selects = ['routeDeviceSelect', 'historyDeviceSelect', 'predictDeviceSelect', 'healthDeviceSelect'];
    selects.forEach(id => {
        const sel = document.getElementById(id);
        if (!sel) return;
        const currentVal = sel.value;
        sel.innerHTML = '<option value="">— Выберите —</option>';
        devices.forEach(d => {
            sel.innerHTML += `<option value="${d.id}">${escapeHtml(d.name || d.node_id)}</option>`;
        });
        if (currentVal) sel.value = currentVal;
    });
}

function initRouteSelects() {
    updateRouteSelects();
    // Update selects when devices load
    const origLoadDevices = loadDevices;
    loadDevices = async function() {
        await origLoadDevices.call(this);
        updateRouteSelects();
    };
}

function initRouteTab() {
    const drawBtn = document.getElementById('drawRouteBtn');
    const saveBtn = document.getElementById('saveRouteBtn');
    const cancelBtn = document.getElementById('cancelDrawBtn');

    if (drawBtn) drawBtn.addEventListener('click', enableDrawMode);
    if (saveBtn) saveBtn.addEventListener('click', saveRoute);
    if (cancelBtn) cancelBtn.addEventListener('click', cancelDrawMode);

    const showHistoryBtn = document.getElementById('showHistoryBtn');
    if (showHistoryBtn) showHistoryBtn.addEventListener('click', showRouteHistory);
}

// ===== Route Drawing =====
function enableDrawMode() {
    if (drawModeActive) return;
    drawModeActive = true;
    drawWaypointsList = [];
    drawMarkersList = [];

    document.getElementById('drawRouteBtn').classList.add('d-none');
    document.getElementById('saveRouteBtn').classList.remove('d-none');
    document.getElementById('cancelDrawBtn').classList.remove('d-none');
    document.getElementById('drawStatus').textContent = '🖱️ Кликните на карте чтобы добавить точки маршрута. Минимум 2 точки.';
    map.getContainer().style.cursor = 'crosshair';
    map.on('click', onDrawMapClick);
}

function onDrawMapClick(e) {
    const lat = e.latlng.lat;
    const lon = e.latlng.lng;
    drawWaypointsList.push({ lat: lat, lon: lon });

    const marker = L.marker([lat, lon], {
        icon: L.divIcon({
            className: 'draw-waypoint',
            html: `<div style="background:#ff6b6b;width:16px;height:16px;border-radius:50%;border:3px solid #fff;box-shadow:0 0 8px rgba(255,107,107,0.8);display:flex;align-items:center;justify-content:center;color:#fff;font-size:9px;font-weight:bold;">${drawWaypointsList.length}</div>`,
            iconSize: [16, 16],
            iconAnchor: [8, 8]
        })
    }).addTo(map);
    marker.bindPopup(`Точка ${drawWaypointsList.length}: ${lat.toFixed(5)}, ${lon.toFixed(5)}`);
    drawMarkersList.push(marker);

    updateDrawPolyline();

    document.getElementById('drawStatus').textContent = `➕ Добавлено точек: ${drawWaypointsList.length}. Нажмите «Сохранить» чтобы завершить.`;
}

function updateDrawPolyline() {
    if (drawPolylineLayer) map.removeLayer(drawPolylineLayer);
    if (drawWaypointsList.length < 2) return;
    const latlngs = drawWaypointsList.map(p => [p.lat, p.lon]);
    drawPolylineLayer = L.polyline(latlngs, {
        color: '#ff6b6b', weight: 3, opacity: 0.8, dashArray: '8 6'
    }).addTo(map);
}

function cancelDrawMode() {
    exitDrawMode();
    document.getElementById('drawStatus').textContent = '✖️ Рисование отменено';
}

function exitDrawMode() {
    drawModeActive = false;
    drawWaypointsList = [];
    drawMarkersList.forEach(m => map.removeLayer(m));
    drawMarkersList = [];
    if (drawPolylineLayer) { map.removeLayer(drawPolylineLayer); drawPolylineLayer = null; }
    map.getContainer().style.cursor = '';
    map.off('click', onDrawMapClick);

    document.getElementById('drawRouteBtn').classList.remove('d-none');
    document.getElementById('saveRouteBtn').classList.add('d-none');
    document.getElementById('cancelDrawBtn').classList.add('d-none');
}

async function saveRoute() {
    if (drawWaypointsList.length < 2) {
        document.getElementById('drawStatus').textContent = '⚠️ Нужно минимум 2 точки для маршрута';
        return;
    }
    const deviceSelect = document.getElementById('routeDeviceSelect');
    const deviceId = parseInt(deviceSelect.value);
    const name = prompt('Название маршрута:', 'Маршрут #' + new Date().toLocaleString('ru-RU'));
    if (!name) return;

    try {
        const response = await fetch(`${API_BASE}/api/routes`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name: name,
                device_id: isNaN(deviceId) ? 0 : deviceId,
                waypoints: drawWaypointsList
            })
        });
        if (!response.ok) throw new Error('Ошибка сохранения');
        document.getElementById('drawStatus').textContent = '✅ Маршрут сохранён!';
        exitDrawMode();
        loadRoutes();
    } catch (error) {
        document.getElementById('drawStatus').textContent = '❌ ' + error.message;
    }
}

// ===== Load & Display Routes =====
async function loadRoutes() {
    try {
        const response = await fetch(`${API_BASE}/api/routes`);
        const routes = await response.json();
        displayRoutesList(routes);
    } catch (error) {
        console.error('Failed to load routes:', error);
    }
}

function displayRoutesList(routes) {
    const container = document.getElementById('savedRoutesList');
    if (!container) return;
    if (!routes || routes.length === 0) {
        container.innerHTML = '<p class="text-muted text-center small py-2">Нет сохранённых маршрутов</p>';
        return;
    }

    container.innerHTML = routes.map((route, idx) => {
        const waypoints = route.waypoints || [];
        const wpCount = Array.isArray(waypoints) ? waypoints.length : 0;
        const time = route.created_at ? new Date(route.created_at).toLocaleString('ru-RU') : '';
        const deviceName = devices.find(d => d.id === route.device_id)?.name || route.device_id || '—';
        return `
            <div class="device-list-item" data-route-id="${route.id}" style="cursor:pointer;">
                <div class="flex-grow-1">
                    <div class="device-list-name">${escapeHtml(route.name)}</div>
                    <div class="device-list-meta">${wpCount} точек · устройство: ${deviceName}</div>
                    <div class="device-list-coords"><small>${time}</small></div>
                </div>
                <div class="device-list-status">
                    <button class="btn btn-sm btn-outline-info me-1 show-route-btn" data-route-id="${route.id}"><i class="bi bi-eye"></i></button>
                    <button class="btn btn-sm btn-outline-danger delete-route-btn" data-route-id="${route.id}"><i class="bi bi-trash"></i></button>
                </div>
            </div>
        `;
    }).join('');

    // Event listeners for show/delete
    container.querySelectorAll('.show-route-btn').forEach(btn => {
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            const id = parseInt(btn.dataset.routeId);
            const route = routes.find(r => r.id === id);
            if (route) showRouteOnMap(route);
        });
    });
    container.querySelectorAll('.delete-route-btn').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const id = parseInt(btn.dataset.routeId);
            if (confirm('Удалить маршрут?')) {
                await fetch(`${API_BASE}/api/routes/delete?id=${id}`, { method: 'DELETE' });
                loadRoutes();
                // Remove from map
                if (savedRoutesLayers[id]) {
                    savedRoutesLayers[id].forEach(l => map.removeLayer(l));
                    delete savedRoutesLayers[id];
                }
            }
        });
    });
    container.querySelectorAll('.device-list-item[data-route-id]').forEach(item => {
        item.addEventListener('click', () => {
            const id = parseInt(item.dataset.routeId);
            const route = routes.find(r => r.id === id);
            if (route) showRouteOnMap(route);
        });
    });
}

function showRouteOnMap(route) {
    const waypoints = route.waypoints || [];
    if (!Array.isArray(waypoints) || waypoints.length < 2) return;

    // Clear previous route layers for this route
    Object.values(savedRoutesLayers).forEach(layers => layers.forEach(l => map.removeLayer(l)));

    const layers = [];
    const latlngs = waypoints.map(wp => [wp.lat, wp.lon]);

    // Draw the polyline
    const polyline = L.polyline(latlngs, {
        color: '#4ecca3', weight: 3, opacity: 0.8
    }).addTo(map);
    polyline.bindPopup(`<strong>${escapeHtml(route.name)}</strong><br>${waypoints.length} точек`);
    layers.push(polyline);

    // Add start/end markers
    const start = waypoints[0];
    const end = waypoints[waypoints.length - 1];
    const startMarker = L.circleMarker([start.lat, start.lon], {
        radius: 8, color: '#4ecca3', fillColor: '#4ecca3', fillOpacity: 1
    }).addTo(map).bindPopup('Старт: ' + route.name);
    layers.push(startMarker);

    const endMarker = L.circleMarker([end.lat, end.lon], {
        radius: 8, color: '#ff6b6b', fillColor: '#ff6b6b', fillOpacity: 1
    }).addTo(map).bindPopup('Финиш: ' + route.name);
    layers.push(endMarker);

    savedRoutesLayers[route.id] = layers;

    // Fit map to show the route
    map.fitBounds(polyline.getBounds(), { padding: [50, 50] });
}

// ===== Route History =====
async function showRouteHistory() {
    const deviceId = parseInt(document.getElementById('historyDeviceSelect').value);
    const dateFrom = document.getElementById('historyDateFrom').value;
    const dateTo = document.getElementById('historyDateTo').value;

    if (!deviceId || isNaN(deviceId)) {
        document.getElementById('historyStatus').textContent = '⚠️ Выберите устройство';
        return;
    }

    const from = dateFrom ? new Date(dateFrom).toISOString() : new Date(Date.now() - 86400000).toISOString();
    const to = dateTo ? new Date(dateTo + 'T23:59:59').toISOString() : new Date().toISOString();

    try {
        const response = await fetch(`${API_BASE}/api/routes/points?device_id=${deviceId}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`);
        const data = await response.json();
        const points = data.points || [];

        document.getElementById('historyStatus').textContent = `📊 Найдено точек: ${data.count}, дистанция: ${(data.total_distance_km || 0).toFixed(2)} км`;

        // Clear previous history layer
        if (historyLayerObj) { historyLayerObj.forEach(l => map.removeLayer(l)); }

        if (points.length < 2) {
            document.getElementById('historyStatus').textContent += ' — недостаточно точек для отображения';
            return;
        }

        const layers = [];
        const latlngs = points.map(p => [p.latitude, p.longitude]);

        const polyline = L.polyline(latlngs, {
            color: '#ffd93d', weight: 2, opacity: 0.6
        }).addTo(map);
        layers.push(polyline);

        // Add dots for each point (sample if too many)
        const step = Math.max(1, Math.floor(points.length / 100));
        for (let i = 0; i < points.length; i += step) {
            const p = points[i];
            const dot = L.circleMarker([p.latitude, p.longitude], {
                radius: 3, color: '#ffd93d', fillColor: '#ffd93d', fillOpacity: 0.5
            }).addTo(map);
            dot.bindPopup(`<small>${new Date(p.timestamp).toLocaleString('ru-RU')}</small>`);
            layers.push(dot);
        }

        historyLayerObj = layers;
        map.fitBounds(polyline.getBounds(), { padding: [50, 50] });
    } catch (error) {
        document.getElementById('historyStatus').textContent = '❌ ' + error.message;
    }
}

// ===================================================================
// ===== ML TAB =====
// ===================================================================

function initMLTab() {
    const predictBtn = document.getElementById('predictBtn');
    if (predictBtn) predictBtn.addEventListener('click', predictPosition);

    const healthSelect = document.getElementById('healthDeviceSelect');
    if (healthSelect) healthSelect.addEventListener('change', loadDeviceAnalysis);
}

function loadMLData() {
    loadAnomalies();
}

async function loadAnomalies() {
    try {
        const response = await fetch(`${API_BASE}/api/ml/anomalies?limit=20`);
        const anomalies = await response.json();
        displayAnomalies(anomalies);
    } catch (error) {
        console.error('Failed to load anomalies:', error);
    }
}

function displayAnomalies(anomalies) {
    const container = document.getElementById('anomaliesList');
    if (!container) return;

    if (!anomalies || anomalies.length === 0) {
        container.innerHTML = `
            <div class="text-center py-3">
                <div class="mb-2" style="font-size:2rem;color:var(--accent);"><i class="bi bi-check-circle"></i></div>
                <p class="text-muted small mb-0">Аномалий не обнаружено ✅</p>
            </div>`;
        return;
    }

    container.innerHTML = anomalies.map(a => {
        const device = devices.find(d => d.id === a.device_id);
        const time = a.timestamp ? new Date(a.timestamp).toLocaleString('ru-RU') : '';
        const severityClass = a.severity === 'critical' ? 'alert-danger' : 'alert-warning';
        const icon = a.type.startsWith('heart') ? '❤️' : a.type.startsWith('co2') ? '💨' : a.type.startsWith('position') ? '📍' : '⚠️';
        return `
            <div class="alert-item ${severityClass}">
                <div class="d-flex justify-content-between">
                    <strong>${icon} ${escapeHtml(a.description)}</strong>
                    <small class="message-meta">${time}</small>
                </div>
                <p class="mb-0 small">
                    Устройство: ${escapeHtml(device?.name || device?.node_id || 'N/A')} ·
                    Тип: ${a.type} ·
                    Значение: ${a.metric_value?.toFixed(1) || '—'}
                </p>
            </div>
        `;
    }).join('');
}

// ===== Position Prediction =====
async function predictPosition() {
    const deviceId = parseInt(document.getElementById('predictDeviceSelect').value);
    if (!deviceId || isNaN(deviceId)) {
        document.getElementById('predictResult').textContent = '⚠️ Выберите устройство';
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/api/ml/predict?device_id=${deviceId}`);
        if (!response.ok) throw new Error('Недостаточно данных для предсказания');
        const data = await response.json();

        // Clear previous prediction
        if (predictMarkerObj) { map.removeLayer(predictMarkerObj); predictMarkerObj = null; }
        if (predictLineObj) { map.removeLayer(predictLineObj); predictLineObj = null; }

        // Current position
        const current = data.current;
        // Predicted position
        const predicted = data.predicted;
        // Smooth path
        const smoothPath = data.smooth_path || [];

        // Draw prediction point
        predictMarkerObj = L.circleMarker([predicted.lat, predicted.lon], {
            radius: 10, color: '#ffd93d', fillColor: '#ffd93d', fillOpacity: 0.6,
            weight: 2
        }).addTo(map);
        predictMarkerObj.bindPopup(`
            <strong>🎯 Предсказанная позиция</strong><br>
            Уверенность: ${(data.confidence * 100).toFixed(0)}%<br>
            Направление: ${data.heading.toFixed(0)}°<br>
            Скорость: ${data.speed_mps?.toFixed(1) || '?'} м/с
        `);

        // Draw smooth path from current to predicted
        if (smoothPath.length >= 2) {
            const latlngs = smoothPath.map(p => [p.lat, p.lon]);
            predictLineObj = L.polyline(latlngs, {
                color: '#ffd93d', weight: 2, opacity: 0.5, dashArray: '5 5'
            }).addTo(map);
        }

        // Animate along the path
        const deviceMarker = markers[deviceId];
        if (deviceMarker && smoothPath.length >= 2) {
            animateMarkerAlongPath(deviceMarker, smoothPath, 3000);
        }

        document.getElementById('predictResult').innerHTML = `
            🎯 Предсказание: (${predicted.lat.toFixed(5)}, ${predicted.lon.toFixed(5)})<br>
            📊 Уверенность: ${(data.confidence * 100).toFixed(0)}% · Направление: ${data.heading.toFixed(0)}°<br>
            <small class="text-muted">⏳ Анимация перемещения запущена</small>
        `;

        // Fly to show both points
        const allPoints = [current, predicted].map(p => [p.lat, p.lon]);
        map.fitBounds(allPoints, { padding: [100, 100] });

    } catch (error) {
        document.getElementById('predictResult').textContent = '❌ ' + error.message;
    }
}

// ===== Device Health Analysis =====
async function loadDeviceAnalysis() {
    const deviceId = parseInt(document.getElementById('healthDeviceSelect').value);
    if (!deviceId || isNaN(deviceId)) return;

    try {
        const response = await fetch(`${API_BASE}/api/ml/analyze?device_id=${deviceId}`);
        const data = await response.json();

        const anomalies = data.anomalies || [];

        // Color indicators based on anomaly count
        const criticalCount = anomalies.filter(a => a.severity === 'critical').length;
        const warningCount = anomalies.filter(a => a.severity === 'warning').length;

        document.getElementById('hrIndicator').className = `badge ${criticalCount > 0 ? 'bg-danger' : warningCount > 0 ? 'bg-warning text-dark' : 'bg-success'}`;
        document.getElementById('hrIndicator').textContent = criticalCount > 0 ? `⚠️ ${criticalCount} аномалий` : warningCount > 0 ? `⚡ ${warningCount} предупреждений` : '✅ Норма';

        document.getElementById('co2Indicator').className = 'badge bg-success';
        document.getElementById('co2Indicator').textContent = '✅ Норма';

        document.getElementById('tempIndicator').className = 'badge bg-success';
        document.getElementById('tempIndicator').textContent = '✅ Норма';

        // Display current anomalies in the health section
        if (anomalies.length > 0) {
            const latest = anomalies[0];
            if (latest.type.startsWith('heart')) {
                document.getElementById('hrIndicator').className = `badge ${latest.severity === 'critical' ? 'bg-danger' : 'bg-warning text-dark'}`;
                document.getElementById('hrIndicator').textContent = `${latest.metric_value?.toFixed(0) || '?'} bpm`;
            }
            if (latest.type.startsWith('co2')) {
                document.getElementById('co2Indicator').className = `badge ${latest.severity === 'critical' ? 'bg-danger' : 'bg-warning text-dark'}`;
                document.getElementById('co2Indicator').textContent = `${latest.metric_value?.toFixed(0) || '?'} ppm`;
            }
        }

    } catch (error) {
        console.error('Failed to load device analysis:', error);
    }
}