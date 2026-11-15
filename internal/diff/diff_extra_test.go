package diff

import (
	"testing"

	"github.com/OWNER/devlatch/internal/infer"
)

func TestDiffMultiEndpoint(t *testing.T) {
	mk := func(kind string) *infer.Node {
		return object(30, map[string]*infer.Prop{"id": prop(leaf(30, kind), 30)})
	}
	base := map[string]*infer.Node{
		"GET /b 200":     mk("INTEGER"),
		"GET /a 200":     mk("INTEGER"),
		"GET /gone 200":  mk("INTEGER"),
		"GET /empty 200": mk("INTEGER"),
		"GET /nil 200":   nil,
	}
	live := map[string]*infer.Node{
		"GET /b 200":     mk("STRING"),
		"GET /a 200":     mk("STRING"),
		"GET /empty 200": {Kinds: map[infer.Kind]int{}, Samples: 0},
		"GET /nil 200":   mk("STRING"),
		"GET /new 200":   mk("STRING"),
	}
	got := Diff(base, live, Options{})
	if len(got) != 2 {
		t.Fatalf("got %d violations %+v, want 2", len(got), got)
	}
	if got[0].Endpoint != "GET /a 200" || got[1].Endpoint != "GET /b 200" {
		t.Fatalf("not sorted by endpoint: %+v", got)
	}
}

func TestMapAndNestedObject(t *testing.T) {
	mapNode := func(kind string) *infer.Node {
		return &infer.Node{
			Kinds: map[infer.Kind]int{"MAP": 30},
			MapValue: object(30, map[string]*infer.Prop{
				"a": prop(leaf(30, kind), 30),
			}),
			Samples: 30,
		}
	}
	wrap := func(kind string) *infer.Node {
		return object(30, map[string]*infer.Prop{
			"stats": prop(mapNode(kind), 30),
			"user":  prop(object(30, map[string]*infer.Prop{"age": prop(leaf(30, kind), 30)}), 30),
		})
	}
	got := DiffEndpoint("GET /x 200", wrap("STRING"), wrap("INTEGER"), Options{MinSamples: 20})
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 violations", got)
	}
	if got[0].Path != "$.stats{*}.a" || got[1].Path != "$.user.age" {
		t.Fatalf("unexpected paths: %s, %s", got[0].Path, got[1].Path)
	}
}

func TestSortViolations(t *testing.T) {
	vs := []Violation{
		{Endpoint: "b", Path: "$.x", Type: AddedKey},
		{Endpoint: "a", Path: "$.z", Type: AddedKey},
		{Endpoint: "a", Path: "$.y", Type: TypeChanged},
		{Endpoint: "a", Path: "$.y", Type: AddedKey},
	}
	SortViolations(vs)
	order := []string{"a$.yADDED_KEY", "a$.yTYPE_CHANGED", "a$.zADDED_KEY", "b$.xADDED_KEY"}
	for i, v := range vs {
		if got := v.Endpoint + v.Path + string(v.Type); got != order[i] {
			t.Fatalf("position %d = %s, want %s", i, got, order[i])
		}
	}
}
