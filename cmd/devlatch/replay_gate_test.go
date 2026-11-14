package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
)

func runWithTraffic(t *testing.T, backendURL string, args ...string) (int, string, string) {
	t.Helper()
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errb bytes.Buffer
	go driveTraffic(port, 8, cancel)
	full := append(args, "--target", backendURL, "--port", strconv.Itoa(port), "--min-samples", "5")
	code := run(ctx, full, &out, &errb)
	return code, out.String(), errb.String()
}

// Phase 3b gate: replaying recorded traffic gives the same result as checking live.
func TestReplayParityWithLive(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var mutated atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mutated.Load() {
			_, _ = w.Write([]byte(`{"id":1,"user_id":7,"name":"Asha"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"role_id":7,"name":"Asha"}`))
	}))
	defer backend.Close()

	if code, _, e := runWithTraffic(t, backend.URL, "record"); code != exitOK {
		t.Fatalf("record failed: %s", e)
	}
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"approve"}, &out, &errb); code != exitOK {
		t.Fatalf("approve failed: %s", errb.String())
	}

	mutated.Store(true)
	if code, _, e := runWithTraffic(t, backend.URL, "record", "--save-traffic", "traffic.jsonl"); code != exitOK {
		t.Fatalf("record --save-traffic failed: %s", e)
	}
	liveCode, liveOut, liveErr := runWithTraffic(t, backend.URL, "check", "--format", "json")
	if liveCode != exitDrift {
		t.Fatalf("live check should exit 1, got %d\n%s\n%s", liveCode, liveOut, liveErr)
	}

	out.Reset()
	errb.Reset()
	replayCode := run(context.Background(), []string{"check", "--from", "traffic.jsonl", "--format", "json", "--min-samples", "5"}, &out, &errb)
	if replayCode != liveCode || out.String() != liveOut {
		t.Fatalf("replay differs from live (codes %d vs %d)\n--- live\n%s\n--- replay\n%s\n%s", liveCode, replayCode, liveOut, out.String(), errb.String())
	}
}
