package gate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type scriptedStreamer struct {
	answer string
	err    error
	got    []runtime.Message
}

func (s *scriptedStreamer) StreamChat(_ context.Context, _ string, msgs []runtime.Message, _ provider.StreamChatOptions, on func(provider.StreamEvent) error) error {
	s.got = msgs
	if s.err != nil {
		return s.err
	}
	return on(provider.StreamEvent{Content: s.answer})
}

func TestParseScore(t *testing.T) {
	ok := map[string]Risk{
		`{"score": 8, "rationale": "x"}`:                  8,
		"```json\n{\"score\":3,\"rationale\":\"y\"}\n```": 3,
		`Sure! {"score": 9.0, "rationale": "z"} done`:     9,
	}
	for in, want := range ok {
		got, _, err := ParseScore(in)
		if err != nil || got != want {
			t.Errorf("ParseScore(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "no json", `{"score": 0}`, `{"score": 11}`, `{"rationale":"x"}`, `{"score":"high"}`} {
		if _, _, err := ParseScore(in); err == nil {
			t.Errorf("ParseScore(%q) accepted", in)
		}
	}
}

func TestLLMScorerFailsClosed(t *testing.T) {
	req := Request{ToolName: "run_command", Args: []byte(`{"command":["ls"]}`), UserIntent: "list files"}
	stats := &ScorerStats{}
	for name, st := range map[string]*scriptedStreamer{
		"error":   {err: errors.New("boom")},
		"garbage": {answer: "I think it is fine"},
	} {
		d, err := LLMScorer{Streamer: st, Model: "m", Stats: stats}.Evaluate(context.Background(), req)
		if err != nil || d.Risk != DefaultFailRisk || d.RuleID != "llm.fail_closed" || d.Kind != Allow {
			t.Fatalf("%s: %+v, %v", name, d, err)
		}
	}
	if stats.Failures.Load() != 2 || stats.Calls.Load() != 2 {
		t.Fatalf("stats = %d/%d", stats.Failures.Load(), stats.Calls.Load())
	}
}

func TestLLMScorerScoresAndRaisesRule(t *testing.T) {
	st := &scriptedStreamer{answer: `{"score": 9, "rationale": "downloads and runs a script"}`}
	chain := Cascade{Deep: LLMScorer{Streamer: st, Model: "m"}}
	req := Request{
		ToolName: "run_command", ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker",
		Args: []byte(`{"command":["npm","test"]}`), UserIntent: "list files",
	}
	d, err := chain.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if d.Risk != 9 || d.Source != "llm:m" {
		t.Fatalf("%+v", d)
	}
	if got := DefaultPolicies.PolicyFor(ModeAuto).Resolve(d); got.Kind != Deny {
		t.Fatalf("auto mode should cancel risk 9, got %s", got.Kind)
	}
	if len(st.got) != 1 || !contains(st.got[0].Content, "list files") {
		t.Fatalf("prompt = %+v", st.got)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

type thoughtOnlyStreamer struct{}

func (thoughtOnlyStreamer) StreamChat(_ context.Context, _ string, _ []runtime.Message, _ provider.StreamChatOptions, on func(provider.StreamEvent) error) error {
	_ = on(provider.StreamEvent{Thought: "let me think about this call"})
	_ = on(provider.StreamEvent{Finish: "length"})
	return errors.New("empty assistant response")
}

func TestLLMScorerExplainsThinkingModelWithNoAnswer(t *testing.T) {
	stats := &ScorerStats{}
	d, err := LLMScorer{Streamer: thoughtOnlyStreamer{}, Model: "m", Stats: stats}.
		Evaluate(context.Background(), Request{ToolName: "run_command", Args: []byte(`{}`)})
	if err != nil || d.Risk != DefaultFailRisk {
		t.Fatalf("%+v, %v", d, err)
	}
	for _, want := range []string{"reasoning and no answer", `finish="length"`, "maxTokens"} {
		if !strings.Contains(d.Rationale, want) {
			t.Errorf("rationale lacks %q: %s", want, d.Rationale)
		}
	}
	if len(stats.Errors()) != 1 {
		t.Fatalf("errors = %v", stats.Errors())
	}
}

func TestParseBand(t *testing.T) {
	for answer, want := range map[string]Risk{
		`{"band":"safe","requested":null,"rationale":"r"}`:        1,
		`{"band":"safe","requested":false}`:                       1,
		`{"band":"low","requested":false}`:                        4,
		`{"band":"low","requested":true}`:                         3,
		`{"band":"elevated","requested":true}`:                    3,
		`{"band":"high","requested":true}`:                        5,
		`{"band":"high","requested":false}`:                       8,
		"```json\n{\"band\": \"High\", \"requested\": null}\n```": 7,
		`{"band":"cancel","requested":true}`:                      9,
		`{"band":"cancel","requested":false}`:                     10,
	} {
		got, _, err := ParseBand(answer)
		if err != nil || got != want {
			t.Errorf("ParseBand(%s) = %d, %v; want %d", answer, got, err, want)
		}
	}
	for _, bad := range []string{`{"band":"extreme"}`, `{"score":3}`, `nope`} {
		if _, _, err := ParseBand(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
