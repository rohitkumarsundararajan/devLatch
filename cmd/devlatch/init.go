package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/OWNER/devlatch/internal/config"
)

func parseTarget(s string) (*url.URL, bool) {
	u, err := url.Parse(s)
	if s == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, false
	}
	return u, true
}

func runInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "backend URL, e.g. http://localhost:8000 (required)")
	port := fs.Int("port", 9000, "proxy port used by record")
	minSamples := fs.Int("min-samples", 20, "responses needed per endpoint before a field counts as required")
	force := fs.Bool("force", false, "overwrite an existing config")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitError
	}
	if _, ok := parseTarget(*target); !ok {
		_, _ = fmt.Fprintln(stderr, "devlatch init: --target is required and must look like http://localhost:8000")
		return exitError
	}
	if _, err := os.Stat(config.ConfigPath()); err == nil && !*force {
		_, _ = fmt.Fprintf(stderr, "devlatch init: %s already exists (use --force to overwrite)\n", config.ConfigPath())
		return exitError
	}
	cfg := config.Default()
	cfg.Target, cfg.Port, cfg.MinSamples = *target, *port, *minSamples
	if err := config.Save(cfg); err != nil {
		_, _ = fmt.Fprintln(stderr, "devlatch init:", err)
		return exitError
	}
	_, _ = fmt.Fprintf(stdout, "Wrote %s\nNext: devlatch record   (then point your app at http://127.0.0.1:%d)\n", config.ConfigPath(), cfg.Port)
	return exitOK
}
