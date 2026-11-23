// Command devlatch is the DevLatch CLI.
//
// Commands so far: watch, init, record, approve, check, version, help.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/rohitkumarsundararajan/devlatch/internal/version"
)

// Exit codes: 0 ok, 1 drift found (check), 2 tool or usage error.
const (
	exitOK    = 0
	exitDrift = 1
	exitError = 2
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitError
	}
	switch args[0] {
	case "watch":
		return runWatch(ctx, args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "record":
		return runRecord(ctx, args[1:], stdout, stderr)
	case "approve":
		return runApprove(args[1:], stdout, stderr)
	case "probe":
		return runProbe(ctx, args[1:], stdout, stderr)
	case "diff":
		return runDiff(args[1:], stdout, stderr)
	case "fix":
		return runFix(args[1:], stdout, stderr)
	case "impact":
		return runImpact(args[1:], stdout, stderr)
	case "check":
		return runCheck(ctx, args[1:], stdout, stderr)
	case "version", "--version":
		_, _ = fmt.Fprintf(stdout, "devlatch %s\n", version.Version)
		return exitOK
	case "help", "--help", "-h":
		usage(stdout)
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "devlatch: unknown command %q\n\n", args[0])
		usage(stderr)
		return exitError
	}
}

func usage(w io.Writer) {
	_, _ = fmt.Fprint(w, `DevLatch: zero-config API contract guard

Usage:
  devlatch <command> [flags]

Commands:
  init      create .devlatch/config.json for a backend
  record    learn a contract proposal from live traffic
  approve   accept the recorded proposal as the contract
  check     compare live traffic with the contract (exit 1 on breaking drift)
  diff      compare two contract files (devlatch diff OLD NEW)
  impact    list frontend files that read a drifted field
  fix       rename drifted fields in frontend code (preview, --apply, --pr)
  probe     GET an API directly (no proxy) to record or check its contract
  watch     run a proxy and print the shapes it learns (no files written)
  version   print the version
  help      show this help

Run "devlatch <command> -h" for flags.
`)
}
