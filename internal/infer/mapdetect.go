package infer

import (
	"strings"

	"github.com/OWNER/devlatch/internal/normalize"
)

// maybeMap converts n from OBJECT to MAP when its keys look dynamic. It runs
// after every build and merge, so a map is recognized whether one payload
// shows it or the keys only add up across many payloads.
func (in *Inferrer) maybeMap(n *Node, path string) {
	if n.Kinds[KindObject] == 0 || len(n.Properties) == 0 {
		return
	}
	if in.cfg.ForceMaps[path] || in.looksLikeMap(n) {
		in.toMap(n, path)
	}
}

func (in *Inferrer) looksLikeMap(n *Node) bool {
	keys := sortedKeys(n.Properties)
	patterned := sameKeyClass(keys)
	switch {
	case n.MapValue != nil:
		// This position is already a MAP in other samples: fold in object
		// samples whose keys look dynamic too.
		return patterned && valuesFit(n, keys)
	case patterned && len(keys) >= in.cfg.MapMinKeys:
		return valuesFit(n, keys)
	case len(keys) >= in.cfg.LargeObjectKeys:
		return valuesFit(n, keys)
	}
	return false
}

func (in *Inferrer) toMap(n *Node, path string) {
	mv := n.MapValue
	for _, k := range sortedKeys(n.Properties) {
		mv = in.mergeInto(mv, n.Properties[k].Node, path+"{}")
	}
	n.MapValue = mv
	n.Kinds[KindMap] += n.Kinds[KindObject]
	delete(n.Kinds, KindObject)
	n.Properties = nil
}

// valuesFit reports whether every property value shares one shape (and fits
// the existing map value, if there is one).
func valuesFit(n *Node, keys []string) bool {
	ref := n.MapValue
	for _, k := range keys {
		v := n.Properties[k].Node
		if ref == nil {
			ref = v
			continue
		}
		if !compatible(ref, v) {
			return false
		}
	}
	return true
}

// compatible: same kinds (ignoring NULL, INTEGER ~ NUMBER) and, for objects,
// mostly the same keys.
func compatible(a, b *Node) bool {
	sa, sb := kindSig(a), kindSig(b)
	if sa == "" || sb == "" { // a NULL-only value fits any shape
		return true
	}
	if sa != sb {
		return false
	}
	if a.Kinds[KindObject] > 0 && b.Kinds[KindObject] > 0 {
		return jaccard(a.Properties, b.Properties) >= 0.5
	}
	return true
}

func kindSig(n *Node) string {
	var parts []string
	for _, k := range kindOrder {
		if n.Kinds[k] == 0 || k == KindNull {
			continue
		}
		if k == KindInteger {
			k = KindNumber
		}
		if len(parts) > 0 && parts[len(parts)-1] == string(k) {
			continue
		}
		parts = append(parts, string(k))
	}
	return strings.Join(parts, "|")
}

func jaccard(a, b map[string]*Prop) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	both := 0
	for k := range a {
		if _, ok := b[k]; ok {
			both++
		}
	}
	return float64(both) / float64(len(a)+len(b)-both)
}

// sameKeyClass reports whether all keys are dates, or all integers, or all UUIDs.
func sameKeyClass(keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	first := keyClass(keys[0])
	if first == "" {
		return false
	}
	for _, k := range keys[1:] {
		if keyClass(k) != first {
			return false
		}
	}
	return true
}

func keyClass(k string) string {
	switch {
	case normalize.IsUUID(k):
		return "uuid"
	case isISODate(k):
		return "date"
	case normalize.IsInteger(k):
		return "int"
	}
	return ""
}

// isISODate matches YYYY-MM, YYYY-MM-DD and YYYY-MM-DDT... style keys.
func isISODate(s string) bool {
	if len(s) < 7 || !normalize.IsInteger(s[0:4]) || s[4] != '-' || !normalize.IsInteger(s[5:7]) {
		return false
	}
	if len(s) == 7 {
		return true
	}
	if len(s) < 10 || s[7] != '-' || !normalize.IsInteger(s[8:10]) {
		return false
	}
	return len(s) == 10 || s[10] == 'T' || s[10] == ' '
}
