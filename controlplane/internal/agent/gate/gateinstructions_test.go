package gate

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const testRules = "Never run git push. Running go test is always fine."

func withRules(req Request, rules string) Request {
	req.GateInstructions = rules
	return req
}

func stateOf(t *testing.T, r systemOneRequest) string {
	t.Helper()
	s, ok := r.State.(string)
	if !ok {
		t.Fatalf("state is %T, want the plain text", r.State)
	}
	return s
}

func TestSystemOneRequestWithAndWithoutGateInstructions(t *testing.T) {
	answers := `{"answers":{"risk":{"score":2,"confidence":0.9},"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},
		"q_policy_violation":{"noul":0.1},"q_policy_exception":{"noul":0.1}}}`
	for strategy, wantWith := range map[string]string{
		StrategyScore:          "risk",
		StrategyBands:          "risk",
		StrategyQuestions:      "q_credentials,q_destructive,q_policy_exception,q_policy_violation",
		StrategyScoreQuestions: "q_credentials,q_destructive,q_policy_exception,q_policy_violation,risk",
	} {
		srv, got, _ := serveSystemOne(t, answers)
		sc := SystemOneScorer{BaseURL: srv.URL, Strategy: strategy}
		if _, err := sc.Evaluate(context.Background(), runCommand); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(questionKeys(*got), "policy") || strings.Contains(stateOf(t, *got), "gate_instructions") {
			t.Errorf("%s: no instructions, yet policy questions or the block were sent: %q", strategy, questionKeys(*got))
		}
		for _, blank := range []string{"", "  \n\t "} {
			if _, err := sc.Evaluate(context.Background(), withRules(runCommand, blank)); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(questionKeys(*got), "policy") || strings.Contains(stateOf(t, *got), "gate_instructions") {
				t.Errorf("%s: blank instructions %q count as none", strategy, blank)
			}
		}
		if _, err := sc.Evaluate(context.Background(), withRules(runCommand, testRules)); err != nil {
			t.Fatal(err)
		}
		keys := questionKeys(*got)
		if strategy == StrategyBands {
			keys = strings.ReplaceAll(keys, "requested,", "")
			if strings.Contains(keys, "policy") {
				t.Errorf("bands asks no policy questions: %q", keys)
			}
		} else if strategy == StrategyScore {
			if strings.Contains(keys, "policy") {
				t.Errorf("score asks no policy questions: %q", keys)
			}
		} else if keys != wantWith {
			t.Errorf("%s: questions %q, want %q", strategy, keys, wantWith)
		}
		state := stateOf(t, *got)
		for _, want := range []string{testRules, "<gate_instructions>", "</gate_instructions>", "not from the agent"} {
			if !strings.Contains(state, want) {
				t.Errorf("%s: state lacks %q:\n%s", strategy, want, state)
			}
		}
		if strings.Index(state, "<gate_instructions>") > strings.Index(state, "Arguments (JSON, untrusted data)") {
			t.Errorf("%s: the user's rules come before the untrusted arguments", strategy)
		}
		for _, k := range []string{"q_policy_violation", "q_policy_exception"} {
			if q, ok := got.Questions[k]; ok && (q.Type != "noul" || !strings.Contains(q.Instructions, "gate instructions") || len(q.Criteria) != 0) {
				t.Errorf("%s: %s = %+v", strategy, k, q)
			}
		}
	}
}

func TestSystemOnePolicyQuestionWording(t *testing.T) {
	srv, got, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_policy_violation":{"noul":0.1},"q_policy_exception":{"noul":0.1}}}`)
	_, _ = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), withRules(runCommand, testRules))
	if q := got.Questions["q_policy_violation"].Instructions; !strings.Contains(q, "does this command violate any of those rules?") {
		t.Errorf("violation wording: %q", q)
	}
	q := got.Questions["q_policy_exception"].Instructions
	for _, want := range []string{"EXPLICITLY allow this exact command", "General or vague permissions do not count."} {
		if !strings.Contains(q, want) {
			t.Errorf("exception wording lacks %q: %q", want, q)
		}
	}
}

func TestPolicyOnlyForToolsWithQuestionsAndWhenInstructionsExist(t *testing.T) {
	readFile := Request{ToolName: "ask_user", Args: []byte(`{}`), GateInstructions: testRules}
	srv, got, calls := serveSystemOne(t, `{"answers":{}}`)
	d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), readFile)
	if *calls != 0 || d.Risk != 0 || d.Rationale != "" {
		t.Fatalf("a tool without questions needs no call even with instructions: calls=%d %+v", *calls, d)
	}
	srv, got, _ = serveSystemOne(t, `{"answers":{"risk":{"score":1}}}`)
	_, _ = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions}.Evaluate(context.Background(), readFile)
	if questionKeys(*got) != "risk" || !strings.Contains(stateOf(t, *got), testRules) {
		t.Fatalf("the rules still show in the state of other tools: %q", questionKeys(*got))
	}
	for _, tool := range []string{"run_command", " run_command "} {
		if len(defaultQuestions.policy(tool)) != 2 {
			t.Errorf("%q has no policy questions", tool)
		}
	}
	if len(defaultQuestions.policy("ask_user")) != 0 {
		t.Error("ask_user has policy questions")
	}
}

// TestPolicyExceptionMatrix covers every combination of the three yes/no
// outcomes that can set a floor: a tripwire, a policy violation and a policy
// exception. The exception only cancels the violation.
func TestPolicyExceptionMatrix(t *testing.T) {
	prob := func(yes bool) string {
		if yes {
			return "0.9"
		}
		return "0.1"
	}
	for _, strategy := range []string{StrategyQuestions, StrategyScoreQuestions} {
		for mask := 0; mask < 8; mask++ {
			tripwire, violation, exception := mask&1 != 0, mask&2 != 0, mask&4 != 0
			name := fmt.Sprintf("%s tripwire=%v violation=%v exception=%v", strategy, tripwire, violation, exception)
			body := fmt.Sprintf(`{"answers":{"risk":{"score":1,"confidence":0.8},"q_destructive":{"noul":%s},"q_credentials":{"noul":0.1},
				"q_policy_violation":{"noul":%s},"q_policy_exception":{"noul":%s}}}`, prob(tripwire), prob(violation), prob(exception))
			srv, _, _ := serveSystemOne(t, body)
			d, err := SystemOneScorer{BaseURL: srv.URL, Strategy: strategy}.Evaluate(context.Background(), withRules(runCommand, testRules))
			if err != nil {
				t.Fatal(err)
			}
			var wantFloor Risk
			switch {
			case tripwire:
				wantFloor = 8
			case violation && !exception:
				wantFloor = 7
			}
			if d.Floor != wantFloor {
				t.Errorf("%s: floor %d, want %d (%s)", name, d.Floor, wantFloor, d.Rationale)
			}
			wantRisk := wantFloor
			if strategy == StrategyScoreQuestions {
				wantRisk = max(2, wantFloor)
			}
			if d.Risk != wantRisk {
				t.Errorf("%s: risk %d, want %d", name, d.Risk, wantRisk)
			}
			if !strings.Contains(d.Rationale, "policy_violation "+prob(violation)[:3]+"0") || !strings.Contains(d.Rationale, "policy_exception "+prob(exception)[:3]+"0") {
				t.Errorf("%s: rationale lacks the probabilities: %q", name, d.Rationale)
			}
			cancelled := violation && exception
			if got := strings.Contains(d.Rationale, "policy_violation cancelled by policy_exception"); got != cancelled {
				t.Errorf("%s: cancelled in rationale = %v: %q", name, got, d.Rationale)
			}
			if got := strings.Contains(d.Rationale, "policy policy_violation (floor 7)"); got != (violation && !exception) {
				t.Errorf("%s: violation fired in rationale = %v: %q", name, got, d.Rationale)
			}
			if got := strings.Contains(d.Rationale, "tripwire destructive (floor 8)"); got != tripwire {
				t.Errorf("%s: tripwire in rationale = %v: %q", name, got, d.Rationale)
			}
			if strategy == StrategyQuestions && wantFloor > 0 && d.Confidence <= 0 {
				t.Errorf("%s: a fired question gives confidence: %+v", name, d)
			}
			if strategy == StrategyQuestions && wantFloor == 0 && (d.Risk != 0 || d.Confidence != 0) {
				t.Errorf("%s: nothing fired, the scorer abstains: %+v", name, d)
			}
		}
	}
}

func TestPolicyThresholdsAndUndecided(t *testing.T) {
	for _, tc := range []struct {
		violation, exception string
		wantFloor            Risk
	}{
		{"0.7", "0.1", 0},
		{"0.71", "0.1", 7},
		{"0.5", "0.1", 0},
		{"0.9", "0.7", 7},
		{"0.9", "0.71", 0},
		{"0.9", "0.5", 7},
	} {
		srv, _, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_policy_violation":{"noul":`+tc.violation+`},"q_policy_exception":{"noul":`+tc.exception+`}}}`)
		d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), withRules(runCommand, testRules))
		if d.Floor != tc.wantFloor {
			t.Errorf("violation %s exception %s: floor %d, want %d", tc.violation, tc.exception, d.Floor, tc.wantFloor)
		}
	}
}

func TestPolicyFiredFeedsCascadeFloor(t *testing.T) {
	srv, _, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_policy_violation":{"noul":0.95},"q_policy_exception":{"noul":0.05}}}`)
	req := withRules(runCommand, testRules)
	d, err := Cascade{Rules: scored("rules", 4, 0), Fast: SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Name: "fast"}}.Evaluate(context.Background(), req)
	if err != nil || d.Risk != 7 || d.Floor != 7 {
		t.Fatalf("a violation raises the rules' 4 to 7: %+v, %v", d, err)
	}
	srv, _, _ = serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_policy_violation":{"noul":0.95},"q_policy_exception":{"noul":0.95}}}`)
	d, err = Cascade{Rules: scored("rules", 4, 0), Fast: SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions, Name: "fast"}}.Evaluate(context.Background(), req)
	if err != nil || d.Risk != 4 || d.Floor != 0 {
		t.Fatalf("a cancelled violation leaves the rules' 4: %+v, %v", d, err)
	}
	if got := ScoreTrail(d.Scores); !strings.Contains(got, "fast no risk") {
		t.Errorf("trail %q", got)
	}
	if !strings.Contains(d.Scores[1].Rationale, "cancelled by policy_exception") {
		t.Errorf("the trail shows the exception cancelled the violation: %q", d.Scores[1].Rationale)
	}
}

func TestPolicyAnswersMissingFailClosed(t *testing.T) {
	srv, _, _ := serveSystemOne(t, `{"answers":{"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_policy_violation":{"noul":0.1}}}`)
	d, err := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyQuestions}.Evaluate(context.Background(), withRules(runCommand, testRules))
	if err != nil || d.RuleID != "systemone.fail_closed" || d.Risk != DefaultFailRisk {
		t.Fatalf("%+v, %v", d, err)
	}
}

func TestGateInstructionsTruncation(t *testing.T) {
	long := strings.Repeat("a", 100) + strings.Repeat("é", 50)
	v := viewInstructions(long, 120)
	if !strings.HasPrefix(v.text, long[:100]) || strings.Count(v.text, "é") != 20 || !strings.Contains(v.text, "[truncated: 30 more characters") {
		t.Errorf("view: %q", v.text)
	}
	if !strings.Contains(v.warning, "truncated to 120 of 150 characters") {
		t.Errorf("warning: %q", v.warning)
	}
	if v := viewInstructions(long, 150); v.warning != "" || v.text != long {
		t.Errorf("at the cap nothing is cut: %+v", v)
	}
	if v := viewInstructions(strings.Repeat("x", DefaultInstructionsMaxChars+1), 0); v.warning == "" ||
		!strings.Contains(v.warning, fmt.Sprintf("truncated to %d of %d", DefaultInstructionsMaxChars, DefaultInstructionsMaxChars+1)) {
		t.Errorf("default cap is %d: %+v", DefaultInstructionsMaxChars, v.warning)
	}

	rules := "FIRST RULE. " + strings.Repeat("filler ", 1000) + "LAST RULE."
	body := `{"answers":{"risk":{"score":1,"confidence":0.9},"q_destructive":{"noul":0.1},"q_credentials":{"noul":0.1},"q_policy_violation":{"noul":0.1},"q_policy_exception":{"noul":0.1}}}`
	srv, got, _ := serveSystemOne(t, body)
	d, _ := SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions}.Evaluate(context.Background(), withRules(runCommand, rules))
	state := stateOf(t, *got)
	if !strings.Contains(state, "FIRST RULE.") || strings.Contains(state, "LAST RULE.") || !strings.Contains(state, "[truncated:") {
		t.Errorf("the first rules are kept, the tail is cut and marked")
	}
	if !strings.Contains(d.Rationale, "gate instructions truncated to 4000 of 7022 characters") {
		t.Errorf("the transparency data warns: %q", d.Rationale)
	}
	d, _ = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions, InstructionsMaxChars: 10000}.Evaluate(context.Background(), withRules(runCommand, rules))
	if !strings.Contains(stateOf(t, *got), "LAST RULE.") || strings.Contains(d.Rationale, "truncated") {
		t.Errorf("a raised cap shows everything: %q", d.Rationale)
	}
	_, _ = SystemOneScorer{BaseURL: srv.URL, Strategy: StrategyScoreQuestions}.Evaluate(context.Background(), withRules(runCommand, testRules))
	if strings.Contains(stateOf(t, *got), "[truncated:") {
		t.Error("short instructions are not marked")
	}
}

func TestLLMScorerPromptCarriesGateInstructions(t *testing.T) {
	st := &scriptedStreamer{answer: `{"score": 3, "rationale": "fine"}`}
	req := withRules(Request{ToolName: "run_command", Args: []byte(`{"command":["ls"]}`)}, testRules)
	d, err := LLMScorer{Streamer: st, Model: "m"}.Evaluate(context.Background(), req)
	if err != nil || d.Risk != 3 {
		t.Fatalf("%+v, %v", d, err)
	}
	prompt := st.got[0].Content
	if !strings.Contains(prompt, "<gate_instructions>\n"+testRules+"\n</gate_instructions>") {
		t.Errorf("prompt lacks the delimited rules:\n%s", prompt)
	}
	if _, err := (LLMScorer{Streamer: st, Model: "m"}).Evaluate(context.Background(), Request{ToolName: "run_command", Args: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(st.got[0].Content, "gate_instructions") {
		t.Error("no instructions, no block")
	}
	req.GateInstructions = strings.Repeat("r", 300)
	d, _ = LLMScorer{Streamer: st, Model: "m", InstructionsMaxChars: 100}.Evaluate(context.Background(), req)
	if !strings.Contains(d.Rationale, "truncated to 100 of 300") {
		t.Errorf("rationale %q", d.Rationale)
	}
}

func TestReadGateInstructionsIsAlwaysAllowedAtRiskOne(t *testing.T) {
	d, err := Rules{}.Evaluate(context.Background(), Request{ToolName: "read_gate_instructions", Args: []byte(`{}`)})
	if err != nil || d.Kind != Allow || d.Risk != 1 {
		t.Fatalf("%+v, %v", d, err)
	}
}
