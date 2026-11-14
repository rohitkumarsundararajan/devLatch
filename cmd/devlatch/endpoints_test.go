package main

import (
	"fmt"
	"testing"

	"github.com/OWNER/devlatch/internal/infer"
)

func TestCollapseEndpointsMergesSchemas(t *testing.T) {
	s := infer.NewStore(infer.DefaultConfig())
	for i := 1; i <= 6; i++ {
		key := fmt.Sprintf("GET /api/orders/ORD-100%d 200", i)
		if err := s.Observe(key, []byte(`{"id":1,"total":9.5}`)); err != nil {
			t.Fatal(err)
		}
	}
	got := collapseEndpoints(s.Snapshot())
	n, ok := got["GET /api/orders/{param} 200"]
	if len(got) != 1 || !ok {
		t.Fatalf("expected one collapsed endpoint, got %d keys: %v", len(got), got)
	}
	if n.Samples != 6 {
		t.Fatalf("merged node should hold 6 samples, got %d", n.Samples)
	}
}
