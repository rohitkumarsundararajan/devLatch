package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
)

func proposal(idKind string) *contract.File {
	n := map[string]*infer.Node{"GET /a 200": {
		Kinds: map[infer.Kind]int{"OBJECT": 25},
		Properties: map[string]*infer.Prop{
			"id": {Node: &infer.Node{Kinds: map[infer.Kind]int{infer.Kind(idKind): 25}, Samples: 25}, SeenIn: 25},
		},
		Samples: 25,
	}}
	return contract.FromNodes(n, 20, "test", time.Now())
}

func TestInitAndApproveFlow(t *testing.T) {
	old, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	var out, errb bytes.Buffer
	ctx := context.Background()
	if c := run(ctx, []string{"init", "--target", "http://localhost:8000"}, &out, &errb); c != exitOK {
		t.Fatalf("init failed: %s", errb.String())
	}
	if c := run(ctx, []string{"init", "--target", "http://localhost:8000"}, &out, &errb); c != exitError {
		t.Fatal("second init without --force must fail")
	}
	if c := run(ctx, []string{"init", "--target", "nope"}, &out, &errb); c != exitError {
		t.Fatal("bad target must fail")
	}
	if c := run(ctx, []string{"approve"}, &out, &errb); c != exitError {
		t.Fatal("approve with no proposal must fail")
	}

	if err := contract.Save(config.ProposedPath(), proposal("INTEGER")); err != nil {
		t.Fatal(err)
	}
	if c := run(ctx, []string{"approve"}, &out, &errb); c != exitOK {
		t.Fatalf("approve failed: %s", errb.String())
	}
	if _, err := os.Stat(config.ProposedPath()); err == nil {
		t.Fatal("proposal should be removed after approve")
	}
	f, err := contract.Load(config.ContractPath())
	if err != nil || f.ApprovedBy == "" || f.ApprovedAt == "" {
		t.Fatalf("contract not approved properly: %+v %v", f, err)
	}

	// Re-approving a changed proposal must show the drift first.
	out.Reset()
	if err := contract.Save(config.ProposedPath(), proposal("STRING")); err != nil {
		t.Fatal(err)
	}
	if c := run(ctx, []string{"approve"}, &out, &errb); c != exitOK {
		t.Fatalf("re-approve failed: %s", errb.String())
	}
	if !strings.Contains(out.String(), "TYPE_CHANGED") {
		t.Fatalf("expected drift in output, got:\n%s", out.String())
	}
}
