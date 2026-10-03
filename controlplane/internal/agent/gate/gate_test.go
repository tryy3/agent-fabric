package gate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

func TestRulesInWorkspaceAllow(t *testing.T) {
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:    "read_file",
		Args:        json.RawMessage(`{"path":"a.txt"}`),
		ProjectRoot: "/workspace",
		POSIX:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Allow {
		t.Fatalf("got %#v", d)
	}
}

func TestRulesEscapeAsk(t *testing.T) {
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:    "read_file",
		Args:        json.RawMessage(`{"path":"/tmp/outside"}`),
		ProjectRoot: "/workspace",
		POSIX:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Ask {
		t.Fatalf("got %#v", d)
	}
}

func TestRulesSensitiveWriteDeny(t *testing.T) {
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:    "write_file",
		Args:        json.RawMessage(`{"path":"/etc/passwd","content":"x"}`),
		ProjectRoot: "/workspace",
		POSIX:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Deny || d.RuleID != "rules.sensitive_deny" {
		t.Fatalf("got %#v", d)
	}
}

func TestChainDenyWinsOverAsk(t *testing.T) {
	chain := Chain{Evaluators: []Evaluator{
		EvaluatorFunc(func(context.Context, Request) (Decision, error) {
			return Decision{Kind: Ask, RuleID: "ask"}, nil
		}),
		EvaluatorFunc(func(context.Context, Request) (Decision, error) {
			return Decision{Kind: Deny, RuleID: "deny"}, nil
		}),
	}}
	d, err := chain.Evaluate(context.Background(), Request{ToolName: "read_file"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Deny || d.RuleID != "deny" {
		t.Fatalf("got %#v", d)
	}
}

func TestChainScriptedClassifierAsk(t *testing.T) {
	chain := Chain{Evaluators: []Evaluator{
		Rules{},
		EvaluatorFunc(func(context.Context, Request) (Decision, error) {
			return Decision{Kind: Ask, Reason: "classifier flag", RuleID: "classifier.test"}, nil
		}),
	}}
	d, err := chain.Evaluate(context.Background(), Request{
		ToolName:    "read_file",
		Args:        json.RawMessage(`{"path":"a.txt"}`),
		ProjectRoot: "/workspace",
		POSIX:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Ask || d.RuleID != "classifier.test" {
		t.Fatalf("got %#v", d)
	}
}

func TestRulesNotWritableAsk(t *testing.T) {
	policy := &sandboxcore.PathPolicy{Grants: []sandboxcore.PathGrant{
		{Path: "/workspace", Read: true, Write: false},
	}}
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:    "write_file",
		Args:        json.RawMessage(`{"path":"a.txt","content":"x"}`),
		ProjectRoot: "/workspace",
		POSIX:       true,
		PathPolicy:  policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Ask {
		t.Fatalf("got %#v", d)
	}
}

// EvaluatorFunc adapts a function to Evaluator (tests / scripted classifiers).
type EvaluatorFunc func(context.Context, Request) (Decision, error)

func (f EvaluatorFunc) Evaluate(ctx context.Context, req Request) (Decision, error) {
	return f(ctx, req)
}

func evalTool(t *testing.T, tool, args string) Decision {
	t.Helper()
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:    tool,
		Args:        json.RawMessage(args),
		ProjectRoot: "/workspace",
		POSIX:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRulesNewToolsInsideRootAllow(t *testing.T) {
	cases := map[string]string{
		"list_files":       `{}`,
		"search_text":      `{"query":"x"}`,
		"write_file":       `{"path":"a.txt","content":"x"}`,
		"append_file":      `{"path":"a.txt","content":"x"}`,
		"create_directory": `{"path":"src/new"}`,
		"move_path":        `{"from":"a.txt","to":"src/a.txt"}`,
		"apply_patch":      `{"diff":"--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-a\n+b\n"}`,
	}
	for tool, args := range cases {
		if d := evalTool(t, tool, args); d.Kind != Allow {
			t.Errorf("%s: got %#v, want Allow", tool, d)
		}
	}
}

func TestRulesNewToolsOutsideRootAsk(t *testing.T) {
	cases := map[string]string{
		"list_files":       `{"path":"/tmp"}`,
		"search_text":      `{"query":"x","path":"/tmp"}`,
		"append_file":      `{"path":"/tmp/x","content":"x"}`,
		"create_directory": `{"path":"/tmp/x"}`,
		"move_path":        `{"from":"a.txt","to":"/tmp/a.txt"}`,
		"apply_patch":      `{"diff":"--- a/../x\n+++ b/../x\n@@ -1 +1 @@\n-a\n+b\n"}`,
	}
	for tool, args := range cases {
		if d := evalTool(t, tool, args); d.Kind != Ask {
			t.Errorf("%s: got %#v, want Ask", tool, d)
		}
	}
}

func TestRulesGitMetadataDeniedForWrites(t *testing.T) {
	cases := map[string]string{
		"write_file":       `{"path":".git/config","content":"x"}`,
		"append_file":      `{"path":".git/config","content":"x"}`,
		"create_directory": `{"path":".git/hooks"}`,
		"delete_path":      `{"path":".git"}`,
		"move_path":        `{"from":"a.txt","to":".git/a.txt"}`,
		"apply_patch":      `{"diff":"--- /dev/null\n+++ b/.git/config\n@@ -0,0 +1 @@\n+x\n"}`,
		"write_file_abs":   `{"path":"/workspace/.git/HEAD","content":"x"}`,
	}
	for tool, args := range cases {
		name := tool
		if tool == "write_file_abs" {
			name = "write_file"
		}
		d := evalTool(t, name, args)
		if d.Kind != Deny || d.RuleID != "rules.git_protected" {
			t.Errorf("%s: got %#v, want git_protected Deny", tool, d)
		}
	}
	// Reads are still fine, and lookalike names are not protected.
	if d := evalTool(t, "read_file", `{"path":".git/HEAD"}`); d.Kind != Allow {
		t.Errorf("read .git/HEAD: got %#v", d)
	}
	if d := evalTool(t, "write_file", `{"path":".gitignore","content":"x"}`); d.Kind != Allow {
		t.Errorf("write .gitignore: got %#v", d)
	}
}

func TestRulesDeleteAlwaysAsks(t *testing.T) {
	d := evalTool(t, "delete_path", `{"path":"a.txt"}`)
	if d.Kind != Ask || d.RuleID != RuleDeleteAsk {
		t.Fatalf("got %#v, want delete Ask", d)
	}
	// Even a session grant over the path does not silence the confirmation.
	policy := sandboxcore.MergePathPolicy(nil, sandboxcore.GrantForResolved("/workspace/a.txt", sandboxcore.PathWrite))
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:    "delete_path",
		Args:        json.RawMessage(`{"path":"a.txt"}`),
		ProjectRoot: "/workspace",
		POSIX:       true,
		PathPolicy:  policy,
	})
	if err != nil || d.Kind != Ask || d.RuleID != RuleDeleteAsk {
		t.Fatalf("with grant: %#v, %v", d, err)
	}
	// Hard denies still beat the confirmation.
	if d := evalTool(t, "delete_path", `{"path":"/etc/passwd"}`); d.Kind != Deny {
		t.Fatalf("sensitive delete: got %#v", d)
	}
}

func TestRulesBadArgsDeny(t *testing.T) {
	cases := map[string]string{
		"write_file":  `{}`,
		"move_path":   `{"from":"a"}`,
		"delete_path": `{}`,
		"apply_patch": `{"diff":"nonsense"}`,
		"append_file": `not json`,
	}
	for tool, args := range cases {
		if d := evalTool(t, tool, args); d.Kind != Deny || d.RuleID != "rules.bad_args" {
			t.Errorf("%s: got %#v, want bad_args Deny", tool, d)
		}
	}
}
