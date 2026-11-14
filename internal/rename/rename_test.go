package rename

import (
	"os"
	"testing"
)

func loadCases(t *testing.T) []Case {
	t.Helper()
	f, err := os.Open("testdata/rename_labeled.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cases, err := LoadDataset(f)
	if err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestTokens(t *testing.T) {
	cases := map[string]string{"userName": "user name", "user_id": "user identifier", "HTTPStatus": "http status", "qty": "quantity"}
	for in, want := range cases {
		got := ""
		for i, tok := range Tokens(in) {
			if i > 0 {
				got += " "
			}
			got += tok
		}
		if got != want {
			t.Errorf("Tokens(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAblation prints the table (go test -run TestAblation -v) and enforces the P5 target.
func TestAblation(t *testing.T) {
	cases := loadCases(t)
	if len(cases) < 100 {
		t.Fatalf("need 100+ labeled cases, have %d", len(cases))
	}
	t.Log("\n" + Ablation(cases))
	train, test := Split(cases)
	cfgs := AblationConfigs()
	base, full := cfgs[0], cfgs[len(cfgs)-1]
	bm := Evaluate(test, base.W, BestThreshold(train, base.W))
	fm := Evaluate(test, full.W, BestThreshold(train, full.W))
	if fm.P < 0.9 || fm.R < 0.7 {
		t.Errorf("full model below target on held-out set: P=%.2f R=%.2f (need P>=0.90, R>=0.70)", fm.P, fm.R)
	}
	if fm.F1 <= bm.F1 {
		t.Errorf("full model F1 %.2f does not beat name-only baseline %.2f", fm.F1, bm.F1)
	}
}

func TestDetectPairsGreedily(t *testing.T) {
	removed := []Field{
		{Name: "user_name", Kinds: []string{"STRING"}, Pos: 1, Siblings: 6, Seen: 1},
		{Name: "phone", Kinds: []string{"STRING"}, Pos: 5, Siblings: 6, Seen: 1},
	}
	added := []Field{
		{Name: "username", Kinds: []string{"STRING"}, Pos: 1, Siblings: 6, Seen: 1},
		{Name: "is_verified", Kinds: []string{"BOOLEAN"}, Pos: 4, Siblings: 6, Seen: 1},
	}
	got := Detect(removed, added, 0.8, DefaultWeights)
	if len(got) != 1 || got[0].Old != "user_name" || got[0].New != "username" || len(got[0].Evidence) == 0 {
		t.Fatalf("got %+v", got)
	}
}
func TestDefaultThresholdMeetsTarget(t *testing.T) {
	m := Evaluate(loadCases(t), DefaultWeights, DefaultThreshold)
	if m.P < 0.9 || m.R < 0.7 {
		t.Fatalf("at DefaultThreshold %.2f: P=%.2f R=%.2f (need P>=0.90, R>=0.70)", DefaultThreshold, m.P, m.R)
	}
}

func TestSharedGenericSuffixIsNotARename(t *testing.T) {
	r := Field{Name: "role_id", Kinds: []string{"INTEGER"}, Seen: 1}
	a := Field{Name: "user_id", Kinds: []string{"INTEGER"}, Seen: 1}
	if s := Score(r, a, DefaultWeights); s >= DefaultThreshold {
		t.Fatalf("role_id -> user_id with unknown order scored %.2f, should stay below %.2f", s, DefaultThreshold)
	}
	r.Pos, r.Siblings, a.Pos, a.Siblings = 2, 6, 5, 7
	if s := Score(r, a, DefaultWeights); s >= DefaultThreshold {
		t.Fatalf("role_id -> user_id at different positions scored %.2f", s)
	}
}

func TestObviousRenameWorksWithUnknownOrder(t *testing.T) {
	r := Field{Name: "createdAt", Kinds: []string{"STRING"}, Seen: 1}
	a := Field{Name: "created_at", Kinds: []string{"STRING"}, Seen: 1}
	if s := Score(r, a, DefaultWeights); s < DefaultThreshold {
		t.Fatalf("createdAt -> created_at scored only %.2f", s)
	}
}

func TestSameStemDifferentKindIsNotARename(t *testing.T) {
	for _, p := range [][2]string{{"item_count", "item_id"}, {"comment_count", "comment_id"}, {"tag_count", "tag_id"}} {
		r := Field{Name: p[0], Kinds: []string{"INTEGER"}, Seen: 1}
		a := Field{Name: p[1], Kinds: []string{"INTEGER"}, Seen: 1}
		if s := Score(r, a, DefaultWeights); s >= DefaultThreshold {
			t.Errorf("%s -> %s with unknown order scored %.2f", p[0], p[1], s)
		}
	}
}
