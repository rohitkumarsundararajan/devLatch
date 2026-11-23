package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/ingest"
	"github.com/rohitkumarsundararajan/devlatch/internal/version"
)

// runRecord learns a contract proposal from live traffic and saves it to
// .devlatch/proposed.json. Nothing is enforced until the human runs approve.
func runRecord(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, _ := config.Load() // defaults when no config yet
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", cfg.Target, "backend URL (default from .devlatch/config.json)")
	port := fs.Int("port", cfg.Port, "proxy port on 127.0.0.1 (0 picks a free port)")
	minSamples := fs.Int("min-samples", cfg.MinSamples, "responses needed per endpoint before a field counts as required")
	dur := fs.Duration("duration", 0, "stop automatically after this long, e.g. 2m (default: until Ctrl+C)")
	ignore := fs.String("ignore", strings.Join(cfg.Ignore, ","), "comma-separated path prefixes to forward but never analyze")
	traffic := fs.String("save-traffic", "", "also save analyzed responses to this JSONL file for later replay (contains raw bodies)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}
	u, ok := parseTarget(*target)
	if !ok {
		_, _ = fmt.Fprintln(stderr, "devlatch record: set --target (or run devlatch init first)")
		return exitError
	}
	if *port < 0 || *port > 65535 {
		_, _ = fmt.Fprintln(stderr, "devlatch record: --port must be between 0 and 65535")
		return exitError
	}

	var tw *ingest.Writer
	var hook func(method, path string, status int, body []byte)
	if *traffic != "" {
		w, err := ingest.NewWriter(*traffic)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch record:", err)
			return exitError
		}
		tw, hook = w, trafficHook(w)
	}

	store, stats, code := collect(ctx, collectOpts{target: u, port: *port, dur: *dur, ignore: splitCSV(*ignore), onSample: hook}, stdout, stderr, "record")
	if tw != nil {
		if err := tw.Close(); err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch record: saving traffic:", err)
			return exitError
		}
	}
	if code != exitOK {
		return code
	}

	nodes := collapseEndpoints(store.Snapshot())
	f := contract.FromNodes(nodes, *minSamples, version.Version, time.Now())
	if len(f.Endpoints) == 0 {
		_, _ = fmt.Fprintln(stderr, "devlatch record: no JSON responses were observed, nothing saved")
		return exitError
	}
	if err := contract.Save(config.ProposedPath(), f); err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch record:", err)
		return exitError
	}
	_, _ = fmt.Fprintf(stdout, "\nRecorded %d endpoints -> %s\n%s\n", len(f.Endpoints), config.ProposedPath(), stats)
	if *traffic != "" {
		_, _ = fmt.Fprintf(stdout, "WARNING: %s contains raw response bodies. Do not commit it; add it to .gitignore.\n", *traffic)
	}
	for _, w := range f.LowSample(*minSamples) {
		_, _ = fmt.Fprintf(stdout, "low confidence: %s; record more traffic\n", w)
	}
	_, _ = fmt.Fprintln(stdout, "Review it, then run: devlatch approve")
	return exitOK
}
