package file_test

import (
	"context"
	"encoding/json"
	"path/filepath"
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
	tools := reg.Available(env)
	if len(tools) != 2 {
		t.Fatalf("tools = %d", len(tools))
	}
	if tools[0].Name == "" || tools[0].Parameters.Properties == nil {
		t.Fatalf("bad tool: %+v", tools[0])
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

func TestFileToolsWhitelistDeniesOutsideAndReadonly(t *testing.T) {
	root := t.TempDir()
	extra := t.TempDir()
	outside := t.TempDir()
	policy := &sandbox.PathPolicy{Grants: []sandbox.PathGrant{
		{Path: root, Read: true, Write: false},
		{Path: extra, Read: true, Write: true},
	}}
	env, err := sandbox.Open(context.Background(), sandbox.OpenOptions{
		Kind:          "local",
		WorkspaceRoot: root,
		PathPolicy:    policy,
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
		"write_file",
		json.RawMessage(`{"path":"notes.txt","content":"nope"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not writable") {
		t.Fatalf("readonly write = %s", out)
	}

	out, err = reg.Call(
		context.Background(),
		env,
		"write_file",
		mustJSON(t, map[string]string{
			"path":    filepath.Join(outside, "secret.txt"),
			"content": "leak",
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not allowed") {
		t.Fatalf("outside write = %s", out)
	}

	out, err = reg.Call(
		context.Background(),
		env,
		"write_file",
		mustJSON(t, map[string]string{
			"path":    filepath.Join(extra, "ok.txt"),
			"content": "hello",
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"error"`) {
		t.Fatalf("extra write = %s", out)
	}
	out, err = reg.Call(
		context.Background(),
		env,
		"read_file",
		mustJSON(t, map[string]string{"path": filepath.Join(extra, "ok.txt")}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("extra read = %s", out)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
