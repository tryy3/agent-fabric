package gate

import (
	"context"
	"encoding/json"
	"testing"
)

var webSearch = Request{
	ToolName: "web_search", Args: json.RawMessage(`{"query":"AccessDenied AKIAIOSFODNN7EXAMPLE s3"}`),
	ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker",
}

func TestWebRequestsAreConsultedByDefaultAndATripwireRaisesThem(t *testing.T) {
	srv, got, calls := serveSystemOne(t, `{"answers":{"q_data_leak":{"noul":0.95},"q_suspicious_host":{"noul":0.05},"q_planted_request":{"noul":0.05}}}`)
	c := Cascade{Fast: SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}, SkipAtOrBelow: DefaultSkipAtOrBelow}
	d, err := c.Evaluate(context.Background(), webSearch)
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || questionKeys(*got) != "q_data_leak,q_planted_request,q_suspicious_host" {
		t.Fatalf("web request not shown to the scorer: %d calls, questions %q", *calls, questionKeys(*got))
	}
	if d.Risk != 8 || d.Floor != 8 || d.RuleID != RuleWebRequest {
		t.Fatalf("data_leak yes must raise the web call to its floor 8: %+v", d)
	}
	if k := DefaultPolicies.PolicyFor(ModeAuto).Resolve(d).Kind; k != Ask {
		t.Errorf("auto mode = %s, want ask at 8", k)
	}
}

func TestWebTierConsultCanBeTurnedOff(t *testing.T) {
	fast := scored("fast", 9, 0.9)
	fast.asks = true
	off := false
	c := Cascade{Rules: Rules{Builtins: map[string]TierOverride{TierWeb: {Consult: &off}}}, Fast: fast, SkipAtOrBelow: DefaultSkipAtOrBelow}
	d, err := c.Evaluate(context.Background(), webSearch)
	if err != nil {
		t.Fatal(err)
	}
	if fast.calls != 0 || d.Risk != riskWebRequest {
		t.Fatalf("web.request consult off settles the call: fast calls %d, %+v", fast.calls, d)
	}
}

// Without a scorer a web call runs in every mode, tainted or not, exactly as
// it did when it scored 1 under rules.unknown_allow.
func TestWebRequestOutcomeWithoutScorerMatchesEveryMode(t *testing.T) {
	before := Decision{Kind: Allow, RuleID: "rules.unknown_allow", Risk: riskReadInProject}
	for _, tool := range []string{"web_search", "fetch_page"} {
		req := webSearch
		req.ToolName = tool
		d, err := Cascade{SkipAtOrBelow: DefaultSkipAtOrBelow}.Evaluate(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if d.RuleID != RuleWebRequest || d.Risk != riskWebRequest || !d.Consult || d.Settled {
			t.Fatalf("%s: %+v", tool, d)
		}
		for _, m := range Modes {
			for _, tainted := range []bool{false, true} {
				p := DefaultPolicies.PolicyFor(m)
				if tainted {
					p = p.Tainted()
				}
				was, now := p.Resolve(before), p.Resolve(d)
				if now.Kind != was.Kind || now.Kind != Allow || now.Band != BandSafe {
					t.Errorf("%s %s tainted=%v: %s (%s), was %s", tool, m, tainted, now.Kind, now.Band, was.Kind)
				}
			}
		}
	}
}

// With a scorer that asks gate questions; see TestSkipBypassNeedsAQuestionScorer
// for score and band scorers.
func TestExplicitConsultWinsOverSkipAtOrBelow(t *testing.T) {
	on := true
	read := Request{ToolName: "read_file", Args: json.RawMessage(`{"path":"customers.csv"}`), ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker"}
	for _, tc := range []struct {
		name      string
		overrides map[string]TierOverride
		req       Request
		skip      Risk
		wantCalls int
	}{
		{"file.read default is settled", nil, read, DefaultSkipAtOrBelow, 0},
		{"file.read consult reaches the scorer at skip 2", map[string]TierOverride{TierFileRead: {Consult: &on}}, read, DefaultSkipAtOrBelow, 1},
		{"file.write consult reaches the scorer at skip 2", map[string]TierOverride{TierFileWrite: {Consult: &on}},
			Request{ToolName: "write_file", Args: json.RawMessage(`{"path":"a.txt","content":"x"}`), ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker"},
			DefaultSkipAtOrBelow, 1},
		{"a default consult tier below a raised skip is still skipped", nil, runCommandIn("make"), 5, 0},
		{"an explicit consult on that tier wins over the raised skip", map[string]TierOverride{TierTooling: {Consult: &on}}, runCommandIn("make"), 5, 1},
	} {
		fast := scored("fast", 7, 0.9)
		fast.asks = true
		_, err := Cascade{Rules: Rules{Builtins: tc.overrides}, Fast: fast, SkipAtOrBelow: tc.skip}.Evaluate(context.Background(), tc.req)
		if err != nil {
			t.Fatal(err)
		}
		if fast.calls != tc.wantCalls {
			t.Errorf("%s: scorer calls %d, want %d", tc.name, fast.calls, tc.wantCalls)
		}
	}
}

func TestScoreRuleKeepsSkipAtOrBelow(t *testing.T) {
	fast := scored("fast", 7, 0.9)
	fast.asks = true
	on := true
	rules := Rules{
		User:     []UserRule{{Tool: "read_file", Match: "*.csv", Action: RuleScore, Risk: 2, Consult: true}},
		Builtins: map[string]TierOverride{TierFileRead: {Consult: &on}},
	}
	req := Request{ToolName: "read_file", Args: json.RawMessage(`{"path":"customers.csv"}`), ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker"}
	if _, err := (Cascade{Rules: rules, Fast: fast, SkipAtOrBelow: DefaultSkipAtOrBelow}).Evaluate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if fast.calls != 0 {
		t.Errorf("a score rule at or below skipAtOrBelow is skipped as before, scorer calls %d", fast.calls)
	}
}

func runCommandIn(prog string) Request {
	return Request{ToolName: "run_command", Args: json.RawMessage(`{"command":["` + prog + `"]}`), ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker"}
}
