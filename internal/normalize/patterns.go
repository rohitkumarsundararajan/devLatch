package normalize

import "strings"

// ApplyPatterns rewrites an endpoint key ("METHOD /path STATUS") using
// user-defined path patterns. A pattern has the same number of segments as the
// path; "*" matches any segment and becomes {param}. The first match wins.
func ApplyPatterns(key string, patterns []string) string {
	if len(patterns) == 0 {
		return key
	}
	p, ok := splitKey(key)
	if !ok {
		return key
	}
	for _, pat := range patterns {
		ps := strings.Split(pat, "/")
		if len(ps) != len(p.segs) {
			continue
		}
		match := true
		for i := range ps {
			if ps[i] != "*" && ps[i] != p.segs[i] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		for i := range ps {
			if ps[i] == "*" {
				p.segs[i] = paramPlaceholder
			}
		}
		return p.String()
	}
	return key
}
