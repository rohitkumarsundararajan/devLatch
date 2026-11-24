package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, p string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
	b, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(b)
}

func TestServesIndexAndFallback(t *testing.T) {
	h := Handler()
	if code, body := get(t, h, "/"); code != 200 || !strings.Contains(body, `id="root"`) {
		t.Fatalf("/ = %d %q", code, body)
	}
	if code, body := get(t, h, "/endpoints/some-route"); code != 200 || !strings.Contains(body, `id="root"`) {
		t.Fatalf("spa fallback = %d %q", code, body)
	}
	if code, _ := get(t, h, "/assets/missing.js"); code != 404 {
		t.Fatalf("missing asset = %d, want 404", code)
	}
	if code, _ := get(t, h, "/api/nope"); code != 404 {
		t.Fatalf("api path = %d, want 404", code)
	}
}

func TestShipsRealBuild(t *testing.T) {
	h := Handler()
	_, body := get(t, h, "/")
	m := regexp.MustCompile(`src="\./(assets/[^"]+\.js)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("dist/index.html has no JS bundle: run npm run build in web/ and commit internal/webui/dist")
	}
	if code, _ := get(t, h, "/"+m[1]); code != 200 {
		t.Fatalf("bundle %s = %d, want 200", m[1], code)
	}
}
