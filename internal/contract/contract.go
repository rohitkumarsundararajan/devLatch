package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
)

const SchemaVersion = "1.0.0"

// Node is the simplified, persisted form of an inferred schema node:
// sorted kinds and a required flag, no raw counts, never any values.
type Node struct {
	Kinds      []string         `json:"kinds"`
	Properties map[string]*Prop `json:"properties,omitempty"`
	Element    *Node            `json:"element,omitempty"`
	MapValue   *Node            `json:"map_value,omitempty"`
}

type Prop struct {
	Order    int   `json:"order,omitempty"` // key order, 1-based; 0 = unknown
	Required bool  `json:"required"`
	Node     *Node `json:"node"`
}

type Endpoint struct {
	SampleCount int   `json:"sample_count"`
	Schema      *Node `json:"schema"`
}

// File is the content of .devlatch/contracts.json.
type File struct {
	SchemaVersion string               `json:"schemaVersion"`
	GeneratedAt   string               `json:"generated_at"`
	ToolVersion   string               `json:"tool_version"`
	ApprovedBy    string               `json:"approved_by,omitempty"`
	ApprovedAt    string               `json:"approved_at,omitempty"`
	Endpoints     map[string]*Endpoint `json:"endpoints"`
}

func objSamples(n *infer.Node) int {
	if c := n.Kinds[infer.Kind("OBJECT")]; c > 0 {
		return c
	}
	return n.Samples
}

func simplify(n *infer.Node, minSamples int) *Node {
	if n == nil {
		return nil
	}
	out := &Node{}
	for k, c := range n.Kinds {
		if c > 0 {
			out.Kinds = append(out.Kinds, string(k))
		}
	}
	sort.Strings(out.Kinds)
	if len(n.Properties) > 0 {
		total := objSamples(n)
		out.Properties = make(map[string]*Prop, len(n.Properties))
		for name, p := range n.Properties {
			out.Properties[name] = &Prop{
				Order:    p.Order,
				Required: p.SeenIn == total && total >= minSamples,
				Node:     simplify(p.Node, minSamples),
			}
		}
	}
	out.Element = simplify(n.Element, minSamples)
	out.MapValue = simplify(n.MapValue, minSamples)
	return out
}

// FromNodes builds a contract from inferred endpoint schemas.
func FromNodes(nodes map[string]*infer.Node, minSamples int, toolVersion string, now time.Time) *File {
	f := &File{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		ToolVersion:   toolVersion,
		Endpoints:     make(map[string]*Endpoint, len(nodes)),
	}
	for ep, n := range nodes {
		if n == nil || n.Samples == 0 {
			continue
		}
		f.Endpoints[ep] = &Endpoint{SampleCount: n.Samples, Schema: simplify(n, minSamples)}
	}
	return f
}

func expand(n *Node, total int) *infer.Node {
	if n == nil {
		return nil
	}
	out := &infer.Node{Kinds: map[infer.Kind]int{}, Samples: total}
	for _, k := range n.Kinds {
		out.Kinds[infer.Kind(k)] = total
	}
	if len(n.Properties) > 0 {
		out.Properties = make(map[string]*infer.Prop, len(n.Properties))
		for name, p := range n.Properties {
			seen := total
			if !p.Required && total > 0 {
				seen = total - 1
			}
			out.Properties[name] = &infer.Prop{Node: expand(p.Node, total), SeenIn: seen, Order: p.Order}
		}
	}
	out.Element = expand(n.Element, total)
	out.MapValue = expand(n.MapValue, total)
	return out
}

// ToNodes rebuilds diff-ready schemas from a loaded contract.
func (f *File) ToNodes() map[string]*infer.Node {
	out := make(map[string]*infer.Node, len(f.Endpoints))
	for ep, e := range f.Endpoints {
		out[ep] = expand(e.Schema, e.SampleCount)
	}
	return out
}

// LowSample lists endpoints recorded with fewer than min samples.
func (f *File) LowSample(min int) []string {
	var eps []string
	for ep, e := range f.Endpoints {
		if e.SampleCount < min {
			eps = append(eps, fmt.Sprintf("%s (%d samples)", ep, e.SampleCount))
		}
	}
	sort.Strings(eps)
	return eps
}

// Save writes the contract atomically with stable key order.
func Save(path string, f *File) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads and validates a contract file.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.SchemaVersion == "" {
		return nil, errors.New("contract file has no schemaVersion")
	}
	if f.Endpoints == nil {
		f.Endpoints = map[string]*Endpoint{}
	}
	return &f, nil
}
