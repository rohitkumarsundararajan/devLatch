package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
)

func TestProbeFlow(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	const secret = "s3cret-value-xyz"
	t.Setenv("DL_TEST_KEY", secret)

	var mode atomic.Int32 // 0 normal, 1 field renamed, 2 server error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch mode.Load() {
		case 2:
			http.Error(w, "boom", http.StatusInternalServerError)
		case 1:
			_, _ = w.Write([]byte(`{"id":1,"full_name":"Asha","tags":["a"]}`))
		default:
			_, _ = w.Write([]byte(`{"id":1,"name":"Asha","tags":["a"]}`))
		}
	}))
	defer srv.Close()

	probe := func(extra ...string) (int, string, string) {
		args := append([]string{"probe", "--base", srv.URL, "--paths", "/api/users/1,/api/users/2", "--header-env", "X-Api-Key=DL_TEST_KEY"}, extra...)
		var out, errb bytes.Buffer
		code := run(context.Background(), args, &out, &errb)
		return code, out.String(), errb.String()
	}

	if code, _, e := probe("--record"); code != exitOK {
		t.Fatalf("probe --record failed: %s", e)
	}
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"approve"}, &out, &errb); code != exitOK {
		t.Fatalf("approve failed: %s", errb.String())
	}
	if b, _ := os.ReadFile(config.ContractPath()); strings.Contains(string(b), secret) {
		t.Fatal("the secret must never be written to the contract")
	}
	if code, o, e := probe(); code != exitOK || !strings.Contains(o, "No drift detected.") {
		t.Fatalf("clean probe should pass (%d): %s %s", code, o, e)
	}

	mode.Store(1)
	if code, o, _ := probe("--renames=false"); code != exitDrift || !strings.Contains(o, "REMOVED_KEY") || !strings.Contains(o, "$.name") {
		t.Fatalf("renamed field should exit 1 with REMOVED_KEY (%d): %s", code, o)
	}
	if code, o, _ := probe(); code != exitDrift || !strings.Contains(o, "POSSIBLE_RENAME") || !strings.Contains(o, "$.full_name") {
		t.Fatalf("default probe should report the rename as POSSIBLE_RENAME (%d): %s", code, o)
	}
	mode.Store(2)
	if code, o, _ := probe(); code != exitDrift || !strings.Contains(o, "ENDPOINT_MISSING") {
		t.Fatalf("server error should exit 1 with ENDPOINT_MISSING (%d): %s", code, o)
	}

	mode.Store(0)
	args := []string{"probe", "--base", srv.URL, "--paths", "/api/users/1", "--header-env", "X-Api-Key=DL_NOT_SET_XYZ"}
	if code := run(context.Background(), args, &out, &errb); code != exitError {
		t.Fatal("missing environment variable must be a tool error")
	}
}
