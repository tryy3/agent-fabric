package gate

import (
	"context"
	"strings"
	"testing"
)

// tier is a scripted evaluator that counts its calls and records its request.
type tier struct {
	d     Decision
	calls int
	req   Request
}

func (t *tier) Evaluate(_ context.Context, req Request) (Decision, error) {
	t.calls++
	t.req = req
	return t.d, nil
}

func scored(source string, risk Risk, confidence float64) *tier {
	return &tier{d: Decision{Kind: Allow, Source: source, Risk: risk, Confidence: confidence, RuleID: source + ".score"}}
}

func TestCascade(t *testing.T) {
	failed := &tier{d: Decision{Kind: Allow, Source: "fast", Risk: DefaultFailRisk, RuleID: "systemone.fail_closed"}}
	cases := []struct {
		name             string
		rules, fast, dep *tier
		skip             Risk
		wantRisk         Risk
		wantSource       string
		wantFast, wantDp int
		wantScores       int
	}{
		{"confident fast raises, deep not asked", scored("rules", 4, 0), scored("fast", 9, 0.9), scored("deep", 2, 0), 0, 9, "fast", 1, 0, 2},
		{"confident fast agrees, rules stand", scored("rules", 4, 0), scored("fast", 3, 0.9), scored("deep", 9, 0), 0, 4, "rules", 1, 0, 2},
		{"unsure fast does not anchor the floor", scored("rules", 4, 0), scored("fast", 8, 0.2), scored("deep", 2, 0), 0, 2, "deep", 1, 1, 3},
		{"confident fast anchors the floor", scored("rules", 6, 0), scored("fast", 8, 0.9), scored("deep", 2, 0), 0, 8, "fast", 1, 0, 2},
		{"unsure fast escalates, deep raises", scored("rules", 4, 0), scored("fast", 4, 0.2), scored("deep", 9, 0), 0, 9, "deep", 1, 1, 3},
		{"fast far below rules escalates, deep lowers", scored("rules", 6, 0), scored("fast", 3, 0.95), scored("deep", 4, 0), 0, 4, "deep", 1, 1, 3},
		{"failed fast escalates", scored("rules", 2, 0), failed, scored("deep", 2, 0), 0, 2, "deep", 1, 1, 3},
		{"rules cancel band is final", scored("rules", 9, 0), scored("fast", 1, 0.9), scored("deep", 1, 0), 0, 9, "rules", 0, 0, 1},
		{"skipped below threshold", scored("rules", 1, 0), scored("fast", 9, 0.9), scored("deep", 9, 0), 2, 1, "rules", 0, 0, 1},
	}
	for _, tc := range cases {
		tc.fast.calls, tc.dep.calls = 0, 0
		d, err := Cascade{Rules: tc.rules, Fast: tc.fast, Deep: tc.dep, SkipAtOrBelow: tc.skip}.Evaluate(context.Background(), Request{ToolName: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if d.Risk != tc.wantRisk || d.Source != tc.wantSource || tc.fast.calls != tc.wantFast || tc.dep.calls != tc.wantDp || len(d.Scores) != tc.wantScores {
			t.Errorf("%s: risk=%d source=%s fast=%d deep=%d scores=%d", tc.name, d.Risk, d.Source, tc.fast.calls, tc.dep.calls, len(d.Scores))
		}
	}
}

func TestCascadeDeepSeesEarlierScoresAndTrailKeepsThem(t *testing.T) {
	deep := scored("llm:m", 3, 0)
	d, _ := Cascade{Rules: scored("rules", 6, 0), Fast: scored("systemone:jev", 3, 0.82), Deep: deep}.Evaluate(context.Background(), Request{ToolName: "x"})
	if len(deep.req.Prior) != 2 || deep.req.Prior[0].Risk != 6 {
		t.Fatalf("deep tier got prior %+v", deep.req.Prior)
	}
	if got, want := ScoreTrail(d.Scores), "rules 6 -> systemone:jev 3 (conf 0.82) -> llm:m 3"; got != want {
		t.Fatalf("trail = %q, want %q", got, want)
	}
}

func TestCascadeKeepsRulesDenyAndAskFields(t *testing.T) {
	fast := scored("fast", 1, 0.9)
	deny := &tier{d: Decision{Kind: Deny, RuleID: "rules.bad_args"}}
	if d, _ := (Cascade{Rules: deny, Fast: fast}).Evaluate(context.Background(), Request{}); d.Kind != Deny || fast.calls != 0 {
		t.Fatalf("deny must be final: %+v, fast calls %d", d, fast.calls)
	}
	ask := &tier{d: Decision{Kind: Ask, RuleID: "rules.path_ask", Path: "/tmp/x", Risk: 7, Source: "rules"}}
	d, _ := Cascade{Rules: ask, Fast: scored("fast", 2, 0.9), Deep: scored("deep", 2, 0)}.Evaluate(context.Background(), Request{})
	if d.Kind != Ask || d.Path != "/tmp/x" || d.Risk != 5 {
		t.Fatalf("lowered ask keeps the rule's verdict and path: %+v", d)
	}
}

func TestCascadeFailedDeepNeverLowers(t *testing.T) {
	deep := &tier{d: Decision{Kind: Allow, Source: "deep", Risk: 5, RuleID: "llm.fail_closed"}}
	d, _ := Cascade{Rules: scored("rules", 6, 0), Fast: scored("fast", 2, 0.9), Deep: deep}.Evaluate(context.Background(), Request{})
	if d.Risk != 6 || d.Source != "rules" {
		t.Fatalf("a failed deep tier must not lower: %+v", d)
	}
}

func TestCascadeMaxLower(t *testing.T) {
	for name, tc := range map[string]struct {
		maxLower int
		want     Risk
	}{
		"default lowers by two": {0, 5},
		"configured":            {4, 3},
		"more than the gap":     {9, 2},
		"negative never lowers": {-1, 7},
	} {
		d, _ := Cascade{Rules: scored("rules", 7, 0), Fast: scored("fast", 3, 0.9), Deep: scored("deep", 2, 0), MaxLower: tc.maxLower}.Evaluate(context.Background(), Request{})
		if d.Risk != tc.want || d.Source != "deep" || d.Scores[2].Risk != 2 {
			t.Errorf("%s: risk %d (%s), scores %+v; want %d", name, d.Risk, d.Rationale, d.Scores, tc.want)
		}
		if tc.want > 2 && !strings.Contains(d.Rationale, "scored 2, limited to") {
			t.Errorf("%s: rationale should explain the limit: %q", name, d.Rationale)
		}
	}
}

func TestCascadeTaintedSessionNeverLowers(t *testing.T) {
	deep := scored("deep", 2, 0)
	c := Cascade{Rules: scored("rules", 6, 0), Fast: scored("fast", 3, 0.9), Deep: deep, MaxLower: 4}
	d, _ := c.Evaluate(context.Background(), Request{Tainted: true})
	if d.Risk != 6 || !deep.req.Tainted {
		t.Fatalf("tainted: risk %d, deep saw tainted=%v", d.Risk, deep.req.Tainted)
	}
	if d, _ = c.Evaluate(context.Background(), Request{}); d.Risk != 2 {
		t.Fatalf("untainted: risk %d, want 2", d.Risk)
	}
}

func TestCascadePermissionRuleIsFinal(t *testing.T) {
	pinned := &tier{d: Decision{Kind: Ask, Risk: 4, Source: "rules", RuleID: RuleUserAsk, Pinned: true}}
	fast, deep := scored("fast", 9, 0.9), scored("deep", 1, 0)
	d, _ := Cascade{Rules: pinned, Fast: fast, Deep: deep}.Evaluate(context.Background(), Request{})
	if d.Risk != 4 || !d.Pinned || fast.calls != 0 || deep.calls != 0 {
		t.Fatalf("a permission rule's verdict must not reach the scorers: %+v fast=%d deep=%d", d, fast.calls, deep.calls)
	}
}
