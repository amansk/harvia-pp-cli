// Package fixture is a mocked Harvia REST surface for tests.
package fixture

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Server records requests and serves testdata/fixtures.
type Server struct {
	URL                 string
	EndpointsURL        string
	http                *httptest.Server
	mu                  sync.Mutex
	Reqs                []Request
	StateOn             bool
	FailAuth            bool
	FailRefresh         bool
	UnauthorizedDevices int
}

// Request is one captured call (bodies never include live secrets; fixtures only).
type Request struct {
	Method string
	Path   string
	Body   map[string]any
}

// New starts the mock API.
func New() *Server {
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/endpoints", s.endpoints)
	mux.HandleFunc("/auth/token", s.authToken)
	mux.HandleFunc("/auth/refresh", s.authRefresh)
	mux.HandleFunc("/devices", s.devices)
	mux.HandleFunc("/devices/state", s.state)
	mux.HandleFunc("/devices/command", s.command)
	mux.HandleFunc("/devices/target", s.target)
	mux.HandleFunc("/devices/profile", s.profile)
	mux.HandleFunc("/data/latest-data", s.telemetry)
	s.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		var body map[string]any
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
		}
		s.Reqs = append(s.Reqs, Request{Method: r.Method, Path: r.URL.Path, Body: body})
		s.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	s.URL = s.http.URL
	s.EndpointsURL = s.http.URL + "/endpoints"
	return s
}

func (s *Server) Close() { s.http.Close() }

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.Reqs))
	copy(out, s.Reqs)
	return out
}

func (s *Server) endpoints(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"endpoints": map[string]any{
			"RestApi": map[string]any{
				"generics": map[string]string{"https": s.URL},
				"device":   map[string]string{"https": s.URL},
				"data":     map[string]string{"https": s.URL},
			},
		},
	})
}

func (s *Server) authToken(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fail := s.FailAuth
	s.mu.Unlock()
	if fail {
		http.Error(w, `{"message":"Invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	serveFixture(w, "auth_token.json")
}

func (s *Server) authRefresh(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fail := s.FailRefresh
	s.mu.Unlock()
	if fail {
		http.Error(w, `{"message":"refresh rejected"}`, http.StatusUnauthorized)
		return
	}
	serveFixture(w, "auth_refresh.json")
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/devices" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	if s.UnauthorizedDevices > 0 {
		s.UnauthorizedDevices--
		s.mu.Unlock()
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	s.mu.Unlock()
	serveFixture(w, "devices.json")
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	if s.StateOn {
		serveFixture(w, "state_on.json")
		return
	}
	serveFixture(w, "state_off.json")
}

func (s *Server) telemetry(w http.ResponseWriter, r *http.Request) {
	serveFixture(w, "telemetry.json")
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) target(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func serveFixture(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "application/json")
	b, err := os.ReadFile(fixturePath(name))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_, _ = w.Write(b)
}

func fixturePath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures", name)
}

// EnvFile is testdata/fixtures/env.
func EnvFile() string {
	return fixturePath("env")
}
