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
	"os"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
	"github.com/rohitkumarsundararajan/devlatch/internal/proxy"
	"github.com/rohitkumarsundararajan/devlatch/internal/server"
	"github.com/rohitkumarsundararajan/devlatch/internal/version"
)

// driftScanner turns the proxy's learned schemas into dashboard events:
// "endpoint" when a new endpoint is first seen, "drift" for each violation
// against the approved contract that has not been reported before.
type driftScanner struct {
	base      map[string]*infer.Node
	opt       diff.Options
	hub       *server.Hub
	seenEP    map[string]bool
	seenDrift map[string]bool
	lastTotal int
}

func newDriftScanner(base map[string]*infer.Node, opt diff.Options, hub *server.Hub) *driftScanner {
	return &driftScanner{base: base, opt: opt, hub: hub, seenEP: map[string]bool{}, seenDrift: map[string]bool{}}
}

// scan runs one pass and returns the violations that were new in this pass.
func (d *driftScanner) scan(store *infer.Store) []diff.Violation {
	snap := store.Snapshot()
	total := 0
	for _, ep := range snap {
		total += ep.Responses
		if !d.seenEP[ep.Key] {
			d.seenEP[ep.Key] = true
			d.hub.Publish("endpoint", map[string]string{"key": ep.Key})
		}
	}
	if total == d.lastTotal || d.base == nil {
		d.lastTotal = total
		return nil
	}
	d.lastTotal = total
	live := collapseEndpoints(snap)
	var fresh []diff.Violation
	for _, v := range diff.Diff(d.base, live, d.opt) {
		k := fmt.Sprintf("%s|%s|%s|%s|%s", v.Endpoint, v.Path, v.Type, v.RenameFrom, v.RenameTo)
		if d.seenDrift[k] {
			continue
		}
		d.seenDrift[k] = true
		d.hub.Publish("drift", v)
		fresh = append(fresh, v)
	}
	return fresh
}

// runUI runs the proxy plus the dashboard API (localhost only) until Ctrl+C.
func runUI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, _ := config.Load()
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", cfg.Target, "backend URL (default from .devlatch/config.json)")
	port := fs.Int("port", cfg.Port, "proxy port on 127.0.0.1 (0 picks a free port)")
	uiPort := fs.Int("ui-port", 7070, "dashboard port on 127.0.0.1 (0 picks a free port)")
	minSamples := fs.Int("min-samples", cfg.MinSamples, "responses needed per endpoint before a field counts as required")
	renames := fs.Bool("renames", true, "report likely renames as POSSIBLE_RENAME")
	ignore := fs.String("ignore", "", "comma-separated path prefixes to forward but never analyze")
	interval := fs.Duration("interval", time.Second, "how often to compare live traffic with the contract")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}

	u, err := url.Parse(*target)
	if *target == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		_, _ = fmt.Fprintln(stderr, "devlatch ui: set --target (or run devlatch init first), e.g. http://localhost:8000")
		return exitError
	}
	if *port < 0 || *port > 65535 || *uiPort < 0 || *uiPort > 65535 {
		_, _ = fmt.Fprintln(stderr, "devlatch ui: ports must be between 0 and 65535")
		return exitError
	}
	if *interval < 100*time.Millisecond {
		*interval = 100 * time.Millisecond
	}

	var base map[string]*infer.Node
	if f, err := contract.Load(config.ContractPath()); err == nil {
		base = f.ToNodes()
	} else if errors.Is(err, os.ErrNotExist) {
		_, _ = fmt.Fprintln(stderr, "devlatch ui: no approved contract yet; learning endpoints only (run devlatch record, then devlatch approve)")
	} else {
		_, _ = fmt.Fprintln(stderr, "devlatch ui:", err)
		return exitError
	}

	store := infer.NewStore(infer.DefaultConfig())
	p, err := proxy.New(proxy.Options{
		Target:         u,
		Store:          store,
		QueueSize:      64,
		Workers:        2,
		MaxBodyBytes:   1 << 20,
		SampleEvery:    1,
		IgnorePrefixes: splitCSV(*ignore),
		Logger:         log.New(stderr, "", log.LstdFlags),
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch ui:", err)
		return exitError
	}
	pln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch ui:", err)
		p.Close()
		return exitError
	}
	uln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *uiPort))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch ui:", err)
		_ = pln.Close()
		p.Close()
		return exitError
	}

	hub := server.NewHub(500)
	ui := server.New(server.Config{
		Version: version.Version,
		Hub:     hub,
		Endpoints: func() []server.EndpointInfo {
			var out []server.EndpointInfo
			for _, ep := range store.Snapshot() {
				out = append(out, server.EndpointInfo{Key: ep.Key, Responses: ep.Responses})
			}
			return out
		},
		Contract: func() (any, error) {
			f, err := contract.Load(config.ContractPath())
			if err != nil {
				return nil, err
			}
			return f, nil
		},
	})

	psrv := &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second}
	usrv := &http.Server{Handler: ui.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 2)
	go func() { errc <- psrv.Serve(pln) }()
	go func() { errc <- usrv.Serve(uln) }()

	hub.Publish("status", map[string]any{"state": "started", "target": u.String(), "contract": base != nil})
	_, _ = fmt.Fprintf(stdout, "DevLatch ui: dashboard API http://%s  (events: /api/events)\n", uln.Addr())
	_, _ = fmt.Fprintf(stdout, "Point your app at the proxy http://%s -> %s. Press Ctrl+C to stop.\n", pln.Addr(), u)

	ds := newDriftScanner(base, diff.Options{MinSamples: *minSamples, Renames: *renames}, hub)
	code := exitOK
	tick := time.NewTicker(*interval)
	defer tick.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case err := <-errc:
			if !errors.Is(err, http.ErrServerClosed) {
				_, _ = fmt.Fprintln(stderr, "devlatch ui:", err)
				code = exitError
			}
			break loop
		case <-tick.C:
			for _, v := range ds.scan(store) {
				_, _ = fmt.Fprintf(stdout, "drift: %s %s %s\n", v.Severity, v.Type, v.Endpoint+" "+v.Path)
			}
		}
	}

	hub.Close() // ends open SSE streams so shutdown does not wait on them
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = usrv.Shutdown(sctx)
	_ = psrv.Shutdown(sctx)
	p.Close()
	return code
}
