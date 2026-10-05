package gate

import (
	"fmt"
	"strings"

	"github.com/tryy3/agent-fabric/internal/sandbox/tools/command"
)

// Actions of a permission rule.
const (
	RuleAllow = "allow"
	RuleAsk   = "ask"
	RuleDeny  = "deny"
)

// UserRuleActions lists every action, most to least restrictive.
var UserRuleActions = []string{RuleDeny, RuleAsk, RuleAllow}

// Rule IDs of decisions made by a permission rule.
const (
	RuleUserAllow = "rules.user_allow"
	RuleUserAsk   = "rules.user_ask"
	RuleUserDeny  = "rules.user_deny"
)

// UserRule is one permission rule from settings: calls of Tool that match
// Match always run, always ask, or are always refused.
//
// For run_command, Match is compared with the argv token by token: "git push
// *" matches "git push" followed by anything, "npm run ?*" any npm script.
// Within a token * matches any characters and ? one. For file tools, Match is a
// glob over the path relative to the project root, where * stays inside one
// path segment and ** crosses them ("**/.env*", "docs/**").
type UserRule struct {
	// Tool is a tool name, or "*" for every tool that has a path or command.
	Tool   string `json:"tool"`
	Match  string `json:"match"`
	Action string `json:"action"`
	// Risk overrides the score the action implies (allow 1, deny 10, ask keeps
	// the built-in score).
	Risk Risk `json:"risk,omitempty"`
}

// Validate reports a rule that can never apply.
func (r UserRule) Validate() error {
	if strings.TrimSpace(r.Tool) == "" {
		return fmt.Errorf("permission rule: tool is required")
	}
	if strings.TrimSpace(r.Match) == "" {
		return fmt.Errorf("permission rule: match is required")
	}
	switch r.Action {
	case RuleAllow, RuleAsk, RuleDeny:
	default:
		return fmt.Errorf("permission rule: action %q must be allow, ask or deny", r.Action)
	}
	if r.Risk != 0 && (r.Risk < MinRisk || r.Risk > MaxRisk) {
		return fmt.Errorf("permission rule: risk %d outside 1-10", r.Risk)
	}
	return nil
}

// applyUserRules overrides a built-in decision with the most restrictive
// matching rule: deny, then ask, then allow. Built-in denies are never
// relaxed. A rule's decision is Pinned: no scorer or mode changes it.
func applyUserRules(rules []UserRule, req Request, d Decision) Decision {
	if len(rules) == 0 || d.Kind == Deny {
		return d
	}
	var hit *UserRule
	rank := func(action string) int {
		for i, a := range UserRuleActions {
			if a == action {
				return i
			}
		}
		return len(UserRuleActions)
	}
	for i := range rules {
		r := &rules[i]
		if r.Validate() != nil || !r.matches(req) {
			continue
		}
		if hit == nil || rank(r.Action) < rank(hit.Action) {
			hit = r
		}
	}
	if hit == nil {
		return d
	}
	d.Pinned = true
	d.Reason = fmt.Sprintf("permission rule %q: %s", hit.Match, hit.Action)
	switch hit.Action {
	case RuleDeny:
		d.Kind, d.RuleID, d.Risk = Deny, RuleUserDeny, riskForbidden
	case RuleAsk:
		if d.Risk == 0 {
			d.Risk = ruleRisk(d)
		}
		d.Kind, d.RuleID, d.NoSessionGrant = Ask, RuleUserAsk, true
	case RuleAllow:
		// A path the sandbox would refuse still needs elevating for this call.
		d.Overridden = d.Kind == Ask && d.Path != "" && d.RuleID == "rules.path_ask"
		d.Kind, d.RuleID, d.Risk = Allow, RuleUserAllow, riskReadInProject
	}
	if hit.Risk > 0 {
		d.Risk = hit.Risk
	}
	return d
}

func (r UserRule) matches(req Request) bool {
	tool := strings.TrimSpace(req.ToolName)
	if r.Tool != "*" && r.Tool != tool {
		return false
	}
	if tool == command.Name {
		args, err := command.Decode(req.Args)
		if err != nil {
			return false
		}
		prog, rest := normalizeProgram(args.Command)
		return matchArgv(strings.Fields(r.Match), append([]string{prog}, rest...))
	}
	paths := toolPaths(req)
	if len(paths) == 0 {
		return false
	}
	// Allowing needs every touched path to match; asking or denying needs one.
	for _, p := range paths {
		ok := globMatch(r.Match, projectRelative(req.ProjectRoot, p), true)
		if ok && r.Action != RuleAllow {
			return true
		}
		if !ok && r.Action == RuleAllow {
			return false
		}
	}
	return r.Action == RuleAllow
}

// toolPaths returns the paths a file tool call touches, or nil.
func toolPaths(req Request) []string {
	var extract pathExtractor
	switch strings.TrimSpace(req.ToolName) {
	case "read_file", "list_files", "search_text":
		extract = optionalPath
	case "write_file", "append_file", "create_directory", "delete_path":
		extract = singlePath
	case "apply_patch":
		extract = patchPaths
	case "move_path":
		extract = movePaths
	default:
		return nil
	}
	paths, err := extract(req)
	if err != nil {
		return nil
	}
	return paths
}

// projectRelative strips the project root from an absolute path inside it and
// a leading "./" from a relative one.
func projectRelative(root, p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	root = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(root), "\\", "/"), "/")
	if root != "" {
		if rel, ok := strings.CutPrefix(p, root+"/"); ok {
			return rel
		}
	}
	return strings.TrimPrefix(p, "./")
}

// matchArgv matches pattern tokens against an argv. A final "*" token matches
// any remaining arguments, including none.
func matchArgv(pattern, argv []string) bool {
	if len(pattern) == 0 {
		return false
	}
	open := pattern[len(pattern)-1] == "*"
	if open {
		pattern = pattern[:len(pattern)-1]
	}
	if len(argv) < len(pattern) || (!open && len(argv) != len(pattern)) {
		return false
	}
	for i, tok := range pattern {
		if !globMatch(tok, argv[i], false) {
			return false
		}
	}
	return true
}

// globMatch matches s against a pattern of literals, ? (one character) and *
// (any run). With segments set, * and ? stop at "/" and ** crosses it.
func globMatch(pattern, s string, segments bool) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			deep := !segments || strings.HasPrefix(pattern, "**")
			pattern = strings.TrimLeft(pattern, "*")
			if deep {
				// "**/" also matches no directory at all.
				if segments && strings.HasPrefix(pattern, "/") && globMatch(pattern[1:], s, segments) {
					return true
				}
			}
			for i := 0; i <= len(s); i++ {
				if globMatch(pattern, s[i:], segments) {
					return true
				}
				if i < len(s) && !deep && s[i] == '/' {
					break
				}
			}
			return false
		case '?':
			if s == "" || (segments && s[0] == '/') {
				return false
			}
		default:
			if s == "" || s[0] != pattern[0] {
				return false
			}
		}
		pattern, s = pattern[1:], s[1:]
	}
	return s == ""
}
