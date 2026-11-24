package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Actions are the operations the dashboard can trigger. Each hook is optional;
// a nil hook answers 501. The real work lives in the devlatch command.
type Actions struct {
	Impact     func(endpoint, path string) (any, error)
	FixPreview func(endpoint, path, to string) (any, error)
	FixApply   func(endpoint, path, to string) (any, error)
	// Approve accepts live traffic as the contract; endpoint "" means all.
	Approve func(endpoint string) (any, error)
}

type actionState struct {
	token string
	mu    sync.Mutex // one action at a time
}

func newActionState() *actionState {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &actionState{token: hex.EncodeToString(b)}
}

type actionReq struct {
	Endpoint string `json:"endpoint"`
	Path     string `json:"path"`
	To       string `json:"to"`
}

func (s *Server) registerActions(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/impact", s.impact)
	mux.HandleFunc("GET /api/fix/preview", s.fixPreview)
	mux.HandleFunc("POST /api/fix/apply", s.fixApply)
	mux.HandleFunc("POST /api/approve", s.approve)
}

// authorized protects every request that changes files: JSON content type, a
// same-origin Origin header (if the browser sent one) and the per-run token.
func (s *Server) authorized(r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || u.Host != r.Host {
			return false
		}
	}
	got := r.Header.Get("X-DevLatch-Token")
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.act.token)) == 1
}

func (s *Server) readAction(w http.ResponseWriter, r *http.Request) (actionReq, bool) {
	var req actionReq
	if !s.authorized(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return req, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request body"})
		return req, false
	}
	return req, true
}

func (s *Server) run(w http.ResponseWriter, available bool, kind, endpoint string, publish bool, f func() (any, error)) {
	if !available {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not available in this session"})
		return
	}
	s.act.mu.Lock()
	defer s.act.mu.Unlock()
	out, err := f()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if publish {
		s.cfg.Hub.Publish("action", map[string]string{"kind": kind, "endpoint": endpoint})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) impact(w http.ResponseWriter, r *http.Request) {
	ep, p := r.URL.Query().Get("endpoint"), r.URL.Query().Get("path")
	if ep == "" || p == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint and path are required"})
		return
	}
	h := s.cfg.Actions.Impact
	s.run(w, h != nil, "impact", ep, false, func() (any, error) { return h(ep, p) })
}

func (s *Server) fixPreview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ep, p, to := q.Get("endpoint"), q.Get("path"), q.Get("to")
	if ep == "" || p == "" || to == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint, path and to are required"})
		return
	}
	h := s.cfg.Actions.FixPreview
	s.run(w, h != nil, "fix-preview", ep, false, func() (any, error) { return h(ep, p, to) })
}

func (s *Server) fixApply(w http.ResponseWriter, r *http.Request) {
	req, ok := s.readAction(w, r)
	if !ok {
		return
	}
	if req.Endpoint == "" || req.Path == "" || req.To == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint, path and to are required"})
		return
	}
	h := s.cfg.Actions.FixApply
	s.run(w, h != nil, "fix-apply", req.Endpoint, true, func() (any, error) { return h(req.Endpoint, req.Path, req.To) })
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	req, ok := s.readAction(w, r)
	if !ok {
		return
	}
	h := s.cfg.Actions.Approve
	s.run(w, h != nil, "approve", req.Endpoint, true, func() (any, error) { return h(req.Endpoint) })
}
