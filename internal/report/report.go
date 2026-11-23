package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
)

const (
	red    = "\x1b[31m"
	yellow = "\x1b[33m"
	green  = "\x1b[32m"
	bold   = "\x1b[1m"
	reset  = "\x1b[0m"
)

func paint(on bool, code, s string) string {
	if !on {
		return s
	}
	return code + s + reset
}

func sevColor(s diff.Severity) string {
	switch s {
	case diff.SeverityBreaking:
		return red
	case diff.SeverityWarning:
		return yellow
	}
	return green
}

// Count returns how many violations fall in each severity.
func Count(vs []diff.Violation) (breaking, warning, safe int) {
	for _, v := range vs {
		switch v.Severity {
		case diff.SeverityBreaking:
			breaking++
		case diff.SeverityWarning:
			warning++
		case diff.SeveritySafe:
			safe++
		}
	}
	return
}

func detail(v diff.Violation) string {
	var parts []string
	if len(v.Expected) > 0 {
		parts = append(parts, "expected "+strings.Join(v.Expected, "|"))
	}
	if len(v.Actual) > 0 {
		parts = append(parts, "got "+strings.Join(v.Actual, "|"))
	}
	if v.RenameFrom != "" {
		parts = append(parts, fmt.Sprintf("%s -> %s, confidence %.2f", v.RenameFrom, v.RenameTo, v.Confidence))
	}
	if len(v.Evidence) > 0 {
		evid := ""
		for i, e := range v.Evidence {
			if i > 0 {
				evid += "; "
			}
			evid += e
		}
		parts = append(parts, "evidence: "+evid)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  (" + strings.Join(parts, "; ") + ")"
}

// Text writes a human-readable report grouped by endpoint.
func Text(w io.Writer, vs []diff.Violation, color bool) {
	if len(vs) == 0 {
		_, _ = fmt.Fprintln(w, paint(color, green, "No drift detected."))
		return
	}
	last := ""
	for _, v := range vs {
		if v.Endpoint != last {
			if last != "" {
				_, _ = fmt.Fprintln(w)
			}
			_, _ = fmt.Fprintln(w, paint(color, bold, v.Endpoint))
			last = v.Endpoint
		}
		tag := paint(color, sevColor(v.Severity), fmt.Sprintf("%-8s", v.Severity))
		_, _ = fmt.Fprintf(w, "  %s %-22s %s%s\n", tag, v.Type, v.Path, detail(v))
	}
	b, wn, s := Count(vs)
	_, _ = fmt.Fprintf(w, "\n%d breaking, %d warning, %d safe\n", b, wn, s)
}

type Summary struct {
	Breaking int `json:"breaking"`
	Warning  int `json:"warning"`
	Safe     int `json:"safe"`
}

type Output struct {
	Violations []diff.Violation `json:"violations"`
	Summary    Summary          `json:"summary"`
}

// JSON writes machine-readable output (used by CI and the PR comment later).
func JSON(w io.Writer, vs []diff.Violation) error {
	if vs == nil {
		vs = []diff.Violation{}
	}
	b, wn, s := Count(vs)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Output{Violations: vs, Summary: Summary{b, wn, s}})
}
