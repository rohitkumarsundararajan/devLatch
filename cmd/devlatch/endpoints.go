package main

import (
	"github.com/rohitkumarsundararajan/devlatch/internal/config"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
	"github.com/rohitkumarsundararajan/devlatch/internal/normalize"
)

// pathParamMin is how many distinct digit-containing values at one path
// position make DevLatch treat that position as a parameter.
const pathParamMin = 5

// collapseEndpoints applies the project's rules (config.json) to a store
// snapshot. Record and check both go through it, so they always agree.
func collapseEndpoints(snap []infer.EndpointSchema) map[string]*infer.Node {
	cfg, _ := config.Load() // defaults when there is no config
	return collapseWith(snap, cfg)
}

// collapseWith: user path patterns first, then cardinality-based parameter
// learning, then redaction; endpoints that end up with the same key are merged.
func collapseWith(snap []infer.EndpointSchema, cfg config.Config) map[string]*infer.Node {
	patterned := make(map[string]string, len(snap))
	seen := map[string]bool{}
	var uniq []string
	for _, ep := range snap {
		pk := normalize.ApplyPatterns(ep.Key, cfg.PathParams)
		patterned[ep.Key] = pk
		if !seen[pk] {
			seen[pk] = true
			uniq = append(uniq, pk)
		}
	}
	mapping := normalize.Collapse(uniq, pathParamMin)
	redact := redactSet(cfg.Redact)

	inf := infer.New(infer.DefaultConfig())
	out := make(map[string]*infer.Node, len(snap))
	for _, ep := range snap {
		nk := mapping[patterned[ep.Key]]
		node := ep.Node.Clone()
		redactNode(node, redact)
		if cur, ok := out[nk]; ok {
			out[nk] = inf.Merge(cur, node)
		} else {
			out[nk] = node
		}
	}
	return out
}
