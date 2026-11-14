package normalize

import "testing"

func TestPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "/"},
		{"/", "/"},
		{"/api/v1/users", "/api/v1/users"},
		{"/api/v1/users/101", "/api/v1/users/{id}"},
		{"/api/v1/users/101/orders/7", "/api/v1/users/{id}/orders/{id}"},
		{"/items/123e4567-e89b-12d3-a456-426614174000", "/items/{id}"},
		{"/items/123E4567-E89B-12D3-A456-426614174000/", "/items/{id}/"},
		{"/users/alice", "/users/alice"}, // names need cardinality inference (Phase 3b)
		{"/v1/users", "/v1/users"},       // "v1" is not an integer
		{"/items/12ab", "/items/12ab"},
	}
	for _, tt := range tests {
		if got := Path(tt.in); got != tt.want {
			t.Errorf("Path(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEndpointKey(t *testing.T) {
	got := EndpointKey("get", "/api/v1/users/7", 200)
	if want := "GET /api/v1/users/{id} 200"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := EndpointKey("GET", "/api/v1/users/999", 404); got != "GET /api/v1/users/{id} 404" {
		t.Errorf("status must be part of the key, got %q", got)
	}
}

func TestIsUUIDRejectsBadShapes(t *testing.T) {
	for _, s := range []string{"", "123", "123e4567-e89b-12d3-a456-42661417400", "123e4567_e89b_12d3_a456_426614174000", "zzze4567-e89b-12d3-a456-426614174000"} {
		if IsUUID(s) {
			t.Errorf("IsUUID(%q) = true", s)
		}
	}
}
