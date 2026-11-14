package rename

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Case is one labeled (removed, added) pair.
type Case struct {
	Rename bool
	Old    Field
	New    Field
}

func splitPair(s string) (string, string) {
	if i := strings.Index(s, ">"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, s
}

func parsePos(s string) (int, int, error) {
	p := strings.SplitN(s, "/", 2)
	if len(p) != 2 {
		return 0, 0, fmt.Errorf("bad position %q", s)
	}
	i, err1 := strconv.Atoi(p[0])
	n, err2 := strconv.Atoi(p[1])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("bad position %q", s)
	}
	return i, n, nil
}

func kindList(s string) []string { return strings.Split(s, "+") }

func children(s string) []string {
	if s == "" || s == "-" {
		return nil
	}
	return strings.Split(s, ",")
}

// LoadDataset reads lines: label|old|new|kinds|pos|children|seen
// label T (rename) or F; kinds like STRING or STRING>INTEGER or STRING+NULL;
// pos like 2/6 or 2/6>3/7; children like a,b>a,b or -; seen like 1>0.5 (optional).
func LoadDataset(r io.Reader) ([]Case, error) {
	var out []Case
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "|")
		for len(f) < 7 {
			f = append(f, "")
		}
		if f[0] != "T" && f[0] != "F" {
			return nil, fmt.Errorf("line %d: label must be T or F", n)
		}
		c := Case{Rename: f[0] == "T", Old: Field{Name: f[1]}, New: Field{Name: f[2]}}
		k1, k2 := splitPair(f[3])
		c.Old.Kinds, c.New.Kinds = kindList(k1), kindList(k2)
		p1, p2 := splitPair(f[4])
		var err error
		if c.Old.Pos, c.Old.Siblings, err = parsePos(p1); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if c.New.Pos, c.New.Siblings, err = parsePos(p2); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		c1, c2 := splitPair(f[5])
		c.Old.Children, c.New.Children = children(c1), children(c2)
		s1, s2 := "1", "1"
		if f[6] != "" {
			s1, s2 = splitPair(f[6])
		}
		if c.Old.Seen, err = strconv.ParseFloat(s1, 64); err != nil {
			return nil, fmt.Errorf("line %d: bad seen", n)
		}
		if c.New.Seen, err = strconv.ParseFloat(s2, 64); err != nil {
			return nil, fmt.Errorf("line %d: bad seen", n)
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

// Metrics holds precision, recall and F1 at one threshold.
type Metrics struct{ P, R, F1 float64 }

// Evaluate classifies each case as rename when Score >= threshold.
func Evaluate(cases []Case, w Weights, threshold float64) Metrics {
	tp, fp, fn := 0, 0, 0
	for _, c := range cases {
		pred := Score(c.Old, c.New, w) >= threshold
		switch {
		case pred && c.Rename:
			tp++
		case pred && !c.Rename:
			fp++
		case !pred && c.Rename:
			fn++
		}
	}
	var m Metrics
	if tp+fp > 0 {
		m.P = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		m.R = float64(tp) / float64(tp+fn)
	}
	if m.P+m.R > 0 {
		m.F1 = 2 * m.P * m.R / (m.P + m.R)
	}
	return m
}

// TargetPrecision is the precision the threshold must reach (blueprint: P >= 0.9).
const TargetPrecision = 0.9

// BestThreshold picks the threshold with the highest recall whose precision is
// at least TargetPrecision on cases. If no threshold reaches it, it falls back
// to the best F1. Ties go to the higher threshold.
func BestThreshold(cases []Case, w Weights) float64 {
	bestT, bestR := -1.0, -1.0
	fbT, fbF := 0.0, -1.0
	for i := 5; i <= 99; i++ {
		t := float64(i) / 100
		m := Evaluate(cases, w, t)
		if m.P >= TargetPrecision && m.R >= bestR {
			bestT, bestR = t, m.R
		}
		if m.F1 >= fbF {
			fbT, fbF = t, m.F1
		}
	}
	if bestT >= 0 {
		return bestT
	}
	return fbT
}

// Split puts every 3rd case (index%3==2) in the held-out test set.
func Split(cases []Case) (train, test []Case) {
	for i, c := range cases {
		if i%3 == 2 {
			test = append(test, c)
		} else {
			train = append(train, c)
		}
	}
	return
}

// Config is one row of the ablation.
type Config struct {
	Name string
	W    Weights
}

// AblationConfigs adds one signal at a time, starting from name-only.
func AblationConfigs() []Config {
	d := DefaultWeights
	return []Config{
		{"name only (baseline)", Weights{Name: d.Name, NoTypeGate: true}},
		{"+ type gate", Weights{Name: d.Name}},
		{"+ structure", Weights{Name: d.Name, Structure: d.Structure}},
		{"+ position", Weights{Name: d.Name, Structure: d.Structure, Position: d.Position}},
		{"+ co-occurrence (full)", d},
	}
}

// Ablation tunes each config's threshold on the training split and reports
// precision/recall/F1 on the held-out split. Returns a markdown table.
func Ablation(cases []Case) string {
	train, test := Split(cases)
	var sb strings.Builder
	fmt.Fprintf(&sb, "cases: %d (train %d, held-out %d)\n\n", len(cases), len(train), len(test))
	sb.WriteString("| config | threshold | precision | recall | F1 |\n|---|---|---|---|---|\n")
	cfgs := AblationConfigs()
	for _, c := range cfgs {
		t := BestThreshold(train, c.W)
		m := Evaluate(test, c.W, t)
		fmt.Fprintf(&sb, "| %s | %.2f | %.2f | %.2f | %.2f |\n", c.Name, t, m.P, m.R, m.F1)
	}
	return sb.String()
}
