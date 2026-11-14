package normalize

import "testing"

func TestCollapse(t *testing.T) {
	keys := []string{
		"GET /api/orders/ORD-1001 200", "GET /api/orders/ORD-1002 200", "GET /api/orders/ORD-1003 200",
		"GET /api/orders/ORD-1004 200", "GET /api/orders/ORD-1005 200", "GET /api/orders/ORD-1006 200",
		"GET /api/orders/ORD-1001 404",                                                                                  // different status: stays alone
		"GET /api/users 200", "GET /api/orders 200", "GET /api/products 200", "GET /api/carts 200", "GET /api/tags 200", // static names
		"GET /api/v1/ping 200", "GET /api/v2/ping 200", // only 2 distinct values with digits
		"GET /shops/s1/items/i1 200", "GET /shops/s2/items/i2 200", "GET /shops/s3/items/i3 200",
		"GET /shops/s4/items/i4 200", "GET /shops/s5/items/i5 200", // nested: both positions collapse
		"weird-key-without-spaces",
	}
	got := Collapse(keys, 5)
	want := map[string]string{
		"GET /api/orders/ORD-1001 200": "GET /api/orders/{param} 200",
		"GET /api/orders/ORD-1006 200": "GET /api/orders/{param} 200",
		"GET /api/orders/ORD-1001 404": "GET /api/orders/ORD-1001 404",
		"GET /api/users 200":           "GET /api/users 200",
		"GET /api/tags 200":            "GET /api/tags 200",
		"GET /api/v1/ping 200":         "GET /api/v1/ping 200",
		"GET /shops/s3/items/i3 200":   "GET /shops/{param}/items/{param} 200",
		"weird-key-without-spaces":     "weird-key-without-spaces",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("Collapse(%q) = %q, want %q", k, got[k], w)
		}
	}
	if len(got) != len(keys) {
		t.Fatalf("every key must be mapped: got %d, want %d", len(got), len(keys))
	}
	if same := Collapse(keys, 1); same["GET /api/orders/ORD-1001 200"] != "GET /api/orders/ORD-1001 200" {
		t.Fatal("minDistinct < 2 must leave keys unchanged")
	}
}
