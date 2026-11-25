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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/fix"
	"github.com/rohitkumarsundararajan/devlatch/internal/impact"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
	"github.com/rohitkumarsundararajan/devlatch/internal/notify"
	"github.com/rohitkumarsundararajan/devlatch/internal/proxy"
	"github.com/rohitkumarsundararajan/devlatch/internal/server"
	"github.com/rohitkumarsundararajan/devlatch/internal/version"
	"github.com/rohitkumarsundararajan/devlatch/internal/webui"
)

// driftScanner turns the proxy's learned schemas into dashboard events:
// "endpoint" when a new endpoint is first seen, "drift" for each violation
// against the approved contract that has not been reported before.
type driftScanner struct {
	mu        sync.Mutex
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
	d.mu.Lock()
	defer d.mu.Unlock()
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

// rebase swaps in a new approved contract and forgets the drift already
// reported for the given endpoints (all of them when none are given).
func (d *driftScanner) rebase(base map[string]*infer.Node, endpoints []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.base = base
	if len(endpoints) == 0 {
		d.seenDrift = map[string]bool{}
	}
	for k := range d.seenDrift {
		for _, e := range endpoints {
			if strings.HasPrefix(k, e+"|") {
				delete(d.seenDrift, k)
			}
		}
	}
	d.lastTotal = -1
}

// approveLive accepts what the proxy has seen as the contract: for one
// endpoint, or for all of them when endpoint is empty.
func approveLive(store *infer.Store, ds *driftScanner, endpoint string, minSamples int) (any, error) {
	live := collapseEndpoints(store.Snapshot())
	if endpoint != "" {
		n, ok := live[endpoint]
		if !ok {
			return nil, fmt.Errorf("endpoint %q has no live traffic yet", endpoint)
		}
		live = map[string]*infer.Node{endpoint: n}
	}
	if len(live) == 0 {
		return nil, errors.New("no live traffic to approve")
	}
	f := contract.FromNodes(live, minSamples, version.Version, time.Now())
	if old, err := contract.Load(config.ContractPath()); err == nil {
		if old.Endpoints == nil {
			old.Endpoints = map[string]*contract.Endpoint{}
		}
		for k, ep := range f.Endpoints {
			old.Endpoints[k] = ep
		}
		f = old
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f.ApprovedBy = approver()
	f.ApprovedAt = time.Now().UTC().Format(time.RFC3339)
	if err := os.MkdirAll(config.Dir, 0o755); err != nil {
		return nil, err
	}
	if err := contract.Save(config.ContractPath(), f); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(live))
	for k := range live {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ds.rebase(f.ToNodes(), keys)
	return map[string]any{"approved": keys, "contract_endpoints": len(f.Endpoints)}, nil
}

type impactOut struct {
	impact.Result
	FileCount int `json:"files"`
}

func breakingSummary(vs []diff.Violation) (int, string) {
	n, first := 0, ""
	for _, v := range vs {
		if v.Severity == diff.SeverityBreaking {
			n++
			if first == "" {
				first = v.Endpoint + " " + v.Path
			}
		}
	}
	return n, first
}

// runUI runs the proxy plus the dashboard (localhost only) until Ctrl+C.
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
	frontend := fs.String("frontend", "", "frontend project folder (enables the impact and fix views)")
	tsconfig := fs.String("tsconfig", "", "path to tsconfig.json (default <frontend>/tsconfig.json)")
	scannerPath := fs.String("scanner", "", "path to scanner/src/cli.mjs (auto-detected)")
	noNotify := fs.Bool("no-notify", false, "do not show desktop notifications for breaking drift")
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
		_, _ = fmt.Fprintln(stderr, "devlatch ui: no approved contract yet; learning endpoints only (approve from the dashboard once traffic is learned)")
	} else {
		_, _ = fmt.Fprintln(stderr, "devlatch ui:", err)
		return exitError
	}

	// Impact and fix need the frontend folder and the scanner.
	var acts server.Actions
	frontendNote := "Impact and fix views are off (pass --frontend <folder> to enable them)."
	ts := *tsconfig
	if ts == "" && *frontend != "" {
		ts = filepath.Join(*frontend, "tsconfig.json")
	}
	if ts != "" {
		if _, err := os.Stat(ts); err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch ui:", err)
			return exitError
		}
		script, err := impact.FindScanner(*scannerPath)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch ui: impact and fix views are off:", err)
		} else {
			frontendNote = "Impact and fix views use " + ts
			acts.Impact = func(ep, p string) (any, error) {
				rs, err := impact.Scan(script, ts, []impact.Drift{{Endpoint: ep, Path: p}})
				if err != nil {
					return nil, err
				}
				out := impactOut{Result: impact.Result{Endpoint: ep, Path: p}}
				if len(rs) > 0 {
					out.Result = rs[0]
				}
				if out.Hits == nil {
					out.Hits = []impact.Hit{}
				}
				out.FileCount = out.Files()
				return out, nil
			}
			fixRun := func(ep, p, to string, apply bool) (any, error) {
				if !fixIdentRe.MatchString(to) {
					return nil, fmt.Errorf("%q is not a valid field name", to)
				}
				res, err := fix.Run(script, ts, []fix.Rename{{Endpoint: ep, Path: p, To: to}}, fix.Options{Apply: apply})
				if err != nil {
					return nil, err
				}
				if apply {
					if res.Reverted {
						return nil, errors.New("the rename would increase TypeScript errors, so nothing was changed")
					}
					if !res.Applied {
						return nil, errors.New("nothing was applied")
					}
				}
				if res.Changes == nil {
					res.Changes = []fix.Change{}
				}
				return res, nil
			}
			acts.FixPreview = func(ep, p, to string) (any, error) { return fixRun(ep, p, to, false) }
			acts.FixApply = func(ep, p, to string) (any, error) { return fixRun(ep, p, to, true) }
		}
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
	ds := newDriftScanner(base, diff.Options{MinSamples: *minSamples, Renames: *renames}, hub)
	acts.Approve = func(ep string) (any, error) { return approveLive(store, ds, ep, *minSamples) }

	ui := server.New(server.Config{
		Version: version.Version,
		Hub:     hub,
		UI:      webui.Handler(),
		Actions: acts,
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
	_, _ = fmt.Fprintln(stdout, frontendNote)

	var nf *notify.Notifier
	if !*noNotify {
		nf = notify.New(10 * time.Second)
	}
	var warnOnce sync.Once

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
			fresh := ds.scan(store)
			for _, v := range fresh {
				_, _ = fmt.Fprintf(stdout, "drift: %s %s %s\n", v.Severity, v.Type, v.Endpoint+" "+v.Path)
			}
			if n, first := breakingSummary(fresh); nf != nil && n > 0 {
				msg := fmt.Sprintf("%d breaking change(s), first: %s", n, first)
				go func() {
					if _, err := nf.Notify("DevLatch: breaking API change", msg); err != nil {
						warnOnce.Do(func() { _, _ = fmt.Fprintln(stderr, "devlatch ui: desktop notification failed:", err) })
					}
				}()
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
