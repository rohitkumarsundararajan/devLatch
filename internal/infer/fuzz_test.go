package infer

import "testing"

// FuzzObserve feeds arbitrary bytes to the inferrer. Whatever it accepts must
// produce a structurally valid Node that merges and renders without panicking.
func FuzzObserve(f *testing.F) {
	seeds := []string{
		`{}`, `[]`, `null`, `0`, `"s"`, `-1.5e10`,
		`{"a":1,"b":[1,2.5,"x",null,{"c":true}]}`,
		`{"2026-01-01":{"a":1},"2026-01-02":{"a":2},"2026-01-03":{"a":3}}`,
		`{"1":[],"2":[],"3":[]}`,
		`[[],[[]],[[],{}]]`,
		`{"a":{"a":{"a":{"a":null}}}}`,
		`{"k":1,"k":"dup"}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	in := New(Config{})
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := in.Observe(data)
		if err != nil {
			return
		}
		checkInvariants(t, a, "$")
		b, err := in.Observe(data)
		if err != nil {
			t.Fatalf("second Observe of identical input failed: %v", err)
		}
		merged := in.Merge(a, b)
		checkInvariants(t, merged, "$")
		_ = Render(merged)
		_ = merged.Clone()
	})
}
