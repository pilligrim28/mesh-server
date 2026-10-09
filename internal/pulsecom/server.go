package pulsecom

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed ui
var uiFS embed.FS

// App связывает устройство, запись и веб-интерфейс.
type App struct {
	Dev *Device
	Rec *Recorder

	mu      sync.Mutex
	clients map[*websocket.Conn]chan []byte
	hist    []histPoint // последние ~15 минут сводок для графика
	raw     [][3]uint32
	logs    []logLine
}

type histPoint struct {
	T      int64 `json:"t"` // unix ms
	BPM    int   `json:"bpm"`
	SpO2   int   `json:"spo2"`
	Finger bool  `json:"finger"`
}

type logLine struct {
	T   int64  `json:"t"`
	Msg string `json:"msg"`
}

func NewApp(dev *Device, rec *Recorder) *App {
	a := &App{Dev: dev, Rec: rec, clients: map[*websocket.Conn]chan []byte{}}
	dev.OnMsg = a.onMsg
	dev.OnState = func(DevState) { a.broadcast(map[string]any{"t": "state", "state": a.state()}) }
	dev.OnLog = a.addLog
	go a.rawPump()
	go func() { // счётчики записи в интерфейсе
		for range time.Tick(2 * time.Second) {
			if c := a.Rec.Current(); c != nil {
				a.broadcast(map[string]any{"t": "rec", "rec": c})
			}
		}
	}()
	return a
}

func (a *App) addLog(s string) {
	l := logLine{time.Now().UnixMilli(), s}
	a.mu.Lock()
	a.logs = append(a.logs, l)
	if len(a.logs) > 100 {
		a.logs = a.logs[len(a.logs)-100:]
	}
	a.mu.Unlock()
	a.broadcast(map[string]any{"t": "log", "log": l})
}

func (a *App) onMsg(at time.Time, m Msg) {
	a.Rec.Write(at, m)
	switch m.Kind {
	case KindData:
		p := histPoint{at.UnixMilli(), m.BPM, m.SpO2, m.Finger}
		a.mu.Lock()
		a.hist = append(a.hist, p)
		if len(a.hist) > 5000 {
			a.hist = a.hist[len(a.hist)-4000:]
		}
		a.mu.Unlock()
		a.broadcast(map[string]any{"t": "d", "at": at.UnixMilli(), "m": m})
	case KindBeat:
		a.broadcast(map[string]any{"t": "b", "at": at.UnixMilli(), "m": m})
	case KindRaw:
		a.mu.Lock()
		if len(a.raw) < 1000 {
			a.raw = append(a.raw, [3]uint32{m.Ms, m.IR, m.Red})
		}
		a.mu.Unlock()
	case KindStatus, KindOK, KindErr, KindHello:
		a.broadcast(map[string]any{"t": "state", "state": a.state()})
	}
}

// rawPump пачками отправляет сырые отсчёты (100 Гц) в браузер 10 раз в секунду.
func (a *App) rawPump() {
	for range time.Tick(100 * time.Millisecond) {
		a.mu.Lock()
		r := a.raw
		a.raw = nil
		a.mu.Unlock()
		if len(r) > 0 {
			a.broadcast(map[string]any{"t": "r", "s": r})
		}
	}
}

func (a *App) state() map[string]any {
	return map[string]any{"dev": a.Dev.State(), "rec": a.Rec.Current(), "dir": a.Rec.Dir()}
}

func (a *App) broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ch := range a.clients {
		select {
		case ch <- b:
		default: // медленный клиент — пропускаем
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// sameOrigin — защита от запросов с чужих сайтов (приложение без пароля, только для localhost).
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	o = strings.TrimPrefix(strings.TrimPrefix(o, "http://"), "https://")
	return o == r.Host
}

var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 8192, CheckOrigin: sameOrigin}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(uiFS, "ui")
	static := http.FileServer(http.FS(sub))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		hist := append([]histPoint(nil), a.hist...)
		logs := append([]logLine(nil), a.logs...)
		a.mu.Unlock()
		st := a.state()
		st["hist"], st["logs"] = hist, logs
		writeJSON(w, 200, st)
	})
	mux.HandleFunc("GET /api/ports", func(w http.ResponseWriter, r *http.Request) {
		p := a.Dev.List()
		if p == nil {
			p = []PortInfo{}
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("POST /api/port", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Port string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, 400, err)
			return
		}
		a.Dev.SetPort(strings.TrimSpace(req.Port))
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/cmd", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Cmd string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, 400, err)
			return
		}
		if err := a.Dev.Send(req.Cmd); err != nil {
			fail(w, 409, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/session", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Action, Label string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, 400, err)
			return
		}
		switch req.Action {
		case "start":
			s, err := a.Rec.Start(req.Label)
			if err != nil {
				fail(w, 500, err)
				return
			}
			a.addLog("Запись начата: " + s.Name)
		case "stop":
			if c := a.Rec.Current(); c != nil {
				a.Rec.Stop()
				a.addLog("Запись остановлена: " + c.Name)
			}
		default:
			fail(w, 400, errBadAction)
			return
		}
		a.broadcast(map[string]any{"t": "state", "state": a.state()})
		writeJSON(w, 200, a.state())
	})
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		l, err := a.Rec.List()
		if err != nil {
			fail(w, 500, err)
			return
		}
		if l == nil {
			l = []SessionInfo{}
		}
		writeJSON(w, 200, l)
	})
	mux.HandleFunc("DELETE /api/sessions/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Rec.Delete(r.PathValue("name")); err != nil {
			fail(w, 409, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /files/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		p, ok := a.Rec.SafeFile(name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		a.Rec.Flush()
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlPathEscape(name))
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, p)
	})
	mux.HandleFunc("GET /ws", a.serveWS)
	return guard(mux)
}

// guard отклоняет изменяющие запросы с чужих страниц.
func guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (a *App) serveWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ch := make(chan []byte, 256)
	a.mu.Lock()
	a.clients[c] = ch
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.clients, c)
		a.mu.Unlock()
		c.Close()
	}()
	go func() { // читаем, чтобы заметить закрытие вкладки
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				c.Close()
				return
			}
		}
	}()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case b := <-ch:
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.WriteMessage(websocket.TextMessage, b) != nil {
				return
			}
		case <-ping.C:
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}

func init() { log.SetFlags(log.Ltime) }
