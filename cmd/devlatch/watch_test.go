package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer lets the test read output while runWatch is still writing it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWatchLearnsSchemasAndPrintsThemOnShutdown(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":5,"name":"widget","price":9.5}`)
	}))
	defer upstream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &syncBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"watch", "--target", upstream.URL, "--port", "0"}, out, io.Discard)
	}()

	var addr string
	re := regexp.MustCompile(`http://(127\.0\.0\.1:\d+) ->`)
	waitUntil(t, "listening line", func() bool {
		m := re.FindStringSubmatch(out.String())
		if m == nil {
			return false
		}
		addr = m[1]
		return true
	})

	resp, err := http.Get("http://" + addr + "/api/things/5")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	waitUntil(t, "new-endpoint line", func() bool { return strings.Contains(out.String(), "+ GET /api/things/{id} 200") })
	cancel()

	select {
	case code := <-done:
		if code != exitOK {
			t.Fatalf("exit code %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch did not stop after cancel")
	}
	final := out.String()
	for _, want := range []string{"=== Inferred schemas", "GET /api/things/{id} 200  (1 responses)", "id: INTEGER", "name: STRING", "price: NUMBER", "=== Proxy stats ===", "1 analyzed"} {
		if !strings.Contains(final, want) {
			t.Errorf("output is missing %q:\n%s", want, final)
		}
	}
}

func TestWatchRejectsBadFlags(t *testing.T) {
	tests := map[string][]string{
		"missing target": {"watch"},
		"target no host": {"watch", "--target", "localhost8000"},
		"target no http": {"watch", "--target", "ftp://x"},
		"port too large": {"watch", "--target", "http://x:1", "--port", "70000"},
		"unknown flag":   {"watch", "--target", "http://x:1", "--nope"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			var errOut bytes.Buffer
			if code := run(context.Background(), args, io.Discard, &errOut); code != exitError {
				t.Errorf("exit code %d, want %d (stderr: %s)", code, exitError, errOut.String())
			}
		})
	}
}
