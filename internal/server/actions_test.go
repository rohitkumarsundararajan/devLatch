package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestActionsGuardAndHooks(t *testing.T) {
	approved := make(chan string, 4)
	hub := NewHub(20)
	s := New(Config{Version: "t", Hub: hub, Actions: Actions{
		Impact: func(e, p string) (any, error) { return map[string]string{"endpoint": e, "path": p}, nil },
		Approve: func(e string) (any, error) {
			approved <- e
			return map[string]bool{"ok": true}, nil
		},
	}})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	defer hub.Close()

	resp, err := http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&st)
	_ = resp.Body.Close()
	if st.Token == "" {
		t.Fatal("/api/state has no token")
	}

	post := func(tok, origin, ctype string) int {
		req, _ := http.NewRequest("POST", ts.URL+"/api/approve", strings.NewReader(`{"endpoint":"GET /a 200"}`))
		req.Header.Set("Content-Type", ctype)
		if tok != "" {
			req.Header.Set("X-DevLatch-Token", tok)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Body.Close() }()
		_, _ = io.Copy(io.Discard, r.Body)
		return r.StatusCode
	}

	if c := post("", "", "application/json"); c != http.StatusForbidden {
		t.Fatalf("no token = %d, want 403", c)
	}
	if c := post(st.Token, "http://evil.example", "application/json"); c != http.StatusForbidden {
		t.Fatalf("foreign origin = %d, want 403", c)
	}
	if c := post(st.Token, "", "text/plain"); c != http.StatusForbidden {
		t.Fatalf("wrong content type = %d, want 403", c)
	}
	select {
	case e := <-approved:
		t.Fatalf("hook ran for a rejected request: %q", e)
	default:
	}
	if c := post(st.Token, ts.URL, "application/json"); c != http.StatusOK {
		t.Fatalf("valid request = %d, want 200", c)
	}
	select {
	case e := <-approved:
		if e != "GET /a 200" {
			t.Fatalf("approve endpoint = %q", e)
		}
	default:
		t.Fatal("approve hook was not called")
	}
	if got := len(hub.Recent("action", 0)); got != 1 {
		t.Fatalf("action events = %d, want 1", got)
	}

	get := func(p string) (int, string) {
		r, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Body.Close() }()
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(b)
	}
	if c, b := get("/api/impact?endpoint=GET%20%2Fa%20200&path=%24.x"); c != http.StatusOK || !strings.Contains(b, `"path":"$.x"`) {
		t.Fatalf("impact = %d %q", c, b)
	}
	if c, _ := get("/api/impact?endpoint=x"); c != http.StatusBadRequest {
		t.Fatalf("impact without path = %d, want 400", c)
	}
	if c, _ := get("/api/fix/preview?endpoint=a&path=b&to=c"); c != http.StatusNotImplemented {
		t.Fatalf("preview without hook = %d, want 501", c)
	}
}
