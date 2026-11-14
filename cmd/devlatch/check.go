package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/OWNER/devlatch/internal/config"
	"github.com/OWNER/devlatch/internal/contract"
	"github.com/OWNER/devlatch/internal/diff"
	"github.com/OWNER/devlatch/internal/infer"
	"github.com/OWNER/devlatch/internal/ingest"
	"github.com/OWNER/devlatch/internal/report"
)

// runCheck compares live traffic (or a replayed traffic file) with the
// approved contract. Exit codes: 0 ok, 1 drift at or above --fail-on, 2 tool error.
func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, _ := config.Load()
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", cfg.Target, "backend URL (default from .devlatch/config.json)")
	port := fs.Int("port", cfg.Port, "proxy port on 127.0.0.1 (0 picks a free port)")
	minSamples := fs.Int("min-samples", cfg.MinSamples, "responses needed per endpoint before a field counts as required")
	renames := fs.Bool("renames", true, "report likely renames as POSSIBLE_RENAME instead of REMOVED_KEY + ADDED_KEY")
	dur := fs.Duration("duration", 0, "stop automatically after this long, e.g. 30s (default: until Ctrl+C)")
	ignore := fs.String("ignore", "", "comma-separated path prefixes to forward but never analyze")
	from := fs.String("from", "", "replay this traffic JSONL file instead of running the proxy (no backend needed)")
	failOn := fs.String("fail-on", "breaking", "exit 1 on this severity or worse: breaking or warning")
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}

	var fail diff.Severity
	switch *failOn {
	case "breaking":
		fail = diff.SeverityBreaking
	case "warning":
		fail = diff.SeverityWarning
	default:
		_, _ = fmt.Fprintln(stderr, "devlatch check: --fail-on must be breaking or warning")
		return exitError
	}
	if *format != "text" && *format != "json" {
		_, _ = fmt.Fprintln(stderr, "devlatch check: --format must be text or json")
		return exitError
	}
	base, err := contract.Load(config.ContractPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, _ = fmt.Fprintln(stderr, "devlatch check: no approved contract; run devlatch record, then devlatch approve")
		} else {
			_, _ = fmt.Fprintln(stderr, "devlatch check:", err)
		}
		return exitError
	}

	info := stdout
	if *format == "json" {
		info = stderr // keep stdout pure JSON
	}

	var store *infer.Store
	var statsLine string
	if *from != "" {
		fh, err := os.Open(*from)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch check:", err)
			return exitError
		}
		store = infer.NewStore(infer.DefaultConfig())
		acc, skip, err := ingest.Replay(fh, store)
		_ = fh.Close()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "devlatch check: %s: %v\n", *from, err)
			return exitError
		}
		statsLine = fmt.Sprintf("replayed %d responses from %s (%d skipped)", acc, *from, skip)
	} else {
		u, ok := parseTarget(*target)
		if !ok {
			_, _ = fmt.Fprintln(stderr, "devlatch check: set --target (or run devlatch init first)")
			return exitError
		}
		if *port < 0 || *port > 65535 {
			_, _ = fmt.Fprintln(stderr, "devlatch check: --port must be between 0 and 65535")
			return exitError
		}
		s, st, code := collect(ctx, collectOpts{target: u, port: *port, dur: *dur, ignore: splitCSV(*ignore)}, info, stderr, "check")
		if code != exitOK {
			return code
		}
		store, statsLine = s, st.String()
	}

	live := collapseEndpoints(store.Snapshot())
	vs := diff.Diff(base.ToNodes(), live, diff.Options{MinSamples: *minSamples, Renames: *renames})

	if *format == "json" {
		if err := report.JSON(stdout, vs); err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch check:", err)
			return exitError
		}
	} else {
		_, _ = fmt.Fprintln(stdout)
		report.Text(stdout, vs, os.Getenv("NO_COLOR") == "")
	}

	var added, missing []string
	for k := range live {
		if _, ok := base.Endpoints[k]; !ok {
			added = append(added, k)
		}
	}
	for k := range base.Endpoints {
		if _, ok := live[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(added)
	sort.Strings(missing)
	for _, k := range added {
		_, _ = fmt.Fprintf(info, "note: new endpoint not in the contract: %s\n", k)
	}
	for _, k := range missing {
		_, _ = fmt.Fprintf(info, "note: contract endpoint not exercised: %s\n", k)
	}
	var thin []string
	for k, n := range live {
		if n.Samples < *minSamples {
			thin = append(thin, fmt.Sprintf("%s (%d samples)", k, n.Samples))
		}
	}
	sort.Strings(thin)
	for _, s := range thin {
		_, _ = fmt.Fprintf(info, "note: low live sample count: %s; required-field checks may be incomplete\n", s)
	}
	_, _ = fmt.Fprintln(info, statsLine)
	return diff.ExitCode(vs, fail)
}
