// Package normalize turns raw request paths into stable endpoint keys.
//
// Phase 1 implements the rule-based part only: integer and UUID path segments
// become {id}. Cardinality-based inference ("many distinct values here, so it
// is a parameter") arrives in Phase 3b.
package normalize

import (
	"fmt"
	"strings"
)

// Path replaces integer and UUID path segments with {id}.
func Path(p string) string {
	if p == "" {
		return "/"
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if IsInteger(s) || IsUUID(s) {
			segs[i] = "{id}"
		}
	}
	return strings.Join(segs, "/")
}

// EndpointKey builds the key contracts are stored under: "METHOD /path STATUS".
func EndpointKey(method, path string, status int) string {
	return fmt.Sprintf("%s %s %d", strings.ToUpper(method), Path(path), status)
}

// IsInteger reports whether s is a non-empty string of ASCII digits.
func IsInteger(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsUUID reports whether s looks like a canonical 8-4-4-4-12 hex UUID.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return false
			}
		default:
			if !isHex(s[i]) {
				return false
			}
		}
	}
	return true
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
