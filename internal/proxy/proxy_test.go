package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OWNER/devlatch/internal/infer"
)

// rawClient never decompresses, so tests see exactly the bytes on the wire.
var rawClient = &http.Client{Transport: &http.Transport{DisableCompression: true}}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// setup starts an upstream and a proxy in front of it.
func setup(t *testing.T, upstream http.Handler, tweak func(*Options)) (*Proxy, *httptest.Server) {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	target, _ := url.Parse(up.URL)
	opts := Options{Target: target, Logger: log.New(io.Discard, "", 0)}
	if tweak != nil {
		tweak(&opts)
	}
	p, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(p)
	t.Cleanup(func() { srv.Close(); p.Close() })
	return p, srv
}

// result is the part of a response the tests look at; returning it instead of
// *http.Response keeps body handling inside get.
type result struct {
	StatusCode int
	Header     http.Header
}

func get(t *testing.T, c *http.Client, url string, header ...string) (result, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, body
}

func jsonHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
}

func TestNewValidatesTarget(t *testing.T) {
	for _, raw := range []string{"", "localhost:8000", "/just/a/path"} {
		u, _ := url.Parse(raw)
		if _, err := New(Options{Target: u}); err == nil {
			t.Errorf("target %q should be rejected", raw)
		}
	}
	if _, err := New(Options{}); err == nil {
		t.Error("nil target should be rejected")
	}
}

func TestForwardsResponseUnchangedAndLearnsSchema(t *testing.T) {
	const body = `{"id":7,"name":"ada"}`
	p, srv := setup(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, body)
	}), nil)

	resp, got := get(t, rawClient, srv.URL+"/api/items/7")
	if resp.StatusCode != 201 || resp.Header.Get("X-Custom") != "yes" || string(got) != body {
		t.Fatalf("response changed: %d %v %q", resp.StatusCode, resp.Header, got)
	}
	waitFor(t, "enqueue", func() bool { return p.Stats().Queued == 1 })
	p.Close()

	snap := p.Store().Snapshot()
	if len(snap) != 1 || snap[0].Key != "GET /api/items/{id} 201" {
		t.Fatalf("unexpected endpoints: %+v", snap)
	}
	if r := infer.Render(snap[0].Node); r != "OBJECT {\n  id: INTEGER\n  name: STRING\n}" {
		t.Errorf("schema:\n%s", r)
	}
}

func TestTargetWithBasePathStillKeysByInboundPath(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/base/things/1" {
			t.Errorf("upstream saw %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()
	target, _ := url.Parse(up.URL + "/base")
	p, _ := New(Options{Target: target, Logger: log.New(io.Discard, "", 0)})
	srv := httptest.NewServer(p)
	defer srv.Close()

	get(t, rawClient, srv.URL+"/things/1")
	waitFor(t, "enqueue", func() bool { return p.Stats().Queued == 1 })
	p.Close()
	if keys := p.Store().Keys(); len(keys) != 1 || keys[0] != "GET /things/{id} 200" {
		t.Errorf("keys = %v", keys)
	}
}

func TestGzipBodyIsForwardedUntouchedAndStillAnalyzed(t *testing.T) {
	var zipped bytes.Buffer
	zw := gzip.NewWriter(&zipped)
	_, _ = zw.Write([]byte(`{"name":"x","n":1}`))
	_ = zw.Close()

	p, srv := setup(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(zipped.Bytes())
	}), nil)

	resp, got := get(t, rawClient, srv.URL+"/z", "Accept-Encoding", "gzip")
	if resp.Header.Get("Content-Encoding") != "gzip" || !bytes.Equal(got, zipped.Bytes()) {
		t.Fatal("gzip body must reach the client byte-for-byte")
	}
	waitFor(t, "enqueue", func() bool { return p.Stats().Queued == 1 })
	p.Close()
	if p.Stats().Analyzed != 1 {
		t.Fatalf("stats: %+v", p.Stats())
	}
	if r := infer.Render(p.Store().Snapshot()[0].Node); r != "OBJECT {\n  n: INTEGER\n  name: STRING\n}" {
		t.Errorf("schema:\n%s", r)
	}
}

func TestBrotliIsForwardedAndSkipped(t *testing.T) {
	p, srv := setup(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "br")
		_, _ = io.WriteString(w, "not-really-brotli")
	}), nil)
	_, got := get(t, rawClient, srv.URL+"/b")
	if string(got) != "not-really-brotli" {
		t.Errorf("body changed: %q", got)
	}
	if s := p.Stats(); s.SkippedEncoding != 1 || s.Queued != 0 {
		t.Errorf("stats: %+v", s)
	}
}

func TestAcceptEncodingNeverInvitesBrotli(t *testing.T) {
	var seen string
	_, srv := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}), nil)
	get(t, rawClient, srv.URL+"/", "Accept-Encoding", "br;q=1.0, gzip, zstd")
	if seen != "gzip" {
		t.Errorf("upstream saw Accept-Encoding %q, want gzip", seen)
	}
}

func TestEventStreamsAreNotBuffered(t *testing.T) {
	release := make(chan struct{})
	p, srv := setup(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
		_, _ = io.WriteString(w, "data: two\n\n")
	}), nil)

	resp, err := rawClient.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	first := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(resp.Body).ReadString('\n')
		first <- line
	}()
	select {
	case line := <-first:
		if line != "data: one\n" {
			t.Errorf("first line = %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first event was held back: the proxy is buffering a stream")
	}
	close(release)
	if s := p.Stats(); s.SkippedStreaming != 1 || s.Queued != 0 {
		t.Errorf("stats: %+v", s)
	}
}

func TestNonJSONAndEmptyResponsesAreForwardedAndIgnored(t *testing.T) {
	p, srv := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/empty" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<h1>hi</h1>")
	}), nil)
	if _, got := get(t, rawClient, srv.URL+"/page"); string(got) != "<h1>hi</h1>" {
		t.Errorf("html changed: %q", got)
	}
	if resp, _ := get(t, rawClient, srv.URL+"/empty"); resp.StatusCode != 204 {
		t.Errorf("status %d", resp.StatusCode)
	}
	if s := p.Stats(); s.SkippedNonJSON != 1 || s.JSONSeen != 0 {
		t.Errorf("stats: %+v", s)
	}
}

func TestLargeBodiesAreForwardedInFullButNotAnalyzed(t *testing.T) {
	payload := `{"data":"` + strings.Repeat("x", 300) + `"}`
	p, srv := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/chunked" { // no Content-Length: only the tee can notice the size
			_, _ = io.WriteString(w, payload[:50])
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, payload[50:])
			return
		}
		_, _ = io.WriteString(w, payload) // Content-Length known up front
	}), func(o *Options) { o.MaxBodyBytes = 64 })

	for _, path := range []string{"/chunked", "/sized"} {
		if _, got := get(t, rawClient, srv.URL+path); string(got) != payload {
			t.Errorf("%s: client got %d bytes, want %d", path, len(got), len(payload))
		}
	}
	waitFor(t, "both skipped", func() bool { return p.Stats().SkippedTooLarge == 2 })
	if p.Stats().Queued != 0 {
		t.Errorf("stats: %+v", p.Stats())
	}
}

func TestIgnoredPrefixesAreForwardedButNotAnalyzed(t *testing.T) {
	p, srv := setup(t, jsonHandler(`{"a":1}`), func(o *Options) { o.IgnorePrefixes = []string{"/__demo"} })
	get(t, rawClient, srv.URL+"/__demo/mutations")
	get(t, rawClient, srv.URL+"/api/x")
	waitFor(t, "enqueue", func() bool { return p.Stats().Queued == 1 })
	p.Close()
	if keys := p.Store().Keys(); len(keys) != 1 || keys[0] != "GET /api/x 200" {
		t.Errorf("keys = %v", keys)
	}
}

func TestSampleEvery(t *testing.T) {
	p, srv := setup(t, jsonHandler(`{"a":1}`), func(o *Options) { o.SampleEvery = 2 })
	for i := 0; i < 10; i++ {
		get(t, rawClient, srv.URL+"/s")
	}
	waitFor(t, "enqueue", func() bool { return p.Stats().Queued == 5 })
	if s := p.Stats(); s.SkippedSampling != 5 || s.JSONSeen != 10 {
		t.Errorf("stats: %+v", s)
	}
}

func TestUpstreamDownReturns502(t *testing.T) {
	up := httptest.NewServer(http.NotFoundHandler())
	target, _ := url.Parse(up.URL)
	up.Close()
	p, _ := New(Options{Target: target, Logger: log.New(io.Discard, "", 0)})
	srv := httptest.NewServer(p)
	defer srv.Close()
	defer p.Close()
	if resp, _ := get(t, rawClient, srv.URL+"/x"); resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status %d, want 502", resp.StatusCode)
	}
}

// The load test from the Phase 1 gate: with nothing draining the queue, a flood
// of requests must still all succeed quickly, and the surplus must be counted
// as dropped rather than slowing anyone down.
func TestFullQueueNeverBlocksTheApp(t *testing.T) {
	p, srv := setup(t, jsonHandler(`{"a":1}`), func(o *Options) {
		o.QueueSize = 2
		o.noWorkers = true
	})
	const total = 200
	start := time.Now()
	for i := 0; i < total; i++ {
		if resp, body := get(t, rawClient, srv.URL+"/flood"); resp.StatusCode != 200 || string(body) != `{"a":1}` {
			t.Fatalf("request %d: %d %q", i, resp.StatusCode, body)
		}
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("%d requests took %v: the proxy is blocking", total, elapsed)
	}
	waitFor(t, "all accounted for", func() bool { s := p.Stats(); return s.Queued+s.DroppedQueueFull == total })
	if s := p.Stats(); s.Queued != 2 || s.DroppedQueueFull != total-2 {
		t.Errorf("stats: %+v", s)
	}
}

func TestConcurrentRequestsAreAllAccountedFor(t *testing.T) {
	p, srv := setup(t, jsonHandler(`{"id":1,"tags":["a","b"]}`), func(o *Options) { o.Workers = 4 })
	const goroutines, each = 8, 25
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				get(t, rawClient, srv.URL+"/users/1")
			}
		}()
	}
	wg.Wait()
	waitFor(t, "all accounted for", func() bool { s := p.Stats(); return s.Queued+s.DroppedQueueFull == goroutines*each })
	p.Close()
	s := p.Stats()
	if s.JSONSeen != goroutines*each || s.Analyzed != s.Queued || s.DecodeErrors != 0 {
		t.Errorf("stats: %+v", s)
	}
	snap := p.Store().Snapshot()
	if len(snap) != 1 || int64(snap[0].Responses) != s.Analyzed {
		t.Errorf("snapshot %+v does not match analyzed=%d", snap, s.Analyzed)
	}
}

func TestInvalidJSONIsCountedNotFatal(t *testing.T) {
	p, srv := setup(t, jsonHandler(`{"broken":`), nil)
	if _, got := get(t, rawClient, srv.URL+"/bad"); string(got) != `{"broken":` {
		t.Errorf("body changed: %q", got)
	}
	waitFor(t, "enqueue", func() bool { return p.Stats().Queued == 1 })
	p.Close()
	if s := p.Stats(); s.DecodeErrors != 1 || s.Analyzed != 0 {
		t.Errorf("stats: %+v", s)
	}
}

func TestStatsString(t *testing.T) {
	out := Stats{JSONSeen: 3, Analyzed: 2, DroppedQueueFull: 1}.String()
	if !strings.Contains(out, "3 seen, 2 analyzed, 1 dropped") {
		t.Errorf("got %q", out)
	}
}
