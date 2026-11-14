package infer

import (
	"fmt"
	"strings"
	"testing"
)

const statsMap = "OBJECT {\n  by_day: MAP <string -> OBJECT {\n    orders: INTEGER\n    revenue: NUMBER\n  }>\n}"

func dayMap(days ...string) string {
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = fmt.Sprintf(`%q:{"orders":%d,"revenue":%d.5}`, d, i, i)
	}
	return `{"by_day":{` + strings.Join(parts, ",") + `}}`
}

func TestDateKeysBecomeMap(t *testing.T) {
	in := New(Config{})
	n := observe(t, in, dayMap("2026-11-01", "2026-11-02", "2026-11-03"))
	if r := Render(n); r != statsMap {
		t.Errorf("got:\n%s\nwant:\n%s", r, statsMap)
	}
}

// Verification scenario 7: a date-keyed map gaining a date must not change the shape.
func TestMapGainingAKeyKeepsTheSameShape(t *testing.T) {
	in := New(Config{})
	base := observe(t, in, dayMap("2026-11-01", "2026-11-02", "2026-11-03"))
	before := Render(base.Clone())
	grown := in.Merge(base, observe(t, in, dayMap("2026-11-01", "2026-11-02", "2026-11-03", "2026-11-04", "2026-11-05")))
	checkInvariants(t, grown, "$")
	if after := Render(grown); after != before {
		t.Errorf("shape changed:\n%s\nvs\n%s", before, after)
	}
}

func TestFewKeysStayAnObject(t *testing.T) {
	in := New(Config{})
	n := observe(t, in, dayMap("2026-11-01", "2026-11-02"))
	if n.Properties["by_day"].Node.Kinds[KindMap] != 0 {
		t.Error("two keys are below MapMinKeys, so this stays an OBJECT")
	}
}

func TestKeysThatAddUpAcrossSamplesBecomeMap(t *testing.T) {
	in := New(Config{})
	a := observe(t, in, dayMap("2026-11-01", "2026-11-02"))
	b := observe(t, in, dayMap("2026-11-03", "2026-11-04"))
	got := in.Merge(a, b)
	checkInvariants(t, got, "$")
	if r := Render(got); r != statsMap {
		t.Errorf("got:\n%s", r)
	}
}

func TestSmallObjectSampleFoldsIntoExistingMap(t *testing.T) {
	in := New(Config{})
	base := observe(t, in, dayMap("2026-11-01", "2026-11-02", "2026-11-03"))
	got := in.Merge(base, observe(t, in, dayMap("2026-11-09")))
	checkInvariants(t, got, "$")
	if r := Render(got); r != statsMap {
		t.Errorf("got:\n%s", r)
	}
}

func TestOtherDynamicKeyClasses(t *testing.T) {
	in := New(Config{})
	tests := map[string]string{
		"integers": `{"1":"a","2":"b","3":"c"}`,
		"uuids":    `{"123e4567-e89b-12d3-a456-426614174000":1,"223e4567-e89b-12d3-a456-426614174000":2,"323e4567-e89b-12d3-a456-426614174000":3}`,
		"months":   `{"2026-01":1.5,"2026-02":2,"2026-03":3}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			n := observe(t, in, body)
			if n.Kinds[KindMap] != 1 || n.MapValue == nil {
				t.Errorf("expected MAP, got %s", Render(n))
			}
		})
	}
}

func TestNotAMap(t *testing.T) {
	in := New(Config{})
	tests := map[string]string{
		"fixed field names":        `{"id":1,"name":"x","email":"y"}`,
		"mixed key classes":        `{"1":1,"2026-01-01":2,"abc":3}`,
		"incompatible value kinds": `{"2026-01-01":"x","2026-01-02":{"a":1},"2026-01-03":4}`,
		"disjoint object values":   `{"2026-01-01":{"a":1},"2026-01-02":{"b":1},"2026-01-03":{"c":1}}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			n := observe(t, in, body)
			if n.Kinds[KindMap] != 0 {
				t.Errorf("should stay an OBJECT:\n%s", Render(n))
			}
		})
	}
}

func TestForcedMapPath(t *testing.T) {
	in := New(Config{ForceMaps: map[string]bool{"$.stats": true}})
	n := observe(t, in, `{"stats":{"alpha":{"n":1}}}`)
	if n.Properties["stats"].Node.Kinds[KindMap] != 1 {
		t.Errorf("forced path should be a MAP:\n%s", Render(n))
	}
}

func TestLargeObjectOfOneShapeBecomesMap(t *testing.T) {
	build := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf(`"user_%d":%d`, i, i)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	in := New(Config{})
	if n := observe(t, in, build(199)); n.Kinds[KindMap] != 0 {
		t.Error("199 keys should stay an OBJECT")
	}
	if n := observe(t, in, build(200)); n.Kinds[KindMap] != 1 {
		t.Error("200 same-shape keys should become a MAP")
	}
}

func TestObjectAndMapUnionWhenKeysDoNotLookDynamic(t *testing.T) {
	in := New(Config{})
	a := observe(t, in, dayMap("2026-11-01", "2026-11-02", "2026-11-03"))
	b := observe(t, in, `{"by_day":{"total":1,"note":"x"}}`)
	got := in.Merge(a, b)
	checkInvariants(t, got, "$")
	k := got.Properties["by_day"].Node.Kinds
	if k[KindMap] != 1 || k[KindObject] != 1 {
		t.Errorf("expected OBJECT|MAP union, got %v", k)
	}
}
