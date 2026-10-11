package gatebench_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/gatebench"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type questionStreamer struct{ instructions *[]string }

func (s questionStreamer) StreamChat(_ context.Context, _ string, _ []runtime.Message, opts provider.StreamChatOptions, on func(provider.StreamEvent) error) error {
	*s.instructions = append(*s.instructions, opts.Instructions)
	return on(provider.StreamEvent{Content: `{"answers":{"destructive":{"yes":false},"prod_cluster":{"yes":true}},"rationale":"x"}`})
}

func TestSetupQuestionsReachTheScorers(t *testing.T) {
	cfg, err := gatebench.LoadConfig([]byte(`{"setups":[{"name":"edited","evaluators":[
		{"type":"rules"},
		{"type":"llm","style":"questions","connection":{"baseUrl":"http://unused"},"model":"fake"}],
		"questions":{"builtins":{"credentials":{"enabled":false}},
		  "custom":[{"id":"prod_cluster","tools":["run_command"],"text":"Does this touch the production cluster?","risk":9}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	setup, err := gatebench.Build(context.Background(), cfg.Setups[0], gatebench.BuildOptions{
		Streamer: func(string, string, string) (provider.ChatStreamer, error) { return questionStreamer{&seen}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := setup.Gate.Evaluate(context.Background(), gate.Request{
		ToolName: "run_command", Args: []byte(`{"command":["make","deploy"]}`), ProjectRoot: "/workspace", POSIX: true, EnvKind: "docker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Risk != 9 || len(seen) != 1 || !strings.Contains(seen[0], "prod_cluster") || strings.Contains(seen[0], "credentials") {
		t.Fatalf("risk %d, instructions %q", d.Risk, seen)
	}

	_, err = gatebench.LoadConfig([]byte(`{"setups":[{"name":"bad","evaluators":[{"type":"rules"}],"questions":{"builtins":{"nope":{"risk":3}}}}]}`))
	if err == nil || !strings.Contains(err.Error(), `unknown gate question "nope"`) {
		t.Fatalf("bad questions: %v", err)
	}
}
