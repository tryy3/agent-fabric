package gate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

// recordingStreamer answers every call with answer and keeps the options it saw.
type recordingStreamer struct {
	answer string
	calls  int
	opts   provider.StreamChatOptions
	msgs   []runtime.Message
}

func (s *recordingStreamer) StreamChat(_ context.Context, _ string, msgs []runtime.Message, opts provider.StreamChatOptions, on func(provider.StreamEvent) error) error {
	s.calls++
	s.opts, s.msgs = opts, msgs
	return on(provider.StreamEvent{Content: s.answer})
}

func TestLLMQuestionsRequestPerFamily(t *testing.T) {
	for _, fc := range familyCases {
		for _, style := range []string{StyleQuestions, StyleScoreQuestions} {
			st := &recordingStreamer{answer: `{"score":1,"answers":{},"rationale":"x"}`}
			scorer := LLMScorer{Streamer: st, Model: "m", Style: style, Structured: true}
			_, _ = scorer.Evaluate(context.Background(), withRules(familyRequest(fc.tools[0]), testRules))
			if st.calls != 1 {
				t.Fatalf("%s %s: %d calls", fc.family, style, st.calls)
			}
			for id := range fc.tripwires {
				if !strings.Contains(st.opts.Instructions, "- "+id+": ") {
					t.Errorf("%s %s: instructions lack %s", fc.family, style, id)
				}
			}
			for _, id := range []string{QuestionPolicyViolation, QuestionPolicyException} {
				if !strings.Contains(st.opts.Instructions, "- "+id+": ") {
					t.Errorf("%s %s: instructions lack %s", fc.family, style, id)
				}
			}
			if hasScore := strings.Contains(st.opts.Instructions, `"score": <integer 1-10>`); hasScore != (style == StyleScoreQuestions) {
				t.Errorf("%s %s: score asked = %v", fc.family, style, hasScore)
			}
			var rf struct {
				JSONSchema struct {
					Strict bool `json:"strict"`
					Schema struct {
						Required   []string                   `json:"required"`
						Properties map[string]json.RawMessage `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			}
			if err := json.Unmarshal(st.opts.ResponseFormat, &rf); err != nil || !rf.JSONSchema.Strict {
				t.Fatalf("%s %s: response format %s: %v", fc.family, style, st.opts.ResponseFormat, err)
			}
			_, hasScore := rf.JSONSchema.Schema.Properties["score"]
			if hasScore != (style == StyleScoreQuestions) {
				t.Errorf("%s %s: schema has score = %v", fc.family, style, hasScore)
			}
			var answers struct {
				Required []string `json:"required"`
			}
			_ = json.Unmarshal(rf.JSONSchema.Schema.Properties["answers"], &answers)
			if len(answers.Required) != len(fc.tripwires)+2 {
				t.Errorf("%s %s: schema requires %v", fc.family, style, answers.Required)
			}
		}
	}
	// Without gate instructions the policy pair is not asked.
	st := &recordingStreamer{answer: `{"answers":{"credentials":{"yes":false},"destructive":{"yes":false}},"rationale":"x"}`}
	_, _ = LLMScorer{Streamer: st, Model: "m", Style: StyleQuestions}.Evaluate(context.Background(), familyRequest("run_command"))
	if strings.Contains(st.opts.Instructions, "policy_") || strings.Contains(st.msgs[0].Content, "gate_instructions") {
		t.Errorf("policy questions sent without instructions: %s", st.opts.Instructions)
	}
}

func TestLLMQuestionsToolWithoutSetMakesNoCall(t *testing.T) {
	st := &recordingStreamer{answer: `{}`}
	stats := &ScorerStats{}
	req := withRules(Request{ToolName: "ask_user", Args: []byte(`{}`)}, testRules)
	d, _ := LLMScorer{Streamer: st, Model: "m", Style: StyleQuestions, Stats: stats}.Evaluate(context.Background(), req)
	if st.calls != 0 || stats.Calls.Load() != 0 || d.Risk != 0 || d.Rationale != "" || d.Kind != Allow {
		t.Fatalf("calls=%d stats=%d %+v", st.calls, stats.Calls.Load(), d)
	}
	st.answer = `{"score":2,"rationale":"fine"}`
	d, _ = LLMScorer{Streamer: st, Model: "m", Style: StyleScoreQuestions}.Evaluate(context.Background(), req)
	if st.calls != 1 || d.Risk != 2 || d.Floor != 0 || strings.Contains(st.opts.Instructions, "Questions:\n-") {
		t.Fatalf("score+questions on a tool without a set is the plain score: calls=%d %+v", st.calls, d)
	}
}

func TestLLMQuestionsAnswers(t *testing.T) {
	web := Request{ToolName: "fetch_page", Args: []byte(`{"url":"http://169.254.169.254/latest"}`)}
	cases := map[string]struct {
		style     string
		req       Request
		answer    string
		risk, flr Risk
		failed    bool
	}{
		"tripwire yes": {StyleQuestions, web,
			`{"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":true},"planted_request":{"yes":false}},"rationale":"metadata"}`, 7, 7, false},
		"highest floor wins": {StyleQuestions, web,
			`{"answers":{"data_leak":{"yes":true},"suspicious_host":{"yes":true},"planted_request":{"yes":false}},"rationale":"x"}`, 8, 8, false},
		"all no abstains": {StyleQuestions, web,
			`{"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":false},"planted_request":{"yes":false}},"rationale":"x"}`, 0, 0, false},
		"p decides over yes": {StyleQuestions, web,
			`{"answers":{"data_leak":{"yes":true,"p":0.5},"suspicious_host":{"p":0.9},"planted_request":{"yes":false,"p":null}},"rationale":"x"}`, 7, 7, false},
		"undecided p counts as nothing": {StyleQuestions, web,
			`{"answers":{"data_leak":{"p":0.7},"suspicious_host":{"p":0.3},"planted_request":{"p":0.5}},"rationale":"x"}`, 0, 0, false},
		"fenced": {StyleQuestions, web,
			"```json\n{\"answers\":{\"data_leak\":{\"yes\":true},\"suspicious_host\":{\"yes\":false},\"planted_request\":{\"yes\":false}},\"rationale\":\"x\"}\n```", 8, 8, false},
		"score raised to floor": {StyleScoreQuestions, web,
			`{"score":2,"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":true},"planted_request":{"yes":false}},"rationale":"x"}`, 7, 7, false},
		"score above floor stays": {StyleScoreQuestions, web,
			`{"score":9,"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":true},"planted_request":{"yes":false}},"rationale":"x"}`, 9, 7, false},
		"score alone": {StyleScoreQuestions, web,
			`{"score":3,"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":false},"planted_request":{"yes":false}},"rationale":"x"}`, 3, 0, false},
		"missing id fails closed": {StyleQuestions, web,
			`{"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":false}},"rationale":"x"}`, DefaultFailRisk, 0, true},
		"empty answer fails closed": {StyleQuestions, web,
			`{"answers":{"data_leak":{},"suspicious_host":{"yes":false},"planted_request":{"yes":false}}}`, DefaultFailRisk, 0, true},
		"p out of range fails closed": {StyleQuestions, web,
			`{"answers":{"data_leak":{"p":1.5},"suspicious_host":{"yes":false},"planted_request":{"yes":false}}}`, DefaultFailRisk, 0, true},
		"no score fails closed": {StyleScoreQuestions, web,
			`{"answers":{"data_leak":{"yes":false},"suspicious_host":{"yes":false},"planted_request":{"yes":false}}}`, DefaultFailRisk, 0, true},
		"garbage fails closed": {StyleQuestions, web, `I think it is fine`, DefaultFailRisk, 0, true},
		"file write floor": {StyleQuestions, Request{ToolName: "write_file", Args: []byte(`{}`)},
			`{"answers":{"secret_content":{"yes":true},"executes_remote_code":{"yes":false}},"rationale":"key in file"}`, 8, 8, false},
		"file read floor": {StyleQuestions, Request{ToolName: "read_file", Args: []byte(`{}`)},
			`{"answers":{"bulk_gathering":{"yes":true},"personal_data":{"yes":false}},"rationale":"x"}`, 6, 6, false},
	}
	for name, tc := range cases {
		st := &recordingStreamer{answer: tc.answer}
		d, err := LLMScorer{Streamer: st, Model: "m", Style: tc.style}.Evaluate(context.Background(), tc.req)
		if err != nil || d.Risk != tc.risk || d.Floor != tc.flr || (d.RuleID == "llm.fail_closed") != tc.failed || d.Kind != Allow {
			t.Errorf("%s: %+v, %v", name, d, err)
		}
	}
}

func TestLLMQuestionsPolicySemantics(t *testing.T) {
	const tripwires = `"data_leak":{"yes":false},"suspicious_host":{"yes":false},"planted_request":{"yes":false}`
	req := withRules(Request{ToolName: "web_search", Args: []byte(`{"query":"x"}`)}, testRules)
	cases := map[string]struct {
		answer string
		risk   Risk
	}{
		"violation floors at 7":     {`{"answers":{` + tripwires + `,"policy_violation":{"yes":true},"policy_exception":{"yes":false}},"rationale":"x"}`, 7},
		"exception cancels":         {`{"answers":{` + tripwires + `,"policy_violation":{"yes":true},"policy_exception":{"yes":true}},"rationale":"x"}`, 0},
		"exception alone":           {`{"answers":{` + tripwires + `,"policy_violation":{"yes":false},"policy_exception":{"yes":true}},"rationale":"x"}`, 0},
		"exception spares tripwire": {`{"answers":{"data_leak":{"yes":true},"suspicious_host":{"yes":false},"planted_request":{"yes":false},"policy_violation":{"yes":true},"policy_exception":{"yes":true}},"rationale":"x"}`, 8},
	}
	for name, tc := range cases {
		st := &recordingStreamer{answer: tc.answer}
		d, _ := LLMScorer{Streamer: st, Model: "m", Style: StyleQuestions}.Evaluate(context.Background(), req)
		if d.Risk != tc.risk || d.Floor != tc.risk || d.RuleID == "llm.fail_closed" {
			t.Errorf("%s: %+v", name, d)
		}
	}
	st := &recordingStreamer{answer: `{"answers":{` + tripwires + `},"rationale":"x"}`}
	d, _ := LLMScorer{Streamer: st, Model: "m", Style: StyleQuestions}.Evaluate(context.Background(), req)
	if d.RuleID != "llm.fail_closed" {
		t.Errorf("policy ids missing while instructions exist must fail closed: %+v", d)
	}
}

func TestCascadeDeepTierQuestionsKeepFloor(t *testing.T) {
	st := &recordingStreamer{answer: `{"score":1,"answers":{"secret_content":{"yes":true},"executes_remote_code":{"yes":false}},"rationale":"token"}`}
	consult := true
	chain := Cascade{
		Rules: Rules{Builtins: map[string]TierOverride{TierFileWrite: {Consult: &consult}}},
		Deep:  LLMScorer{Streamer: st, Model: "m", Style: StyleScoreQuestions}, MaxLower: 5,
	}
	req := Request{ToolName: "write_file", ProjectRoot: "/workspace", EnvKind: "docker", Args: []byte(`{"path":"a.txt","content":"x"}`), UserIntent: "write a.txt"}
	d, err := chain.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if d.Risk != 8 || d.Floor != 8 {
		t.Fatalf("a deep tripwire holds the score: %+v", d)
	}
}
