package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/report"
)

// runDiff compares two contract files: devlatch diff [flags] OLD NEW.
// Exit codes: 0 ok, 1 drift at or above --fail-on, 2 tool error.
func runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	failOn := fs.String("fail-on", "breaking", "exit 1 on this severity or worse: breaking or warning")
	format := fs.String("format", "text", "output format: text or json")
	minSamples := fs.Int("min-samples", 20, "baseline responses needed before a field counts as required")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}
	if fs.NArg() != 2 {
		_, _ = fmt.Fprintln(stderr, "usage: devlatch diff [flags] OLD.json NEW.json")
		return exitError
	}
	var fail diff.Severity
	switch *failOn {
	case "breaking":
		fail = diff.SeverityBreaking
	case "warning":
		fail = diff.SeverityWarning
	default:
		_, _ = fmt.Fprintln(stderr, "devlatch diff: --fail-on must be breaking or warning")
		return exitError
	}
	if *format != "text" && *format != "json" {
		_, _ = fmt.Fprintln(stderr, "devlatch diff: --format must be text or json")
		return exitError
	}
	oldC, err := contract.Load(fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch diff:", err)
		return exitError
	}
	newC, err := contract.Load(fs.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch diff:", err)
		return exitError
	}
	vs := diff.Diff(oldC.ToNodes(), newC.ToNodes(), diff.Options{MinSamples: *minSamples})
	if *format == "json" {
		if err := report.JSON(stdout, vs); err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch diff:", err)
			return exitError
		}
	} else {
		for _, k := range missingKeys(oldC.Endpoints, newC.Endpoints) {
			_, _ = fmt.Fprintf(stdout, "+ new endpoint %s\n", k)
		}
		for _, k := range missingKeys(newC.Endpoints, oldC.Endpoints) {
			_, _ = fmt.Fprintf(stdout, "- removed endpoint %s\n", k)
		}
		report.Text(stdout, vs, os.Getenv("NO_COLOR") == "")
	}
	return diff.ExitCode(vs, fail)
}
