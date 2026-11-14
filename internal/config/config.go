package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Dir is the project folder DevLatch keeps in git.
const Dir = ".devlatch"

func ConfigPath() string   { return filepath.Join(Dir, "config.json") }
func ContractPath() string { return filepath.Join(Dir, "contracts.json") }
func ProposedPath() string { return filepath.Join(Dir, "proposed.json") }

// Config is .devlatch/config.json.
type Config struct {
	Target     string   `json:"target"`
	Port       int      `json:"port"`
	MinSamples int      `json:"min_samples"`
	Ignore     []string `json:"ignore,omitempty"`      // path prefixes forwarded but never analyzed
	Redact     []string `json:"redact,omitempty"`      // field names dropped from every schema (case-insensitive)
	PathParams []string `json:"path_params,omitempty"` // e.g. "/posts/*": each * becomes {param}
	Probe      []string `json:"probe,omitempty"`       // GET paths for devlatch probe, e.g. "/users/1"
	// ProbeHeaders maps a header name to the NAME of an environment variable
	// holding its value. Only names are stored, never secrets.
	ProbeHeaders map[string]string `json:"probe_headers,omitempty"`
}

func Default() Config { return Config{Port: 9000, MinSamples: 20} }

// Load returns defaults plus whatever config.json overrides. A missing file
// returns the defaults together with the read error.
func Load() (Config, error) {
	c := Default()
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", ConfigPath(), err)
	}
	if c.MinSamples <= 0 {
		c.MinSamples = 20
	}
	return c, nil
}

func Save(c Config) error {
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), append(b, '\n'), 0o644)
}
