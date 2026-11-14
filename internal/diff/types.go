package diff

// Severity ranks how dangerous a violation is.
type Severity string

const (
	SeverityBreaking Severity = "BREAKING"
	SeverityWarning  Severity = "WARNING"
	SeveritySafe     Severity = "SAFE"
)

// Rank gives an orderable number: BREAKING > WARNING > SAFE > unknown.
func (s Severity) Rank() int {
	switch s {
	case SeverityBreaking:
		return 3
	case SeverityWarning:
		return 2
	case SeveritySafe:
		return 1
	}
	return 0
}

type ViolationType string

const (
	RemovedKey          ViolationType = "REMOVED_KEY"
	AddedKey            ViolationType = "ADDED_KEY"
	TypeChanged         ViolationType = "TYPE_CHANGED"
	NullabilityChanged  ViolationType = "NULLABILITY_CHANGED"
	OptionalityChanged  ViolationType = "OPTIONALITY_CHANGED"
	ArrayElementChanged ViolationType = "ARRAY_ELEMENT_CHANGED"
	PossibleRename      ViolationType = "POSSIBLE_RENAME"
)

// Violation is one difference between the approved contract and live traffic.
// Expected/Actual hold kind names (STRING, INTEGER, ...) as plain strings so
// this package does not depend on the inference package.
type Violation struct {
	Endpoint   string        `json:"endpoint"`
	Path       string        `json:"path"`
	Type       ViolationType `json:"type"`
	Severity   Severity      `json:"severity"`
	Expected   []string      `json:"expected,omitempty"`
	Actual     []string      `json:"actual,omitempty"`
	RenameFrom string        `json:"rename_from,omitempty"`
	RenameTo   string        `json:"rename_to,omitempty"`
	Confidence float64       `json:"confidence,omitempty"`
	Evidence   []string      `json:"evidence,omitempty"`
}
