package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OWNER/devlatch/internal/infer"
	"github.com/OWNER/devlatch/internal/proxy"
)

// runWatch starts the proxy and runs until ctx is cancelled (Ctrl+C), then
// prints everything it learned.
func runWatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "backend URL to forward to, e.g. http://localhost:8000 (required)")
	port := fs.Int("port", 9000, "port for the DevLatch proxy to listen on (127.0.0.1 only; 0 picks a free port)")
	maxBody := fs.Int("max-body", 1<<20, "largest response body in bytes to analyze")
	queue := fs.Int("queue", 64, "analysis queue size; samples are dropped when it is full")
	workers := fs.Int("workers", 2, "number of analysis workers")
	sampleEvery := fs.Int("sample-every", 1, "analyze every Nth JSON response")
	ignore := fs.String("ignore", "", "comma-separated path prefixes to forward but never analyze")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}

	u, err := url.Parse(*target)
	if *target == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		_, _ = fmt.Fprintln(stderr, "devlatch watch: --target is required and must look like http://localhost:8000")
		return exitError
	}
	if *port < 0 || *port > 65535 {
		_, _ = fmt.Fprintln(stderr, "devlatch watch: --port must be between 0 and 65535")
		return exitError
	}
	var prefixes []string
	for _, p := range strings.Split(*ignore, ",") {
		if p = strings.TrimSpace(p); p != "" {
			prefixes = append(prefixes, p)
		}
	}

	store := infer.NewStore(infer.DefaultConfig())
	p, err := proxy.New(proxy.Options{
		Target:         u,
		Store:          store,
		QueueSize:      *queue,
		Workers:        *workers,
		MaxBodyBytes:   *maxBody,
		SampleEvery:    *sampleEvery,
		IgnorePrefixes: prefixes,
		Logger:         log.New(stderr, "", log.LstdFlags),
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch watch:", err)
		return exitError
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch watch:", err)
		p.Close()
		return exitError
	}
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	_, _ = fmt.Fprintf(stdout, "DevLatch watch: http://%s -> %s\n", ln.Addr(), u)
	_, _ = fmt.Fprintln(stdout, "Point your app at the proxy. Press Ctrl+C to stop and print what was learned.")

	code := exitOK
	printed := map[string]bool{}
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case err := <-serveErr:
			if !errors.Is(err, http.ErrServerClosed) {
				_, _ = fmt.Fprintln(stderr, "devlatch watch:", err)
				code = exitError
			}
			break loop
		case <-tick.C:
			for _, key := range store.Keys() {
				if !printed[key] {
					printed[key] = true
					_, _ = fmt.Fprintf(stdout, "+ %s\n", key)
				}
			}
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	p.Close() // analyze whatever is still queued

	printReport(stdout, store, p.Stats())
	return code
}

func printReport(w io.Writer, store *infer.Store, stats proxy.Stats) {
	snap := store.Snapshot()
	_, _ = fmt.Fprintln(w, "\n=== Inferred schemas (name? = optional key) ===")
	if len(snap) == 0 {
		_, _ = fmt.Fprintln(w, "(no JSON responses were analyzed)")
	}
	for _, ep := range snap {
		_, _ = fmt.Fprintf(w, "\n%s  (%d responses)\n%s\n", ep.Key, ep.Responses, infer.Render(ep.Node))
	}
	_, _ = fmt.Fprintf(w, "\n=== Proxy stats ===\n%s\n", stats)
}
