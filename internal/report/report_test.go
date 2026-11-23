package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
)

var sample = []diff.Violation{
	{Endpoint: "GET /u 200", Path: "$.price", Type: diff.TypeChanged, Severity: diff.SeverityBreaking,
		Expected: []string{"NUMBER"}, Actual: []string{"STRING"}},
	{Endpoint: "GET /u 200", Path: "$.name", Type: diff.NullabilityChanged, Severity: diff.SeverityWarning},
	{Endpoint: "GET /v 200", Path: "$.email", Type: diff.AddedKey, Severity: diff.SeveritySafe},
	{Endpoint: "GET /v 200", Path: "$.user_id", Type: diff.PossibleRename, Severity: diff.SeverityBreaking,
		RenameFrom: "userId", RenameTo: "user_id", Confidence: 0.93},
}

func TestTextPlainAndColor(t *testing.T) {
	var plain bytes.Buffer
	Text(&plain, sample, false)
	out := plain.String()
	for _, want := range []string{"GET /u 200", "BREAKING", "expected NUMBER; got STRING", "userId -> user_id, confidence 0.93", "2 breaking, 1 warning, 1 safe"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("plain output must not contain ANSI codes")
	}
	var colored bytes.Buffer
	Text(&colored, sample, true)
	if !strings.Contains(colored.String(), "\x1b[31m") || !strings.Contains(colored.String(), "\x1b[33m") {
		t.Fatal("colored output should contain red and yellow codes")
	}
}

func TestTextEmpty(t *testing.T) {
	var b bytes.Buffer
	Text(&b, nil, false)
	if !strings.Contains(b.String(), "No drift detected.") {
		t.Fatal("empty report message missing")
	}
	b.Reset()
	Text(&b, nil, true)
	if !strings.Contains(b.String(), "\x1b[32m") {
		t.Fatal("empty colored report should be green")
	}
}

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, sample); err != nil {
		t.Fatal(err)
	}
	var out Output
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Violations) != 4 || out.Summary.Breaking != 2 || out.Summary.Warning != 1 || out.Summary.Safe != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
	b.Reset()
	_ = JSON(&b, nil)
	if !strings.Contains(b.String(), `"violations": []`) {
		t.Fatalf("nil violations must encode as [], got:\n%s", b.String())
	}
}
