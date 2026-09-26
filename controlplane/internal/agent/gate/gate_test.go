package gate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

func TestRulesInWorkspaceAllow(t *testing.T) {
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:      "read_file",
		Args:          json.RawMessage(`{"path":"a.txt"}`),
		WorkspaceRoot: "/workspace",
		POSIX:         true,
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
		ToolName:      "read_file",
		Args:          json.RawMessage(`{"path":"/tmp/outside"}`),
		WorkspaceRoot: "/workspace",
		POSIX:         true,
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
		ToolName:      "write_file",
		Args:          json.RawMessage(`{"path":"/etc/passwd","content":"x"}`),
		WorkspaceRoot: "/workspace",
		POSIX:         true,
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
		ToolName:      "read_file",
		Args:          json.RawMessage(`{"path":"a.txt"}`),
		WorkspaceRoot: "/workspace",
		POSIX:         true,
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
		ToolName:      "write_file",
		Args:          json.RawMessage(`{"path":"a.txt","content":"x"}`),
		WorkspaceRoot: "/workspace",
		POSIX:         true,
		PathPolicy:    policy,
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
