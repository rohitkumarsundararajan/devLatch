package rename

import (
	"fmt"
	"sort"
)

// Field holds the type-level features of one key. No values, ever.
type Field struct {
	Name     string
	Kinds    []string // STRING, INTEGER, NUMBER, BOOLEAN, ARRAY, OBJECT, MAP, NULL
	Pos      int      // index among siblings
	Siblings int      // number of siblings (0 = unknown)
	Seen     float64  // fraction of samples that contained the key (0..1)
	Children []string // child key names for OBJECT / ARRAY of OBJECT; nil otherwise
	Class    string   // optional type-level value class (date, uuid, email...); "" = unknown
}

// Weights controls the signal mix. A zero weight turns a signal off.
type Weights struct {
	Name, Structure, Position, Cooc, Class float64
	NoTypeGate                             bool // baseline only: skip the hard type gate
}

// DefaultWeights is the full model.
var DefaultWeights = Weights{Name: 0.35, Structure: 0.15, Position: 0.35, Cooc: 0.15, Class: 0.05}

func stripNull(k []string) []string {
	var out []string
	for _, x := range k {
		if x != "NULL" {
			out = append(out, x)
		}
	}
	return out
}

func has(k []string, s string) bool {
	for _, x := range k {
		if x == s {
			return true
		}
	}
	return false
}

// Compatible is the hard type gate.
func Compatible(a, b []string) bool {
	a, b = stripNull(a), stripNull(b)
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, x := range a {
		if has(b, x) {
			return true
		}
	}
	return (has(a, "INTEGER") && has(b, "NUMBER")) || (has(a, "NUMBER") && has(b, "INTEGER"))
}

func sameKinds(a, b []string) bool {
	a, b = stripNull(a), stripNull(b)
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !has(b, x) {
			return false
		}
	}
	return true
}

func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	set := map[string]bool{}
	for _, x := range a {
		set[joined(x)] = true
	}
	inter, union := 0, len(set)
	seen := map[string]bool{}
	for _, x := range b {
		k := joined(x)
		if seen[k] {
			continue
		}
		seen[k] = true
		if set[k] {
			inter++
		} else {
			union++
		}
	}
	return float64(inter) / float64(union)
}

func structure(r, a Field) float64 {
	kind := 0.6
	if sameKinds(r.Kinds, a.Kinds) {
		kind = 1
	}
	if len(r.Children) == 0 && len(a.Children) == 0 {
		return kind
	}
	return 0.5*kind + 0.5*jaccard(r.Children, a.Children)
}

func position(r, a Field) float64 {
	if r.Siblings == 0 || a.Siblings == 0 {
		return 0.5
	}
	d := r.Pos - a.Pos
	if d < 0 {
		d = -d
	}
	return 1 / float64(1+d)
}

func cooc(r, a Field) float64 {
	d := r.Seen - a.Seen
	if d < 0 {
		d = -d
	}
	return 1 - d
}

// Score returns 0..1 for "removed r became added a". 0 if the type gate fails.
func Score(r, a Field, w Weights) float64 {
	if !w.NoTypeGate && !Compatible(r.Kinds, a.Kinds) {
		return 0
	}
	sum, total := 0.0, 0.0
	add := func(weight, s float64) {
		if weight > 0 {
			sum += weight * s
			total += weight
		}
	}
	add(w.Name, NameSimilarity(r.Name, a.Name))
	add(w.Structure, structure(r, a))
	if r.Siblings > 0 && a.Siblings > 0 { // unknown order is left out, not guessed
		add(w.Position, position(r, a))
	}
	add(w.Cooc, cooc(r, a))
	if r.Class != "" && a.Class != "" {
		c := 0.0
		if r.Class == a.Class {
			c = 1
		}
		add(w.Class, c)
	}
	if total == 0 {
		return 0
	}
	return sum / total
}

// Match is one accepted rename.
type Match struct {
	Old, New   string
	Confidence float64
	Evidence   []string
}

// Detect pairs removed with added fields greedily by highest score and keeps
// pairs with score >= threshold. Output is deterministic.
func Detect(removed, added []Field, threshold float64, w Weights) []Match {
	type cand struct {
		r, a  int
		score float64
	}
	var cands []cand
	for i, r := range removed {
		for j, a := range added {
			if s := Score(r, a, w); s >= threshold {
				cands = append(cands, cand{i, j, s})
			}
		}
	}
	sort.Slice(cands, func(x, y int) bool {
		if cands[x].score != cands[y].score {
			return cands[x].score > cands[y].score
		}
		if removed[cands[x].r].Name != removed[cands[y].r].Name {
			return removed[cands[x].r].Name < removed[cands[y].r].Name
		}
		return added[cands[x].a].Name < added[cands[y].a].Name
	})
	usedR, usedA := map[int]bool{}, map[int]bool{}
	var out []Match
	for _, c := range cands {
		if usedR[c.r] || usedA[c.a] {
			continue
		}
		usedR[c.r], usedA[c.a] = true, true
		out = append(out, Match{
			Old: removed[c.r].Name, New: added[c.a].Name,
			Confidence: round2(c.score), Evidence: evidence(removed[c.r], added[c.a]),
		})
	}
	return out
}

func round2(x float64) float64 { return float64(int(x*100+0.5)) / 100 }

func evidence(r, a Field) []string {
	var ev []string
	if sameKinds(r.Kinds, a.Kinds) {
		ev = append(ev, "same type")
	} else {
		ev = append(ev, "compatible type")
	}
	if n := NameSimilarity(r.Name, a.Name); n >= 0.6 {
		ev = append(ev, fmt.Sprintf("name similarity %.2f", n))
	}
	if r.Siblings > 0 && a.Siblings > 0 && r.Pos == a.Pos {
		ev = append(ev, fmt.Sprintf("same position %d", r.Pos))
	}
	if len(r.Children) > 0 && jaccard(r.Children, a.Children) >= 0.8 {
		ev = append(ev, "same child fields")
	}
	if cooc(r, a) >= 0.95 {
		ev = append(ev, "seen in same fraction of samples")
	}
	return ev
}
