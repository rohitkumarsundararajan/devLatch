package ingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/OWNER/devlatch/internal/infer"
	"github.com/OWNER/devlatch/internal/normalize"
)

// Entry is one analyzed response, stored as one JSON line.
// Body holds the raw response text: treat traffic files as sensitive.
type Entry struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// Writer appends entries to a JSONL file. Safe for concurrent use.
type Writer struct {
	mu sync.Mutex
	f  *os.File
	bw *bufio.Writer
}

func NewWriter(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f, bw: bufio.NewWriter(f)}, nil
}

func (w *Writer) Write(e Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.bw.Write(b); err != nil {
		return err
	}
	return w.bw.WriteByte('\n')
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.bw.Flush(); err != nil {
		_ = w.f.Close()
		return err
	}
	return w.f.Close()
}

// Replay feeds a traffic file into store exactly like live traffic would be.
// accepted counts bodies the store took; skipped counts bodies it rejected.
func Replay(r io.Reader, store *infer.Store) (accepted, skipped int, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 32<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return accepted, skipped, fmt.Errorf("line %d: %w", line, err)
		}
		key := normalize.EndpointKey(e.Method, e.Path, e.Status)
		if err := store.Observe(key, []byte(e.Body)); err != nil {
			skipped++
			continue
		}
		accepted++
	}
	return accepted, skipped, sc.Err()
}
