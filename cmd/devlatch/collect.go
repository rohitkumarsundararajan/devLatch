package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OWNER/devlatch/internal/infer"
	"github.com/OWNER/devlatch/internal/ingest"
	"github.com/OWNER/devlatch/internal/proxy"
)

type collectOpts struct {
	target   *url.URL
	port     int
	dur      time.Duration
	ignore   []string
	onSample func(method, path string, status int, body []byte)
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// trafficHook turns a traffic Writer into a proxy OnSample hook.
func trafficHook(w *ingest.Writer) func(method, path string, status int, body []byte) {
	return func(method, path string, status int, body []byte) {
		_ = w.Write(ingest.Entry{Method: method, Path: path, Status: status, Body: string(body)})
	}
}

// collect runs the proxy until ctx is cancelled (Ctrl+C) or dur elapses, then
// returns everything it learned. A non-zero code means the caller should exit.
func collect(ctx context.Context, o collectOpts, info, stderr io.Writer, name string) (*infer.Store, proxy.Stats, int) {
	store := infer.NewStore(infer.DefaultConfig())
	p, err := proxy.New(proxy.Options{
		Target:         o.target,
		Store:          store,
		IgnorePrefixes: o.ignore,
		OnSample:       o.onSample,
		Logger:         log.New(stderr, "", log.LstdFlags),
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "devlatch %s: %v\n", name, err)
		return nil, proxy.Stats{}, exitError
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", o.port))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "devlatch %s: %v\n", name, err)
		p.Close()
		return nil, proxy.Stats{}, exitError
	}
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	_, _ = fmt.Fprintf(info, "DevLatch %s: http://%s -> %s\n", name, ln.Addr(), o.target)
	_, _ = fmt.Fprintln(info, "Use your app through the proxy. Press Ctrl+C when done.")

	var timer <-chan time.Time
	if o.dur > 0 {
		timer = time.After(o.dur)
	}
	code := exitOK
	select {
	case <-ctx.Done():
	case <-timer:
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			_, _ = fmt.Fprintf(stderr, "devlatch %s: %v\n", name, err)
			code = exitError
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	p.Close()
	return store, p.Stats(), code
}
