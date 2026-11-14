package diff

import (
	"sort"

	"github.com/OWNER/devlatch/internal/infer"
	"github.com/OWNER/devlatch/internal/rename"
)

func sortedProps(n *infer.Node) []string {
	keys := make([]string, 0, len(n.Properties))
	for k := range n.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// childNames lists the key names of an object (or of an array's object elements).
func childNames(n *infer.Node) []string {
	switch {
	case has(n, kObject):
		return sortedProps(n)
	case has(n, kArray) && n.Element != nil && has(n.Element, kObject):
		return sortedProps(n.Element)
	}
	return nil
}

func fieldOf(name string, p *infer.Prop, siblings, samples int) rename.Field {
	f := rename.Field{Name: name, Kinds: kindNames(p.Node), Children: childNames(p.Node), Seen: 1}
	if samples > 0 {
		f.Seen = float64(p.SeenIn) / float64(samples)
		if f.Seen > 1 {
			f.Seen = 1
		}
	}
	if p.Order > 0 { // Order 0 = unknown (contract recorded before key order was kept)
		f.Pos = p.Order - 1
		f.Siblings = siblings
	}
	return f
}

// renames finds removed keys that most likely became one of the added keys.
// Only removals that would be reported anyway are candidates, so turning
// renames on never creates a violation that was not there before.
func (d *differ) renames(b, l *infer.Node, bs, ls int, enforced bool) []rename.Match {
	var removed, added []rename.Field
	for _, k := range sortedProps(b) {
		if l.Properties[k] != nil {
			continue
		}
		bp := b.Properties[k]
		if _, ok := RemovedSeverity(enforced && bp.SeenIn == bs); !ok {
			continue
		}
		removed = append(removed, fieldOf(k, bp, len(b.Properties), bs))
	}
	for _, k := range sortedProps(l) {
		if b.Properties[k] != nil {
			continue
		}
		added = append(added, fieldOf(k, l.Properties[k], len(l.Properties), ls))
	}
	if len(removed) == 0 || len(added) == 0 {
		return nil
	}
	return rename.Detect(removed, added, rename.DefaultThreshold, rename.DefaultWeights)
}
