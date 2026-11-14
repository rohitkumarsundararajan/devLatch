package diff

import "testing"

func TestClassifyKinds(t *testing.T) {
	cases := []struct {
		name       string
		base, live []string
		changed    bool
		sev        Severity
		nullAdded  bool
	}{
		{"number to string", []string{"NUMBER"}, []string{"STRING"}, true, SeverityBreaking, false},
		{"integer to number", []string{"INTEGER"}, []string{"NUMBER"}, true, SeverityWarning, false},
		{"number to integer", []string{"NUMBER"}, []string{"INTEGER"}, false, "", false},
		{"string gains null", []string{"STRING"}, []string{"STRING", "NULL"}, false, "", true},
		{"already nullable", []string{"STRING", "NULL"}, []string{"NULL"}, false, "", false},
		{"object to map", []string{"OBJECT"}, []string{"MAP"}, true, SeverityWarning, false},
		{"map to object", []string{"MAP"}, []string{"OBJECT"}, true, SeverityWarning, false},
		{"union grows", []string{"STRING"}, []string{"STRING", "BOOLEAN"}, true, SeverityBreaking, false},
		{"same", []string{"STRING"}, []string{"STRING"}, false, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyKinds(c.base, c.live)
			if got.TypeChanged != c.changed || got.TypeSeverity != c.sev || got.NullAdded != c.nullAdded {
				t.Fatalf("got %+v, want changed=%v sev=%q null=%v", got, c.changed, c.sev, c.nullAdded)
			}
		})
	}
}

func TestOptionalityAndRemoved(t *testing.T) {
	if s, ok := RemovedSeverity(true); !ok || s != SeverityBreaking {
		t.Fatal("removed required key must be BREAKING")
	}
	if _, ok := RemovedSeverity(false); ok {
		t.Fatal("removed optional key must not be reported")
	}
	if s, ok := OptionalitySeverity(true, false); !ok || s != SeverityBreaking {
		t.Fatal("required->optional must be BREAKING")
	}
	if s, ok := OptionalitySeverity(false, true); !ok || s != SeveritySafe {
		t.Fatal("optional->required must be SAFE")
	}
	if _, ok := OptionalitySeverity(true, true); ok {
		t.Fatal("no change expected")
	}
}

func TestExitCode(t *testing.T) {
	warn := []Violation{{Severity: SeverityWarning}}
	brk := []Violation{{Severity: SeverityBreaking}}
	safe := []Violation{{Severity: SeveritySafe}}
	if ExitCode(warn, "") != 0 || ExitCode(safe, "") != 0 {
		t.Fatal("warning/safe must pass by default")
	}
	if ExitCode(brk, "") != 1 {
		t.Fatal("breaking must fail")
	}
	if ExitCode(warn, SeverityWarning) != 1 {
		t.Fatal("--fail-on warning must fail on warning")
	}
}
