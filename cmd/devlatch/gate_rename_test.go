package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rohitkumarsundararajan/devlatch/internal/report"
)

// Phase 5 gate: a renamed key is reported as one POSSIBLE_RENAME with evidence.
func TestGateRenameDetected(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var mutated atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mutated.Load() {
			_, _ = w.Write([]byte(`{"id":1,"username":"asha","email":"a@x.in","role_id":7,"phone":"99"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"user_name":"asha","email":"a@x.in","role_id":7,"phone":"99"}`))
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
	mutated.Store(true)

	code, o, e := withTraffic("check", "--format", "json")
	if code != exitDrift {
		t.Fatalf("renamed backend should exit 1, got %d:\n%s\n%s", code, o, e)
	}
	var res report.Output
	if err := json.Unmarshal([]byte(o), &res); err != nil {
		t.Fatalf("stdout must be pure JSON: %v\n%s", err, o)
	}
	if len(res.Violations) != 1 {
		t.Fatalf("want exactly one violation (the rename), got %+v", res.Violations)
	}
	v := res.Violations[0]
	if string(v.Type) != "POSSIBLE_RENAME" || v.RenameFrom != "$.user_name" || v.RenameTo != "$.username" ||
		v.Confidence < 0.6 || len(v.Evidence) == 0 || string(v.Severity) != "BREAKING" {
		t.Fatalf("bad rename violation: %+v", v)
	}

	code, o, e = withTraffic("check")
	if code != exitDrift || !strings.Contains(o, "$.user_name -> $.username") || !strings.Contains(o, "evidence:") {
		t.Fatalf("text report should show the rename and its evidence (%d):\n%s\n%s", code, o, e)
	}
}
