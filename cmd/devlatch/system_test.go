package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/report"
)

var demoPaths = []string{"/api/v1/users/7", "/api/v1/orders", "/api/v1/stats/daily"}

func buildDemoBackend(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "examples", "demo-backend"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Skipf("demo backend not found: %v", err)
	}
	name := "demo-backend"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the demo backend failed: %v\n%s", err, out)
	}
	return bin
}

// startDemo runs the real demo backend (optionally with a mutation) and waits until it answers.
func startDemo(t *testing.T, bin, mutate string) (string, func()) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	args := []string{"-addr", addr}
	if mutate != "" {
		args = append(args, "-mutate", mutate)
	}
	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		})
	}
	t.Cleanup(stop)
	base := "http://" + addr
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/api/v1/orders")
		if err == nil {
			_ = resp.Body.Close()
			return base, stop
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()
	t.Fatalf("demo backend did not start on %s", addr)
	return "", stop
}

// cliWithTraffic runs a devlatch command with a proxy and sends 5 requests to
// every demo endpoint through it, then stops the command.
func cliWithTraffic(t *testing.T, target string, args ...string) (int, string, string) {
	t.Helper()
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer cancel()
		client := &http.Client{Timeout: 2 * time.Second}
		base := fmt.Sprintf("http://127.0.0.1:%d", port)
		deadline := time.Now().Add(8 * time.Second)
		for _, p := range demoPaths {
			for i := 0; i < 5; i++ {
				for {
					resp, err := client.Get(base + p)
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
	}()
	var out, errb bytes.Buffer
	full := append(args, "--target", target, "--port", strconv.Itoa(port), "--min-samples", "3")
	code := run(ctx, full, &out, &errb)
	return code, out.String(), errb.String()
}

type scenario struct {
	n        int
	name     string
	mutate   string
	wantType string // "" means: no violation at all
	altType  string // accepted instead of wantType (low-confidence rename)
	pathHas  string
	severity string // "" means: do not assert
}

// Blueprint verification scenarios 1 to 7, driven by the demo backend's mutations.
var scenarios = []scenario{
	{1, "field rename", "rename_user_name", "POSSIBLE_RENAME", "REMOVED_KEY", "user_name", "BREAKING"},
	{2, "type conversion", "balance_to_string", "TYPE_CHANGED", "", "balance", "BREAKING"},
	{3, "array element change", "tags_to_objects", "ARRAY_ELEMENT_CHANGED", "", "tags", ""},
	{4, "removed key", "remove_email", "REMOVED_KEY", "", "email", "BREAKING"},
	{5, "new nullability", "nullable_nickname", "NULLABILITY_CHANGED", "", "nickname", "WARNING"},
	{6, "added key", "add_currency", "ADDED_KEY", "", "currency", "SAFE"},
	{7, "map stability", "new_map_date", "", "", "", ""},
}

func parseReport(t *testing.T, out string) report.Output {
	t.Helper()
	var res report.Output
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout must be pure JSON: %v\n%s", err, out)
	}
	return res
}

// System test for blueprint scenarios 1 to 7 against the real demo backend.
func TestSystemScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("system test skipped in -short mode")
	}
	bin := buildDemoBackend(t)

	for _, sc := range scenarios {
		sc := sc
		t.Run(fmt.Sprintf("%d_%s", sc.n, strings.ReplaceAll(sc.name, " ", "_")), func(t *testing.T) {
			old, _ := os.Getwd()
			if err := os.Chdir(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(old) })

			// Baseline from the clean backend.
			cleanURL, stopClean := startDemo(t, bin, "")
			if code, _, e := cliWithTraffic(t, cleanURL, "record"); code != exitOK {
				t.Fatalf("record failed (%d): %s", code, e)
			}
			var out, errb bytes.Buffer
			if code := run(context.Background(), []string{"approve"}, &out, &errb); code != exitOK {
				t.Fatalf("approve failed: %s", errb.String())
			}
			code, o, e := cliWithTraffic(t, cleanURL, "check", "--format", "json")
			if res := parseReport(t, o); code != exitOK || len(res.Violations) != 0 {
				t.Fatalf("clean backend must pass its own contract (%d): %+v\n%s", code, res.Violations, e)
			}
			stopClean()

			// Same contract, mutated backend.
			mutURL, _ := startDemo(t, bin, sc.mutate)
			code, o, e = cliWithTraffic(t, mutURL, "check", "--format", "json")
			res := parseReport(t, o)
			for _, v := range res.Violations {
				t.Logf("  %s %s %s %s%s", v.Severity, v.Type, v.Endpoint, v.Path, map[bool]string{true: " (" + v.RenameFrom + " -> " + v.RenameTo + ")"}[v.RenameFrom != ""])
			}

			anyBreaking := false
			for _, v := range res.Violations {
				if string(v.Severity) == "BREAKING" {
					anyBreaking = true
				}
			}
			wantCode := exitOK
			if anyBreaking {
				wantCode = exitDrift
			}
			if code != wantCode {
				t.Fatalf("exit code = %d, want %d (breaking present: %v)\n%s", code, wantCode, anyBreaking, e)
			}

			if sc.wantType == "" {
				if len(res.Violations) != 0 {
					t.Fatalf("scenario %d must not raise anything, got %+v", sc.n, res.Violations)
				}
				return
			}
			found := false
			for _, v := range res.Violations {
				typ := string(v.Type)
				if typ != sc.wantType && (sc.altType == "" || typ != sc.altType) {
					continue
				}
				if !strings.Contains(v.Path+v.RenameFrom+v.RenameTo, sc.pathHas) {
					continue
				}
				if sc.severity != "" && string(v.Severity) != sc.severity {
					t.Fatalf("scenario %d: %s on %s has severity %s, want %s", sc.n, typ, v.Path, v.Severity, sc.severity)
				}
				found = true
				break
			}
			if !found {
				t.Fatalf("scenario %d: expected %s (or %s) on a path containing %q, got %+v",
					sc.n, sc.wantType, sc.altType, sc.pathHas, res.Violations)
			}
		})
	}
}
