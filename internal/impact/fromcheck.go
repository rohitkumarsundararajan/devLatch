package impact

import (
	"bytes"
	"encoding/json"
	"sort"
	"unicode/utf16"
)

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

// FromCheck extracts drifts (breaking or warning) from `devlatch check --format json` output.
func FromCheck(data []byte) ([]Drift, error) {
	var root any
	if err := json.Unmarshal(decodeText(data), &root); err != nil {
		return nil, err
	}
	seen := map[Drift]bool{}
	var out []Drift
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			ep, _ := x["endpoint"].(string)
			p, _ := x["path"].(string)
			sev, _ := x["severity"].(string)
			typ, _ := x["type"].(string)
			if ep != "" && p != "" && sev != "SAFE" && typ != "ADDED_KEY" {
				if rf, _ := x["rename_from"].(string); rf != "" {
					p = rf
				}
				d := Drift{Endpoint: ep, Path: p}
				if !seen[d] {
					seen[d] = true
					out = append(out, d)
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].Endpoint != out[j].Endpoint {
			return out[i].Endpoint < out[j].Endpoint
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}
