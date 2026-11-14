package rename

import (
	"strings"
	"unicode"
)

var abbreviations = map[string]string{
	"qty": "quantity", "desc": "description", "descr": "description", "addr": "address",
	"amt": "amount", "num": "number", "img": "image", "msg": "message", "usr": "user",
	"ts": "timestamp", "dt": "date", "tel": "telephone", "cnt": "count", "pwd": "password",
	"info": "information", "lat": "latitude", "lng": "longitude", "lon": "longitude",
	"ref": "reference", "val": "value", "idx": "index", "cat": "category", "org": "organization",
	"dept": "department", "id": "identifier", "zip": "zipcode",
}

// Tokens splits a field name into lowercase tokens: camelCase, snake_case and
// kebab-case boundaries, abbreviations expanded, naive singular form.
func Tokens(name string) []string {
	var raw []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			raw = append(raw, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(name)
	for i, r := range rs {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
		case unicode.IsUpper(r) && i > 0 && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1]) ||
			(i+1 < len(rs) && unicode.IsLower(rs[i+1]) && unicode.IsUpper(rs[i-1]))):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		if full, ok := abbreviations[t]; ok {
			t = full
		}
		out = append(out, singular(t))
	}
	return out
}

func singular(t string) string {
	switch {
	case len(t) > 4 && strings.HasSuffix(t, "ies"):
		return t[:len(t)-3] + "y"
	case len(t) > 3 && strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "ss") &&
		!strings.HasSuffix(t, "us") && !strings.HasSuffix(t, "is"):
		return t[:len(t)-1]
	}
	return t
}

func joined(name string) string { return strings.Join(Tokens(name), "") }

// genericTokens say what kind of field it is (id, count, date...) more than
// which field it is. role_id and user_id share "identifier" but are different
// fields; item_count and item_id share "item" but are different fields.
var genericTokens = map[string]bool{
	"identifier": true, "name": true, "count": true, "date": true, "time": true, "timestamp": true,
	"at": true, "url": true, "type": true, "number": true, "code": true, "key": true, "value": true,
}

func split(ts []string) (distinct, generic []string) {
	for _, t := range ts {
		if genericTokens[t] {
			generic = append(generic, t)
		} else {
			distinct = append(distinct, t)
		}
	}
	return
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}

// genericFactor scales similarity by how the generic tokens line up:
// same kind of field (1), one side has no kind word (0.9), different kinds (0.4).
func genericFactor(ga, gb []string) float64 {
	switch {
	case sameSet(ga, gb):
		return 1
	case len(ga) == 0 || len(gb) == 0:
		return 0.9
	}
	return 0.4
}

// NameSimilarity returns 0..1 using Jaro-Winkler, normalized Levenshtein and
// token overlap on normalized names. When both names have distinctive tokens,
// those are compared and the generic ones only scale the result.
func NameSimilarity(a, b string) float64 {
	ta, tb := Tokens(a), Tokens(b)
	if strings.Join(ta, "") == strings.Join(tb, "") {
		return 1
	}
	factor := 1.0
	ea, eb := ta, tb
	if da, ga := split(ta); len(da) > 0 {
		if db, gb := split(tb); len(db) > 0 {
			ea, eb = da, db
			factor = genericFactor(ga, gb)
		}
	}
	ja, jb := strings.Join(ea, ""), strings.Join(eb, "")
	s := 1.0
	if ja != jb {
		s = (jaroWinkler(ja, jb) + levSim(ja, jb)) / 2
		if tok := tokenScore(ea, eb); tok > s {
			s = tok
		}
	}
	// Unrelated strings still score ~0.3 on edit distance; stretch so they sit near 0.
	s = (s*factor - nameFloor) / (1 - nameFloor)
	if s < 0 {
		return 0
	}
	return s
}

const nameFloor = 0.35

func tokenScore(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, t := range a {
		set[t] = true
	}
	inter := 0
	seen := map[string]bool{}
	for _, t := range b {
		if set[t] && !seen[t] {
			inter++
			seen[t] = true
		}
	}
	union := len(set)
	for _, t := range b {
		if !set[t] {
			union++
			set[t] = true
		}
	}
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	jac := float64(inter) / float64(union)
	overlap := float64(inter) / float64(minLen)
	return 0.5*jac + 0.5*overlap
}

func levSim(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	max := len(ra)
	if len(rb) > max {
		max = len(rb)
	}
	return 1 - float64(prev[len(rb)])/float64(max)
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func jaroWinkler(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	window := len(ra)
	if len(rb) > window {
		window = len(rb)
	}
	window = window/2 - 1
	if window < 0 {
		window = 0
	}
	ma, mb := make([]bool, len(ra)), make([]bool, len(rb))
	matches := 0
	for i := range ra {
		lo, hi := i-window, i+window+1
		if lo < 0 {
			lo = 0
		}
		if hi > len(rb) {
			hi = len(rb)
		}
		for j := lo; j < hi; j++ {
			if !mb[j] && ra[i] == rb[j] {
				ma[i], mb[j] = true, true
				matches++
				break
			}
		}
	}
	if matches == 0 {
		return 0
	}
	trans, k := 0, 0
	for i := range ra {
		if !ma[i] {
			continue
		}
		for !mb[k] {
			k++
		}
		if ra[i] != rb[k] {
			trans++
		}
		k++
	}
	m := float64(matches)
	jaro := (m/float64(len(ra)) + m/float64(len(rb)) + (m-float64(trans)/2)/m) / 3
	prefix := 0
	for i := 0; i < len(ra) && i < len(rb) && i < 4 && ra[i] == rb[i]; i++ {
		prefix++
	}
	return jaro + float64(prefix)*0.1*(1-jaro)
}
