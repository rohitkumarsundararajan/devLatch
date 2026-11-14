package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/OWNER/devlatch/internal/impact"
)

// runImpact maps drifted fields to the frontend files that read them.
func runImpact(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("impact", flag.ContinueOnError)
	fs.SetOutput(stderr)
	frontend := fs.String("frontend", "", "frontend project folder (contains tsconfig.json)")
	tsconfig := fs.String("tsconfig", "", "path to tsconfig.json (default <frontend>/tsconfig.json)")
	endpoint := fs.String("endpoint", "", `endpoint, e.g. "GET /api/v1/users/{id}"`)
	paths := fs.String("path", "", "drifted field path(s), comma separated, e.g. '$.user_name'")
	driftsFile := fs.String("drifts", "", `JSON file: [{"endpoint":"...","path":"..."}]`)
	fromCheck := fs.String("from-check", "", "read drifts from devlatch check --format json output (file path, or - for stdin)")
	asJSON := fs.Bool("json", false, "print raw JSON")
	noName := fs.Bool("no-name-only", false, "hide low-confidence name-only matches")
	scannerPath := fs.String("scanner", "", "path to scanner/src/cli.mjs (auto-detected)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}

	var drifts []impact.Drift
	if *driftsFile != "" {
		b, err := os.ReadFile(*driftsFile)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch impact:", err)
			return exitError
		}
		if err := json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &drifts); err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch impact: bad drifts file:", err)
			return exitError
		}
	}
	if *fromCheck != "" {
		var raw []byte
		var rerr error
		if *fromCheck == "-" {
			raw, rerr = io.ReadAll(os.Stdin)
		} else {
			raw, rerr = os.ReadFile(*fromCheck)
		}
		if rerr != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch impact:", rerr)
			return exitError
		}
		more, perr := impact.FromCheck(raw)
		if perr != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch impact: bad check output:", perr)
			return exitError
		}
		drifts = append(drifts, more...)
		if len(drifts) == 0 {
			_, _ = fmt.Fprintln(stdout, "no breaking or warning drift to scan")
			return exitOK
		}
	}
	if *endpoint != "" {
		for _, p := range strings.Split(*paths, ",") {
			if p = strings.TrimSpace(p); p != "" {
				drifts = append(drifts, impact.Drift{Endpoint: *endpoint, Path: p})
			}
		}
	}
	if len(drifts) == 0 {
		_, _ = fmt.Fprintln(stderr, "devlatch impact: give --endpoint and --path (or --drifts file.json)")
		return exitError
	}

	cfg := *tsconfig
	if cfg == "" {
		if *frontend == "" {
			_, _ = fmt.Fprintln(stderr, "devlatch impact: give --frontend <folder> or --tsconfig <file>")
			return exitError
		}
		cfg = filepath.Join(*frontend, "tsconfig.json")
	}
	if _, err := os.Stat(cfg); err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch impact:", err)
		return exitError
	}
	script, err := impact.FindScanner(*scannerPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch impact:", err)
		return exitError
	}
	results, err := impact.Scan(script, cfg, drifts)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch impact:", err)
		return exitError
	}
	if *noName {
		for i := range results {
			var keep []impact.Hit
			for _, h := range results[i].Hits {
				if h.Confidence != "name-only" {
					keep = append(keep, h)
				}
			}
			results[i].Hits = keep
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return exitOK
	}
	for _, r := range results {
		_, _ = fmt.Fprintf(stdout, "%s  (%s)  affects %d files\n", r.Path, r.Endpoint, r.Files())
		if r.EndpointCalls == 0 {
			_, _ = fmt.Fprintln(stdout, "  note: no frontend calls to this endpoint were found; only name matches are shown")
		}
		for _, h := range r.Hits {
			_, _ = fmt.Fprintf(stdout, "  %s:%d  %s  (%s, %s)\n", h.File, h.Line, h.Snippet, h.Confidence, h.Kind)
		}
		if len(r.Hits) == 0 {
			_, _ = fmt.Fprintln(stdout, "  no usages found")
		}
	}
	return exitOK
}
