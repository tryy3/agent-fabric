package gatebench_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/gatebench"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type constStreamer struct{ answer string }

func (s constStreamer) StreamChat(_ context.Context, _ string, _ []runtime.Message, _ provider.StreamChatOptions, on func(provider.StreamEvent) error) error {
	return on(provider.StreamEvent{Content: s.answer})
}

func TestRunRulesOnlyAndPessimisticScorer(t *testing.T) {
	cases := loadCases(t)
	ctx := context.Background()

	rules := gatebench.Run(ctx, gatebench.RulesSetup(), cases, gatebench.RunOptions{})
	if rules.Metrics.Cases != len(cases) || rules.Metrics.Errors != 0 {
		t.Fatalf("metrics = %+v", rules.Metrics)
	}
	if rules.Metrics.DangerCases == 0 || rules.Metrics.SafeCases == 0 {
		t.Fatalf("dataset lacks safe or danger cases: %+v", rules.Metrics)
	}
	if rules.Metrics.DangerMissRate == 0 {
		t.Error("rules are known to miss some cancel-worthy cases; the dataset should show it")
	}

	cfg, err := gatebench.LoadConfig([]byte(`{"setups":[{"name":"paranoid","evaluators":[
		{"type":"rules"},
		{"type":"llm","connection":{"baseUrl":"http://unused"},"model":"fake"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	setup, err := gatebench.Build(ctx, cfg.Setups[0], gatebench.BuildOptions{
		Streamer: func(string, string, string) (provider.ChatStreamer, error) {
			return constStreamer{`{"score": 10, "rationale": "always"}`}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	para := gatebench.Run(ctx, setup, cases, gatebench.RunOptions{Concurrency: 8})
	if para.Metrics.DangerMissRate != 0 {
		t.Errorf("a scorer that says 10 for everything must catch every dangerous case: %v", para.Metrics.DangerMissRate)
	}
	if para.Metrics.OverAskRate != 1 {
		t.Errorf("and over-ask on every safe case: %v", para.Metrics.OverAskRate)
	}
	if para.Usage == nil || para.Usage.Calls == 0 || para.Usage.Calls > int64(len(cases)) { // hard denies skip the scorer
		t.Errorf("usage = %+v", para.Usage)
	}

	var out bytes.Buffer
	rep := gatebench.Report{Cases: len(cases), Setups: []gatebench.SetupResult{rules, para}}
	gatebench.WriteText(&out, rep, false)
	gatebench.WriteComparison(&out, rep, rep)
	for _, want := range []string{"rules", "paranoid", "By category", "Comparison with baseline"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestToolFilterAbstainsOnOtherTools(t *testing.T) {
	cfg, err := gatebench.LoadConfig([]byte(`{"setups":[{"name":"cmd-only","evaluators":[
		{"type":"llm","tools":["run_command"],"connection":{"baseUrl":"http://unused"},"model":"fake"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s, err := gatebench.Build(context.Background(), cfg.Setups[0], gatebench.BuildOptions{
		Streamer: func(string, string, string) (provider.ChatStreamer, error) { return constStreamer{`{"score":9}`}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	read, _ := s.Chain.Evaluate(context.Background(), gate.Request{ToolName: "read_file", Args: []byte(`{}`)})
	cmd, _ := s.Chain.Evaluate(context.Background(), gate.Request{ToolName: "run_command", Args: []byte(`{}`)})
	if read.Risk != 0 || cmd.Risk != 9 {
		t.Fatalf("read=%d cmd=%d", read.Risk, cmd.Risk)
	}
}

func TestLoadConfigRejectsBadSetups(t *testing.T) {
	for _, bad := range []string{
		`{}`,
		`{"setups":[{"name":"a","evaluators":[]}]}`,
		`{"setups":[{"name":"a","evaluators":[{"type":"magic"}]}]}`,
		`{"setups":[{"name":"a","evaluators":[{"type":"llm"}]}]}`,
		`{"setups":[{"name":"a","evaluators":[{"type":"rules"}]},{"name":"a","evaluators":[{"type":"rules"}]}]}`,
		`{"setups":[{"name":"a","evaluators":[{"type":"rules","bogus":1}]}]}`,
	} {
		if _, err := gatebench.LoadConfig([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
