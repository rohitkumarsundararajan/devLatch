package infer

import "testing"

func TestObserveRecordsKeyOrder(t *testing.T) {
	in := New(Config{})
	n, err := in.Observe([]byte(`{"b":1,"a":{"z":true,"y":"s"},"c":[{"q":1,"p":2}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if n.Properties["b"].Order != 1 || n.Properties["a"].Order != 2 || n.Properties["c"].Order != 3 {
		t.Fatalf("top-level order wrong: b=%d a=%d c=%d", n.Properties["b"].Order, n.Properties["a"].Order, n.Properties["c"].Order)
	}
	a := n.Properties["a"].Node
	if a.Properties["z"].Order != 1 || a.Properties["y"].Order != 2 {
		t.Fatal("nested order wrong")
	}
	el := n.Properties["c"].Node.Element
	if el.Properties["q"].Order != 1 || el.Properties["p"].Order != 2 {
		t.Fatal("array element order wrong")
	}
}

func TestMergeKeepsFirstOrderAndClones(t *testing.T) {
	in := New(Config{})
	first, _ := in.Observe([]byte(`{"x":1,"y":2}`))
	second, _ := in.Observe([]byte(`{"y":2,"x":1,"w":3}`))
	m := in.Merge(first, second)
	if m.Properties["x"].Order != 1 || m.Properties["y"].Order != 2 || m.Properties["w"].Order != 3 {
		t.Fatalf("merged order wrong: %d %d %d", m.Properties["x"].Order, m.Properties["y"].Order, m.Properties["w"].Order)
	}
	if c := m.Clone(); c.Properties["w"].Order != 3 {
		t.Fatal("clone dropped Order")
	}
}

func TestObserveStillRejectsBadJSON(t *testing.T) {
	in := New(Config{})
	for _, bad := range []string{``, `{`, `{"a" 1}`, `{"a":1} x`, `[1,`, `{"a":1}{"b":2}`} {
		if _, err := in.Observe([]byte(bad)); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
	for _, good := range []string{`5`, `null`, `"s"`, `[]`, `{}`, `[{"a":1.5e3}]`} {
		if _, err := in.Observe([]byte(good)); err != nil {
			t.Errorf("unexpected error for %q: %v", good, err)
		}
	}
}
