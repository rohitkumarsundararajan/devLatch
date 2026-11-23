package contract

import (
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
)

func TestContractKeepsKeyOrder(t *testing.T) {
	in := infer.New(infer.Config{})
	n, err := in.Observe([]byte(`{"b":1,"a":2}`))
	if err != nil {
		t.Fatal(err)
	}
	f := FromNodes(map[string]*infer.Node{"GET /x 200": n}, 20, "test", time.Unix(0, 0))
	got := f.ToNodes()["GET /x 200"]
	if got == nil || got.Properties["b"] == nil || got.Properties["a"] == nil ||
		got.Properties["b"].Order != 1 || got.Properties["a"].Order != 2 {
		t.Fatalf("key order lost through the contract: %+v", got)
	}
}
