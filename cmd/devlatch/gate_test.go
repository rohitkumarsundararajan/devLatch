package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/report"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// driveTraffic waits for the proxy, sends n requests through it, then cancels ctx.
func driveTraffic(port, n int, cancel context.CancelFunc) {
	defer cancel()
	base := fmt.Sprintf("http://127.0.0.1:%d/api/users/", port)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for i := 1; i <= n; i++ {
		for {
			resp, err := client.Get(base + strconv.Itoa(i))
			if err == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				break
			}
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// Phase 3a gate: record, mutate the backend, check fails.
func TestGateRecordMutateCheck(t *testing.T) {
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

	withTraffic := func(args ...string) (int, string, string) {
		port := freePort(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var out, errb bytes.Buffer
		go driveTraffic(port, 8, cancel)
		full := append(args, "--target", backend.URL, "--port", strconv.Itoa(port), "--min-samples", "5")
		code := run(ctx, full, &out, &errb)
		return code, out.String(), errb.String()
	}

	if code, _, e := withTraffic("record"); code != exitOK {
		t.Fatalf("record failed (%d): %s", code, e)
	}
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"approve"}, &out, &errb); code != exitOK {
		t.Fatalf("approve failed: %s", errb.String())
	}

	code, o, e := withTraffic("check")
	if code != exitOK || !bytes.Contains([]byte(o), []byte("No drift detected.")) {
		t.Fatalf("clean check should pass (%d):\n%s\n%s", code, o, e)
	}

	mutated.Store(true)
	code, o, e = withTraffic("check", "--format", "json", "--renames=false")
	if code != exitDrift {
		t.Fatalf("mutated check should exit 1, got %d:\n%s\n%s", code, o, e)
	}
	var res report.Output
	if err := json.Unmarshal([]byte(o), &res); err != nil {
		t.Fatalf("stdout must be pure JSON: %v\n%s", err, o)
	}
	var removed, added bool
	for _, v := range res.Violations {
		if string(v.Type) == "REMOVED_KEY" && v.Path == "$.role_id" && string(v.Severity) == "BREAKING" {
			removed = true
		}
		if string(v.Type) == "ADDED_KEY" && v.Path == "$.user_id" {
			added = true
		}
	}
	if !removed || !added {
		t.Fatalf("expected REMOVED_KEY $.role_id and ADDED_KEY $.user_id, got %+v", res.Violations)
	}

	if code := run(context.Background(), []string{"check", "--fail-on", "nonsense"}, &out, &errb); code != exitError {
		t.Fatal("bad --fail-on must be a tool error")
	}
}
