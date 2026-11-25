package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// EndpointInfo is one endpoint the proxy has learned.
type EndpointInfo struct {
	Key       string `json:"key"`
	Responses int    `json:"responses"`
}

// Config wires the server to the rest of DevLatch. Every func is optional.
type Config struct {
	Version   string
	Hub       *Hub
	Endpoints func() []EndpointInfo
	Contract  func() (any, error)
	Heartbeat time.Duration
	UI        http.Handler
	Actions   Actions
}

type Server struct {
	cfg Config
	act *actionState
}

func New(cfg Config) *Server {
	if cfg.Hub == nil {
		cfg.Hub = NewHub(0)
	}
	if cfg.Heartbeat <= 0 {
		cfg.Heartbeat = 15 * time.Second
	}
	return &Server{cfg: cfg, act: newActionState()}
}

func (s *Server) Hub() *Hub { return s.cfg.Hub }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/drifts", s.drifts)
	mux.HandleFunc("GET /api/contract", s.contract)
	mux.HandleFunc("GET /api/events", s.events)
	s.registerActions(mux)
	if s.cfg.UI != nil {
		mux.Handle("GET /", s.cfg.UI)
	}
	return hostGuard(mux)
}

// hostGuard blocks DNS-rebinding: only loopback Host headers are served.
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch strings.ToLower(host) {
		case "127.0.0.1", "localhost", "::1":
		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden host"})
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.cfg.Version})
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	eps := []EndpointInfo{}
	if s.cfg.Endpoints != nil {
		if got := s.cfg.Endpoints(); got != nil {
			eps = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       s.cfg.Version,
		"endpoints":     eps,
		"last_event_id": s.cfg.Hub.LastID(),
		"token":         s.act.token,
		"actions":       map[string]bool{"impact": s.cfg.Actions.Impact != nil, "fix": s.cfg.Actions.FixPreview != nil && s.cfg.Actions.FixApply != nil, "approve": s.cfg.Actions.Approve != nil},
		"drift_events":  len(s.cfg.Hub.Recent("drift", 0)),
	})
}

func (s *Server) drifts(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	evs := s.cfg.Hub.Recent("drift", limit)
	if evs == nil {
		evs = []Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"drifts": evs})
}

func (s *Server) contract(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Contract == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no contract loaded"})
		return
	}
	c, err := s.cfg.Contract()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	var last int64
	v := r.Header.Get("Last-Event-ID")
	if v == "" {
		v = r.URL.Query().Get("last_id")
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
		last = n
	}
	replay, ch, cancel := s.cfg.Hub.Subscribe(last)
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, ": connected\n\n")
	for _, ev := range replay {
		writeSSE(w, ev)
	}
	fl.Flush()

	tk := time.NewTicker(s.cfg.Heartbeat)
	defer tk.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, ev)
			fl.Flush()
		case <-tk.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, ev Event) {
	_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, ev.Data)
}
