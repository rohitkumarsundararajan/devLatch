package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// do performs a request against the handler and returns status and raw body.
func do(t *testing.T, h http.Handler, method, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Code, rec.Body.String()
}

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	return m
}

func TestHealthz(t *testing.T) {
	code, body := do(t, newServer().handler(), "GET", "/healthz")
	if code != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("got %d %s", code, body)
	}
}

func TestUserBaselineShape(t *testing.T) {
	h := newServer().handler()

	_, odd := do(t, h, "GET", "/api/v1/users/7")
	u := decode(t, odd)
	for _, k := range []string{"id", "user_name", "email", "role_id", "nickname", "permissions"} {
		if _, ok := u[k]; !ok {
			t.Errorf("user 7 missing key %q", k)
		}
	}
	if _, ok := u["phone"]; ok {
		t.Error("odd user should not have optional phone")
	}
	if u["id"] != "7" {
		t.Errorf("id = %v, want \"7\"", u["id"])
	}
	if !strings.Contains(odd, `"role_id":2`) { // integers must stay integers in raw JSON
		t.Errorf("role_id should be a plain integer: %s", odd)
	}

	_, even := do(t, h, "GET", "/api/v1/users/8")
	if _, ok := decode(t, even)["phone"]; !ok {
		t.Error("even user should have optional phone")
	}
}

func TestUserNotFound(t *testing.T) {
	h := newServer().handler()
	for _, path := range []string{"/api/v1/users/999", "/api/v1/users/0", "/api/v1/users/abc"} {
		code, body := do(t, h, "GET", path)
		if code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, code)
		}
		if _, ok := decode(t, body)["error"]; !ok {
			t.Errorf("%s: 404 body should have an error key: %s", path, body)
		}
	}
}

func TestListUsersPaging(t *testing.T) {
	h := newServer().handler()
	_, p1 := do(t, h, "GET", "/api/v1/users")
	if n := len(decode(t, p1)["users"].([]any)); n != pageSize {
		t.Errorf("page 1 has %d users, want %d", n, pageSize)
	}
	_, p3 := do(t, h, "GET", "/api/v1/users?page=3")
	if !strings.Contains(p3, `"users":[]`) {
		t.Errorf("empty page must encode users as [] not null: %s", p3)
	}
}

func TestBaselineOrdersAndStats(t *testing.T) {
	h := newServer().handler()
	_, orders := do(t, h, "GET", "/api/v1/orders")
	if !strings.Contains(orders, `"balance":104.5`) || strings.Contains(orders, "currency") {
		t.Errorf("unexpected baseline orders: %s", orders)
	}
	_, stats := do(t, h, "GET", "/api/v1/stats/daily")
	if n := len(decode(t, stats)["by_day"].(map[string]any)); n != 7 {
		t.Errorf("by_day has %d entries, want 7", n)
	}
}

func TestMutations(t *testing.T) {
	tests := []struct {
		mutation string
		path     string
		check    func(t *testing.T, raw string)
	}{
		{MutRenameUserName, "/api/v1/users/7", func(t *testing.T, raw string) {
			u := decode(t, raw)
			if _, ok := u["user_name"]; ok {
				t.Error("user_name should be gone")
			}
			if u["username"] != "user7" {
				t.Errorf("username = %v", u["username"])
			}
		}},
		{MutBalanceToString, "/api/v1/orders", func(t *testing.T, raw string) {
			if !strings.Contains(raw, `"balance":"104.50"`) {
				t.Errorf("balance should be a string: %s", raw)
			}
		}},
		{MutTagsToObjects, "/api/v1/orders", func(t *testing.T, raw string) {
			if !strings.Contains(raw, `"tags":[{"label":"gift"},{"label":"express"}]`) {
				t.Errorf("tags should be objects: %s", raw)
			}
		}},
		{MutRemoveEmail, "/api/v1/users/7", func(t *testing.T, raw string) {
			if _, ok := decode(t, raw)["email"]; ok {
				t.Error("email should be gone")
			}
		}},
		{MutNullableNickname, "/api/v1/users/7", func(t *testing.T, raw string) {
			u := decode(t, raw)
			if v, ok := u["nickname"]; !ok || v != nil {
				t.Errorf("nickname should be null, got %v (present=%v)", v, ok)
			}
		}},
		{MutAddCurrency, "/api/v1/orders", func(t *testing.T, raw string) {
			if !strings.Contains(raw, `"currency":"INR"`) {
				t.Errorf("currency should be added: %s", raw)
			}
		}},
		{MutNewMapDate, "/api/v1/stats/daily", func(t *testing.T, raw string) {
			by := decode(t, raw)["by_day"].(map[string]any)
			if _, ok := by["2026-11-08"]; !ok || len(by) != 8 {
				t.Errorf("by_day should gain 2026-11-08, got %d keys", len(by))
			}
		}},
	}
	if len(tests) != len(allMutations) {
		t.Fatalf("test table covers %d mutations, server defines %d", len(tests), len(allMutations))
	}
	for _, tt := range tests {
		t.Run(tt.mutation, func(t *testing.T) {
			s := newServer()
			s.set(tt.mutation, true)
			code, raw := do(t, s.handler(), "GET", tt.path)
			if code != 200 {
				t.Fatalf("status %d: %s", code, raw)
			}
			tt.check(t, raw)
		})
	}
}

func TestControlEndpoints(t *testing.T) {
	h := newServer().handler()

	code, body := do(t, h, "POST", "/__demo/mutations/"+MutRenameUserName)
	if code != 200 || !strings.Contains(body, `"active":["rename_user_name"]`) {
		t.Fatalf("enable: %d %s", code, body)
	}
	_, user := do(t, h, "GET", "/api/v1/users/1")
	if _, ok := decode(t, user)["username"]; !ok {
		t.Error("enabled mutation should affect responses")
	}

	if code, _ := do(t, h, "POST", "/__demo/mutations/nope"); code != http.StatusNotFound {
		t.Errorf("unknown mutation: status %d, want 404", code)
	}

	_, body = do(t, h, "DELETE", "/__demo/mutations/"+MutRenameUserName)
	if !strings.Contains(body, `"active":[]`) {
		t.Errorf("disable: %s", body)
	}

	do(t, h, "POST", "/__demo/mutations/"+MutAddCurrency)
	do(t, h, "POST", "/__demo/mutations/"+MutRemoveEmail)
	_, body = do(t, h, "POST", "/__demo/reset")
	if !strings.Contains(body, `"active":[]`) {
		t.Errorf("reset: %s", body)
	}
	_, body = do(t, h, "GET", "/__demo/mutations")
	if !strings.Contains(body, `"available"`) {
		t.Errorf("list: %s", body)
	}
}

// Run with -race: toggling mutations while serving must be safe.
func TestConcurrentToggleAndRead(t *testing.T) {
	s := newServer()
	h := s.handler()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.set(MutRenameUserName, j%2 == 0)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				do(t, h, "GET", "/api/v1/users/3")
			}
		}()
	}
	wg.Wait()
}
