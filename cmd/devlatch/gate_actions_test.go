package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
)

func startUIWith(t *testing.T, backend string, extra ...string) *uiRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuf{}
	done := make(chan int, 1)
	args := append([]string{"--target", backend, "--port", "0", "--ui-port", "0",
		"--interval", "100ms", "--min-samples", "3", "--no-notify"}, extra...)
	go func() { done <- runUI(ctx, args, out, io.Discard) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(out.String(), "Press Ctrl+C") {
		time.Sleep(20 * time.Millisecond)
	}
	m := uiAddrRe.FindAllStringSubmatch(out.String(), -1)
	if len(m) < 2 {
		cancel()
		t.Fatalf("could not read ports from output: %q", out.String())
	}
	return &uiRun{proxyAddr: m[1][1], uiAddr: m[0][1], stop: func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runUI did not stop within 5s")
		}
	}}
}

func apiDo(t *testing.T, method, addr, p, token, body string) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, "http://"+addr+p, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-DevLatch-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// tokenOf reads the per-run action token and checks that every action is enabled.
func tokenOf(t *testing.T, addr string) string {
	t.Helper()
	_, b := apiDo(t, "GET", addr, "/api/state", "", "")
	var st struct {
		Token   string `json:"token"`
		Actions struct {
			Impact  bool `json:"impact"`
			Fix     bool `json:"fix"`
			Approve bool `json:"approve"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	if st.Token == "" || !st.Actions.Impact || !st.Actions.Fix || !st.Actions.Approve {
		t.Fatalf("state does not offer all actions: %s", b)
	}
	return st.Token
}

// driftAfterTraffic opens the live stream, sends traffic and returns the first drift event.
func driftAfterTraffic(t *testing.T, r *uiRun) diff.Violation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+r.uiAddr+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	hitProxy(t, r.proxyAddr, 5)

	var typ string
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			typ = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") && typ == "drift" {
			var v diff.Violation
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
				t.Fatal(err)
			}
			return v
		}
	}
	t.Fatalf("no drift event arrived (stream ended: %v)", sc.Err())
	return diff.Violation{}
}

// Gate for P12: approve from the dashboard API, then a drift comes with the
// affected frontend files and a fix preview, and file-changing calls are guarded.
func TestGateUIActionsEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	scanner, _ := filepath.Abs(filepath.Join("..", "..", "scanner", "src", "cli.mjs"))
	frontend, _ := filepath.Abs(filepath.Join("..", "..", "examples", "demo-frontend"))
	for _, p := range []string{scanner, filepath.Join(frontend, "tsconfig.json")} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("missing %s", p)
		}
	}

	const oldBody = `{"id":1,"user_name":"ann","email":"a@b.c"}`
	const newBody = `{"id":1,"username":"ann","email":"a@b.c"}`
	var mutated atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mutated.Load() {
			_, _ = io.WriteString(w, newBody)
			return
		}
		_, _ = io.WriteString(w, oldBody)
	}))
	defer backend.Close()

	wd, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	extra := []string{"--frontend", frontend, "--scanner", scanner}

	// Run 1: learn the endpoint and approve it through the API.
	r1 := startUIWith(t, backend.URL, extra...)
	hitProxy(t, r1.proxyAddr, 5)
	key := waitEndpointKey(t, r1.uiAddr)
	tok1 := tokenOf(t, r1.uiAddr)
	body, _ := json.Marshal(map[string]string{"endpoint": key})
	if code, _ := apiDo(t, "POST", r1.uiAddr, "/api/approve", "", string(body)); code != http.StatusForbidden {
		t.Fatalf("approve without token = %d, want 403", code)
	}
	if code, b := apiDo(t, "POST", r1.uiAddr, "/api/approve", tok1, string(body)); code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, b)
	}
	if f, err := contract.Load(config.ContractPath()); err != nil || f.Endpoints[key] == nil || f.ApprovedBy == "" {
		t.Fatalf("contract not saved by approve: %v %+v", err, f)
	}
	r1.stop()

	// Run 2: the backend renames a field; the dashboard reports it live.
	mutated.Store(true)
	r2 := startUIWith(t, backend.URL, extra...)
	defer r2.stop()
	tok2 := tokenOf(t, r2.uiAddr)
	got := driftAfterTraffic(t, r2)
	if got.Severity != diff.SeverityBreaking || got.RenameFrom == "" || got.RenameTo == "" {
		t.Fatalf("expected a breaking rename, got %+v", got)
	}

	q := url.Values{"endpoint": {got.Endpoint}, "path": {got.Path}}.Encode()
	code, b := apiDo(t, "GET", r2.uiAddr, "/api/impact?"+q, "", "")
	if code == http.StatusInternalServerError && strings.Contains(string(b), "Cannot find") {
		t.Skipf("scanner dependencies are not installed: %s", b)
	}
	if code != http.StatusOK {
		t.Fatalf("impact = %d %s", code, b)
	}
	var imp struct {
		Hits  []struct{ File string } `json:"hits"`
		Files int                     `json:"files"`
	}
	if err := json.Unmarshal(b, &imp); err != nil || len(imp.Hits) == 0 || imp.Files == 0 {
		t.Fatalf("impact shows no affected files: %v %s", err, b)
	}

	to := got.RenameTo[strings.LastIndex(got.RenameTo, ".")+1:]
	q = url.Values{"endpoint": {got.Endpoint}, "path": {got.RenameFrom}, "to": {to}}.Encode()
	code, b = apiDo(t, "GET", r2.uiAddr, "/api/fix/preview?"+q, "", "")
	if code != http.StatusOK {
		t.Fatalf("fix preview = %d %s", code, b)
	}
	var pv struct {
		Edits   int  `json:"edits"`
		Applied bool `json:"applied"`
		Changes []struct {
			After string `json:"after"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(b, &pv); err != nil || pv.Edits == 0 || pv.Applied {
		t.Fatalf("bad fix preview: %v %s", err, b)
	}
	if !strings.Contains(pv.Changes[0].After, to) {
		t.Fatalf("preview does not use the new name %q: %s", to, b)
	}

	// Changing files requires the token; a call without it must not run.
	apply, _ := json.Marshal(map[string]string{"endpoint": got.Endpoint, "path": got.RenameFrom, "to": to})
	if code, _ := apiDo(t, "POST", r2.uiAddr, "/api/fix/apply", "", string(apply)); code != http.StatusForbidden {
		t.Fatalf("apply without token = %d, want 403", code)
	}

	// Approving the new shape updates the contract.
	if code, b := apiDo(t, "POST", r2.uiAddr, "/api/approve", tok2, string(body)); code != http.StatusOK {
		t.Fatalf("approve new shape = %d %s", code, b)
	}
	if _, b := apiDo(t, "GET", r2.uiAddr, "/api/contract", "", ""); !strings.Contains(string(b), `"username"`) {
		t.Fatalf("contract does not contain the new field: %s", b)
	}
}
