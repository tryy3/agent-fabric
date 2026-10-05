// Package gate decides allow / ask / deny before sandbox tool execution.
package gate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/command"
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
	// Command and Cwd are set for run_command decisions.
	Command []string
	Cwd     string
	// GrantKey is the command prefix an "Allow for this session" answer
	// remembers (run_command only).
	GrantKey string
	// NoSessionGrant withholds the "Allow for this session" option.
	NoSessionGrant bool
	// Risk is the 1–10 score (0 = unscored). Chain reports the highest score of
	// its evaluators; Policy.Resolve turns it into the final outcome.
	Risk Risk
	// Band is the Risk band, set by Policy.Resolve.
	Band Band
	// Source names the evaluator that produced the highest score.
	Source string
	// Rationale is the scoring evaluator's explanation (LLM scorers).
	Rationale string
	// Confidence is how sure the scoring evaluator is, 0–1 (0 = not reported).
	// System One scorers report it; a Cascade uses it to decide whether to ask
	// the next tier.
	Confidence float64
	// Scores lists every evaluator's score (Chain fills it), for transparency.
	Scores []Score
	// Overridden is set by Policy.Resolve when a rule asked but the mode runs
	// the call anyway.
	Overridden bool
	// Settled marks a rules decision of a tier the rules judge reliably: a
	// Cascade does not consult its scorers. The permission mode still applies.
	Settled bool
	// Pinned marks a decision made by a permission rule (UserRule): scorers do
	// not rescore it and no permission mode changes its verdict.
	Pinned bool
}

// Request carries tool-call facts for evaluators.
type Request struct {
	ToolName    string
	Args        json.RawMessage
	ProjectRoot string
	// POSIX is true for docker/exec path checks; false for host (local) OS paths.
	POSIX bool
	// EnvKind is the sandbox kind ("docker" or "local"). run_command runs
	// only in "docker".
	EnvKind    string
	PathPolicy *sandboxcore.PathPolicy
	// CommandGrants are the session's remembered run_command grant keys.
	CommandGrants []string
	// UserIntent is what the user asked for, when known. Scorers use it to
	// tell a requested delete from an unrequested one.
	UserIntent string
	// Tainted is set once the session has read web content, which may carry
	// instructions planted for the agent. Scorers are told, and a Cascade does
	// not let its deep tier lower a score.
	Tainted bool
	// Prior are the scores earlier tiers of a Cascade gave this call. A scorer
	// may weigh them; they are hints, not limits.
	Prior []Score
}

// Evaluator inspects a tool call and returns a decision.
// Future classifier models implement this interface and join a Chain.
type Evaluator interface {
	Evaluate(ctx context.Context, req Request) (Decision, error)
}

// Chain runs evaluators in order. First Deny wins; else first Ask; else Allow.
// The returned Risk is the highest score any evaluator gave, so a scorer can
// raise a rule's Allow into an ask or a cancel but never lower a rule's Ask.
type Chain struct {
	Evaluators []Evaluator
}

// Evaluate implements Evaluator.
func (c Chain) Evaluate(ctx context.Context, req Request) (Decision, error) {
	var ask, top Decision
	var scores []Score
	haveAsk := false
	for _, ev := range c.Evaluators {
		if ev == nil {
			continue
		}
		d, err := ev.Evaluate(ctx, req)
		if err != nil {
			return Decision{}, err
		}
		scores = appendScore(scores, d)
		if d.Risk > top.Risk {
			top = d
		}
		switch d.Kind {
		case Deny:
			return withTop(d, top, scores), nil
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
		return withTop(ask, top, scores), nil
	}
	return withTop(Decision{Kind: Allow, RuleID: "gate.default_allow"}, top, scores), nil
}

// withTop copies the highest-scoring decision's score and explanation onto d.
func withTop(d, top Decision, scores []Score) Decision {
	d.Scores = scores
	if top.Risk > d.Risk {
		d.Risk, d.Source, d.Rationale, d.Confidence = top.Risk, top.Source, top.Rationale, top.Confidence
		if d.Reason == "" {
			d.Reason = top.Reason
		}
	}
	return d
}

// DefaultChain returns the built-in rules evaluator (classifier slot empty).
func DefaultChain() Chain {
	return Chain{Evaluators: []Evaluator{Rules{}}}
}

// Rules is the deterministic policy: the built-in path, sensitivity and
// command tiers, then the user's permission rules on top.
type Rules struct {
	// User are permission rules from settings. They override the built-in
	// tiers but never a built-in deny.
	User []UserRule
	// Builtins overrides built-in tiers by ID: base score, whether scorers
	// are consulted, and the programs of a program-list tier.
	Builtins map[string]TierOverride
}

// Evaluate implements Evaluator.
func (r Rules) Evaluate(_ context.Context, req Request) (Decision, error) {
	rs := defaultRuleset
	if len(r.Builtins) > 0 {
		rs = newRuleset(r.Builtins)
	}
	d, err := r.evaluate(req, rs)
	if err != nil {
		return d, err
	}
	if d.Risk == 0 {
		d.Risk = ruleRisk(d)
	}
	d = rs.finish(d)
	d = applyUserRules(r.User, req, d)
	d.Source = "rules"
	return d, nil
}

func (Rules) evaluate(req Request, rs *ruleset) (Decision, error) {
	name := strings.TrimSpace(req.ToolName)
	switch name {
	case "ask_user":
		return Decision{Kind: Allow, RuleID: "rules.ask_user_skip"}, nil
	case command.Name:
		return evaluateCommand(req, rs)
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
		d := Decision{
			Kind:     Allow,
			RuleID:   "rules.in_policy",
			Path:     path,
			Access:   access,
			Resolved: resolved,
		}
		write := access == sandboxcore.PathWrite
		switch {
		case write && runsLaterPayload(req, path):
			d.Kind, d.RuleID, d.Risk = Ask, RuleRunsLater, riskRunsLaterPayload
			d.Reason = "writes a file that runs later, and its content downloads and runs code or wipes the system: " + path
		case write && startupFile(path):
			d.Kind, d.RuleID, d.Risk = Ask, RuleSecretPath, riskStartupWrite
			d.Reason = "startup files run later with your privileges: " + path
		case write && deceptiveName(path):
			d.Kind, d.RuleID, d.Risk = Ask, RuleOddPath, riskDeceptiveName
			d.Reason = "file name contains control or text-direction characters"
		case write && secretPath(path):
			d.Kind, d.RuleID, d.Risk = Ask, RuleSecretPath, riskSecretWrite
			d.Reason = "writes a secret file: " + path
		case secretPath(path):
			d.RuleID, d.Risk = RuleSecretPath, riskSecretRead
			d.Reason = "reads a secret file: " + path
		case write && runsLaterFile(path):
			d.RuleID, d.Risk = RuleRunsLater, riskRunsLater
			d.Reason = "writes a file that runs later: " + path
		case write && strings.HasPrefix(pathBase(path), "-"):
			d.RuleID, d.Risk = RuleOddPath, riskOddName
			d.Reason = "file name starts with a dash and reads as an option to later commands"
		}
		return d
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

// Rule IDs for in-project paths that still deserve attention.
const (
	RuleSecretPath = "rules.secret_path"
	RuleOddPath    = "rules.odd_path"
	RuleRunsLater  = "rules.runs_later"
)

var runsLaterNames = map[string]bool{
	"Makefile": true, "makefile": true, "GNUmakefile": true, "Dockerfile": true, "Justfile": true, "justfile": true,
	".gitlab-ci.yml": true, "Jenkinsfile": true, "Taskfile.yml": true,
}

// manifestNames hold install or build hooks among ordinary data; they are
// edited constantly, so only a dangerous payload makes them stand out.
var manifestNames = map[string]bool{"package.json": true, "pyproject.toml": true, "setup.py": true, "Cargo.toml": true}

// runsLaterFile reports files a later build, CI run or git operation executes:
// CI workflows, git hook directories, build files and shell scripts.
func runsLaterFile(p string) bool {
	clean := strings.ReplaceAll(p, "\\", "/")
	base := pathBase(p)
	if runsLaterNames[base] || strings.HasSuffix(base, ".sh") {
		return true
	}
	for _, dir := range []string{".github/workflows/", ".husky/", ".githooks/", ".circleci/"} {
		if strings.HasPrefix(clean, dir) || strings.Contains(clean, "/"+dir) {
			return true
		}
	}
	return false
}

// runsLaterPayload reports a write to a runs-later file or package manifest
// whose content would download and run code or wipe the system when it runs.
func runsLaterPayload(req Request, p string) bool {
	if !runsLaterFile(p) && !manifestNames[pathBase(p)] {
		return false
	}
	var payload struct {
		Content string `json:"content"`
		Diff    string `json:"diff"`
	}
	if json.Unmarshal(req.Args, &payload) != nil {
		return false
	}
	text := payload.Content + "\n" + payload.Diff
	return fetchExecText.MatchString(text) || catastrophicText.MatchString(text)
}

func pathBase(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, "\\", "/"), "/")
	return p[strings.LastIndex(p, "/")+1:]
}

var secretNames = map[string]bool{
	"id_rsa": true, "id_ed25519": true, "id_ecdsa": true, "id_dsa": true,
	".npmrc": true, ".netrc": true, ".pgpass": true, ".pypirc": true, "credentials": true,
}

var secretDirs = map[string]bool{".ssh": true, ".aws": true, ".gnupg": true, ".kube": true}

// secretPath reports a path that names credentials: .env files, private keys,
// and anything under .ssh, .aws, .gnupg or .kube.
func secretPath(p string) bool {
	segs := strings.Split(strings.ReplaceAll(p, "\\", "/"), "/")
	base := pathBase(p)
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	if secretNames[base] && (base != "credentials" || len(segs) > 1 && secretDirs[segs[len(segs)-2]]) {
		return true
	}
	for _, s := range segs[:len(segs)-1] {
		if secretDirs[s] {
			return true
		}
	}
	return false
}

var startupNames = map[string]bool{
	".bashrc": true, ".bash_profile": true, ".bash_login": true, ".profile": true,
	".zshrc": true, ".zshenv": true, ".zprofile": true, ".gitconfig": true,
}

// startupFile reports a shell or git startup file.
func startupFile(p string) bool { return startupNames[pathBase(p)] }

// deceptiveName reports control characters or Unicode direction overrides,
// which make a file name display as something it is not.
func deceptiveName(p string) bool {
	for _, r := range p {
		if r < 0x20 || r == 0x7f || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return true
		}
	}
	return false
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

// Score is one evaluator's contribution to a chain decision.
type Score struct {
	Source    string
	Risk      Risk
	RuleID    string
	Rationale string
	// Confidence is 0–1 when the evaluator reports one (0 = not reported).
	Confidence float64
}
