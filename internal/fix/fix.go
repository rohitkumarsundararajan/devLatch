package fix

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

// Rename is one field rename: Path is the old field path (for example $.user_name), To is the new field name.
type Rename struct {
	Endpoint string `json:"endpoint"`
	Path     string `json:"path"`
	To       string `json:"to"`
}

// Change is one rewritten line.
type Change struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Confidence string `json:"confidence"`
	Before     string `json:"before"`
	After      string `json:"after"`
}

// Diagnostics counts TypeScript errors before and after applying.
type Diagnostics struct {
	Before int `json:"before"`
	After  int `json:"after"`
}

// Result is the scanner's answer for a fix run.
type Result struct {
	Edits       int          `json:"edits"`
	Files       int          `json:"files"`
	Changes     []Change     `json:"changes"`
	Applied     bool         `json:"applied"`
	Reverted    bool         `json:"reverted"`
	Diagnostics *Diagnostics `json:"diagnostics,omitempty"`
}

// Options controls a fix run. With Apply false it only previews.
type Options struct {
	Apply           bool
	IncludeNameOnly bool
	NoVerify        bool
}

func decodeText(b []byte) []byte {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		u := make([]uint16, (len(b)-2)/2)
		for i := range u {
			u[i] = uint16(b[2+2*i]) | uint16(b[3+2*i])<<8
		}
		return []byte(string(utf16.Decode(u)))
	}
	return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
}

// LastSegment returns the final field name of a JSON path: "$.users[].user_name" gives "user_name".
func LastSegment(p string) string {
	p = strings.TrimPrefix(strings.TrimPrefix(p, "$"), ".")
	parts := strings.Split(p, ".")
	return strings.TrimSuffix(parts[len(parts)-1], "[]")
}

func parentPath(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 {
		return p[:i]
	}
	return ""
}

// FromCheck extracts simple renames (same parent, new last name) from `devlatch check --format json` output.
// skipped counts renames that move a field to a different parent, which renaming cannot fix.
func FromCheck(data []byte) (renames []Rename, skipped int, err error) {
	var root any
	if err := json.Unmarshal(decodeText(data), &root); err != nil {
		return nil, 0, err
	}
	seen := map[Rename]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			ep, _ := x["endpoint"].(string)
			typ, _ := x["type"].(string)
			from, _ := x["rename_from"].(string)
			to, _ := x["rename_to"].(string)
			if typ == "POSSIBLE_RENAME" && ep != "" && from != "" && to != "" {
				name := LastSegment(to)
				if parentPath(from) != parentPath(to) || name == "" || name == LastSegment(from) {
					skipped++
				} else if r := (Rename{Endpoint: ep, Path: from, To: name}); !seen[r] {
					seen[r] = true
					renames = append(renames, r)
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(root)
	sort.Slice(renames, func(i, j int) bool {
		if renames[i].Endpoint != renames[j].Endpoint {
			return renames[i].Endpoint < renames[j].Endpoint
		}
		return renames[i].Path < renames[j].Path
	})
	return renames, skipped, nil
}

// Run asks the Node scanner to plan (and with Apply, write) the renames.
// After writing it re-checks TypeScript and reverts everything if errors increased (Result.Reverted).
func Run(script, tsconfig string, renames []Rename, o Options) (Result, error) {
	var res Result
	node, err := exec.LookPath("node")
	if err != nil {
		return res, errors.New("node not found on PATH; the scanner needs Node 18+")
	}
	root := filepath.Dir(filepath.Dir(script))
	if _, err := os.Stat(filepath.Join(root, "node_modules", "ts-morph")); err != nil {
		return res, fmt.Errorf("scanner dependencies missing; run: cd %s; npm install", root)
	}
	in, err := json.Marshal(map[string]any{"tsconfig": tsconfig, "drifts": renames})
	if err != nil {
		return res, err
	}
	tmp, err := os.CreateTemp("", "devlatch-fix-*.json")
	if err != nil {
		return res, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(in); err != nil {
		_ = tmp.Close()
		return res, err
	}
	if err := tmp.Close(); err != nil {
		return res, err
	}

	args := []string{script, "--input", tmp.Name(), "--fix", "--json"}
	if o.Apply {
		args = append(args, "--apply")
	}
	if o.IncludeNameOnly {
		args = append(args, "--include-name-only")
	}
	if o.NoVerify {
		args = append(args, "--no-verify")
	}
	cmd := exec.Command(node, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	if jerr := json.Unmarshal(out.Bytes(), &res); jerr != nil {
		if runErr != nil {
			return res, fmt.Errorf("scanner failed: %w\n%s", runErr, errb.String())
		}
		return res, fmt.Errorf("could not read scanner output: %w", jerr)
	}
	return res, nil
}
