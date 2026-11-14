package diff

import "sort"

const (
	kindInteger = "INTEGER"
	kindNumber  = "NUMBER"
	kindObject  = "OBJECT"
	kindMap     = "MAP"
	kindNull    = "NULL"
)

// KindChange is the result of comparing baseline kinds with live kinds.
type KindChange struct {
	TypeChanged  bool
	TypeSeverity Severity
	NullAdded    bool
}

func toSet(kinds []string) map[string]bool {
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

// ClassifyKinds applies blueprint section 9.4 to a pair of kind unions.
func ClassifyKinds(base, live []string) KindChange {
	b, l := toSet(base), toSet(live)
	res := KindChange{NullAdded: l[kindNull] && !b[kindNull]}
	delete(b, kindNull)
	delete(l, kindNull)

	var worst Severity
	for k := range l {
		if b[k] {
			continue
		}
		sev := SeverityBreaking
		switch {
		case k == kindInteger && b[kindNumber]:
			continue // NUMBER -> INTEGER is safe narrowing
		case k == kindNumber && b[kindInteger]:
			sev = SeverityWarning // INTEGER -> NUMBER widening
		case k == kindObject && b[kindMap], k == kindMap && b[kindObject]:
			sev = SeverityWarning
		}
		if sev.Rank() > worst.Rank() {
			worst = sev
		}
	}
	if worst != "" {
		res.TypeChanged = true
		res.TypeSeverity = worst
	}
	return res
}

// RemovedSeverity: a missing key only matters if the baseline said required.
func RemovedSeverity(baseRequired bool) (Severity, bool) {
	if baseRequired {
		return SeverityBreaking, true
	}
	return "", false
}

// OptionalitySeverity: required->optional is breaking, optional->required is safe.
func OptionalitySeverity(baseRequired, liveRequired bool) (Severity, bool) {
	switch {
	case baseRequired && !liveRequired:
		return SeverityBreaking, true
	case !baseRequired && liveRequired:
		return SeveritySafe, true
	}
	return "", false
}

// ExitCode returns 1 if any violation reaches the failOn level, else 0.
// Empty failOn means BREAKING. (Exit code 2 = tool error is handled by the CLI.)
func ExitCode(vs []Violation, failOn Severity) int {
	threshold := failOn.Rank()
	if threshold == 0 {
		threshold = SeverityBreaking.Rank()
	}
	for _, v := range vs {
		if v.Severity.Rank() >= threshold {
			return 1
		}
	}
	return 0
}

// SortViolations gives deterministic output order.
func SortViolations(vs []Violation) {
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].Endpoint != vs[j].Endpoint {
			return vs[i].Endpoint < vs[j].Endpoint
		}
		if vs[i].Path != vs[j].Path {
			return vs[i].Path < vs[j].Path
		}
		return vs[i].Type < vs[j].Type
	})
}
