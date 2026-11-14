package normalize

import "testing"

func TestApplyPatterns(t *testing.T) {
	pats := []string{"/posts/*", "/shops/*/items/*"}
	cases := map[string]string{
		"GET /posts/hello 200":          "GET /posts/{param} 200",
		"GET /posts/{id} 200":           "GET /posts/{param} 200",
		"GET /shops/acme/items/pen 200": "GET /shops/{param}/items/{param} 200",
		"GET /posts 200":                "GET /posts 200",
		"GET /posts/a/comments 200":     "GET /posts/a/comments 200",
		"garbage":                       "garbage",
	}
	for in, want := range cases {
		if got := ApplyPatterns(in, pats); got != want {
			t.Errorf("ApplyPatterns(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ApplyPatterns("GET /posts/x 200", nil); got != "GET /posts/x 200" {
		t.Fatal("no patterns must leave the key unchanged")
	}
}
