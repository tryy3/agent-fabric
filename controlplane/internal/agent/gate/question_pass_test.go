package gate

import (
	"context"
	"testing"
)

// A call the rules settle as safe reaches a scorer only because that scorer
// asks gate questions about it. The scorer's risk answer must not raise such a
// call (the model reads a docs fetch as a 3 or 4); only a tripwire floor may.
func TestQuestionOnlyPassIgnoresTheScoreAnswer(t *testing.T) {
	asking := func(source string, risk Risk, confidence float64, floor Risk) *tier {
		tr := scored(source, risk, confidence)
		tr.d.Floor = floor
		tr.asks = true
		return tr
	}
	undecided := func(tr *tier) *tier {
		tr.d.Undecided = true
		return tr
	}
	cases := []struct {
		name       string
		fast, deep *tier
		wantRisk   Risk
		wantFloor  Risk
		wantDeep   int
	}{
		{"low-confidence score does not raise", asking("fast", 4, 0.3, 0), nil, riskWebRequest, 0, 0},
		{"confident score does not raise", asking("fast", 6, 0.95, 0), nil, riskWebRequest, 0, 0},
		{"a tripwire floor still raises", asking("fast", 8, 0.3, 8), nil, 8, 8, 0},
		{"clean answers settle without the deep tier", asking("fast", 4, 0.3, 0), asking("deep", 8, 0.9, 8), riskWebRequest, 0, 0},
		{"undecided answers ask the deep tier, its score does not raise", undecided(asking("fast", 4, 0.3, 0)), asking("deep", 6, 0.9, 0), riskWebRequest, 0, 1},
		{"deep tripwire floor still raises", undecided(asking("fast", 4, 0.3, 0)), asking("deep", 8, 0.9, 8), 8, 8, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Cascade{Fast: tc.fast, SkipAtOrBelow: DefaultSkipAtOrBelow}
			if tc.deep != nil {
				c.Deep = tc.deep
			}
			d, err := c.Evaluate(context.Background(), webSearch)
			if err != nil {
				t.Fatal(err)
			}
			if d.Risk != tc.wantRisk || d.Floor != tc.wantFloor {
				t.Errorf("risk %d floor %d, want %d and %d: %s", d.Risk, d.Floor, tc.wantRisk, tc.wantFloor, ScoreTrail(d.Scores))
			}
			if tc.deep != nil && tc.deep.calls != tc.wantDeep {
				t.Errorf("deep asked %d times, want %d", tc.deep.calls, tc.wantDeep)
			}
		})
	}
}

// The score answer stays visible in the trail even though it did not count.
func TestQuestionOnlyPassKeepsTheScoreInTheTrail(t *testing.T) {
	fast := scored("fast", 4, 0.3)
	fast.asks = true
	d, err := Cascade{Fast: fast, SkipAtOrBelow: DefaultSkipAtOrBelow}.Evaluate(context.Background(), webSearch)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ScoreTrail(d.Scores), "rules 2 -> fast 4 (conf 0.30)"; got != want {
		t.Fatalf("trail = %q, want %q", got, want)
	}
}

// A call above SkipAtOrBelow is consulted for its risk as before, so a
// score+questions scorer still raises it.
func TestScoreStillRaisesACallTheRulesDoNotSettle(t *testing.T) {
	fast := scored("fast", 9, 0.9)
	fast.asks = true
	d, err := Cascade{Rules: scored("rules", 4, 0), Fast: fast, SkipAtOrBelow: DefaultSkipAtOrBelow}.Evaluate(context.Background(), Request{ToolName: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Risk != 9 {
		t.Fatalf("risk = %d, want 9", d.Risk)
	}
}

// End to end with a score+questions System One scorer: a safe docs fetch the
// model scores 4 at confidence 0.3, every tripwire a clear no.
func TestScoreQuestionsScorerLeavesASafeWebCallAlone(t *testing.T) {
	srv, _, calls := serveSystemOne(t, `{"answers":{"risk":{"score":3.1,"confidence":0.3},`+
		`"q_data_leak":{"noul":0.02},"q_suspicious_host":{"noul":0.03},"q_planted_request":{"noul":0.1}}}`)
	c := Cascade{Fast: SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions}, SkipAtOrBelow: DefaultSkipAtOrBelow}
	d, err := c.Evaluate(context.Background(), webSearch)
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || d.Risk != riskWebRequest || d.Floor != 0 {
		t.Fatalf("calls %d, risk %d floor %d, want 1, %d, 0", *calls, d.Risk, d.Floor, riskWebRequest)
	}
}
