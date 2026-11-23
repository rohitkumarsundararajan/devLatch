package diff

import (
	"sort"

	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
	"github.com/rohitkumarsundararajan/devlatch/internal/rename"
)

const (
	kObject = infer.Kind("OBJECT")
	kArray  = infer.Kind("ARRAY")
	kMap    = infer.Kind("MAP")
)

// Options tunes the diff. MinSamples is the minimum baseline samples before
// a field may be treated as required (default 20).
type Options struct {
	MinSamples int
	// Renames turns on POSSIBLE_RENAME detection: a removed key plus a likely
	// replacement key becomes one violation instead of REMOVED_KEY + ADDED_KEY.
	Renames bool
}

func (o Options) minSamples() int {
	if o.MinSamples <= 0 {
		return 20
	}
	return o.MinSamples
}

// Diff compares every endpoint present in both baseline and live.
func Diff(base, live map[string]*infer.Node, opt Options) []Violation {
	var out []Violation
	for ep, b := range base {
		l, ok := live[ep]
		if !ok || b == nil || l == nil || l.Samples == 0 {
			continue
		}
		out = append(out, DiffEndpoint(ep, b, l, opt)...)
	}
	SortViolations(out)
	return out
}

// DiffEndpoint compares one endpoint's baseline schema with the live schema.
func DiffEndpoint(endpoint string, base, live *infer.Node, opt Options) []Violation {
	d := &differ{endpoint: endpoint, opt: opt}
	d.node("$", base, live, false)
	SortViolations(d.out)
	return d.out
}

type differ struct {
	endpoint string
	opt      Options
	out      []Violation
}

func (d *differ) add(v Violation) {
	v.Endpoint = d.endpoint
	d.out = append(d.out, v)
}

func kindNames(n *infer.Node) []string {
	if n == nil {
		return nil
	}
	var ks []string
	for k, c := range n.Kinds {
		if c > 0 {
			ks = append(ks, string(k))
		}
	}
	sort.Strings(ks)
	return ks
}

func has(n *infer.Node, k infer.Kind) bool {
	return n != nil && n.Kinds[k] > 0
}

func objSamples(n *infer.Node) int {
	if c := n.Kinds[kObject]; c > 0 {
		return c
	}
	return n.Samples
}

func (d *differ) node(path string, b, l *infer.Node, elemRoot bool) {
	if b == nil || l == nil {
		return
	}
	bk, lk := kindNames(b), kindNames(l)
	kc := ClassifyKinds(bk, lk)
	if kc.TypeChanged {
		t := TypeChanged
		if elemRoot {
			t = ArrayElementChanged
		}
		d.add(Violation{Path: path, Type: t, Severity: kc.TypeSeverity, Expected: bk, Actual: lk})
	}
	if kc.NullAdded {
		d.add(Violation{Path: path, Type: NullabilityChanged, Severity: SeverityWarning, Expected: bk, Actual: lk})
	}
	if has(b, kObject) && has(l, kObject) {
		d.object(path, b, l)
	}
	if has(b, kArray) && has(l, kArray) {
		d.node(path+"[]", b.Element, l.Element, true)
	}
	if has(b, kMap) && has(l, kMap) {
		d.node(path+"{*}", b.MapValue, l.MapValue, false)
	}
}

func (d *differ) object(path string, b, l *infer.Node) {
	bs, ls := objSamples(b), objSamples(l)
	enforced := bs >= d.opt.minSamples()

	seen := map[string]bool{}
	var keys []string
	for k := range b.Properties {
		seen[k] = true
		keys = append(keys, k)
	}
	for k := range l.Properties {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	renamedOld := map[string]rename.Match{}
	renamedNew := map[string]bool{}
	if d.opt.Renames {
		for _, m := range d.renames(b, l, bs, ls, enforced) {
			renamedOld[m.Old] = m
			renamedNew[m.New] = true
		}
	}

	for _, k := range keys {
		bp, lp := b.Properties[k], l.Properties[k]
		p := path + "." + k
		switch {
		case bp != nil && lp == nil:
			req := enforced && bp.SeenIn == bs
			sev, ok := RemovedSeverity(req)
			if m, isRename := renamedOld[k]; isRename {
				d.add(Violation{
					Path: p, Type: PossibleRename, Severity: sev,
					Expected: kindNames(bp.Node), Actual: kindNames(l.Properties[m.New].Node),
					RenameFrom: p, RenameTo: path + "." + m.New,
					Confidence: m.Confidence, Evidence: m.Evidence,
				})
			} else if ok {
				d.add(Violation{Path: p, Type: RemovedKey, Severity: sev, Expected: kindNames(bp.Node)})
			}
		case bp == nil && lp != nil:
			if !renamedNew[k] {
				d.add(Violation{Path: p, Type: AddedKey, Severity: SeveritySafe, Actual: kindNames(lp.Node)})
			}
		case bp != nil && lp != nil:
			if enforced {
				bReq := bp.SeenIn == bs
				lReq := lp.SeenIn == ls
				if sev, ok := OptionalitySeverity(bReq, lReq); ok {
					d.add(Violation{Path: p, Type: OptionalityChanged, Severity: sev})
				}
			}
			d.node(p, bp.Node, lp.Node, false)
		}
	}
}
