package diff

import (
	"testing"

	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
)

func TestMissingEndpoints(t *testing.T) {
	observed := map[string]*infer.Node{"GET /a 200": {}}
	got := MissingEndpoints([]string{"GET /b 200", "GET /a 200"}, observed)
	if len(got) != 1 || got[0].Endpoint != "GET /b 200" || got[0].Type != EndpointMissing || got[0].Severity != SeverityBreaking {
		t.Fatalf("unexpected result: %+v", got)
	}
	if len(MissingEndpoints(nil, observed)) != 0 {
		t.Fatal("nothing expected, nothing missing")
	}
}
