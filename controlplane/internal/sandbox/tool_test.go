package sandbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file"
)

type capsEnv struct {
	caps sandbox.Capabilities
}

func (e *capsEnv) ID() string {
	return "caps-only"
}

func (e *capsEnv) Caps() sandbox.Capabilities {
	return e.caps
}

func (e *capsEnv) FS() (sandbox.FS, bool) {
	return nil, false
}

func (e *capsEnv) Exec() (sandbox.Executor, bool) {
	return nil, false
}

func (e *capsEnv) Close(context.Context) error {
	return nil
}

func TestAvailableEmptyWithoutFS(t *testing.T) {
	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}

	env := &capsEnv{caps: sandbox.Capabilities{}}
	if len(reg.Available(env)) != 0 {
		t.Fatal("expected no tools")
	}
}

func TestRegistryCallUnknownToolReturnsError(t *testing.T) {
	reg := sandbox.NewRegistry()

	_, err := reg.Call(
		context.Background(),
		&capsEnv{},
		"missing",
		json.RawMessage(`{}`),
	)
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
}

func TestRegistryCallReturnsToolErrorsAsJSON(t *testing.T) {
	reg := sandbox.NewRegistry()
	reg.Register(sandbox.Tool{
		Name: "broken",
		Run: func(context.Context, sandbox.Environment, json.RawMessage) (string, error) {
			return "", errors.New("failed")
		},
	})

	out, err := reg.Call(
		context.Background(),
		&capsEnv{},
		"broken",
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"error":"failed"}` {
		t.Fatalf("out = %s", out)
	}
}

func TestParametersMarshalJSONSchemaObject(t *testing.T) {
	raw, err := json.Marshal(sandbox.Parameters{
		Properties: map[string]sandbox.Property{
			"path": {Type: "string", Description: "file path"},
		},
		Required: []string{"path"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "object" {
		t.Fatalf("type = %v", got["type"])
	}
	if got["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %v", got["additionalProperties"])
	}
}
