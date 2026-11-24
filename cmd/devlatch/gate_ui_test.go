package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

var uiAddrRe = regexp.MustCompile(`http://(127\.0\.0\.1:\d+)`)

type uiRun struct {
	proxyAddr, uiAddr string
	stop              func()
}

func startUI(t *testing.T, backend string) *uiRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuf{}
	done := make(chan int, 1)
	go func() {
		done <- runUI(ctx, []string{"--target", backend, "--port", "0", "--ui-port", "0",
			"--interval", "100ms", "--min-samples", "3"}, out, io.Discard)
	}()
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

func hitProxy(t *testing.T, addr string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		resp, err := http.Get("http://" + addr + "/api/v1/users/1")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func waitEndpointKey(t *testing.T, uiAddr string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + uiAddr + "/api/state")
		if err == nil {
			var st struct {
				Endpoints []struct {
					Key       string `json:"key"`
					Responses int    `json:"responses"`
				} `json:"endpoints"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&st)
			_ = resp.Body.Close()
			if len(st.Endpoints) > 0 && st.Endpoints[0].Responses >= 3 {
				return st.Endpoints[0].Key
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("proxy never learned an endpoint")
	return ""
}

// Gate for P10: with an approved contract, a changed backend response shows
// up as a drift event on the live SSE stream, and the REST API agrees.
func TestGateUIDriftArrivesLive(t *testing.T) {
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

	// Run 1: no contract yet. Learn the exact endpoint key through the real proxy.
	r1 := startUI(t, backend.URL)
	hitProxy(t, r1.proxyAddr, 5)
	key := waitEndpointKey(t, r1.uiAddr)
	r1.stop()

	// Approve a contract from the old shape under that key.
	st := infer.NewStore(infer.DefaultConfig())
	for i := 0; i < 5; i++ {
		if err := st.Observe(key, []byte(oldBody)); err != nil {
			t.Fatal(err)
		}
	}
	f := contract.FromNodes(collapseEndpoints(st.Snapshot()), 3, "test", time.Now())
	if err := os.MkdirAll(filepath.Dir(config.ContractPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := contract.Save(config.ContractPath(), f); err != nil {
		t.Fatal(err)
	}

	// Run 2: contract present, backend now returns the renamed field.
	mutated.Store(true)
	r2 := startUI(t, backend.URL)
	defer r2.stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+r2.uiAddr+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	hitProxy(t, r2.proxyAddr, 5)

	var typ string
	var got diff.Violation
	found := false
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			typ = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") && typ == "drift" {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &got); err != nil {
				t.Fatal(err)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no drift event arrived live (stream ended: %v)", sc.Err())
	}
	if got.Severity != diff.SeverityBreaking || !strings.Contains(got.Path+got.RenameFrom, "user_name") {
		t.Fatalf("unexpected drift event: %+v", got)
	}

	// The REST API agrees with the stream.
	r, err := http.Get("http://" + r2.uiAddr + "/api/drifts")
	if err != nil {
		t.Fatal(err)
	}
	var dr struct {
		Drifts []json.RawMessage `json:"drifts"`
	}
	_ = json.NewDecoder(r.Body).Decode(&dr)
	_ = r.Body.Close()
	if len(dr.Drifts) == 0 {
		t.Fatal("/api/drifts is empty after a drift event")
	}
	c, err := http.Get("http://" + r2.uiAddr + "/api/contract")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Body.Close()
	if c.StatusCode != http.StatusOK {
		t.Fatalf("/api/contract status = %d", c.StatusCode)
	}
}

// Gate for P11: the dashboard page and the API come from the same local port.
func TestGateUIServedWithAPI(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	r := startUI(t, backend.URL)
	defer r.stop()

	get := func(p string) (int, string) {
		resp, err := http.Get("http://" + r.uiAddr + p)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get("/"); code != http.StatusOK || !strings.Contains(body, `id="root"`) {
		t.Fatalf("/ = %d %q", code, body)
	}
	if code, body := get("/api/health"); code != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("/api/health = %d %q", code, body)
	}
}
