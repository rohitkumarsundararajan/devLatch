package diff

import (
	"testing"

	"github.com/OWNER/devlatch/internal/infer"
)

// learn merges n copies of one payload into a Node, like the proxy store does.
func learn(t *testing.T, n int, body string) *infer.Node {
	t.Helper()
	in := infer.New(infer.Config{})
	var node *infer.Node
	for i := 0; i < n; i++ {
		obs, err := in.Observe([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		node = in.Merge(node, obs)
	}
	return node
}

func byType(vs []Violation, typ ViolationType) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Type == typ {
			out = append(out, v)
		}
	}
	return out
}

func TestRenameReplacesRemovedAndAdded(t *testing.T) {
	base := learn(t, 25, `{"id":1,"user_name":"a","email":"e","role_id":2}`)
	live := learn(t, 25, `{"id":1,"username":"a","email":"e","role_id":2}`)

	plain := DiffEndpoint("GET /u 200", base, live, Options{})
	if len(byType(plain, RemovedKey)) != 1 || len(byType(plain, AddedKey)) != 1 {
		t.Fatalf("baseline behaviour changed: %+v", plain)
	}

	got := DiffEndpoint("GET /u 200", base, live, Options{Renames: true})
	if len(got) != 1 || got[0].Type != PossibleRename {
		t.Fatalf("want exactly one POSSIBLE_RENAME, got %+v", got)
	}
	r := got[0]
	if r.RenameFrom != "$.user_name" || r.RenameTo != "$.username" || r.Confidence < 0.6 || len(r.Evidence) == 0 {
		t.Fatalf("bad rename violation: %+v", r)
	}
	if r.Severity != byType(plain, RemovedKey)[0].Severity {
		t.Fatalf("rename severity %s differs from the REMOVED_KEY it replaces (%s)", r.Severity, byType(plain, RemovedKey)[0].Severity)
	}
}

func TestUnrelatedKeysAreNotRenames(t *testing.T) {
	base := learn(t, 25, `{"id":1,"name":"a","phone":"1","role_id":2,"email":"e"}`)
	live := learn(t, 25, `{"id":1,"name":"a","role_id":2,"email":"e","is_verified":true}`)
	got := DiffEndpoint("GET /u 200", base, live, Options{Renames: true})
	if len(byType(got, PossibleRename)) != 0 || len(byType(got, RemovedKey)) != 1 || len(byType(got, AddedKey)) != 1 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRenameWithUnknownOrderStillWorksWhenNamesMatch(t *testing.T) {
	base := learn(t, 25, `{"id":1,"createdAt":"x","email":"e"}`)
	live := learn(t, 25, `{"id":1,"created_at":"x","email":"e"}`)
	for _, p := range base.Properties { // simulate a contract recorded before Order existed
		p.Order = 0
	}
	got := DiffEndpoint("GET /u 200", base, live, Options{Renames: true})
	if len(got) != 1 || got[0].Type != PossibleRename {
		t.Fatalf("want one rename with neutral position, got %+v", got)
	}
}
