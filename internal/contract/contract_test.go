package contract

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OWNER/devlatch/internal/diff"
	"github.com/OWNER/devlatch/internal/infer"
)

func leaf(n int, k string) *infer.Node {
	return &infer.Node{Kinds: map[infer.Kind]int{infer.Kind(k): n}, Samples: n}
}

func sample(n int) *infer.Node {
	return &infer.Node{
		Kinds: map[infer.Kind]int{"OBJECT": n},
		Properties: map[string]*infer.Prop{
			"id":   {Node: leaf(n, "INTEGER"), SeenIn: n},
			"nick": {Node: leaf(n, "STRING"), SeenIn: n / 2},
			"tags": {Node: &infer.Node{Kinds: map[infer.Kind]int{"ARRAY": n}, Element: leaf(n, "STRING"), Samples: n}, SeenIn: n},
			"days": {Node: &infer.Node{Kinds: map[infer.Kind]int{"MAP": n}, MapValue: leaf(n, "INTEGER"), Samples: n}, SeenIn: n},
		},
		Samples: n,
	}
}

func TestRoundTripAndDiff(t *testing.T) {
	const ep = "GET /api/users/{id} 200"
	f := FromNodes(map[string]*infer.Node{ep: sample(30), "GET /empty 200": {}}, 20, "test", time.Unix(0, 0))
	if len(f.Endpoints) != 1 {
		t.Fatalf("empty endpoint should be skipped, got %d endpoints", len(f.Endpoints))
	}
	if !f.Endpoints[ep].Schema.Properties["id"].Required || f.Endpoints[ep].Schema.Properties["nick"].Required {
		t.Fatal("id must be required and nick optional")
	}

	path := filepath.Join(t.TempDir(), ".devlatch", "contracts.json")
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Same traffic as the contract: no drift.
	base := loaded.ToNodes()
	if vs := diff.Diff(base, map[string]*infer.Node{ep: sample(30)}, diff.Options{}); len(vs) != 0 {
		t.Fatalf("expected no drift, got %+v", vs)
	}
	// Required key removed from live traffic: must be breaking.
	live := sample(30)
	delete(live.Properties, "id")
	vs := diff.Diff(base, map[string]*infer.Node{ep: live}, diff.Options{})
	if len(vs) != 1 || vs[0].Type != diff.RemovedKey || vs[0].Severity != diff.SeverityBreaking {
		t.Fatalf("expected one breaking REMOVED_KEY, got %+v", vs)
	}
}

func TestLowSampleAndLoadErrors(t *testing.T) {
	f := FromNodes(map[string]*infer.Node{"GET /a 200": sample(6)}, 20, "t", time.Now())
	if low := f.LowSample(20); len(low) != 1 || !strings.Contains(low[0], "6 samples") {
		t.Fatalf("unexpected low-sample list: %v", low)
	}
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file must error")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := Save(bad, &File{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("file without schemaVersion must error")
	}
}
