package gateinfo_test

import (
	"context"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/gateinfo"
)

func TestToolIsReadOnlyAndHandledByTheAgent(t *testing.T) {
	tools := gateinfo.Tools()
	if len(tools) != 1 || tools[0].Name != "read_gate_instructions" {
		t.Fatalf("tools = %+v", tools)
	}
	if tools[0].Requires != (sandbox.Capabilities{}) {
		t.Fatalf("requires = %+v, want none", tools[0].Requires)
	}
	if len(tools[0].Parameters.Properties) != 0 {
		t.Fatal("the tool takes no arguments, so it has no write path")
	}
	if _, err := tools[0].Run(context.Background(), nil, nil); err == nil {
		t.Fatal("the sandbox runner must not answer: only the agent holds the pin")
	}
}

func TestResult(t *testing.T) {
	if got := gateinfo.Result(""); got != gateinfo.NoneText {
		t.Fatalf("empty = %q", got)
	}
	if got := gateinfo.Result("rules"); got != "rules" {
		t.Fatalf("got %q", got)
	}
}
