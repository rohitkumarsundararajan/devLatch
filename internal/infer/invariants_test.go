package infer

import "testing"

// checkInvariants verifies structural rules that must hold for every Node the
// package produces, however the payloads were merged.
func checkInvariants(t testing.TB, n *Node, path string) {
	t.Helper()
	if n == nil {
		t.Fatalf("%s: nil node", path)
		return
	}
	sum := 0
	for _, c := range n.Kinds {
		sum += c
	}
	if sum != n.Samples {
		t.Errorf("%s: kinds sum %d != samples %d", path, sum, n.Samples)
	}
	if len(n.Properties) > 0 && n.Kinds[KindObject] == 0 {
		t.Errorf("%s: properties without OBJECT kind", path)
	}
	if n.Element != nil && n.Kinds[KindArray] == 0 {
		t.Errorf("%s: element without ARRAY kind", path)
	}
	if n.MapValue != nil && n.Kinds[KindMap] == 0 {
		t.Errorf("%s: map value without MAP kind", path)
	}
	for name, p := range n.Properties {
		if p.SeenIn > n.Kinds[KindObject] {
			t.Errorf("%s.%s: seen in %d > object samples %d", path, name, p.SeenIn, n.Kinds[KindObject])
		}
		if p.SeenIn != p.Node.Samples {
			t.Errorf("%s.%s: seen in %d != node samples %d", path, name, p.SeenIn, p.Node.Samples)
		}
		checkInvariants(t, p.Node, path+"."+name)
	}
	if n.Element != nil {
		checkInvariants(t, n.Element, path+"[]")
	}
	if n.MapValue != nil {
		checkInvariants(t, n.MapValue, path+"{}")
	}
}
