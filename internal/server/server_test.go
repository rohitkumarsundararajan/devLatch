package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
)

func TestEventsStreamLive(t *testing.T) {
	hub := NewHub(100)
	ts := httptest.NewServer(New(Config{Version: "t", Hub: hub}).Handler())
	defer ts.Close()
	defer hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	if !sc.Scan() || !strings.HasPrefix(sc.Text(), ": connected") {
		t.Fatalf("no connected comment, got %q", sc.Text())
	}

	hub.Publish("drift", diff.Violation{Endpoint: "GET /api/v1/users/{id} 200", Path: "$.user_name",
		Type: diff.PossibleRename, Severity: diff.SeverityBreaking, RenameFrom: "user_name", RenameTo: "username"})

	var typ, data string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			typ = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if typ != "drift" {
		t.Fatalf("event type = %q", typ)
	}
	var v diff.Violation
	if err := json.Unmarshal([]byte(data), &v); err != nil || v.RenameFrom != "user_name" || v.RenameTo != "username" {
		t.Fatalf("bad data %q (%v)", data, err)
	}
}

func TestReplayStateAndHostGuard(t *testing.T) {
	hub := NewHub(10)
	for i := 0; i < 3; i++ {
		hub.Publish("drift", map[string]int{"n": i})
	}
	s := New(Config{Version: "t", Hub: hub, Endpoints: func() []EndpointInfo {
		return []EndpointInfo{{Key: "GET /a 200", Responses: 4}}
	}})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	defer hub.Close()

	resp, err := http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Endpoints   []EndpointInfo `json:"endpoints"`
		LastEventID int64          `json:"last_event_id"`
		DriftEvents int            `json:"drift_events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(st.Endpoints) != 1 || st.LastEventID != 3 || st.DriftEvents != 3 {
		t.Fatalf("state = %+v", st)
	}

	bad, _ := http.NewRequest("GET", ts.URL+"/api/state", nil)
	bad.Host = "evil.example.com"
	r2, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host status = %d", r2.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	r3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r3.Body.Close()
	var ids []string
	sc := bufio.NewScanner(r3.Body)
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "id: ") {
			ids = append(ids, strings.TrimPrefix(line, "id: "))
			if len(ids) == 2 {
				break
			}
		}
	}
	if strings.Join(ids, ",") != "2,3" {
		t.Fatalf("replayed ids = %v, want [2 3]", ids)
	}
}
