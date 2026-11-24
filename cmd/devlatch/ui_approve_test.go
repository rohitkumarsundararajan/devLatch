package main

import (
	"os"
	"testing"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/contract"
	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/server"
)

func TestApproveLiveRebases(t *testing.T) {
	wd, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	baseStore := feedStore(t, `{"id":1,"user_name":"ann","email":"a@b.c"}`)
	f := contract.FromNodes(collapseEndpoints(baseStore.Snapshot()), 3, "test", time.Now())
	hub := server.NewHub(50)
	defer hub.Close()
	ds := newDriftScanner(f.ToNodes(), diff.Options{MinSamples: 3, Renames: true}, hub)

	live := feedStore(t, `{"id":1,"username":"ann","email":"a@b.c"}`)
	if len(ds.scan(live)) == 0 {
		t.Fatal("expected drift before approving")
	}
	key := ""
	for k := range collapseEndpoints(live.Snapshot()) {
		key = k
	}
	if _, err := approveLive(live, ds, key, 3); err != nil {
		t.Fatal(err)
	}
	saved, err := contract.Load(config.ContractPath())
	if err != nil || saved.ApprovedBy == "" || saved.Endpoints[key] == nil {
		t.Fatalf("contract not saved correctly: %v %+v", err, saved)
	}
	if again := ds.scan(live); len(again) != 0 {
		t.Fatalf("drift after approve should be gone, got %+v", again)
	}
	if _, err := approveLive(live, ds, "GET /nope 200", 3); err == nil {
		t.Fatal("approving an endpoint with no traffic should fail")
	}
}
