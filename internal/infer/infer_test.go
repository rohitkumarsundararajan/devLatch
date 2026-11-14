package infer

import (
	"fmt"
	"strings"
	"testing"
)

func observe(t *testing.T, in *Inferrer, body string) *Node {
	t.Helper()
	n, err := in.Observe([]byte(body))
	if err != nil {
		t.Fatalf("Observe(%s): %v", body, err)
	}
	checkInvariants(t, n, "$")
	return n
}

func TestObserveRender(t *testing.T) {
	in := New(Config{})
	tests := []struct{ name, body, want string }{
		{"string", `"x"`, "STRING"},
		{"integer", `5`, "INTEGER"},
		{"negative integer", `-7`, "INTEGER"},
		{"float literal", `5.0`, "NUMBER"},
		{"exponent", `1e3`, "NUMBER"},
		{"boolean", `true`, "BOOLEAN"},
		{"null", `null`, "NULL"},
		{"empty object", `{}`, "OBJECT {\n}"},
		{"empty array", `[]`, "ARRAY [?]"},
		{"mixed array", `[1,"a",null]`, "ARRAY [STRING|INTEGER|NULL]"},
		{"int and float in array", `[1,2.5]`, "ARRAY [INTEGER|NUMBER]"},
		{"nested object", `{"id":"7","role_id":2,"balance":104.5,"ok":true,"nick":null,"tags":["a","b"],"meta":{"x":1}}`,
			"OBJECT {\n  balance: NUMBER\n  id: STRING\n  meta: OBJECT {\n    x: INTEGER\n  }\n  nick: NULL\n  ok: BOOLEAN\n  role_id: INTEGER\n  tags: ARRAY [STRING]\n}"},
		{"array elements are all merged", `[{"a":1,"b":2},{"a":3}]`,
			"ARRAY [OBJECT {\n  a: INTEGER\n  b?: INTEGER\n}]"},
		{"later array element adds a key", `[{"a":1},{"a":2,"late":"x"}]`,
			"ARRAY [OBJECT {\n  a: INTEGER\n  late?: STRING\n}]"},
		{"array of arrays", `[[1],[2,3]]`, "ARRAY [ARRAY [INTEGER]]"},
		{"top-level array of objects", `[{"id":1}]`, "ARRAY [OBJECT {\n  id: INTEGER\n}]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Render(observe(t, in, tt.body)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestObserveRejectsInvalidJSON(t *testing.T) {
	in := New(Config{})
	for _, body := range []string{``, `   `, `{`, `{"a":}`, `{} {}`, `{"a":1} x`, `nope`, `[1,2`} {
		if _, err := in.Observe([]byte(body)); err == nil {
			t.Errorf("Observe(%q) should fail", body)
		}
	}
}

func TestMergeUnionsAndOptionality(t *testing.T) {
	in := New(Config{})
	a := observe(t, in, `{"a":1}`)
	b := observe(t, in, `{"a":"x","b":null}`)
	got := in.Merge(a, b)
	checkInvariants(t, got, "$")

	want := "OBJECT {\n  a: STRING|INTEGER\n  b?: NULL\n}"
	if r := Render(got); r != want {
		t.Errorf("got:\n%s\nwant:\n%s", r, want)
	}
	if got.Kinds[KindObject] != 2 || got.Properties["a"].SeenIn != 2 || got.Properties["b"].SeenIn != 1 {
		t.Errorf("unexpected counts: %+v", got)
	}
}

func TestMergeNullThenObjectIsNullable(t *testing.T) {
	in := New(Config{})
	got := in.Merge(observe(t, in, `null`), observe(t, in, `{"a":1}`))
	if !got.Nullable() {
		t.Error("expected nullable")
	}
	if r := Render(got); r != "OBJECT|NULL {\n  a: INTEGER\n}" {
		t.Errorf("got:\n%s", r)
	}
	if !got.Required("a", 1) {
		t.Error("a is present in every object sample, so it is required")
	}
}

func TestMergeIsOrderIndependent(t *testing.T) {
	in := New(Config{})
	bodies := []string{`{"a":1,"b":[1]}`, `{"a":"x"}`, `{"a":null,"c":{"d":1.5}}`, `[]`}
	forward, backward := observe(t, in, bodies[0]), observe(t, in, bodies[len(bodies)-1])
	for _, b := range bodies[1:] {
		forward = in.Merge(forward, observe(t, in, b))
	}
	for i := len(bodies) - 2; i >= 0; i-- {
		backward = in.Merge(backward, observe(t, in, bodies[i]))
	}
	if f, b := Render(forward), Render(backward); f != b {
		t.Errorf("merge order changed the result:\n%s\nvs\n%s", f, b)
	}
}

func TestRequiredNeedsEnoughSamples(t *testing.T) {
	in := New(Config{})
	var n *Node
	for i := 1; i <= 25; i++ {
		n = in.Merge(n, observe(t, in, `{"id":1}`))
		wantRequired := i >= 20
		if got := n.Required("id", 20); got != wantRequired {
			t.Fatalf("after %d samples Required = %v, want %v", i, got, wantRequired)
		}
	}
	if n.Required("missing", 1) {
		t.Error("unknown key must not be required")
	}
}

func TestEmptyArrayThenElementsLearnsElement(t *testing.T) {
	in := New(Config{})
	got := in.Merge(observe(t, in, `{"tags":[]}`), observe(t, in, `{"tags":["a"]}`))
	if r := Render(got); r != "OBJECT {\n  tags: ARRAY [STRING]\n}" {
		t.Errorf("got:\n%s", r)
	}
}

func TestCloneIsDeep(t *testing.T) {
	in := New(Config{})
	orig := observe(t, in, `{"a":{"b":[1]}}`)
	c := orig.Clone()
	in.Merge(orig, observe(t, in, `{"a":{"b":["x"],"z":1}}`))
	if r := Render(c); r != "OBJECT {\n  a: OBJECT {\n    b: ARRAY [INTEGER]\n  }\n}" {
		t.Errorf("clone changed after merging into the original:\n%s", r)
	}
}

func TestDeepNestingDoesNotPanic(t *testing.T) {
	in := New(Config{})
	body := strings.Repeat(`{"a":`, 500) + `1` + strings.Repeat(`}`, 500)
	observe(t, in, body)
}

func TestIntegerVersusNumberFromSourceText(t *testing.T) {
	in := New(Config{})
	for body, want := range map[string]Kind{`100`: KindInteger, `100.0`: KindNumber, `1E2`: KindNumber, `-0`: KindInteger, `12345678901234567890`: KindInteger} {
		n := observe(t, in, body)
		if n.Kinds[want] != 1 {
			t.Errorf("%s: kinds %v, want %s", body, n.Kinds, want)
		}
	}
}

func ExampleRender() {
	in := New(Config{})
	n, _ := in.Observe([]byte(`{"id":"7","tags":["a"],"phone":null}`))
	fmt.Println(Render(n))
	// Output:
	// OBJECT {
	//   id: STRING
	//   phone: NULL
	//   tags: ARRAY [STRING]
	// }
}
