// Package gate decides allow / ask / deny before sandbox tool execution.
package gate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file/udiff"
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
	ToolName    string
	Args        json.RawMessage
	ProjectRoot string
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
	case "read_file", "list_files", "search_text":
		return evaluatePaths(req, sandboxcore.PathRead, optionalPath)
	case "write_file", "append_file", "create_directory":
		return evaluatePaths(req, sandboxcore.PathWrite, singlePath)
	case "apply_patch":
		return evaluatePaths(req, sandboxcore.PathWrite, patchPaths)
	case "move_path":
		return evaluatePaths(req, sandboxcore.PathWrite, movePaths)
	case "delete_path":
		d, err := evaluatePaths(req, sandboxcore.PathWrite, singlePath)
		if err != nil || d.Kind != Allow {
			return d, err
		}
		// Deleting is the least reversible edit: always confirm, even
		// inside the project root.
		d.Kind = Ask
		d.RuleID = RuleDeleteAsk
		d.Reason = "delete requires confirmation: " + d.Path
		return d, nil
	default:
		return Decision{Kind: Allow, RuleID: "rules.unknown_allow"}, nil
	}
}

// RuleDeleteAsk is the RuleID of the always-confirm decision for delete_path.
const RuleDeleteAsk = "rules.delete_ask"

// pathExtractor returns the paths a tool call touches.
type pathExtractor func(req Request) ([]string, error)

func singlePath(req Request) ([]string, error) {
	p, err := extractPath(req.Args)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return nil, errors.New(req.ToolName + " path is required")
	}
	return []string{p}, nil
}

// optionalPath is for read tools whose path defaults to the project root.
func optionalPath(req Request) ([]string, error) {
	p, err := extractPath(req.Args)
	if err != nil {
		return nil, err
	}
	if p == "" {
		p = "."
	}
	return []string{p}, nil
}

func movePaths(req Request) ([]string, error) {
	var payload struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal(req.Args, &payload); err != nil {
		return nil, err
	}
	from, to := strings.TrimSpace(payload.From), strings.TrimSpace(payload.To)
	if from == "" || to == "" {
		return nil, errors.New("move_path from and to are required")
	}
	return []string{from, to}, nil
}

func patchPaths(req Request) ([]string, error) {
	var payload struct {
		Diff string `json:"diff"`
	}
	if err := json.Unmarshal(req.Args, &payload); err != nil {
		return nil, err
	}
	files, err := udiff.Parse(payload.Diff)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths, nil
}

// evaluatePaths checks every path a call touches. Any Deny wins, then the
// first Ask, else Allow.
func evaluatePaths(req Request, access sandboxcore.PathAccess, extract pathExtractor) (Decision, error) {
	paths, err := extract(req)
	if err != nil {
		return Decision{Kind: Deny, Reason: err.Error(), RuleID: "rules.bad_args"}, nil
	}
	var ask, allow Decision
	haveAsk, haveAllow := false, false
	for _, p := range paths {
		d := evaluatePath(req, p, access)
		switch d.Kind {
		case Deny:
			return d, nil
		case Ask:
			if !haveAsk {
				ask, haveAsk = d, true
			}
		default:
			if !haveAllow {
				allow, haveAllow = d, true
			}
		}
	}
	if haveAsk {
		return ask, nil
	}
	return allow, nil
}

func evaluatePath(req Request, path string, access sandboxcore.PathAccess) Decision {
	root := strings.TrimSpace(req.ProjectRoot)
	if root == "" {
		return Decision{
			Kind:   Deny,
			Reason: "workspace root is unavailable",
			RuleID: "rules.no_root",
		}
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
		}
	}
	if access == sandboxcore.PathWrite && gitMetadata(root, candidate, resolved) {
		return Decision{
			Kind:     Deny,
			Reason:   "repository metadata is protected: " + path,
			RuleID:   "rules.git_protected",
			Path:     path,
			Access:   access,
			Resolved: resolved,
		}
	}

	if violation == nil {
		return Decision{
			Kind:     Allow,
			RuleID:   "rules.in_policy",
			Path:     path,
			Access:   access,
			Resolved: resolved,
		}
	}

	// Escape / not allowed / wrong mode → ask (user can elevate).
	return Decision{
		Kind:     Ask,
		Reason:   violation.Message,
		RuleID:   "rules.path_ask",
		Path:     path,
		Access:   access,
		Resolved: candidate,
	}
}

// gitMetadata reports whether either path is the project's .git directory or
// inside it.
func gitMetadata(root string, paths ...string) bool {
	root = strings.TrimRight(strings.ReplaceAll(root, "\\", "/"), "/")
	for _, p := range paths {
		p = strings.ReplaceAll(p, "\\", "/")
		if rel, ok := strings.CutPrefix(p, root+"/"); ok && (rel == ".git" || strings.HasPrefix(rel, ".git/")) {
			return true
		}
	}
	return false
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
