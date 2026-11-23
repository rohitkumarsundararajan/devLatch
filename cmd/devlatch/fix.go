package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/fix"
	"github.com/rohitkumarsundararajan/devlatch/internal/impact"
)

var (
	fixIdentRe  = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	fixBranchRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)
)

// runFix rewrites frontend code after an API field rename. Preview by default.
func runFix(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	frontend := fs.String("frontend", "", "frontend project folder (contains tsconfig.json)")
	tsconfig := fs.String("tsconfig", "", "path to tsconfig.json (default <frontend>/tsconfig.json)")
	endpoint := fs.String("endpoint", "", `endpoint, e.g. "GET /api/v1/users/{id} 200"`)
	oldPath := fs.String("path", "", "old field path, e.g. '$.user_name'")
	newName := fs.String("to", "", "new field name, e.g. username")
	fromCheck := fs.String("from-check", "", "read renames from devlatch check --format json output (file path, or - for stdin)")
	apply := fs.Bool("apply", false, "write the changes (default: preview only)")
	pr := fs.Bool("pr", false, "create a branch, commit, push and open a pull request (implies --apply; needs git and gh)")
	allowDirty := fs.Bool("allow-dirty", false, "with --pr: allow uncommitted changes in the working tree")
	includeName := fs.Bool("include-name-only", false, "also rename low-confidence name-only matches")
	noVerify := fs.Bool("no-verify", false, "skip the TypeScript check after applying")
	asJSON := fs.Bool("json", false, "print raw JSON")
	scannerPath := fs.String("scanner", "", "path to scanner/src/cli.mjs (auto-detected)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}

	var renames []fix.Rename
	if *fromCheck != "" {
		var raw []byte
		var rerr error
		if *fromCheck == "-" {
			raw, rerr = io.ReadAll(os.Stdin)
		} else {
			raw, rerr = os.ReadFile(*fromCheck)
		}
		if rerr != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch fix:", rerr)
			return exitError
		}
		rs, skipped, perr := fix.FromCheck(raw)
		if perr != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch fix: bad check output:", perr)
			return exitError
		}
		if skipped > 0 {
			_, _ = fmt.Fprintf(stderr, "devlatch fix: skipped %d rename(s) that move a field to a different parent (not auto-fixable)\n", skipped)
		}
		renames = append(renames, rs...)
	}
	if *endpoint != "" || *oldPath != "" || *newName != "" {
		if *endpoint == "" || *oldPath == "" || *newName == "" {
			_, _ = fmt.Fprintln(stderr, "devlatch fix: give --endpoint, --path and --to together")
			return exitError
		}
		renames = append(renames, fix.Rename{Endpoint: *endpoint, Path: *oldPath, To: *newName})
	}
	if len(renames) == 0 {
		if *fromCheck != "" {
			_, _ = fmt.Fprintln(stdout, "no renames to fix")
			return exitOK
		}
		_, _ = fmt.Fprintln(stderr, "devlatch fix: give --endpoint, --path and --to (or --from-check file.json)")
		return exitError
	}
	for _, r := range renames {
		if !fixIdentRe.MatchString(r.To) {
			_, _ = fmt.Fprintf(stderr, "devlatch fix: %q is not a valid field name\n", r.To)
			return exitError
		}
	}

	cfg := *tsconfig
	if cfg == "" {
		if *frontend == "" {
			_, _ = fmt.Fprintln(stderr, "devlatch fix: give --frontend <folder> or --tsconfig <file>")
			return exitError
		}
		cfg = filepath.Join(*frontend, "tsconfig.json")
	}
	if _, err := os.Stat(cfg); err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch fix:", err)
		return exitError
	}
	script, err := impact.FindScanner(*scannerPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch fix:", err)
		return exitError
	}
	opts := fix.Options{IncludeNameOnly: *includeName, NoVerify: *noVerify}

	show := func(res fix.Result) {
		if *asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(res)
			return
		}
		fixPrint(stdout, res)
	}

	if !*apply && !*pr {
		res, err := fix.Run(script, cfg, renames, opts)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch fix:", err)
			return exitError
		}
		show(res)
		if res.Edits > 0 && !*asJSON {
			_, _ = fmt.Fprintln(stdout, "\nnothing was written. Run again with --apply to write these changes, or --pr to open a pull request.")
		}
		return exitOK
	}

	branch := ""
	if *pr {
		for _, tool := range []string{"git", "gh"} {
			if _, err := exec.LookPath(tool); err != nil {
				_, _ = fmt.Fprintf(stderr, "devlatch fix: --pr needs %s on PATH\n", tool)
				return exitError
			}
		}
		if out, err := fixExec("git", "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
			_, _ = fmt.Fprintln(stderr, "devlatch fix: --pr must be run inside a git repository")
			return exitError
		}
		if !*allowDirty {
			st, _ := fixExec("git", "status", "--porcelain")
			if strings.TrimSpace(st) != "" {
				_, _ = fmt.Fprintln(stderr, "devlatch fix: the working tree has uncommitted changes; commit or stash them, or pass --allow-dirty")
				return exitError
			}
		}
		preview, err := fix.Run(script, cfg, renames, opts)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "devlatch fix:", err)
			return exitError
		}
		if preview.Edits == 0 {
			_, _ = fmt.Fprintln(stdout, "nothing to change: no frontend usages of the renamed field were found")
			return exitOK
		}
		label := fmt.Sprintf("%s-to-%s", fix.LastSegment(renames[0].Path), renames[0].To)
		if len(renames) > 1 {
			label = fmt.Sprintf("%d-renames", len(renames))
		}
		branch = "devlatch/fix-" + fixBranchRe.ReplaceAllString(label, "-") + "-" + time.Now().Format("20060102-150405")
		if out, err := fixExec("git", "checkout", "-b", branch); err != nil {
			_, _ = fmt.Fprintf(stderr, "devlatch fix: could not create branch: %v\n%s", err, out)
			return exitError
		}
	}

	opts.Apply = true
	res, err := fix.Run(script, cfg, renames, opts)
	if err == nil && !res.Applied {
		err = errors.New("nothing was applied")
	}
	if err != nil || res.Reverted {
		show(res)
		if res.Reverted {
			_, _ = fmt.Fprintln(stderr, "devlatch fix: the rename would increase TypeScript errors, so nothing was changed")
		} else {
			_, _ = fmt.Fprintln(stderr, "devlatch fix:", err)
		}
		if branch != "" {
			_, _ = fixExec("git", "checkout", "-")
			_, _ = fixExec("git", "branch", "-D", branch)
		}
		return exitError
	}
	show(res)
	if !*pr {
		return exitOK
	}

	seen := map[string]bool{}
	files := []string{"add", "--"}
	for _, c := range res.Changes {
		if !seen[c.File] {
			seen[c.File] = true
			files = append(files, c.File)
		}
	}
	title := fmt.Sprintf("Follow API rename: %s -> %s", fix.LastSegment(renames[0].Path), renames[0].To)
	if len(renames) > 1 {
		title = fmt.Sprintf("Follow %d API renames", len(renames))
	}
	var body strings.Builder
	body.WriteString("Generated by `devlatch fix`.\n\n")
	for _, r := range renames {
		_, _ = fmt.Fprintf(&body, "- `%s`: `%s` -> `%s`\n", r.Endpoint, r.Path, r.To)
	}
	_, _ = fmt.Fprintf(&body, "\n%d edits in %d files. Only type-linked and flow-linked usages were changed", res.Edits, res.Files)
	if *includeName {
		body.WriteString(" (name-only matches included)")
	} else {
		body.WriteString("; name-only matches were skipped")
	}
	body.WriteString(".\n")
	if res.Diagnostics != nil {
		_, _ = fmt.Fprintf(&body, "TypeScript errors: %d before, %d after.\n", res.Diagnostics.Before, res.Diagnostics.After)
	}
	body.WriteString("\nReview the diff before merging.\n")

	steps := [][]string{files, {"commit", "-m", title + " (DevLatch)"}, {"push", "-u", "origin", branch}}
	for _, s := range steps {
		if out, err := fixExec("git", s...); err != nil {
			_, _ = fmt.Fprintf(stderr, "devlatch fix: git %s failed: %v\n%s", s[0], err, out)
			return exitError
		}
	}
	out, err := fixExec("gh", "pr", "create", "--title", title, "--body", body.String())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "devlatch fix: gh pr create failed: %v\n%s", err, out)
		return exitError
	}
	_, _ = fmt.Fprintf(stdout, "\nbranch %s pushed\n%s", branch, out)
	return exitOK
}

func fixExec(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
}

func fixPrint(w io.Writer, res fix.Result) {
	status := "preview"
	if res.Applied {
		status = "applied"
	} else if res.Reverted {
		status = "REVERTED (TypeScript errors increased)"
	}
	_, _ = fmt.Fprintf(w, "%s: %d edits in %d files\n", status, res.Edits, res.Files)
	for _, c := range res.Changes {
		_, _ = fmt.Fprintf(w, "  %s:%d  (%s)\n    - %s\n    + %s\n", c.File, c.Line, c.Confidence, c.Before, c.After)
	}
	if res.Diagnostics != nil {
		_, _ = fmt.Fprintf(w, "TypeScript errors: %d before, %d after\n", res.Diagnostics.Before, res.Diagnostics.After)
	}
}
