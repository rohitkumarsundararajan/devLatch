// Package version holds the build version, set at link time via -ldflags.
package version

// Version is overridden by the Makefile / goreleaser with -ldflags "-X ...".
var Version = "dev"
