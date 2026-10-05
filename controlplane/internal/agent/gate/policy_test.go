package gate

import (
	"context"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"slices"
	"testing"
)

func TestBandOf(t *testing.T) {
	want := map[Risk]Band{0: "", 1: BandSafe, 2: BandSafe, 3: BandLow, 4: BandLow, 5: BandElevated,
		6: BandElevated, 7: BandHigh, 8: BandHigh, 9: BandCancel, 10: BandCancel}
	for r, b := range want {
		if got := BandOf(r); got != b {
			t.Errorf("BandOf(%d) = %q, want %q", r, got, b)
		}
	}
}

func TestPolicyResolve(t *testing.T) {
	cases := []struct {
		mode Mode
		in   Decision
		want DecisionKind
	}{
		{ModeAsk, Decision{Kind: Allow, Risk: 1}, Allow},
		{ModeAsk, Decision{Kind: Allow, Risk: 2}, Allow},
		{ModeAsk, Decision{Kind: Ask, Risk: 4}, Ask},
		{ModeAsk, Decision{Kind: Allow, Risk: 5}, Ask}, // scorer raised a rule's allow
		{ModeAsk, Decision{Kind: Allow, Risk: 9}, Deny},
		{ModeAutoApprove, Decision{Kind: Ask, Risk: 4}, Allow},
		{ModeAutoApprove, Decision{Kind: Ask, Risk: 6}, Ask},
		{ModeAuto, Decision{Kind: Ask, Risk: 6}, Allow},
		{ModeAuto, Decision{Kind: Ask, Risk: 7}, Ask},
		{ModeAuto, Decision{Kind: Allow, Risk: 9}, Deny},
		{ModeFull, Decision{Kind: Ask, Risk: 8}, Allow},
		{ModeFull, Decision{Kind: Allow, Risk: 9}, Allow},
		{ModeFull, Decision{Kind: Allow, Risk: 10}, Deny},
		// A Deny is never relaxed; an unscored decision is untouched.
		{ModeFull, Decision{Kind: Deny, Risk: 3}, Deny},
		{ModeFull, Decision{Kind: Deny}, Deny},
		{ModeAuto, Decision{Kind: Ask}, Ask},
	}
	for _, tc := range cases {
		got := DefaultPolicies.PolicyFor(tc.mode).Resolve(tc.in)
		if got.Kind != tc.want {
			t.Errorf("%s %+v = %s, want %s", tc.mode, tc.in, got.Kind, tc.want)
		}
	}
}

func TestPolicyResolveMarksOverrideAndWithholdsGrant(t *testing.T) {
	got := DefaultPolicies.PolicyFor(ModeAuto).Resolve(Decision{Kind: Ask, Risk: 4, Path: "/x"})
	if !got.Overridden || got.Kind != Allow {
		t.Fatalf("%+v", got)
	}
	got = DefaultPolicies.PolicyFor(ModeAsk).Resolve(Decision{Kind: Allow, Risk: 6, Rationale: "wipes tree"})
	if got.Kind != Ask || !got.NoSessionGrant || got.Band != BandElevated {
		t.Fatalf("%+v", got)
	}
}

func TestParseMode(t *testing.T) {
	if m, err := ParseMode(""); err != nil || m != ModeAsk {
		t.Fatalf("empty = %q, %v", m, err)
	}
	if _, err := ParseMode("yolo"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

type fixedEval struct{ d Decision }

func (f fixedEval) Evaluate(context.Context, Request) (Decision, error) { return f.d, nil }

func TestChainReportsHighestRisk(t *testing.T) {
	c := Chain{Evaluators: []Evaluator{
		fixedEval{Decision{Kind: Ask, Risk: 4, Source: "rules", Reason: "rule"}},
		fixedEval{Decision{Kind: Allow, Risk: 8, Source: "llm", Rationale: "looks like exfiltration"}},
	}}
	d, err := c.Evaluate(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Ask || d.Risk != 8 || d.Source != "llm" || d.Rationale == "" || d.Reason != "rule" {
		t.Fatalf("%+v", d)
	}
}

func TestRulesScoreEveryDecision(t *testing.T) {
	d := evalCommand(t, []string{"ls"}, "", true)
	if d.Risk != 1 || d.Source != "rules" {
		t.Fatalf("%+v", d)
	}
	d = evalCommand(t, []string{"rm", "x"}, "", true)
	if d.Risk != 6 {
		t.Fatalf("rm risk = %d", d.Risk)
	}
	d = evalCommand(t, []string{"rm", "-rf", "x"}, "", true)
	if d.Risk != 7 {
		t.Fatalf("rm -rf risk = %d", d.Risk)
	}
	d = evalCommand(t, []string{"sudo", "ls"}, "", true)
	if d.Kind != Deny || d.Risk != 10 {
		t.Fatalf("sudo = %+v", d)
	}
	d = evalCommand(t, []string{"ls"}, "", false)
	if d.Kind != Deny || d.Risk != 0 {
		t.Fatalf("local env deny must stay unscored: %+v", d)
	}
}

func TestModesMatchCatalog(t *testing.T) {
	var got []string
	for _, m := range Modes {
		got = append(got, string(m))
	}
	if !slices.Equal(got, catalog.PermissionModes) {
		t.Fatalf("catalog.PermissionModes = %v, gate.Modes = %v", catalog.PermissionModes, got)
	}
}

func TestRuleListsMatchCatalog(t *testing.T) {
	if !slices.Equal(UserRuleActions, catalog.PermissionRuleActions) {
		t.Fatalf("catalog.PermissionRuleActions = %v, gate.UserRuleActions = %v", catalog.PermissionRuleActions, UserRuleActions)
	}
	if want := []string{StrategyScore, StrategyQuestions, StrategyBands}; !slices.Equal(want, catalog.PermissionScorerStrategies) {
		t.Fatalf("catalog.PermissionScorerStrategies = %v, gate strategies = %v", catalog.PermissionScorerStrategies, want)
	}
}

func TestPolicyTainted(t *testing.T) {
	for mode, want := range map[Mode]Risk{ModeAsk: 3, ModeAutoApprove: 5, ModeAuto: 5, ModeFull: NeverAsk} {
		if got := DefaultPolicies.PolicyFor(mode).Tainted().AskAt; got != want {
			t.Errorf("%s: tainted AskAt = %d, want %d", mode, got, want)
		}
	}
	d := Decision{Kind: Allow, Risk: 6}
	if DefaultPolicies.PolicyFor(ModeAuto).Resolve(d).Kind != Allow || DefaultPolicies.PolicyFor(ModeAuto).Tainted().Resolve(d).Kind != Ask {
		t.Fatal("a tainted session asks from 5 in Run automatically")
	}
}
