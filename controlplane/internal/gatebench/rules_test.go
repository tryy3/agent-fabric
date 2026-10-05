package gatebench_test

import (
	"context"
	"testing"

	gatecases "github.com/tryy3/agent-fabric/bench/gate"
	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/gatebench"
)

func loadCases(t *testing.T) []gatebench.Case {
	t.Helper()
	cases, err := gatebench.LoadCases(gatecases.Cases, "cases")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 50 {
		t.Fatalf("only %d cases loaded", len(cases))
	}
	return cases
}

// TestRulesMatchDataset is the regression contract of the deterministic
// rules: every case's rules score equals its documented value (expect.score
// unless the case records a known rules gap), and the default modes resolve
// it to the matching decision.
func TestRulesMatchDataset(t *testing.T) {
	for _, c := range loadCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			d, err := gate.Rules{}.Evaluate(context.Background(), c.Request())
			if err != nil {
				t.Fatal(err)
			}
			got := gatebench.EffectiveScore(d)
			if c.Rules != nil {
				if got != c.Rules.Score && !(c.Rules.Score == 0 && got == gate.MaxRisk) {
					t.Fatalf("rules score = %d (%s), documented %d", got, d.RuleID, c.Rules.Score)
				}
				return
			}
			if diff := got - c.Expect.Score; diff < -c.Expect.Tol() || diff > c.Expect.Tol() {
				t.Fatalf("rules score = %d (%s), expected %d ±%d", got, d.RuleID, c.Expect.Score, c.Expect.Tol())
			}
			for _, m := range gate.Modes {
				if want, got := c.Decision(m), c.Policy(m).Resolve(d).Kind; want != got {
					t.Errorf("%s: decision = %s, want %s (score %d)", m, got, want, d.Risk)
				}
			}
		})
	}
}
