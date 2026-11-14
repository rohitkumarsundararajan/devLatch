package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/OWNER/devlatch/internal/config"
	"github.com/OWNER/devlatch/internal/contract"
	"github.com/OWNER/devlatch/internal/diff"
	"github.com/OWNER/devlatch/internal/report"
)

// runApprove promotes the recorded proposal to the enforced contract.
func runApprove(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}
	prop, err := contract.Load(config.ProposedPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, _ = fmt.Fprintln(stderr, "devlatch approve: nothing to approve; run devlatch record first")
		} else {
			_, _ = fmt.Fprintln(stderr, "devlatch approve:", err)
		}
		return exitError
	}
	if old, err := contract.Load(config.ContractPath()); err == nil {
		printChanges(stdout, old, prop)
	}
	prop.ApprovedBy = approver()
	prop.ApprovedAt = time.Now().UTC().Format(time.RFC3339)
	if err := contract.Save(config.ContractPath(), prop); err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch approve:", err)
		return exitError
	}
	_ = os.Remove(config.ProposedPath())
	_, _ = fmt.Fprintf(stdout, "Approved %d endpoints -> %s\nCommit the %s folder to git.\n", len(prop.Endpoints), config.ContractPath(), config.Dir)
	return exitOK
}

func approver() string {
	for _, k := range []string{"USERNAME", "USER"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "unknown"
}

func missingKeys(have, want map[string]*contract.Endpoint) []string {
	var out []string
	for k := range want {
		if _, ok := have[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func printChanges(w io.Writer, old, prop *contract.File) {
	_, _ = fmt.Fprintln(w, "Changes versus the current contract:")
	for _, k := range missingKeys(old.Endpoints, prop.Endpoints) {
		_, _ = fmt.Fprintf(w, "  + new endpoint %s\n", k)
	}
	for _, k := range missingKeys(prop.Endpoints, old.Endpoints) {
		_, _ = fmt.Fprintf(w, "  - endpoint no longer recorded %s\n", k)
	}
	vs := diff.Diff(old.ToNodes(), prop.ToNodes(), diff.Options{})
	report.Text(w, vs, false)
	_, _ = fmt.Fprintln(w)
}
