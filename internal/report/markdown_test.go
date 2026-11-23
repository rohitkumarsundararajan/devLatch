package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
)

func TestMarkdown(t *testing.T) {
	var clean bytes.Buffer
	Markdown(&clean, nil)
	if !strings.Contains(clean.String(), Marker) || !strings.Contains(clean.String(), "no drift detected") {
		t.Fatalf("clean report wrong:\n%s", clean.String())
	}

	vs := []diff.Violation{{
		Endpoint:   "GET /api/users/{id} 200",
		Path:       "$.user_name",
		Severity:   diff.SeverityBreaking,
		RenameFrom: "user_name",
		RenameTo:   "username",
		Confidence: 0.91,
		Evidence:   []string{"a|b"},
	}}
	var out bytes.Buffer
	Markdown(&out, vs)
	got := out.String()
	for _, want := range []string{Marker, "**FAILED**", "1 breaking", "`user_name` -> `username`", "a\\|b", "| Severity |"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
