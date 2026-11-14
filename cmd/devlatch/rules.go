package main

import (
	"strings"

	"github.com/OWNER/devlatch/internal/infer"
)

func redactSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			set[n] = true
		}
	}
	return set
}

// redactNode removes every property whose name is in names, at any depth.
func redactNode(n *infer.Node, names map[string]bool) {
	if n == nil || len(names) == 0 {
		return
	}
	for name, p := range n.Properties {
		if names[strings.ToLower(name)] {
			delete(n.Properties, name)
			continue
		}
		redactNode(p.Node, names)
	}
	redactNode(n.Element, names)
	redactNode(n.MapValue, names)
}
