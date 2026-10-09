'use strict';
const $ = id => document.getElementById(id);
const S = {dev: null, rec: null, hist: [], raw: [], rangeMin: 5, lastRaw: 0, rawCount: 0};

async function api(path, opt = {}) {
  if (opt.body && typeof opt.body !== 'string') {
    opt.body = JSON.stringify(opt.body);
    opt.headers = {'Content-Type': 'application/json'};
  }
  const r = await fetch(path, opt);
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || r.statusText);
  return j;
}
function toast(msg, err) {
  const t = $('toast');
  t.textContent = msg; t.className = 'toast' + (err ? ' err' : '');
  clearTimeout(t._h); t._h = setTimeout(() => t.classList.add('hidden'), 3000);
}
const cmd = c => api('/api/cmd', {method: 'POST', body: {cmd: c}}).catch(e => toast(e.message, true));
const fmtN = n => n ? n.toLocaleString('ru-RU') : '—';
const fmtT = ms => new Date(ms).toLocaleTimeString('ru-RU');
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]));
function fmtSize(b) { return b < 1024 ? b + ' Б' : b < 1048576 ? (b / 1024).toFixed(1) + ' КБ' : (b / 1048576).toFixed(1) + ' МБ'; }

// ---------- состояние ----------
function renderState() {
  const d = S.dev || {};
  const c = $('conn'), dot = c.querySelector('.dot'), txt = c.querySelector('span:last-child');
  if (d.connected) {
    dot.className = 'dot ' + (d.sensorOk ? 'ok' : 'warn');
    txt.textContent = `${d.port} · MaxPulse ${d.firmware}` + (d.sensorOk ? '' : ' · датчик не найден!');
  } else {
    dot.className = 'dot bad';
    txt.textContent = d.error || 'нет связи';
  }
  if ($('portSel').value !== (d.want || '') && document.activeElement !== $('portSel')) ensurePortOption(d.want || '');
  const st = d.status;
  ['run', 'raw', 'rate', 'led'].forEach(id => $(id).disabled = !d.connected);
  if (st) {
    $('run').textContent = st.running ? '■ Стоп измерений' : '▶ Старт измерений';
    $('run').classList.toggle('on', !!st.running);
    $('raw').textContent = 'Сырые данные: ' + (st.raw ? 'вкл' : 'выкл');
    $('raw').classList.toggle('on', !!st.raw);
    if (document.activeElement !== $('rate')) {
      if (![...$('rate').options].some(o => +o.value === st.rate)) $('rate').add(new Option(st.rate / 1000 + ' с', st.rate));
      $('rate').value = st.rate;
    }
    if (document.activeElement !== $('led')) { $('led').value = st.led || 0; }
    $('ledV').textContent = $('led').value;
    $('temp').textContent = st.temp ? st.temp.toFixed(1) + ' °C' : '—';
    $('ppgHint').classList.toggle('hidden', !!st.raw);
  }
  const r = S.rec, box = $('recBox');
  box.className = 'rec' + (r ? ' on' : '');
  box.innerHTML = r
    ? `<span class="rdot"></span><div><div>Идёт запись · ${esc(r.label || 'без метки')}</div><div class="nm">${esc(r.name)}.csv · строк ${r.rows}, ударов ${r.beats}${r.raw ? ', raw ' + r.raw : ''}</div></div>`
    : `<span class="rdot"></span><div>Запись остановлена — данные не сохраняются</div>`;
  $('recStop').disabled = !r;
}
function ensurePortOption(v) {
  const sel = $('portSel');
  if (v && ![...sel.options].some(o => o.value === v)) sel.add(new Option(v, v));
  sel.value = v;
}
async function loadPorts() {
  const ps = await api('/api/ports').catch(() => []);
  const sel = $('portSel'), want = (S.dev && S.dev.want) || '';
  sel.innerHTML = '<option value="">Автопоиск</option>';
  ps.forEach(p => sel.add(new Option(`${p.name}${p.desc ? ' — ' + p.desc : ''}${p.arduino ? ' ★' : ''}`, p.name)));
  ensurePortOption(want);
}

// ---------- живые данные ----------
function onData(at, m) {
  $('bpm').textContent = m.bpm || '—';
  $('spo2').textContent = m.spo2 || '—';
  $('finger').textContent = m.finger ? 'да' : 'нет';
  $('finger').style.color = m.finger ? 'var(--ok)' : 'var(--warn)';
  $('ir').textContent = fmtN(m.ir);
  $('red').textContent = fmtN(m.red);
  $('bpmSub').textContent = !m.finger ? 'приложите палец к датчику' : m.bpm ? 'среднее по последним ударам' : 'измеряю… держите палец неподвижно';
  S.hist.push({t: at, bpm: m.bpm, spo2: m.spo2, finger: m.finger});
  if (S.hist.length > 5000) S.hist.splice(0, 1000);
}
function pump() {
  ['beat', 'hdrHeart'].forEach(id => { const h = $(id); h.classList.add('pump'); setTimeout(() => h.classList.remove('pump'), 140); });
}
function onRaw(samples) {
  for (const s of samples) S.raw.push(s);
  if (S.raw.length > 1500) S.raw.splice(0, S.raw.length - 1500);
  S.rawCount += samples.length; S.lastRaw = Date.now();
}
setInterval(() => { $('rawRate').textContent = S.rawCount ? S.rawCount + ' отсч/с' : ''; S.rawCount = 0; }, 1000);

// ---------- графики ----------
function prep(cv) {
  const dpr = window.devicePixelRatio || 1, w = cv.clientWidth, h = cv.clientHeight || +cv.getAttribute('height');
  if (cv.width !== w * dpr || cv.height !== h * dpr) { cv.width = w * dpr; cv.height = h * dpr; }
  const g = cv.getContext('2d'); g.setTransform(dpr, 0, 0, dpr, 0, 0); g.clearRect(0, 0, w, h);
  return [g, w, h];
}
function drawTrend() {
  const [g, w, h] = prep($('trend'));
  const L = 38, R = 38, T = 8, B = 20, now = Date.now(), span = S.rangeMin * 60000;
  const x = t => L + (t - (now - span)) / span * (w - L - R);
  const yb = v => T + (1 - (v - 40) / 140) * (h - T - B);   // пульс 40..180
  const ys = v => T + (1 - (v - 80) / 20) * (h - T - B);    // SpO2 80..100
  g.font = '11px system-ui'; g.lineWidth = 1;
  for (let v = 40; v <= 180; v += 20) {
    g.strokeStyle = '#1f2a3d'; g.beginPath(); g.moveTo(L, yb(v)); g.lineTo(w - R, yb(v)); g.stroke();
    g.fillStyle = '#f87171'; g.textAlign = 'right'; g.fillText(v, L - 6, yb(v) + 4);
  }
  g.fillStyle = '#2dd4bf'; g.textAlign = 'left';
  for (let v = 80; v <= 100; v += 5) g.fillText(v + '%', w - R + 6, ys(v) + 4);
  g.fillStyle = '#8a9bb4'; g.textAlign = 'center';
  const step = S.rangeMin <= 1 ? 15000 : S.rangeMin <= 5 ? 60000 : 180000;
  for (let t = Math.ceil((now - span) / step) * step; t <= now; t += step) g.fillText(new Date(t).toLocaleTimeString('ru-RU', {hour: '2-digit', minute: '2-digit', second: S.rangeMin <= 1 ? '2-digit' : undefined}), x(t), h - 5);
  const pts = S.hist.filter(p => p.t >= now - span);
  const line = (key, y, col) => {
    g.strokeStyle = col; g.lineWidth = 2; g.beginPath(); let on = false;
    for (const p of pts) {
      const v = p[key];
      if (!v) { on = false; continue; }
      on ? g.lineTo(x(p.t), y(v)) : g.moveTo(x(p.t), y(v)); on = true;
    }
    g.stroke();
  };
  line('spo2', ys, '#2dd4bf'); line('bpm', yb, '#f87171');
  if (!pts.some(p => p.bpm)) { g.fillStyle = '#8a9bb4'; g.font = '13px system-ui'; g.fillText('нет данных пульса за выбранный период', w / 2, h / 2); }
}
function drawPPG() {
  const [g, w, h] = prep($('ppg'));
  const N = 500, d = S.raw.slice(-N - 75);
  if (d.length < 100 || Date.now() - S.lastRaw > 2000) { return; }
  // убираем постоянную составляющую скользящим средним (0,75 с) и переворачиваем: рост крови = рост волны
  const v = [];
  let sum = 0;
  for (let i = 0; i < d.length; i++) {
    sum += d[i][1]; if (i >= 75) sum -= d[i - 75][1];
    if (i >= 75) v.push(sum / 75 - d[i][1]);
  }
  let mn = Infinity, mx = -Infinity;
  for (const a of v) { if (a < mn) mn = a; if (a > mx) mx = a; }
  const pad = (mx - mn) * 0.1 || 1;
  g.strokeStyle = '#a78bfa'; g.lineWidth = 2; g.beginPath();
  v.forEach((a, i) => { const X = (i + N - v.length) / N * w, Y = h - 6 - (a - mn + pad) / (mx - mn + 2 * pad) * (h - 12); i ? g.lineTo(X, Y) : g.moveTo(X, Y); });
  g.stroke();
}
function frame() { drawTrend(); drawPPG(); requestAnimationFrame(frame); }

// ---------- журнал и сессии ----------
function addLog(l) {
  const div = document.createElement('div');
  div.textContent = fmtT(l.t) + '  ' + l.msg;
  if (/ERR|потеряно|не отвечает/.test(l.msg)) div.className = 'e';
  const box = $('log'); box.appendChild(div);
  while (box.children.length > 200) box.firstChild.remove();
  box.scrollTop = box.scrollHeight;
}
async function loadSessions() {
  const list = await api('/api/sessions').catch(() => []);
  const tb = $('sessions');
  if (!list.length) { tb.innerHTML = '<tr><td colspan="5" class="muted">Пока нет записей</td></tr>'; return; }
  const kind = f => f.endsWith('_beats.csv') ? 'удары' : f.endsWith('_raw.csv') ? 'raw' : 'сводка';
  tb.innerHTML = list.map(s => `<tr>
    <td>${s.started && !s.started.startsWith('0001') ? new Date(s.started).toLocaleString('ru-RU') : esc(s.name)}${s.active ? '<span class="tag">запись</span>' : ''}</td>
    <td>${esc(s.label || '—')}</td><td>${fmtSize(s.size)}</td>
    <td>${s.files.map(f => `<a href="/files/${encodeURIComponent(f)}" download>${kind(f)}</a>`).join('')}</td>
    <td>${s.active ? '' : `<button class="btn danger" data-del="${esc(s.name)}">Удалить</button>`}</td></tr>`).join('');
}

// ---------- WebSocket ----------
function connectWS() {
  const ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws');
  ws.onmessage = e => {
    const m = JSON.parse(e.data);
    switch (m.t) {
      case 'state': S.dev = m.state.dev; S.rec = m.state.rec; renderState(); break;
      case 'rec': S.rec = m.rec; renderState(); break;
      case 'd': onData(m.at, m.m); break;
      case 'b': pump(); break;
      case 'r': onRaw(m.s); break;
      case 'log': addLog(m.log); break;
    }
  };
  ws.onclose = () => { $('conn').querySelector('.dot').className = 'dot bad'; $('conn').querySelector('span:last-child').textContent = 'приложение не отвечает'; setTimeout(connectWS, 1500); };
  ws.onopen = loadState;
}
async function loadState() {
  const st = await api('/api/state');
  S.dev = st.dev; S.rec = st.rec;
  S.hist = (st.hist || []).map(p => ({t: p.t, bpm: p.bpm, spo2: p.spo2, finger: p.finger}));
  $('dir').textContent = 'Папка с файлами: ' + st.dir;
  $('log').innerHTML = ''; (st.logs || []).forEach(addLog);
  if (st.dev.last) onData(Date.now(), st.dev.last);
  renderState();
}

// ---------- обработчики ----------
$('run').onclick = () => cmd(S.dev?.status?.running ? 'STOP' : 'START');
$('raw').onclick = () => cmd(S.dev?.status?.raw ? 'RAW OFF' : 'RAW ON');
document.querySelectorAll('[data-cmd]').forEach(b => b.onclick = () => cmd(b.dataset.cmd));
$('rate').onchange = e => cmd('RATE ' + e.target.value);
$('led').oninput = e => $('ledV').textContent = e.target.value;
$('led').onchange = e => cmd('LED ' + e.target.value);
$('portSel').onchange = e => api('/api/port', {method: 'POST', body: {port: e.target.value}}).then(() => toast('Переподключаюсь…'));
$('portRefresh').onclick = loadPorts;
$('recStart').onclick = async () => {
  try { const st = await api('/api/session', {method: 'POST', body: {action: 'start', label: $('label').value}}); S.rec = st.rec; renderState(); loadSessions(); toast('Новая сессия: ' + st.rec.name); }
  catch (e) { toast(e.message, true); }
};
$('recStop').onclick = async () => { const st = await api('/api/session', {method: 'POST', body: {action: 'stop'}}); S.rec = st.rec; renderState(); loadSessions(); };
$('sessRefresh').onclick = loadSessions;
$('sessions').onclick = async e => {
  const n = e.target.dataset.del; if (!n || !confirm('Удалить файлы сессии ' + n + '?')) return;
  try { await api('/api/sessions/' + encodeURIComponent(n), {method: 'DELETE'}); loadSessions(); } catch (er) { toast(er.message, true); }
};
$('range').onclick = e => {
  const m = +e.target.dataset.m; if (!m) return;
  S.rangeMin = m; [...$('range').children].forEach(b => b.classList.toggle('on', b === e.target));
};

loadPorts(); loadSessions(); setInterval(loadSessions, 15000); connectWS(); requestAnimationFrame(frame);
