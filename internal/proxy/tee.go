package proxy

import (
	"io"
	"sync"
)

// teeBody wraps a response body. Every byte is passed straight through to the
// reader (the client), and a bounded copy is kept on the side. When the body
// ends, onDone gets the copy. Nothing here ever waits on the analysis side.
type teeBody struct {
	rc     io.ReadCloser
	max    int
	onDone func(body []byte, complete, truncated bool)

	mu        sync.Mutex
	buf       []byte
	truncated bool
	eof       bool
	once      sync.Once
}

func (t *teeBody) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	t.mu.Lock()
	if n > 0 && !t.truncated {
		if len(t.buf)+n > t.max {
			t.truncated = true
			t.buf = nil // stop holding memory for a body we will not analyze
		} else {
			t.buf = append(t.buf, p[:n]...)
		}
	}
	if err == io.EOF {
		t.eof = true
	}
	t.mu.Unlock()
	if err == io.EOF {
		t.finish()
	}
	return n, err
}

func (t *teeBody) Close() error {
	err := t.rc.Close()
	t.finish()
	return err
}

func (t *teeBody) finish() {
	t.once.Do(func() {
		t.mu.Lock()
		body, eof, truncated := t.buf, t.eof, t.truncated
		t.mu.Unlock()
		t.onDone(body, eof, truncated)
	})
}
