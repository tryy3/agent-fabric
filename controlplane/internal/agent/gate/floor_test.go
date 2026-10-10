package gate

import (
	"context"
	"strings"
	"testing"
)

func floored(source string, risk, floor Risk, confidence float64) *tier {
	t := scored(source, risk, confidence)
	t.d.Floor = floor
	return t
}

func TestCascadeFloorSurvivesDeepLowering(t *testing.T) {
	d, _ := Cascade{Rules: scored("rules", 4, 0), Fast: floored("fast", 8, 8, 0.5), Deep: scored("deep", 2, 0)}.Evaluate(context.Background(), Request{})
	if d.Risk != 8 || d.Floor != 8 {
		t.Fatalf("risk %d floor %d (%s), want 8 and 8", d.Risk, d.Floor, d.Rationale)
	}
	if !strings.Contains(d.Rationale, "tripwire floor 8") {
		t.Errorf("rationale should explain the floor: %q", d.Rationale)
	}
	if len(d.Scores) != 3 || d.Scores[1].Floor != 8 || d.Scores[2].Risk != 2 {
		t.Errorf("trail keeps each tier's own score and the floor: %+v", d.Scores)
	}
	if got, want := ScoreTrail(d.Scores), "rules 4 -> fast 8 (conf 0.50) (floor 8) -> deep 2"; got != want {
		t.Errorf("trail = %q, want %q", got, want)
	}
}

func TestCascadeFloorBeatsMaxLower(t *testing.T) {
	control := scored("fast", 8, 0.5)
	d, _ := Cascade{Rules: scored("rules", 4, 0), Fast: control, Deep: scored("deep", 1, 0), MaxLower: 9}.Evaluate(context.Background(), Request{})
	if d.Risk != 1 {
		t.Fatalf("without a floor MaxLower 9 lets the deep tier reach 1, got %d", d.Risk)
	}
	d, _ = Cascade{Rules: scored("rules", 4, 0), Fast: floored("fast", 8, 8, 0.5), Deep: scored("deep", 1, 0), MaxLower: 9}.Evaluate(context.Background(), Request{})
	if d.Risk != 8 || d.Floor != 8 {
		t.Fatalf("with a floor the score stays at 8, got %d floor %d", d.Risk, d.Floor)
	}
}

func TestCascadeFloorInTaintedSession(t *testing.T) {
	deep := scored("deep", 1, 0)
	d, _ := Cascade{Rules: scored("rules", 4, 0), Fast: floored("fast", 8, 8, 0.5), Deep: deep, MaxLower: 4}.Evaluate(context.Background(), Request{Tainted: true})
	if d.Risk != 8 || !deep.req.Tainted {
		t.Fatalf("tainted: risk %d, deep saw tainted=%v", d.Risk, deep.req.Tainted)
	}
}

func TestCascadeFloorRaisesAScoreBelowIt(t *testing.T) {
	// A scorer that reports a floor above its own score still cannot leave the
	// call below the floor, with or without a deep tier.
	d, _ := Cascade{Rules: scored("rules", 4, 0), Fast: floored("fast", 5, 8, 0.9)}.Evaluate(context.Background(), Request{})
	if d.Risk != 8 || d.Floor != 8 {
		t.Fatalf("risk %d floor %d, want 8 and 8", d.Risk, d.Floor)
	}
}

func TestCascadeAbstainingScorerShowsInTrail(t *testing.T) {
	abstain := &tier{d: Decision{Kind: Allow, Source: "fast", RuleID: "systemone.score", Rationale: "p(yes) destructive 0.05"}}
	deep := scored("deep", 3, 0)
	d, _ := Cascade{Rules: scored("rules", 5, 0), Fast: abstain, Deep: deep}.Evaluate(context.Background(), Request{})
	if d.Risk != 5 || d.Floor != 0 || deep.calls != 0 {
		t.Fatalf("an abstaining scorer leaves the rules' score and asks nobody: %+v deep calls %d", d, deep.calls)
	}
	if len(d.Scores) != 2 || d.Scores[1].Source != "fast" || d.Scores[1].Risk != 0 {
		t.Fatalf("the abstaining scorer should be in the trail: %+v", d.Scores)
	}
	if got, want := ScoreTrail(d.Scores), "rules 5 -> fast no risk"; got != want {
		t.Errorf("trail = %q, want %q", got, want)
	}
}

func TestQuestionRegistry(t *testing.T) {
	var ids []string
	for _, q := range Tripwires("run_command") {
		ids = append(ids, q.ID)
		if q.Floor() != 8 || q.Kind != KindTripwire || q.Text == "" {
			t.Errorf("%+v", q)
		}
	}
	if strings.Join(ids, ",") != "destructive,credentials" {
		t.Fatalf("run_command tripwires = %v", ids)
	}
	for _, tool := range []string{"fetch_page", "web_search", "read_file", "write_file", "ask_user", ""} {
		if qs := QuestionsFor(tool); len(qs) != 0 {
			t.Errorf("%s has questions today: %+v", tool, qs)
		}
	}
	if got := (Question{}).Floor(); got != DefaultTripwireRisk {
		t.Errorf("default floor = %d", got)
	}
	QuestionsFor("run_command")[0].Enabled = false
	if len(Tripwires("run_command")) != 2 {
		t.Error("callers must not be able to edit the built-in set")
	}
}
