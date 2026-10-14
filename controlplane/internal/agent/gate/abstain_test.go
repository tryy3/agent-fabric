package gate

import (
	"context"
	"testing"
)

// A fast tier that asks only questions abstains when no tripwire fires. The
// deep tier must still be asked when the fast tier could not decide.
func TestAbstainingFastTierEscalatesOnlyWhenUndecided(t *testing.T) {
	for _, tc := range []struct {
		name     string
		answers  string
		req      Request
		wantDeep int
		wantRisk Risk
	}{
		{"undecided tripwire asks deep", `{"answers":{"q_destructive":{"noul":0.69},"q_credentials":{"noul":0.05},"q_remote_code_or_exfil":{"noul":0.1}}}`, runCommand, 1, 9},
		{"undecided at the low edge asks deep", `{"answers":{"q_destructive":{"noul":0.3},"q_credentials":{"noul":0.05},"q_remote_code_or_exfil":{"noul":0.1}}}`, runCommand, 1, 9},
		{"clean decided answers abstain", `{"answers":{"q_destructive":{"noul":0.05},"q_credentials":{"noul":0.29},"q_remote_code_or_exfil":{"noul":0.1}}}`, runCommand, 0, 6},
		{"undecided policy violation asks deep",
			`{"answers":{"q_destructive":{"noul":0.05},"q_credentials":{"noul":0.05},"q_remote_code_or_exfil":{"noul":0.1},"q_policy_violation":{"noul":0.5},"q_policy_exception":{"noul":0.05}}}`,
			withRules(runCommand, testRules), 1, 9},
		{"nothing to ask asks deep", `{}`, Request{ToolName: "ask_user", Args: []byte(`{}`)}, 1, 9},
	} {
		srv, _, calls := serveSystemOne(t, tc.answers)
		deep := scored("deep", 9, 0)
		c := Cascade{Rules: scored("rules", 6, 0), Fast: SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}, Deep: deep}
		d, err := c.Evaluate(context.Background(), tc.req)
		if err != nil {
			t.Fatal(err)
		}
		if deep.calls != tc.wantDeep || d.Risk != tc.wantRisk {
			t.Errorf("%s: deep calls %d, risk %d; want %d, %d (fast calls %d, %+v)", tc.name, deep.calls, d.Risk, tc.wantDeep, tc.wantRisk, *calls, d)
		}
	}
}
