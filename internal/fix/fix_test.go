package fix

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromCheck(t *testing.T) {
	in := []byte(`{"violations":[
      {"endpoint":"GET /a/{id} 200","path":"$.x","type":"POSSIBLE_RENAME","severity":"BREAKING","rename_from":"$.x","rename_to":"$.y"},
      {"endpoint":"GET /a/{id} 200","path":"$.x","type":"POSSIBLE_RENAME","severity":"BREAKING","rename_from":"$.x","rename_to":"$.y"},
      {"endpoint":"GET /a/{id} 200","path":"$.new","type":"ADDED_KEY","severity":"SAFE"},
      {"endpoint":"GET /b 200","path":"$.u","type":"POSSIBLE_RENAME","severity":"BREAKING","rename_from":"$.u[].a","rename_to":"$.u[].b"}
    ]}`)
	got, skipped, err := FromCheck(in)
	if err != nil || skipped != 0 || len(got) != 2 {
		t.Fatalf("got %v skipped %d err %v", got, skipped, err)
	}
	if got[0] != (Rename{Endpoint: "GET /a/{id} 200", Path: "$.x", To: "y"}) {
		t.Fatalf("first rename wrong: %+v", got[0])
	}
	if got[1].To != "b" || got[1].Path != "$.u[].a" {
		t.Fatalf("nested rename wrong: %+v", got[1])
	}
}

func TestFromCheckSkipsMoves(t *testing.T) {
	in := []byte(`[{"endpoint":"GET /a 200","type":"POSSIBLE_RENAME","rename_from":"$.a.x","rename_to":"$.b.x"}]`)
	got, skipped, err := FromCheck(in)
	if err != nil || len(got) != 0 || skipped != 1 {
		t.Fatalf("got %v skipped %d err %v", got, skipped, err)
	}
}

func TestLastSegment(t *testing.T) {
	for in, want := range map[string]string{"$.user_name": "user_name", "$.users[].user_name": "user_name", "$.a.b.c": "c", "$": ""} {
		if got := LastSegment(in); got != want {
			t.Errorf("LastSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

// Runs the real scanner on the bench fixture; skipped when Node or the scanner's dependencies are missing.
func TestRunPreview(t *testing.T) {
	script, _ := filepath.Abs(filepath.Join("..", "..", "scanner", "src", "cli.mjs"))
	if _, err := os.Stat(filepath.Join("..", "..", "scanner", "node_modules", "ts-morph")); err != nil {
		t.Skip("scanner dependencies not installed")
	}
	cfg := filepath.Join("..", "..", "scanner", "fixtures", "bench", "tsconfig.json")
	res, err := Run(script, cfg, []Rename{{Endpoint: "GET /api/users/{id} 200", Path: "$.user_name", To: "username"}}, Options{})
	if err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	if res.Applied || res.Edits < 15 || res.Files < 2 {
		t.Fatalf("unexpected preview: %+v", res)
	}
}
