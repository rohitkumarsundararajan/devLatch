package normalize

import (
	"fmt"
	"strings"
)

const paramPlaceholder = "{param}"

type parsedKey struct {
	method, status string
	segs           []string
}

func splitKey(k string) (parsedKey, bool) {
	i := strings.IndexByte(k, ' ')
	j := strings.LastIndexByte(k, ' ')
	if i < 0 || j <= i {
		return parsedKey{}, false
	}
	return parsedKey{method: k[:i], status: k[j+1:], segs: strings.Split(k[i+1:j], "/")}, true
}

func (p parsedKey) String() string {
	return p.method + " " + strings.Join(p.segs, "/") + " " + p.status
}

func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

func sigWithout(segs []string, pos int) string {
	parts := make([]string, 0, len(segs))
	for i, s := range segs {
		if i == pos {
			continue
		}
		// Other parameter-looking segments are masked so that nested
		// parameters (/shops/s1/items/i1) can be learned together.
		if hasDigit(s) && !strings.HasPrefix(s, "{") {
			s = "*"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "/")
}

// Collapse learns path parameters from observed endpoint keys
// ("METHOD /path STATUS"). When one path position holds at least minDistinct
// distinct digit-containing values (same method, status and surrounding
// segments), those values become {param}. It returns old key -> new key for
// every input key; unchanged keys map to themselves.
func Collapse(keys []string, minDistinct int) map[string]string {
	result := make(map[string]string, len(keys))
	cur := make(map[string]parsedKey, len(keys))
	for _, k := range keys {
		result[k] = k
		if p, ok := splitKey(k); ok {
			cur[k] = p
		}
	}
	if minDistinct < 2 {
		return result
	}
	for pass := 0; pass < 8; pass++ {
		changed := false
		groups := map[string][]string{}
		for k, p := range cur {
			g := fmt.Sprintf("%s|%s|%d", p.method, p.status, len(p.segs))
			groups[g] = append(groups[g], k)
		}
		for _, members := range groups {
			n := len(cur[members[0]].segs)
			for pos := 0; pos < n; pos++ {
				bySig := map[string]map[string]bool{}
				for _, k := range members {
					segs := cur[k].segs
					v := segs[pos]
					if !hasDigit(v) || strings.HasPrefix(v, "{") {
						continue
					}
					sig := sigWithout(segs, pos)
					if bySig[sig] == nil {
						bySig[sig] = map[string]bool{}
					}
					bySig[sig][v] = true
				}
				for sig, vals := range bySig {
					if len(vals) < minDistinct {
						continue
					}
					for _, k := range members {
						p := cur[k]
						if vals[p.segs[pos]] && sigWithout(p.segs, pos) == sig {
							p.segs[pos] = paramPlaceholder
							changed = true
						}
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	for k, p := range cur {
		result[k] = p.String()
	}
	return result
}
