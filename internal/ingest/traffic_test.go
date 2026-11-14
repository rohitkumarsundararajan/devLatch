package ingest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OWNER/devlatch/internal/infer"
	"github.com/OWNER/devlatch/internal/normalize"
)

var entries = []Entry{
	{Method: "GET", Path: "/api/users/1", Status: 200, Body: `{"id":1,"name":"Asha","tags":["a","b"]}`},
	{Method: "GET", Path: "/api/users/2", Status: 200, Body: `{"id":2,"name":null,"tags":[]}`},
	{Method: "GET", Path: "/api/orders", Status: 200, Body: `[{"id":9,"total":10.5},{"id":10,"total":11}]`},
	{Method: "GET", Path: "/api/broken", Status: 200, Body: `not json {`},
}

func render(s *infer.Store) string {
	var b strings.Builder
	for _, ep := range s.Snapshot() {
		b.WriteString(ep.Key + "\n" + infer.Render(ep.Node) + "\n")
	}
	return b.String()
}

func TestReplayMatchesLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.jsonl")
	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if err := w.Write(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// "Live": observe the same bodies directly.
	live := infer.NewStore(infer.DefaultConfig())
	for _, e := range entries {
		_ = live.Observe(normalize.EndpointKey(e.Method, e.Path, e.Status), []byte(e.Body))
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	replayed := infer.NewStore(infer.DefaultConfig())
	acc, skip, err := Replay(f, replayed)
	if err != nil {
		t.Fatal(err)
	}
	if acc+skip != len(entries) || acc < 3 {
		t.Fatalf("accepted=%d skipped=%d, want 4 total and at least 3 accepted", acc, skip)
	}
	if render(live) == "" || render(live) != render(replayed) {
		t.Fatalf("replay differs from live:\n--- live\n%s\n--- replay\n%s", render(live), render(replayed))
	}
}

func TestReplayErrors(t *testing.T) {
	s := infer.NewStore(infer.DefaultConfig())
	if _, _, err := Replay(strings.NewReader("{not an entry\n"), s); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("expected line-numbered error, got %v", err)
	}
	if acc, skip, err := Replay(bytes.NewReader([]byte("\n\n")), s); err != nil || acc != 0 || skip != 0 {
		t.Fatalf("blank lines should be ignored, got %d %d %v", acc, skip, err)
	}
	if _, err := NewWriter(filepath.Join(t.TempDir(), "no-such-dir", "x.jsonl")); err == nil {
		t.Fatal("writer in a missing directory must fail")
	}
}
