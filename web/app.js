'use strict';
// Mesh Pulse — клиент панели мониторинга. Без внешних библиотек.
const $ = id => document.getElementById(id);
const state = { nodes: new Map(), spark: new Map(), alerts: [], gateways: new Map(), cfg: {}, selected: null };
const ALERT_KINDS = { hr_high: 'bad', hr_low: 'bad', spo2_low: 'bad', offline: 'warn', no_contact: 'warn' };
const GW_STATE = { connected: ['ok', 'подключён'], scanning: ['warn', 'поиск…'], found: ['warn', 'найден'],
  connecting: ['warn', 'подключение…'], disconnected: ['bad', 'отключён'], error: ['bad', 'ошибка'] };

async function api(path, opts = {}) {
  const r = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts });
  if (r.status === 401) { location.replace('/login'); throw new Error('unauthorized'); }
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || r.statusText);
  return data;
}

const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const nodeName = n => n.alias || n.long_name || n.node_id;
const ago = t => {
  if (!t) return '—';
  const s = Math.max(0, Math.round((Date.now() - new Date(t)) / 1000));
  return s < 60 ? `${s} с назад` : s < 3600 ? `${Math.floor(s / 60)} мин назад` : new Date(t).toLocaleString('ru-RU');
};

// ---------- состояние узла ----------
function nodeStatus(n) {
  const off = !n.last_health || (Date.now() - new Date(n.last_health)) / 1000 > (state.cfg.offline_after_s || 180);
  if (off) return ['off', 'нет связи'];
  if (n.heart_bpm == null) return ['warn', 'нет контакта'];
  if (n.heart_bpm > state.cfg.hr_high || n.heart_bpm < state.cfg.hr_low) return ['bad', 'пульс вне нормы'];
  if (n.spo2 && n.spo2 < state.cfg.spo2_low) return ['bad', 'низкий SpO₂'];
  return ['ok', 'норма'];
}

function renderCards() {
  const nodes = [...state.nodes.values()].filter(n => n.last_health || n.heart_bpm != null);
  const box = $('cards');
  $('empty').classList.toggle('hidden', nodes.length > 0);
  let online = 0, sum = 0, cnt = 0;
  const seen = new Set();
  for (const n of nodes) {
    seen.add(String(n.num));
    const [cls, label] = nodeStatus(n);
    if (cls !== 'off') online++;
    if (cls !== 'off' && n.heart_bpm != null) { sum += n.heart_bpm; cnt++; }
    let el = box.querySelector(`[data-num="${n.num}"]`);
    if (!el) {
      el = document.createElement('div');
      el.className = 'card'; el.dataset.num = n.num;
      el.innerHTML = `<div class="top"><div><div class="name"></div><div class="id"></div></div><span class="state"></span></div>
        <div class="hr"><span class="heart">❤</span><span class="v"></span><span class="u">уд/мин</span></div>
        <div class="meta"></div><canvas class="spark"></canvas>`;
      el.onclick = () => select(n.num);
      box.appendChild(el);
    }
    el.classList.toggle('alert', cls === 'bad');
    el.classList.toggle('offline', cls === 'off');
    el.classList.toggle('sel', state.selected === n.num);
    el.classList.toggle('beat', cls !== 'off' && n.heart_bpm != null);
    if (n.heart_bpm) el.style.setProperty('--period', (60 / n.heart_bpm).toFixed(2) + 's');
    el.querySelector('.name').textContent = nodeName(n);
    el.querySelector('.id').textContent = `${n.node_id}${n.short_name ? ' · ' + n.short_name : ''}`;
    const st = el.querySelector('.state'); st.className = 'state ' + cls; st.textContent = label;
    el.querySelector('.v').textContent = n.heart_bpm ?? '—';
    el.querySelector('.meta').innerHTML =
      `<span>SpO₂ <b>${n.spo2 ? n.spo2 + '%' : '—'}</b></span>` +
      `<span>🔋 <b>${n.battery != null ? (n.battery > 100 ? 'USB' : n.battery + '%') : '—'}</b></span>` +
      `<span>📶 <b>${n.last_rssi || '—'}</b> dBm / ${(n.last_snr ?? 0).toFixed(1)} dB</span>` +
      `<span>${n.hops_away ? 'хопов: ' + n.hops_away + ' · ' : ''}${ago(n.last_health)}</span>`;
    drawSpark(el.querySelector('canvas'), state.spark.get(n.num) || []);
  }
  for (const el of [...box.children]) if (!seen.has(el.dataset.num)) el.remove();
  $('sNodes').textContent = nodes.length;
  $('sOnline').textContent = online;
  $('sAvg').textContent = cnt ? Math.round(sum / cnt) : '—';
}

// ---------- графики (canvas) ----------
function setupCanvas(c) {
  const dpr = window.devicePixelRatio || 1, w = c.clientWidth, h = c.clientHeight;
  if (c.width !== w * dpr || c.height !== h * dpr) { c.width = w * dpr; c.height = h * dpr; }
  const ctx = c.getContext('2d'); ctx.setTransform(dpr, 0, 0, dpr, 0, 0); ctx.clearRect(0, 0, w, h);
  return [ctx, w, h];
}

function drawSpark(c, pts) {
  const [ctx, w, h] = setupCanvas(c);
  const v = pts.filter(p => p.heart_bpm != null);
  if (v.length < 2) return;
  const ys = v.map(p => p.heart_bpm), lo = Math.min(...ys) - 5, hi = Math.max(...ys) + 5;
  const t0 = new Date(v[0].time), t1 = new Date(v[v.length - 1].time);
  ctx.strokeStyle = '#2dd4bf'; ctx.lineWidth = 1.6; ctx.beginPath();
  v.forEach((p, i) => {
    const x = ((new Date(p.time) - t0) / Math.max(1, t1 - t0)) * (w - 2) + 1;
    const y = h - 3 - ((p.heart_bpm - lo) / (hi - lo)) * (h - 6);
    i ? ctx.lineTo(x, y) : ctx.moveTo(x, y);
  });
  ctx.stroke();
}

function drawChart(pts) {
  const c = $('chart'); const [ctx, w, h] = setupCanvas(c);
  const pad = { l: 40, r: 40, t: 12, b: 24 };
  const minutes = +$('dRange').value, tEnd = Date.now(), tStart = tEnd - minutes * 60000;
  const X = t => pad.l + ((t - tStart) / (tEnd - tStart)) * (w - pad.l - pad.r);
  const hrs = pts.filter(p => p.heart_bpm != null).map(p => p.heart_bpm);
  const lo = Math.min(40, ...hrs) - 5, hi = Math.max(state.cfg.hr_high + 10 || 160, ...hrs) + 5;
  const Y = v => h - pad.b - ((v - lo) / (hi - lo)) * (h - pad.t - pad.b);
  const Y2 = v => h - pad.b - ((v - 80) / 20) * (h - pad.t - pad.b); // SpO2 80..100
  ctx.font = '11px system-ui'; ctx.fillStyle = '#8a9bb4'; ctx.strokeStyle = '#1f2a3d'; ctx.lineWidth = 1;
  for (let v = Math.ceil(lo / 20) * 20; v <= hi; v += 20) {
    ctx.beginPath(); ctx.moveTo(pad.l, Y(v)); ctx.lineTo(w - pad.r, Y(v)); ctx.stroke(); ctx.fillText(v, 8, Y(v) + 4);
  }
  for (let i = 0; i <= 4; i++) {
    const t = tStart + (i / 4) * (tEnd - tStart);
    ctx.fillText(new Date(t).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }), X(t) - 15, h - 6);
  }
  // пороги
  ctx.setLineDash([4, 4]); ctx.strokeStyle = '#7f1d1d';
  for (const v of [state.cfg.hr_low, state.cfg.hr_high]) { ctx.beginPath(); ctx.moveTo(pad.l, Y(v)); ctx.lineTo(w - pad.r, Y(v)); ctx.stroke(); }
  ctx.setLineDash([]);
  const line = (key, color, Yf) => {
    ctx.strokeStyle = color; ctx.lineWidth = 2; ctx.beginPath(); let prev = null;
    for (const p of pts) {
      const t = new Date(p.time).getTime(), v = p[key];
      if (v == null || v === 0) { prev = null; continue; }
      const x = X(t), y = Yf(v);
      // разрыв линии, если пропуск больше 3 минут
      if (prev && t - prev < 180000) ctx.lineTo(x, y); else ctx.moveTo(x, y);
      prev = t;
    }
    ctx.stroke();
  };
  line('spo2', '#60a5fa', Y2);
  line('heart_bpm', '#2dd4bf', Y);
  ctx.fillStyle = '#60a5fa'; ctx.fillText('SpO₂', w - pad.r + 6, Y2(100) + 10);
  ctx.fillText('80%', w - pad.r + 6, Y2(80));
  if (!hrs.length) { ctx.fillStyle = '#8a9bb4'; ctx.font = '14px system-ui'; ctx.fillText('Нет данных за период', w / 2 - 70, h / 2); }
  if (hrs.length) {
    const avg = Math.round(hrs.reduce((a, b) => a + b, 0) / hrs.length);
    $('dStats').innerHTML = `<span>Показаний: <b>${pts.length}</b></span><span>Мин: <b>${Math.min(...hrs)}</b></span>` +
      `<span>Средн: <b>${avg}</b></span><span>Макс: <b>${Math.max(...hrs)}</b></span>` +
      `<span style="color:#2dd4bf">— пульс</span><span style="color:#60a5fa">— SpO₂</span>`;
  } else $('dStats').innerHTML = '';
}

// ---------- детали узла ----------
async function select(num) {
  state.selected = num;
  $('detail').classList.remove('hidden');
  renderCards();
  await loadDetail();
  $('detail').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

async function loadDetail() {
  const n = state.nodes.get(state.selected);
  if (!n) return;
  $('dName').textContent = nodeName(n);
  $('dId').textContent = `${n.node_id} · ${n.hw_model || ''}`;
  state.detail = await api(`/api/nodes/${n.num}/readings?minutes=${$('dRange').value}&limit=20000`);
  drawChart(state.detail);
}

$('dRange').onchange = loadDetail;
$('dClose').onclick = () => { state.selected = null; $('detail').classList.add('hidden'); renderCards(); };
$('dRename').onclick = async () => {
  const n = state.nodes.get(state.selected); if (!n) return;
  const alias = prompt('Имя носителя датчика (пусто — имя узла Meshtastic):', n.alias || '');
  if (alias === null) return;
  await api(`/api/nodes/${n.num}`, { method: 'PATCH', body: JSON.stringify({ alias }) });
  await loadNodes(); loadDetail();
};
$('dDelete').onclick = async () => {
  const n = state.nodes.get(state.selected); if (!n) return;
  if (!confirm(`Удалить «${nodeName(n)}» и всю историю показаний?`)) return;
  await api(`/api/nodes/${n.num}`, { method: 'DELETE' });
  $('dClose').onclick(); loadNodes();
};

// ---------- алерты ----------
function renderAlerts() {
  const box = $('alerts');
  $('sAlerts').textContent = state.alerts.filter(a => !a.resolved).length;
  if (!state.alerts.length) { box.innerHTML = '<div class="muted">Нет алертов</div>'; return; }
  box.innerHTML = state.alerts.map(a => {
    const n = state.nodes.get(a.node_num), who = n ? nodeName(n) : '!' + a.node_num.toString(16).padStart(8, '0');
    const cls = a.resolved ? 'resolved' : ALERT_KINDS[a.kind] || 'bad';
    return `<div class="al ${cls}"><b>${esc(who)}</b> — ${esc(a.message)}
      <div class="t"><span>${new Date(a.time).toLocaleTimeString('ru-RU')}${a.resolved ? ' · закрыт' : ''}</span>
      ${a.acked ? '<span>✓ принят</span>' : `<button class="btn small" data-ack="${a.id}">Принять</button>`}</div></div>`;
  }).join('');
}
$('alerts').onclick = async e => {
  const id = e.target.dataset.ack; if (!id) return;
  await api(`/api/alerts/${id}/ack`, { method: 'POST' }); loadAlerts();
};

// ---------- шлюзы ----------
function renderGateways() {
  const list = [...state.gateways.values()];
  $('gws').innerHTML = list.map(g => {
    const [cls, txt] = GW_STATE[g.state] || ['', g.state];
    const name = g.name || (g.transport === 'ble' && g.id === 'bluetooth' ? 'Bluetooth' : g.id);
    return `<span class="chip" title="${esc(g.error || '')}"><span class="dot ${cls}"></span>${esc(g.transport.toUpperCase())}: ${esc(name)} — ${txt}</span>`;
  }).join('') || '<span class="chip">Шлюзы не настроены</span>';
}

// ---------- загрузка и live ----------
async function loadNodes() {
  const nodes = await api('/api/nodes');
  state.nodes = new Map(nodes.map(n => [n.num, n]));
  renderCards(); renderAlerts();
}
async function loadAlerts() { state.alerts = await api('/api/alerts?limit=100'); renderAlerts(); }
async function loadGateways() {
  const g = await api('/api/gateways'); state.gateways = new Map(g.map(x => [x.id, x])); renderGateways();
}
async function loadSparks() {
  await Promise.all([...state.nodes.keys()].map(async num => {
    state.spark.set(num, await api(`/api/nodes/${num}/readings?minutes=10&limit=300`));
  }));
  renderCards();
}

let reloadTimer;
const debounced = fn => () => { clearTimeout(reloadTimer); reloadTimer = setTimeout(fn, 400); };
const reloadNodes = debounced(loadNodes);

function connectWS() {
  const ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws');
  ws.onopen = () => { $('wsdot').className = 'dot ok'; $('wstxt').textContent = 'онлайн'; };
  ws.onclose = () => {
    $('wsdot').className = 'dot bad'; $('wstxt').textContent = 'нет связи с сервером';
    setTimeout(() => { fetch('/api/me').then(r => r.status === 401 ? location.replace('/login') : connectWS()).catch(() => setTimeout(connectWS, 3000)); }, 2000);
  };
  ws.onmessage = ev => {
    const { type, data } = JSON.parse(ev.data);
    switch (type) {
      case 'reading': {
        let n = state.nodes.get(data.node_num);
        if (!n) { reloadNodes(); return; }
        Object.assign(n, { heart_bpm: data.heart_bpm, spo2: data.spo2, last_health: data.time, last_seen: data.time,
          last_rssi: data.rssi, last_snr: data.snr, gateway: data.gateway });
        const s = state.spark.get(data.node_num) || [];
        s.push(data); while (s.length && Date.now() - new Date(s[0].time) > 600000) s.shift();
        state.spark.set(data.node_num, s);
        if (state.selected === data.node_num && state.detail) { state.detail.push(data); drawChart(state.detail); }
        renderCards(); break;
      }
      case 'battery': { const n = state.nodes.get(data.node_num); if (n) { n.battery = data.battery; renderCards(); } break; }
      case 'alert': state.alerts.unshift(data); renderAlerts(); renderCards(); break;
      case 'alerts_changed': loadAlerts(); break;
      case 'nodes_changed': reloadNodes(); break;
      case 'gateway': state.gateways.set(data.id, data); renderGateways(); break;
      case 'gateway_removed': state.gateways.delete(data); renderGateways(); break;
    }
  };
}

// ---------- аккаунт ----------
$('logout').onclick = async () => { await fetch('/api/logout', { method: 'POST' }); location.replace('/login'); };
$('pwBtn').onclick = () => { $('pwErr').style.display = 'none'; $('pwForm').reset(); $('pwDlg').showModal(); };
$('pwForm').addEventListener('submit', async e => {
  if (e.submitter?.value !== 'ok') return;
  e.preventDefault();
  try {
    await api('/api/password', { method: 'POST', body: JSON.stringify({ old: $('pwOld').value, new: $('pwNew').value }) });
    $('pwDlg').close(); alert('Пароль изменён');
  } catch (err) { $('pwErr').textContent = err.message; $('pwErr').style.display = 'block'; }
});

(async () => {
  state.cfg = await api('/api/config');
  $('me').textContent = (await api('/api/me')).username;
  await Promise.all([loadNodes(), loadAlerts(), loadGateways()]);
  await loadSparks();
  connectWS();
  setInterval(renderCards, 5000); // обновить «N секунд назад» и статус «нет связи»
  window.addEventListener('resize', () => { renderCards(); if (state.detail) drawChart(state.detail); });
})();
