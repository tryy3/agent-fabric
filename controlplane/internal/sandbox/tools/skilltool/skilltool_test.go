package skilltool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/skilltool"
)

func TestToolIsReadOnlyAndHandledByTheAgent(t *testing.T) {
	tools := skilltool.Tools()
	if len(tools) != 1 || tools[0].Name != "load_skill" {
		t.Fatalf("tools = %+v", tools)
	}
	if tools[0].Requires != (sandbox.Capabilities{}) {
		t.Fatalf("requires = %+v, want none", tools[0].Requires)
	}
	if _, err := tools[0].Run(context.Background(), nil, nil); err == nil {
		t.Fatal("the sandbox runner must not answer: only the agent holds the pin")
	}
}

func TestParseArgs(t *testing.T) {
	got, err := skilltool.ParseArgs(json.RawMessage(`{"name":" a ","path":" references/x.md "}`))
	if err != nil || got.Name != "a" || got.Path != "references/x.md" {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, bad := range []string{`{}`, `{"name":"  "}`, `not json`} {
		if _, err := skilltool.ParseArgs(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
}
