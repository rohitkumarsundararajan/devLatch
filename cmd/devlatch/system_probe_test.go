package main

import (
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Blueprint scenario 10: probing a drifted API exits 1 and the auth values
// never end up in any file or any output.
func TestSystemProbeNeverLeaksAuth(t *testing.T) {
	const (
		keySecret  = "s3cret-api-key-xyz"
		authSecret = "Bearer tok-abc123-very-secret"
	)
	t.Setenv("DL_SYS_KEY", keySecret)
	t.Setenv("DL_SYS_AUTH", authSecret)

	old, _ := os.Getwd()
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var mutated atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != keySecret || r.Header.Get("Authorization") != authSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if mutated.Load() {
			_, _ = w.Write([]byte(`{"id":1,"username":"asha","email":"a@x.in","role_id":7,"phone":"99"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"user_name":"asha","email":"a@x.in","role_id":7,"phone":"99"}`))
	}))
	defer srv.Close()

	var all bytes.Buffer // everything devlatch printed, for the leak scan
	probe := func(extra ...string) int {
		args := append([]string{"probe", "--base", srv.URL, "--paths", "/api/users/1,/api/users/2",
			"--header-env", "X-Api-Key=DL_SYS_KEY,Authorization=DL_SYS_AUTH"}, extra...)
		var out, errb bytes.Buffer
		code := run(context.Background(), args, &out, &errb)
		all.Write(out.Bytes())
		all.Write(errb.Bytes())
		return code
	}

	if code := probe("--record"); code != exitOK {
		t.Fatalf("probe --record failed: %d\n%s", code, all.String())
	}
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"approve"}, &out, &errb); code != exitOK {
		t.Fatalf("approve failed: %s", errb.String())
	}
	if code := probe(); code != exitOK {
		t.Fatalf("a clean API must pass (%d)\n%s", code, all.String())
	}

	mutated.Store(true)
	for _, format := range []string{"text", "json", "markdown"} {
		if code := probe("--format", format); code != exitDrift {
			t.Fatalf("drifted API with --format %s must exit 1, got %d\n%s", format, code, all.String())
		}
	}

	for _, s := range []string{keySecret, authSecret, "tok-abc123"} {
		if strings.Contains(all.String(), s) {
			t.Fatalf("a secret appeared in devlatch output: %q", s)
		}
	}
	err := filepath.WalkDir(work, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, s := range []string{keySecret, "tok-abc123"} {
			if strings.Contains(string(b), s) {
				t.Errorf("secret found in file %s", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
