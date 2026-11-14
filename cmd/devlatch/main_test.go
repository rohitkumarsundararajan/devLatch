package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"version command", []string{"version"}, exitOK, "devlatch dev", ""},
		{"version flag", []string{"--version"}, exitOK, "devlatch dev", ""},
		{"help", []string{"help"}, exitOK, "watch", ""},
		{"no args", nil, exitError, "", "Usage:"},
		{"unknown command", []string{"nope"}, exitError, "", `unknown command "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run(context.Background(), tt.args, &out, &errOut)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(out.String(), tt.wantStdout) {
				t.Errorf("stdout %q does not contain %q", out.String(), tt.wantStdout)
			}
			if !strings.Contains(errOut.String(), tt.wantStderr) {
				t.Errorf("stderr %q does not contain %q", errOut.String(), tt.wantStderr)
			}
		})
	}
}
