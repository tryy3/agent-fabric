package file_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file"
)

func TestFileToolsRoundTrip(t *testing.T) {
	root := t.TempDir()
	env, err := sandbox.Open(context.Background(), sandbox.OpenOptions{
		Kind: "local", WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())

	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}
	defs := reg.Definitions(env)
	if len(defs) != 2 {
		t.Fatalf("defs = %d", len(defs))
	}
	if defs[0].Type != "function" || defs[0].Function.Name == "" {
		t.Fatalf("bad def: %+v", defs[0])
	}

	out, err := reg.Call(
		context.Background(),
		env,
		"write_file",
		json.RawMessage(`{"path":"nested/a.txt","content":"hello"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"error"`) {
		t.Fatal(out)
	}
	out, err = reg.Call(
		context.Background(),
		env,
		"read_file",
		json.RawMessage(`{"path":"nested/a.txt"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("out = %s", out)
	}
}

func TestFileToolsReturnArgumentErrorsAsJSON(t *testing.T) {
	root := t.TempDir()
	env, err := sandbox.Open(context.Background(), sandbox.OpenOptions{
		Kind: "local", WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())

	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}

	out, err := reg.Call(
		context.Background(),
		env,
		"read_file",
		json.RawMessage(`{"path":`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("out = %s", out)
	}
}
