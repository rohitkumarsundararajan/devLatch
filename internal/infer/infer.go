// Package infer learns the structure of JSON payloads: which kinds of values
// appear at each position, which object keys are optional, and where objects
// are really maps with dynamic keys. It stores structure only, never values.
package infer

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
)

// Kind is a JSON value kind. INTEGER and NUMBER are kept apart on purpose:
// widening int -> float is mostly safe, number -> string is breaking.
type Kind string

// The kinds a position in a payload can have.
const (
	KindString  Kind = "STRING"
	KindInteger Kind = "INTEGER" // number literal with no fraction or exponent
	KindNumber  Kind = "NUMBER"  // any other number literal
	KindBoolean Kind = "BOOLEAN"
	KindArray   Kind = "ARRAY"
	KindObject  Kind = "OBJECT"
	KindMap     Kind = "MAP" // object with dynamic keys, e.g. {"2026-01-01": {...}}
	KindNull    Kind = "NULL"
)

// kindOrder is the stable display order of kinds.
var kindOrder = []Kind{KindString, KindInteger, KindNumber, KindBoolean, KindArray, KindObject, KindMap, KindNull}

// Node describes the structure observed at one position in a JSON payload.
// A position can be several kinds at once (a union such as STRING|NULL).
type Node struct {
	Kinds      map[Kind]int     `json:"kinds"`                // kind -> times observed
	Properties map[string]*Prop `json:"properties,omitempty"` // when OBJECT is among Kinds
	Element    *Node            `json:"element,omitempty"`    // when ARRAY is among Kinds; nil = unknown (only empty arrays seen)
	MapValue   *Node            `json:"map_value,omitempty"`  // when MAP is among Kinds
	Samples    int              `json:"samples"`              // times this position was observed (sum of Kinds)
}

// Prop is one key of an object node.
type Prop struct {
	Node   *Node `json:"node"`
	SeenIn int   `json:"seen_in"`         // object samples that contained this key
	Order  int   `json:"order,omitempty"` // 1-based index of first sighting among siblings; 0 = unknown
}

// ObjectSamples is how many times this position was an OBJECT.
func (n *Node) ObjectSamples() int { return n.Kinds[KindObject] }

// Required reports whether key was present in every object sample and there
// were at least minSamples of them. Below the threshold nothing is required.
func (n *Node) Required(key string, minSamples int) bool {
	p, ok := n.Properties[key]
	if !ok {
		return false
	}
	obj := n.ObjectSamples()
	return p.SeenIn == obj && obj >= minSamples
}

// Nullable reports whether NULL was seen alongside another kind.
func (n *Node) Nullable() bool { return n.Kinds[KindNull] > 0 && len(n.Kinds) > 1 }

// KindList returns the kinds seen, in stable display order.
func (n *Node) KindList() []Kind {
	out := make([]Kind, 0, len(n.Kinds))
	for _, k := range kindOrder {
		if n.Kinds[k] > 0 {
			out = append(out, k)
		}
	}
	return out
}

// Clone returns a deep copy.
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}
	c := &Node{Samples: n.Samples, Kinds: make(map[Kind]int, len(n.Kinds))}
	for k, v := range n.Kinds {
		c.Kinds[k] = v
	}
	if n.Properties != nil {
		c.Properties = make(map[string]*Prop, len(n.Properties))
		for name, p := range n.Properties {
			c.Properties[name] = &Prop{Node: p.Node.Clone(), SeenIn: p.SeenIn, Order: p.Order}
		}
	}
	c.Element = n.Element.Clone()
	c.MapValue = n.MapValue.Clone()
	return c
}

// Config tunes inference. Zero values fall back to the defaults.
type Config struct {
	// MapMinKeys: an object with at least this many keys that all look like one
	// dynamic-key pattern (dates, integers, UUIDs) and share a value shape is a MAP.
	MapMinKeys int
	// LargeObjectKeys: an object with at least this many keys of one value shape
	// is a MAP even without a recognizable key pattern.
	LargeObjectKeys int
	// ForceMaps lists JSON paths always treated as maps, e.g. "$.stats.byDay".
	// Array elements are written "[]" and map values "{}" in paths.
	ForceMaps map[string]bool
}

// DefaultConfig returns the default inference settings.
func DefaultConfig() Config { return Config{MapMinKeys: 3, LargeObjectKeys: 200} }

// Inferrer turns JSON payloads into Nodes and merges Nodes together.
type Inferrer struct{ cfg Config }

// New returns an Inferrer using cfg (zero fields take defaults).
func New(cfg Config) *Inferrer {
	d := DefaultConfig()
	if cfg.MapMinKeys <= 0 {
		cfg.MapMinKeys = d.MapMinKeys
	}
	if cfg.LargeObjectKeys <= 0 {
		cfg.LargeObjectKeys = d.LargeObjectKeys
	}
	return &Inferrer{cfg: cfg}
}

// Observe parses one JSON payload and returns its structure with every count at 1.
func (in *Inferrer) Observe(body []byte) (*Node, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // keep the source text so 1 and 1.0 stay distinguishable
	v, err := decodeOrdered(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("infer: unexpected data after JSON value")
	}
	return in.build(v, "$"), nil
}

// Merge folds src into dst and returns dst (or src when dst is nil).
// It takes ownership of src's contents: do not use src afterwards.
func (in *Inferrer) Merge(dst, src *Node) *Node { return in.mergeInto(dst, src, "$") }

func leaf(k Kind) *Node { return &Node{Kinds: map[Kind]int{k: 1}, Samples: 1} }

func (in *Inferrer) build(v any, path string) *Node {
	switch x := v.(type) {
	case bool:
		return leaf(KindBoolean)
	case string:
		return leaf(KindString)
	case json.Number:
		if isIntegerLiteral(string(x)) {
			return leaf(KindInteger)
		}
		return leaf(KindNumber)
	case []any:
		n := leaf(KindArray)
		for _, el := range x { // every element is merged, not just the first
			n.Element = in.mergeInto(n.Element, in.build(el, path+"[]"), path+"[]")
		}
		return n
	case *ordObj:
		n := leaf(KindObject)
		n.Properties = make(map[string]*Prop, len(x.keys))
		for i, k := range x.keys {
			n.Properties[k] = &Prop{Node: in.build(x.vals[k], path+"."+k), SeenIn: 1, Order: i + 1}
		}
		in.maybeMap(n, path)
		return n
	case map[string]any:
		n := leaf(KindObject)
		n.Properties = make(map[string]*Prop, len(x))
		for k, val := range x {
			n.Properties[k] = &Prop{Node: in.build(val, path+"."+k), SeenIn: 1}
		}
		in.maybeMap(n, path)
		return n
	default: // nil (JSON null)
		return leaf(KindNull)
	}
}

func (in *Inferrer) mergeInto(dst, src *Node, path string) *Node {
	if src == nil {
		return dst
	}
	if dst == nil {
		return src
	}
	for k, c := range src.Kinds {
		dst.Kinds[k] += c
	}
	dst.Samples += src.Samples
	for name, sp := range src.Properties {
		if dst.Properties == nil {
			dst.Properties = make(map[string]*Prop, len(src.Properties))
		}
		if dp, ok := dst.Properties[name]; ok {
			in.mergeInto(dp.Node, sp.Node, path+"."+name)
			dp.SeenIn += sp.SeenIn
			if dp.Order == 0 {
				dp.Order = sp.Order
			}
		} else {
			dst.Properties[name] = sp
		}
	}
	dst.Element = in.mergeInto(dst.Element, src.Element, path+"[]")
	dst.MapValue = in.mergeInto(dst.MapValue, src.MapValue, path+"{}")
	in.maybeMap(dst, path)
	return dst
}

// isIntegerLiteral reports whether a JSON number's source text has no
// fractional part or exponent.
func isIntegerLiteral(s string) bool { return !strings.ContainsAny(s, ".eE") }

func sortedKeys(m map[string]*Prop) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
