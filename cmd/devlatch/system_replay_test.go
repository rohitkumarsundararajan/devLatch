package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rohitkumarsundararajan/devlatch/internal/report"
)

// chdirTemp moves into a fresh temp dir for the rest of the test and restores the old one after.
func chdirTemp(t *testing.T) string {
	t.Helper()
	old, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

// approvedBaseline records and approves a contract from the clean demo backend in a fresh dir.
func approvedBaseline(t *testing.T, bin string) string {
	t.Helper()
	dir := chdirTemp(t)
	cleanURL, stop := startDemo(t, bin, "")
	if code, _, e := cliWithTraffic(t, cleanURL, "record"); code != exitOK {
		t.Fatalf("record failed (%d): %s", code, e)
	}
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"approve"}, &out, &errb); code != exitOK {
		t.Fatalf("approve failed: %s", errb.String())
	}
	stop()
	return dir
}

// Blueprint scenario 8: replaying recorded traffic gives the same result as the live check.
func TestSystemReplayParity(t *testing.T) {
	if testing.Short() {
		t.Skip("system test skipped in -short mode")
	}
	bin := buildDemoBackend(t)
	mutURL, _ := startDemo(t, bin, "rename_user_name")

	// Capture the mutated traffic to a JSONL file (in a scratch dir, so no contract is touched).
	capDir := chdirTemp(t)
	traffic := filepath.Join(capDir, "mutated.jsonl")
	if code, _, e := cliWithTraffic(t, mutURL, "record", "--save-traffic", traffic); code != exitOK {
		t.Fatalf("capture failed (%d): %s", code, e)
	}

	approvedBaseline(t, bin)

	code, o, e := cliWithTraffic(t, mutURL, "check", "--format", "json")
	live := parseReport(t, o)
	if code != exitDrift || len(live.Violations) == 0 {
		t.Fatalf("live check must find drift (%d): %+v\n%s", code, live.Violations, e)
	}

	var out, errb bytes.Buffer
	rcode := run(context.Background(), []string{"check", "--from", traffic, "--format", "json", "--min-samples", "3"}, &out, &errb)
	replay := parseReport(t, out.String())
	if rcode != code {
		t.Fatalf("replay exit code %d, live %d\n%s", rcode, code, errb.String())
	}
	a, _ := json.Marshal(live.Violations)
	b, _ := json.Marshal(replay.Violations)
	if !bytes.Equal(a, b) {
		t.Fatalf("replay differs from live\nlive:   %s\nreplay: %s", a, b)
	}
}

// Blueprint scenario 9: what CI relies on. Breaking drift exits 1 with the sticky-comment
// marker, warnings only fail with --fail-on warning.
func TestSystemCIExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("system test skipped in -short mode")
	}
	bin := buildDemoBackend(t)
	approvedBaseline(t, bin)

	renameURL, _ := startDemo(t, bin, "rename_user_name")
	code, o, e := cliWithTraffic(t, renameURL, "check", "--format", "markdown")
	if code != exitDrift {
		t.Fatalf("breaking drift must exit %d, got %d\n%s", exitDrift, code, e)
	}
	if !strings.Contains(o, report.Marker) {
		t.Fatalf("markdown output is missing the sticky comment marker:\n%s", o)
	}

	warnURL, _ := startDemo(t, bin, "nullable_nickname")
	if code, _, e := cliWithTraffic(t, warnURL, "check"); code != exitOK {
		t.Fatalf("a warning alone must exit 0 by default, got %d\n%s", code, e)
	}
	if code, _, e := cliWithTraffic(t, warnURL, "check", "--fail-on", "warning"); code != exitDrift {
		t.Fatalf("--fail-on warning must exit %d on a warning, got %d\n%s", exitDrift, code, e)
	}
}
