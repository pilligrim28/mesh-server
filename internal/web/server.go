// Package web — HTTP API, WebSocket и встроенный веб-интерфейс.
package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"mesh-server/internal/core"
	"mesh-server/internal/store"
)

const sessionCookie = "mesh_session"

type Server struct {
	svc    *core.Service
	assets fs.FS
}

func New(svc *core.Service, assets fs.FS) *Server { return &Server{svc: svc, assets: assets} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	st := s.svc.Store()

	// без авторизации
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "time": time.Now()})
	})
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)

	// с авторизацией
	api := http.NewServeMux()
	api.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"username": r.Header.Get("X-User")})
	})
	api.HandleFunc("POST /api/password", s.changePassword)
	api.HandleFunc("GET /api/nodes", func(w http.ResponseWriter, r *http.Request) {
		nodes, err := st.Nodes()
		respond(w, nodes, err)
	})
	api.HandleFunc("PATCH /api/nodes/{num}", func(w http.ResponseWriter, r *http.Request) {
		num, ok := nodeNum(w, r)
		if !ok {
			return
		}
		var body struct {
			Alias string `json:"alias"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			writeErr(w, 400, "некорректный JSON")
			return
		}
		err := st.SetAlias(num, strings.TrimSpace(body.Alias))
		s.svc.Hub().Publish("nodes_changed", nil)
		respond(w, map[string]bool{"ok": true}, err)
	})
	api.HandleFunc("DELETE /api/nodes/{num}", func(w http.ResponseWriter, r *http.Request) {
		num, ok := nodeNum(w, r)
		if !ok {
			return
		}
		err := st.DeleteNode(num)
		s.svc.Hub().Publish("nodes_changed", nil)
		respond(w, map[string]bool{"ok": true}, err)
	})
	api.HandleFunc("GET /api/nodes/{num}/readings", func(w http.ResponseWriter, r *http.Request) {
		num, ok := nodeNum(w, r)
		if !ok {
			return
		}
		minutes := atoiDef(r.URL.Query().Get("minutes"), 60)
		limit := min(atoiDef(r.URL.Query().Get("limit"), 2000), 20000)
		rs, err := st.Readings(num, time.Now().Add(-time.Duration(minutes)*time.Minute), limit)
		respond(w, rs, err)
	})
	api.HandleFunc("GET /api/alerts", func(w http.ResponseWriter, r *http.Request) {
		as, err := st.Alerts(r.URL.Query().Get("active") == "1", min(atoiDef(r.URL.Query().Get("limit"), 200), 2000))
		respond(w, as, err)
	})
	api.HandleFunc("POST /api/alerts/{id}/ack", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeErr(w, 400, "некорректный id")
			return
		}
		err = st.AckAlert(id)
		s.svc.Hub().Publish("alerts_changed", nil)
		respond(w, map[string]bool{"ok": true}, err)
	})
	api.HandleFunc("GET /api/gateways", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.svc.Gateways())
	})
	api.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		c := s.svc.Config()
		writeJSON(w, 200, map[string]any{"hr_low": c.HRLow, "hr_high": c.HRHigh, "spo2_low": c.SpO2Low,
			"offline_after_s": int(c.OfflineAfter.Seconds()), "transports": c.Transports})
	})
	api.HandleFunc("GET /ws", s.ws)
	mux.Handle("/api/", s.auth(api))
	mux.Handle("/ws", s.auth(api))

	// статика: index.html только для вошедших, login.html — для остальных
	static := http.FileServerFS(s.assets)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/", "/index.html":
			if _, ok := s.user(r); !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			serveFile(w, s.assets, "index.html")
		case "/login":
			if _, ok := s.user(r); ok {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			serveFile(w, s.assets, "login.html")
		default:
			static.ServeHTTP(w, r)
		}
	})
	return logRequests(mux)
}

func serveFile(w http.ResponseWriter, fsys fs.FS, name string) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

func (s *Server) user(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	return s.svc.Store().SessionUser(c.Value)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.user(r)
		if !ok {
			writeErr(w, 401, "требуется вход")
			return
		}
		r.Header.Set("X-User", u)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct{ Username, Password string }
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeErr(w, 400, "некорректный JSON")
		return
	}
	id, ok := s.svc.Store().CheckPassword(strings.TrimSpace(body.Username), body.Password)
	if !ok {
		time.Sleep(500 * time.Millisecond)
		writeErr(w, 401, "Неверный логин или пароль")
		return
	}
	token, err := s.svc.Store().CreateSession(id, 30*24*time.Hour)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 30 * 24 * 3600})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.svc.Store().DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.New) < 6 {
		writeErr(w, 400, "новый пароль — минимум 6 символов")
		return
	}
	user := r.Header.Get("X-User")
	if _, ok := s.svc.Store().CheckPassword(user, body.Old); !ok {
		writeErr(w, 403, "текущий пароль неверен")
		return
	}
	respond(w, map[string]bool{"ok": true}, s.svc.Store().SetPassword(user, body.New))
}

var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ch, unsub := s.svc.Hub().Subscribe()
	defer unsub()
	go func() { // читаем, чтобы заметить закрытие вкладки
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				unsub()
				return
			}
		}
	}()
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteJSON(m) != nil {
				return
			}
		case <-ping.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}

// ---------- helpers ----------

func nodeNum(w http.ResponseWriter, r *http.Request) (uint32, bool) {
	v := strings.TrimPrefix(r.PathValue("num"), "!")
	base := 10
	if strings.HasPrefix(r.PathValue("num"), "!") {
		base = 16
	}
	n, err := strconv.ParseUint(v, base, 32)
	if err != nil {
		writeErr(w, 400, "некорректный номер узла")
		return 0, false
	}
	return uint32(n), true
}

func atoiDef(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}

func respond(w http.ResponseWriter, v any, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "не найдено")
	case err != nil:
		writeErr(w, 500, err.Error())
	default:
		writeJSON(w, 200, v)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.Method != http.MethodGet {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
