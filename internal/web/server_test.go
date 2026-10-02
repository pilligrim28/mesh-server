package web

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"mesh-server/internal/config"
	"mesh-server/internal/core"
	"mesh-server/internal/store"
)

func TestLoginFlow(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer st.Close()
	st.EnsureUser("admin", "secret")
	svc := core.NewService(config.Config{OfflineAfter: time.Minute}, st, core.NewHub())
	assets := fstest.MapFS{"index.html": {Data: []byte("DASH")}, "login.html": {Data: []byte("LOGIN")}}
	ts := httptest.NewServer(New(svc, assets).Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	get := func(p string) (int, string) {
		r, err := c.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b := new(strings.Builder)
		_, _ = strings.NewReader("").WriteTo(b)
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		return r.StatusCode, string(buf[:n])
	}
	if code, body := get("/"); body != "LOGIN" {
		t.Fatalf("anon / → %d %q", code, body)
	}
	if code, _ := get("/api/nodes"); code != 401 {
		t.Fatalf("anon api → %d", code)
	}
	r, _ := c.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"username":"admin","password":"bad"}`))
	if r.StatusCode != 401 {
		t.Fatalf("bad password → %d", r.StatusCode)
	}
	r, _ = c.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"username":"admin","password":"secret"}`))
	if r.StatusCode != 200 {
		t.Fatalf("login → %d", r.StatusCode)
	}
	if _, body := get("/"); body != "DASH" {
		t.Fatalf("after login / → %q", body)
	}
	if code, body := get("/api/nodes"); code != 200 || !strings.HasPrefix(body, "[]") {
		t.Fatalf("nodes → %d %q", code, body)
	}
}
