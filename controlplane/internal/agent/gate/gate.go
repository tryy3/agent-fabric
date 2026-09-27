// Package gate decides allow / ask / deny before sandbox tool execution.
package gate

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

// DecisionKind is the gate outcome for one tool call.
type DecisionKind string

const (
	Allow DecisionKind = "allow"
	Ask   DecisionKind = "ask"
	Deny  DecisionKind = "deny"
)

// Decision is the gate result for a tool call.
type Decision struct {
	Kind   DecisionKind
	Reason string
	RuleID string
	// Path is set when the decision concerns a filesystem path.
	Path string
	// Access is the FS access mode under consideration.
	Access sandboxcore.PathAccess
	// Resolved is the absolute path when preflight resolved one.
	Resolved string
}

// Request carries tool-call facts for evaluators.
type Request struct {
	ToolName      string
	Args          json.RawMessage
	WorkspaceRoot string
	// POSIX is true for docker/exec path checks; false for host (local) OS paths.
	POSIX      bool
	PathPolicy *sandboxcore.PathPolicy
}

// Evaluator inspects a tool call and returns a decision.
// Future classifier models implement this interface and join a Chain.
type Evaluator interface {
	Evaluate(ctx context.Context, req Request) (Decision, error)
}

// Chain runs evaluators in order. First Deny wins; else first Ask; else Allow.
type Chain struct {
	Evaluators []Evaluator
}

// Evaluate implements Evaluator.
func (c Chain) Evaluate(ctx context.Context, req Request) (Decision, error) {
	var ask Decision
	haveAsk := false
	for _, ev := range c.Evaluators {
		if ev == nil {
			continue
		}
		d, err := ev.Evaluate(ctx, req)
		if err != nil {
			return Decision{}, err
		}
		switch d.Kind {
		case Deny:
			return d, nil
		case Ask:
			if !haveAsk {
				ask = d
				haveAsk = true
			}
		case Allow, "":
			// continue
		default:
			return Decision{
				Kind:   Deny,
				Reason: "unknown gate decision " + string(d.Kind),
				RuleID: "gate.invalid",
			}, nil
		}
	}
	if haveAsk {
		return ask, nil
	}
	return Decision{Kind: Allow, RuleID: "gate.default_allow"}, nil
}

// DefaultChain returns the built-in rules evaluator (classifier slot empty).
func DefaultChain() Chain {
	return Chain{Evaluators: []Evaluator{Rules{}}}
}

// Rules is the hardcoded path / sensitivity policy.
type Rules struct{}

// Evaluate implements Evaluator.
func (Rules) Evaluate(_ context.Context, req Request) (Decision, error) {
	name := strings.TrimSpace(req.ToolName)
	switch name {
	case "ask_user":
		return Decision{Kind: Allow, RuleID: "rules.ask_user_skip"}, nil
	case "read_file", "write_file":
		return evaluateFileTool(req)
	default:
		return Decision{Kind: Allow, RuleID: "rules.unknown_allow"}, nil
	}
}

func evaluateFileTool(req Request) (Decision, error) {
	access := sandboxcore.PathRead
	if req.ToolName == "write_file" {
		access = sandboxcore.PathWrite
	}
	path, err := extractPath(req.Args)
	if err != nil {
		return Decision{
			Kind:   Deny,
			Reason: err.Error(),
			RuleID: "rules.bad_args",
		}, nil
	}
	if path == "" {
		return Decision{
			Kind:   Deny,
			Reason: req.ToolName + " path is required",
			RuleID: "rules.bad_args",
		}, nil
	}

	root := strings.TrimSpace(req.WorkspaceRoot)
	if root == "" {
		return Decision{
			Kind:   Deny,
			Reason: "workspace root is unavailable",
			RuleID: "rules.no_root",
		}, nil
	}

	candidate := sandboxcore.CandidateOS(root, path)
	if req.POSIX {
		candidate = sandboxcore.CandidatePOSIX(root, path)
	}

	var resolved string
	var violation *sandboxcore.AccessViolation
	if req.POSIX {
		resolved, violation = sandboxcore.CheckAccessPOSIX(root, path, req.PathPolicy, access)
	} else {
		resolved, violation = sandboxcore.CheckAccessOS(root, path, req.PathPolicy, access)
	}

	sensPath := candidate
	if resolved != "" {
		sensPath = resolved
	}
	if sensitiveDeny(sensPath, access) {
		return Decision{
			Kind:     Deny,
			Reason:   "path is blocked as sensitive: " + path,
			RuleID:   "rules.sensitive_deny",
			Path:     path,
			Access:   access,
			Resolved: resolved,
		}, nil
	}

	if violation == nil {
		return Decision{
			Kind:     Allow,
			RuleID:   "rules.in_policy",
			Path:     path,
			Access:   access,
			Resolved: resolved,
		}, nil
	}

	// Escape / not allowed / wrong mode → ask (user can elevate).
	return Decision{
		Kind:     Ask,
		Reason:   violation.Message,
		RuleID:   "rules.path_ask",
		Path:     path,
		Access:   access,
		Resolved: candidate,
	}, nil
}

func extractPath(args json.RawMessage) (string, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if len(args) == 0 {
		return "", nil
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Path), nil
}

// sensitivePrefixes are hard-denied for write/exec (and for any write-capable access).
var sensitivePrefixes = []string{
	"/etc",
	"/proc",
	"/sys",
	"/dev",
}

func sensitiveDeny(path string, access sandboxcore.PathAccess) bool {
	if access != sandboxcore.PathWrite && access != sandboxcore.PathExec {
		return false
	}
	clean := strings.TrimSpace(path)
	if clean == "" {
		return false
	}
	// Normalize for prefix checks (POSIX-style).
	clean = strings.ReplaceAll(clean, "\\", "/")
	for _, prefix := range sensitivePrefixes {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return true
		}
	}
	return false
}
