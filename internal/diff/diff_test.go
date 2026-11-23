package diff

import (
	"testing"

	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
)

func mixed(total int, counts map[string]int) *infer.Node {
	k := map[infer.Kind]int{}
	for name, c := range counts {
		k[infer.Kind(name)] = c
	}
	return &infer.Node{Kinds: k, Samples: total}
}

func leaf(n int, kind string) *infer.Node { return mixed(n, map[string]int{kind: n}) }

func prop(n *infer.Node, seen int) *infer.Prop { return &infer.Prop{Node: n, SeenIn: seen} }

func object(n int, props map[string]*infer.Prop) *infer.Node {
	return &infer.Node{Kinds: map[infer.Kind]int{"OBJECT": n}, Properties: props, Samples: n}
}

func array(n int, el *infer.Node) *infer.Node {
	return &infer.Node{Kinds: map[infer.Kind]int{"ARRAY": n}, Element: el, Samples: n}
}

type want struct {
	T ViolationType
	P string
	S Severity
}

func TestDiffScenarios(t *testing.T) {
	baseUser := func() *infer.Node {
		return object(30, map[string]*infer.Prop{
			"id":      prop(leaf(30, "INTEGER"), 30),
			"role_id": prop(leaf(30, "INTEGER"), 30),
			"price":   prop(leaf(30, "NUMBER"), 30),
			"name":    prop(leaf(30, "STRING"), 30),
			"tags":    prop(array(30, leaf(30, "STRING")), 30),
			"nick":    prop(leaf(30, "STRING"), 10),
		})
	}
	// liveWith clones the baseline shape and lets each case tweak it.
	liveWith := func(tweak func(p map[string]*infer.Prop)) *infer.Node {
		n := baseUser()
		tweak(n.Properties)
		return n
	}

	cases := []struct {
		name string
		base *infer.Node
		live *infer.Node
		want []want
	}{
		{"no drift", baseUser(), baseUser(), nil},
		{"removed required key", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { delete(p, "role_id") }),
			[]want{{RemovedKey, "$.role_id", SeverityBreaking}}},
		{"removed optional key ignored", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { delete(p, "nick") }), nil},
		{"added key", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { p["email"] = prop(leaf(30, "STRING"), 30) }),
			[]want{{AddedKey, "$.email", SeveritySafe}}},
		{"type conversion number to string", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { p["price"] = prop(leaf(30, "STRING"), 30) }),
			[]want{{TypeChanged, "$.price", SeverityBreaking}}},
		{"integer widens to number", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { p["id"] = prop(leaf(30, "NUMBER"), 30) }),
			[]want{{TypeChanged, "$.id", SeverityWarning}}},
		{"new nullability", baseUser(),
			liveWith(func(p map[string]*infer.Prop) {
				p["name"] = prop(mixed(30, map[string]int{"STRING": 20, "NULL": 10}), 30)
			}),
			[]want{{NullabilityChanged, "$.name", SeverityWarning}}},
		{"array element mutation", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { p["tags"] = prop(array(30, leaf(30, "OBJECT")), 30) }),
			[]want{{ArrayElementChanged, "$.tags[]", SeverityBreaking}}},
		{"required becomes optional", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { p["name"] = prop(leaf(30, "STRING"), 20) }),
			[]want{{OptionalityChanged, "$.name", SeverityBreaking}}},
		{"optional becomes required", baseUser(),
			liveWith(func(p map[string]*infer.Prop) { p["nick"] = prop(leaf(30, "STRING"), 30) }),
			[]want{{OptionalityChanged, "$.nick", SeveritySafe}}},
		{"low-sample baseline does not enforce required",
			object(5, map[string]*infer.Prop{"id": prop(leaf(5, "INTEGER"), 5), "x": prop(leaf(5, "STRING"), 5)}),
			object(30, map[string]*infer.Prop{"id": prop(leaf(30, "INTEGER"), 30)}), nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DiffEndpoint("GET /api/users/{id} 200", c.base, c.live, Options{MinSamples: 20})
			if len(got) != len(c.want) {
				t.Fatalf("got %d violations %+v, want %d", len(got), got, len(c.want))
			}
			for i, w := range c.want {
				g := got[i]
				if g.Type != w.T || g.Path != w.P || g.Severity != w.S {
					t.Fatalf("violation %d = %s %s %s, want %s %s %s", i, g.Type, g.Path, g.Severity, w.T, w.P, w.S)
				}
			}
		})
	}
}
