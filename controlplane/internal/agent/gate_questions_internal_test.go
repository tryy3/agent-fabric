package agent

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestGateForGivesBothScorersThePinnedQuestions(t *testing.T) {
	off := false
	pin := runtime.SessionPin{Gate: runtime.GatePin{
		Questions: runtime.GateQuestions{
			Builtins: map[string]runtime.QuestionOverride{"credentials": {Enabled: &off}},
			Custom:   []runtime.CustomQuestion{{ID: "prod_cluster", Tools: []string{"command"}, Text: "Does this touch the production cluster?", Risk: 9}},
		},
		Fast: &runtime.GateScorer{BaseURL: "http://127.0.0.1:1", Model: "jev", Strategy: gate.StrategyQuestions},
		Deep: &runtime.GateScorer{ConnectionType: catalog.TypeOpenAICompatible, BaseURL: "http://127.0.0.1:1/v1", Model: "m", Style: gate.StyleQuestions},
	}}
	c, ok := (&Agent{}).gateFor(pin, "s", nil).(gate.Cascade)
	if !ok {
		t.Fatal("not a cascade")
	}
	fast, deep := c.Fast.(gate.SystemOneScorer), c.Deep.(gate.LLMScorer)
	for name, qs := range map[string]*gate.QuestionSet{"fast": fast.Questions, "deep": deep.Questions} {
		var ids []string
		for _, q := range qs.Tripwires("run_command") {
			ids = append(ids, q.ID)
		}
		if strings.Join(ids, ",") != "destructive,prod_cluster" {
			t.Errorf("%s scorer tripwires = %v", name, ids)
		}
	}
}
