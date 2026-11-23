package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
	"github.com/rohitkumarsundararajan/devlatch/internal/normalize"
	"github.com/rohitkumarsundararajan/devlatch/internal/report"
	"github.com/rohitkumarsundararajan/devlatch/internal/version"
)

func parseHeaderEnv(s string, into map[string]string) error {
	for _, pair := range splitCSV(s) {
		i := strings.IndexByte(pair, '=')
		if i <= 0 || i == len(pair)-1 {
			return fmt.Errorf("--header-env %q must look like Header-Name=ENV_VAR", pair)
		}
		into[pair[:i]] = pair[i+1:]
	}
	return nil
}

func probeOnce(ctx context.Context, client *http.Client, u string, headers map[string]string) (int, []byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "devlatch-probe/"+version.Version)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, body, resp.Header.Get("Content-Type"), err
}

// cleanErr drops the URL from network errors so query strings never reach logs.
func cleanErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// runProbe sends GET requests straight to an API (no proxy) and either records
// a proposal (--record) or checks the responses against the approved contract.
// Exit codes: 0 ok, 1 drift at or above --fail-on, 2 tool error.
func runProbe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, _ := config.Load()
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", cfg.Target, "base URL of the API to probe")
	paths := fs.String("paths", strings.Join(cfg.Probe, ","), "comma-separated GET paths, e.g. /posts,/users/1")
	hdr := fs.String("header-env", "", "comma-separated Header=ENV_VAR pairs; values are read from the environment, never saved")
	repeat := fs.Int("repeat", 1, "requests per path")
	minSamples := fs.Int("min-samples", 1, "responses needed before a field counts as required")
	record := fs.Bool("record", false, "save what was observed as a proposal (then run devlatch approve) instead of checking")
	failOn := fs.String("fail-on", "breaking", "exit 1 on this severity or worse: breaking or warning")
	format := fs.String("format", "text", "output format: text, json or markdown")
	renames := fs.Bool("renames", true, "report likely renames as POSSIBLE_RENAME instead of REMOVED_KEY + ADDED_KEY")
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
		_, _ = fmt.Fprintln(stderr, "devlatch probe: --fail-on must be breaking or warning")
		return exitError
	}
	if *format != "text" && *format != "json" && *format != "markdown" {
		_, _ = fmt.Fprintln(stderr, "devlatch probe: --format must be text, json or markdown")
		return exitError
	}
	baseURL, ok := parseTarget(*base)
	if !ok {
		_, _ = fmt.Fprintln(stderr, "devlatch probe: --base is required and must look like https://api.example.com")
		return exitError
	}
	var plist []string
	for _, p := range splitCSV(*paths) {
		if !strings.HasPrefix(p, "/") {
			_, _ = fmt.Fprintf(stderr, "devlatch probe: path %q must start with /\n", p)
			return exitError
		}
		plist = append(plist, p)
	}
	if len(plist) == 0 {
		_, _ = fmt.Fprintln(stderr, "devlatch probe: --paths is required (or set \"probe\" in config.json)")
		return exitError
	}
	if *repeat < 1 {
		*repeat = 1
	}

	envNames := map[string]string{}
	for k, v := range cfg.ProbeHeaders {
		envNames[k] = v
	}
	if err := parseHeaderEnv(*hdr, envNames); err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch probe:", err)
		return exitError
	}
	headers := map[string]string{}
	for h, env := range envNames {
		v := os.Getenv(env)
		if v == "" {
			_, _ = fmt.Fprintf(stderr, "devlatch probe: environment variable %s (for header %s) is not set\n", env, h)
			return exitError
		}
		headers[h] = v
	}

	var baseline *contract.File
	if !*record {
		var err error
		baseline, err = contract.Load(config.ContractPath())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				_, _ = fmt.Fprintln(stderr, "devlatch probe: no approved contract; run probe --record, then devlatch approve")
			} else {
				_, _ = fmt.Fprintln(stderr, "devlatch probe:", err)
			}
			return exitError
		}
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		// Never follow redirects: custom auth headers must not travel to another host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	store := infer.NewStore(infer.DefaultConfig())
	var failures []string
	root := strings.TrimRight(baseURL.String(), "/")
	for _, path := range plist {
		for i := 0; i < *repeat; i++ {
			if ctx.Err() != nil {
				break
			}
			status, body, ct, err := probeOnce(ctx, client, root+path, headers)
			if err != nil {
				failures = append(failures, path+": "+cleanErr(err))
				break
			}
			if !strings.Contains(strings.ToLower(ct), "json") {
				continue
			}
			clean := path
			if q := strings.IndexByte(clean, '?'); q >= 0 {
				clean = clean[:q]
			}
			if err := store.Observe(normalize.EndpointKey("GET", clean, status), body); err != nil {
				failures = append(failures, path+": response is not valid JSON")
				break
			}
		}
	}
	if len(failures) > 0 {
		for _, f := range failures {
			_, _ = fmt.Fprintln(stderr, "devlatch probe:", f)
		}
		return exitError
	}
	live := collapseEndpoints(store.Snapshot())

	if *record {
		f := contract.FromNodes(live, *minSamples, version.Version, time.Now())
		if len(f.Endpoints) == 0 {
			_, _ = fmt.Fprintln(stderr, "devlatch probe: no JSON responses were observed, nothing saved")
			return exitError
		}
		if err := contract.Save(config.ProposedPath(), f); err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch probe:", err)
			return exitError
		}
		_, _ = fmt.Fprintf(stdout, "Recorded %d endpoints -> %s\nReview it, then run: devlatch approve\n", len(f.Endpoints), config.ProposedPath())
		return exitOK
	}

	vs := diff.Diff(baseline.ToNodes(), live, diff.Options{MinSamples: *minSamples, Renames: *renames})
	expected := make([]string, 0, len(baseline.Endpoints))
	for k := range baseline.Endpoints {
		expected = append(expected, k)
	}
	sort.Strings(expected)
	vs = append(vs, diff.MissingEndpoints(expected, live)...)
	diff.SortViolations(vs)

	info := stdout
	switch *format {
	case "json":
		info = stderr
		if err := report.JSON(stdout, vs); err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch probe:", err)
			return exitError
		}
	case "markdown":
		info = stderr
		report.Markdown(stdout, vs)
	default:
		report.Text(stdout, vs, os.Getenv("NO_COLOR") == "")
	}
	var fresh []string
	for k := range live {
		if _, ok := baseline.Endpoints[k]; !ok {
			fresh = append(fresh, k)
		}
	}
	sort.Strings(fresh)
	for _, k := range fresh {
		_, _ = fmt.Fprintf(info, "note: new endpoint not in the contract: %s\n", k)
	}
	return diff.ExitCode(vs, fail)
}
