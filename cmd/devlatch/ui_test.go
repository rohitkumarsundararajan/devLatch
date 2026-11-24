package main

import (
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
	"github.com/rohitkumarsundararajan/devlatch/internal/server"
)

func feedStore(t *testing.T, body string) *infer.Store {
	t.Helper()
	s := infer.NewStore(infer.DefaultConfig())
	for i := 0; i < 5; i++ {
		if err := s.Observe("GET /api/v1/users/{id} 200", []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestDriftScannerPublishesOnce(t *testing.T) {
	baseStore := feedStore(t, `{"id":1,"user_name":"ann","email":"a@b.c"}`)
	f := contract.FromNodes(collapseEndpoints(baseStore.Snapshot()), 3, "test", time.Now())

	hub := server.NewHub(50)
	defer hub.Close()
	ds := newDriftScanner(f.ToNodes(), diff.Options{MinSamples: 3, Renames: true}, hub)

	live := feedStore(t, `{"id":1,"username":"ann","email":"a@b.c"}`)
	fresh := ds.scan(live)
	breaking := false
	for _, v := range fresh {
		if v.Severity == diff.SeverityBreaking {
			breaking = true
		}
	}
	if !breaking {
		t.Fatalf("expected a BREAKING violation, got %+v", fresh)
	}
	if again := ds.scan(live); len(again) != 0 {
		t.Fatalf("second scan should report nothing new, got %+v", again)
	}
	if got := len(hub.Recent("drift", 0)); got != len(fresh) {
		t.Fatalf("drift events = %d, want %d", got, len(fresh))
	}
	if got := len(hub.Recent("endpoint", 0)); got != 1 {
		t.Fatalf("endpoint events = %d, want 1", got)
	}
}
