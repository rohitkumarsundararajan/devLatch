package impact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Drift is one drifted field on one endpoint.
type Drift struct {
	Endpoint string `json:"endpoint"`
	Path     string `json:"path"`
}

// Hit is one place in the frontend that touches the drifted field.
type Hit struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	Snippet    string `json:"snippet"`
}

// Result is the scanner's answer for one drift.
type Result struct {
	Endpoint      string `json:"endpoint"`
	Path          string `json:"path"`
	EndpointCalls int    `json:"endpointCalls"`
	Hits          []Hit  `json:"hits"`
}

// Files returns the number of distinct files in the result.
func (r Result) Files() int {
	seen := map[string]bool{}
	for _, h := range r.Hits {
		seen[h.File] = true
	}
	return len(seen)
}

// FindScanner locates scanner/src/cli.mjs.
func FindScanner(explicit string) (string, error) {
	var cands []string
	if explicit != "" {
		cands = append(cands, explicit)
	}
	if env := os.Getenv("DEVLATCH_SCANNER"); env != "" {
		cands = append(cands, env)
	}
	cands = append(cands, filepath.Join("scanner", "src", "cli.mjs"))
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands,
			filepath.Join(d, "scanner", "src", "cli.mjs"),
			filepath.Join(d, "..", "scanner", "src", "cli.mjs"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
	}
	return "", errors.New("scanner not found (expected scanner/src/cli.mjs); use --scanner or set DEVLATCH_SCANNER")
}

// Scan runs the Node scanner and returns its results.
func Scan(script, tsconfig string, drifts []Drift) ([]Result, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, errors.New("node not found on PATH; the impact scanner needs Node 18+")
	}
	root := filepath.Dir(filepath.Dir(script))
	if _, err := os.Stat(filepath.Join(root, "node_modules", "ts-morph")); err != nil {
		return nil, fmt.Errorf("scanner dependencies missing; run: cd %s; npm install", root)
	}
	in, err := json.Marshal(map[string]any{"tsconfig": tsconfig, "drifts": drifts})
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "devlatch-impact-*.json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(in); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	cmd := exec.Command(node, script, "--input", tmp.Name(), "--json")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("scanner failed: %w\n%s", err, errb.String())
	}
	var parsed struct {
		Results []Result `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("could not read scanner output: %w", err)
	}
	return parsed.Results, nil
}
