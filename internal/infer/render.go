package infer

import "strings"

// Render formats a Node as a readable tree. "name?" marks an optional key
// (missing from some object samples); "[?]" is an array whose element type is
// unknown because only empty arrays were seen.
func Render(n *Node) string {
	var b strings.Builder
	writeNode(&b, n, 0)
	return b.String()
}

func writeNode(b *strings.Builder, n *Node, depth int) {
	b.WriteString(kindLabel(n))
	if n.Kinds[KindObject] > 0 {
		total := n.Kinds[KindObject]
		b.WriteString(" {\n")
		for _, name := range sortedKeys(n.Properties) {
			p := n.Properties[name]
			b.WriteString(strings.Repeat("  ", depth+1))
			b.WriteString(name)
			if p.SeenIn < total {
				b.WriteString("?")
			}
			b.WriteString(": ")
			writeNode(b, p.Node, depth+1)
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString("}")
	}
	if n.Kinds[KindArray] > 0 {
		b.WriteString(" [")
		if n.Element == nil {
			b.WriteString("?")
		} else {
			writeNode(b, n.Element, depth)
		}
		b.WriteString("]")
	}
	if n.Kinds[KindMap] > 0 {
		b.WriteString(" <string -> ")
		if n.MapValue == nil {
			b.WriteString("?")
		} else {
			writeNode(b, n.MapValue, depth)
		}
		b.WriteString(">")
	}
}

func kindLabel(n *Node) string {
	kinds := n.KindList()
	if len(kinds) == 0 {
		return "UNKNOWN"
	}
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = string(k)
	}
	return strings.Join(parts, "|")
}
