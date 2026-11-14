package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OWNER/devlatch/internal/config"
	"github.com/OWNER/devlatch/internal/contract"
	"github.com/OWNER/devlatch/internal/infer"
)

func TestConfigRulesApply(t *testing.T) {
	s := infer.NewStore(infer.DefaultConfig())
	for _, slug := range []string{"hello", "world"} {
		if err := s.Observe("GET /posts/"+slug+" 200", []byte(`{"title":"x","Password":"p","meta":{"token":"t","ok":true}}`)); err != nil {
			t.Fatal(err)
		}
	}
	got := collapseWith(s.Snapshot(), config.Config{PathParams: []string{"/posts/*"}, Redact: []string{"password", "TOKEN"}})
	n, ok := got["GET /posts/{param} 200"]
	if len(got) != 1 || !ok {
		t.Fatalf("expected one endpoint GET /posts/{param} 200, got %v", got)
	}
	if n.Samples != 2 {
		t.Fatalf("slugs should merge into 2 samples, got %d", n.Samples)
	}
	if _, bad := n.Properties["Password"]; bad {
		t.Fatal("Password must be redacted (case-insensitive)")
	}
	if _, bad := n.Properties["meta"].Node.Properties["token"]; bad {
		t.Fatal("nested token must be redacted")
	}
	if _, good := n.Properties["title"]; !good {
		t.Fatal("title must be kept")
	}
}

func TestDiffCommand(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json"), filepath.Join(dir, "c.json")
	for path, kind := range map[string]string{a: "INTEGER", b: "STRING", c: "INTEGER"} {
		if err := contract.Save(path, proposal(kind)); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	ctx := context.Background()
	if code := run(ctx, []string{"diff", a, c}, &out, &errb); code != exitOK || !strings.Contains(out.String(), "No drift detected.") {
		t.Fatalf("identical contracts should pass (%d): %s %s", code, out.String(), errb.String())
	}
	out.Reset()
	if code := run(ctx, []string{"diff", a, b}, &out, &errb); code != exitDrift || !strings.Contains(out.String(), "TYPE_CHANGED") {
		t.Fatalf("changed contracts should exit 1 with TYPE_CHANGED (%d): %s", code, out.String())
	}
	if code := run(ctx, []string{"diff", a}, &out, &errb); code != exitError {
		t.Fatal("one argument must be a usage error")
	}
	if code := run(ctx, []string{"diff", a, filepath.Join(dir, "missing.json")}, &out, &errb); code != exitError {
		t.Fatal("missing file must be a tool error")
	}
}
