package infer

import (
	"sync"
	"testing"
)

func TestStoreObserveAndSnapshot(t *testing.T) {
	s := NewStore(Config{})
	for _, body := range []string{`{"id":1}`, `{"id":2,"x":"y"}`} {
		if err := s.Observe("GET /a 200", []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Observe("GET /b 404", []byte(`{"error":"no"}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Observe("GET /c 200", []byte(`not json`)); err == nil {
		t.Error("invalid JSON should return an error")
	}

	keys := s.Keys()
	if len(keys) != 2 || keys[0] != "GET /a 200" || keys[1] != "GET /b 404" {
		t.Fatalf("keys = %v (a failed observation must not create an endpoint)", keys)
	}
	snap := s.Snapshot()
	if snap[0].Responses != 2 || Render(snap[0].Node) != "OBJECT {\n  id: INTEGER\n  x?: STRING\n}" {
		t.Errorf("unexpected snapshot: %d responses\n%s", snap[0].Responses, Render(snap[0].Node))
	}

	// A snapshot is a copy: later observations must not change it.
	_ = s.Observe("GET /a 200", []byte(`{"id":"str"}`))
	if Render(snap[0].Node) != "OBJECT {\n  id: INTEGER\n  x?: STRING\n}" {
		t.Error("snapshot changed after a later Observe")
	}
}

func TestStoreConcurrentObserve(t *testing.T) {
	s := NewStore(Config{})
	var wg sync.WaitGroup
	const goroutines, each = 8, 100
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				_ = s.Observe("GET /users/{id} 200", []byte(`{"id":1,"tags":["a","b"]}`))
				_ = s.Snapshot()
			}
		}()
	}
	wg.Wait()
	snap := s.Snapshot()
	if len(snap) != 1 || snap[0].Responses != goroutines*each {
		t.Fatalf("got %+v", snap)
	}
	checkInvariants(t, snap[0].Node, "$")
	if snap[0].Node.Kinds[KindObject] != goroutines*each {
		t.Errorf("object samples = %d", snap[0].Node.Kinds[KindObject])
	}
}
