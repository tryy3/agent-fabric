package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/askuser"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/command"
)

const (
	permAllowOnce    = "allow_once"
	permAllowSession = "allow_session"
	permRejectOnce   = "reject_once"
)

func (a *Agent) sessionGrants(sessionID string) []sandbox.PathGrant {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.grants == nil {
		return nil
	}
	src := a.grants[sessionID]
	out := make([]sandbox.PathGrant, len(src))
	copy(out, src)
	return out
}

func (a *Agent) addSessionGrant(sessionID string, grant sandbox.PathGrant) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.grants == nil {
		a.grants = make(map[string][]sandbox.PathGrant)
	}
	a.grants[sessionID] = append(a.grants[sessionID], grant)
}

func (a *Agent) sessionCommandGrants(sessionID string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.cmdGrants[sessionID])
}

func (a *Agent) addSessionCommandGrant(sessionID, key string) {
	if key == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cmdGrants == nil {
		a.cmdGrants = make(map[string][]string)
	}
	if !slices.Contains(a.cmdGrants[sessionID], key) {
		a.cmdGrants[sessionID] = append(a.cmdGrants[sessionID], key)
	}
}

func (a *Agent) clientSupportsElicitationForm() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	caps := a.clientCaps
	if caps.Elicitation != nil && caps.Elicitation.Form != nil {
		return true
	}
	// acpd 1.0.0 has no typed elicitation capability; Flutter advertises via initialize meta.
	if a.clientMeta != nil {
		if elicitation, ok := a.clientMeta["elicitation"].(map[string]any); ok {
			if _, hasForm := elicitation["form"]; hasForm {
				return true
			}
		}
	}
	return false
}

func mergeOpenPolicy(opts sandbox.OpenOptions, grants []sandbox.PathGrant) sandbox.OpenOptions {
	if len(grants) == 0 {
		return opts
	}
	opts.PathPolicy = sandbox.MergePathPolicy(opts.PathPolicy, grants...)
	return opts
}

func gateRequest(
	toolName string,
	args json.RawMessage,
	opts sandbox.OpenOptions,
	commandGrants []string,
) gate.Request {
	posix := opts.Kind != "local"
	return gate.Request{
		ToolName:      toolName,
		Args:          args,
		ProjectRoot:   opts.ProjectRoot,
		POSIX:         posix,
		EnvKind:       opts.Kind,
		PathPolicy:    opts.PathPolicy,
		CommandGrants: commandGrants,
	}
}

// permissionRawInput is the ACP rawInput of a permission request. Command
// decisions add the argv and working directory so clients can show them.
func permissionRawInput(decision gate.Decision) map[string]any {
	raw := map[string]any{
		"reason": decision.Reason,
		"path":   decision.Path,
		"ruleId": decision.RuleID,
	}
	if len(decision.Command) > 0 {
		raw["command"] = decision.Command
		raw["cwd"] = decision.Cwd
		if decision.GrantKey != "" {
			raw["grantKey"] = decision.GrantKey
		}
	}
	return raw
}

func (a *Agent) runGatedTool(
	ctx context.Context,
	conn *acp.AgentSideConnection,
	sessionID acp.SessionId,
	callID string,
	toolName string,
	args json.RawMessage,
	opts sandbox.OpenOptions,
	env sandbox.Environment,
	registry *sandbox.Registry,
	open func(context.Context, sandbox.OpenOptions) (sandbox.Environment, error),
	chain gate.Chain,
) (string, error) {
	decision, err := chain.Evaluate(ctx, gateRequest(toolName, args, opts, a.sessionCommandGrants(string(sessionID))))
	if err != nil {
		return "", err
	}
	switch decision.Kind {
	case gate.Deny:
		return "", fmt.Errorf("denied: %s", decision.Reason)
	case gate.Ask:
		outcome, permErr := requestToolPermission(ctx, conn, sessionID, callID, toolName, decision)
		if permErr != nil {
			return "", permErr
		}
		if outcome == nil || outcome.Cancelled != nil {
			return "", fmt.Errorf("permission cancelled")
		}
		if outcome.Selected == nil {
			return "", fmt.Errorf("permission rejected")
		}
		switch string(outcome.Selected.OptionId) {
		case permRejectOnce:
			return "", fmt.Errorf("permission rejected: %s", decision.Reason)
		case permAllowOnce, permAllowSession:
			if toolName == command.Name {
				// Commands run in the existing environment: approval only
				// authorizes this call and, for a session grant, the command
				// prefix. Nothing to elevate.
				if string(outcome.Selected.OptionId) == permAllowSession {
					if decision.NoSessionGrant {
						return "", fmt.Errorf("session grant is not offered for this command")
					}
					a.addSessionCommandGrant(string(sessionID), decision.GrantKey)
				}
				return registry.Call(ctx, env, toolName, args)
			}
			grantPath := decision.Resolved
			if grantPath == "" {
				grantPath = decision.Path
			}
			grant := sandbox.GrantForResolved(grantPath, decision.Access)
			if string(outcome.Selected.OptionId) == permAllowSession {
				a.addSessionGrant(string(sessionID), grant)
			}
			elevOpts := mergeOpenPolicy(opts, []sandbox.PathGrant{grant})
			elevEnv, openErr := open(ctx, elevOpts)
			if openErr != nil {
				return "", fmt.Errorf("elevate sandbox: %w", openErr)
			}
			defer func() { _ = elevEnv.Close(context.Background()) }()
			return registry.Call(ctx, elevEnv, toolName, args)
		default:
			return "", fmt.Errorf("unknown permission option %q", outcome.Selected.OptionId)
		}
	default:
		return registry.Call(ctx, env, toolName, args)
	}
}

func requestToolPermission(
	ctx context.Context,
	conn *acp.AgentSideConnection,
	sessionID acp.SessionId,
	callID string,
	toolName string,
	decision gate.Decision,
) (*acp.RequestPermissionOutcome, error) {
	title, kind := toolPresentation(toolName)
	status := acp.ToolCallStatusPending
	resp, err := conn.RequestPermission(ctx, acp.RequestPermissionRequest{
		SessionId: sessionID,
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId(callID),
			Title:      &title,
			Kind:       &kind,
			Status:     &status,
			RawInput:   permissionRawInput(decision),
		},
		Options: permissionOptions(toolName, decision),
	})
	if err != nil {
		return nil, err
	}
	return &resp.Outcome, nil
}

func (a *Agent) runAskUser(
	ctx context.Context,
	conn *acp.AgentSideConnection,
	sessionID acp.SessionId,
	callID string,
	args json.RawMessage,
) (string, error) {
	if !a.clientSupportsElicitationForm() {
		return "", fmt.Errorf("ask_user requires client elicitation form support")
	}
	parsed, err := askuser.ParseAndValidate(args)
	if err != nil {
		return "", err
	}
	schema := elicitationSchema(parsed)
	msg := "Please answer the following question"
	if len(parsed.Questions) > 1 {
		msg = "Please answer the following questions"
	}
	req := acp.NewUnstableCreateElicitationRequestForm(schema)
	req.Form.Message = msg
	req.Form.Meta = map[string]any{
		"sessionId":  string(sessionID),
		"toolCallId": callID,
		"kind":       "ask_user",
		"questions":  parsed.Questions,
	}
	resp, err := conn.UnstableCreateElicitation(ctx, req)
	if err != nil {
		return "", err
	}
	if resp.Cancel != nil {
		return "", fmt.Errorf("ask_user cancelled")
	}
	if resp.Decline != nil {
		return "", fmt.Errorf("ask_user declined")
	}
	if resp.Accept == nil {
		return "", fmt.Errorf("ask_user missing response")
	}
	answers := normalizeAskUserAnswers(parsed, resp.Accept.Content)
	out, err := json.Marshal(map[string]any{"answers": answers})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func elicitationSchema(args askuser.Args) acp.UnstableElicitationSchema {
	props := make(map[string]any, len(args.Questions)*2)
	required := make([]string, 0, len(args.Questions))
	for _, q := range args.Questions {
		enums := make([]string, 0, len(q.Options)+1)
		for _, opt := range q.Options {
			enums = append(enums, opt.Label)
		}
		enums = append(enums, "Other")
		props[q.ID] = map[string]any{
			"type":        "string",
			"title":       q.Question,
			"description": q.Question,
			"enum":        enums,
		}
		props[q.ID+"_other"] = map[string]any{
			"type":        "string",
			"title":       q.Question + " (Other)",
			"description": "Free-text answer when Other is selected",
		}
		required = append(required, q.ID)
	}
	return acp.UnstableElicitationSchema{
		Type:       acp.UnstableElicitationSchemaTypeObject,
		Properties: props,
		Required:   required,
	}
}

func normalizeAskUserAnswers(args askuser.Args, content map[string]any) []map[string]string {
	out := make([]map[string]string, 0, len(args.Questions))
	for _, q := range args.Questions {
		selected, _ := content[q.ID].(string)
		other, _ := content[q.ID+"_other"].(string)
		answer := strings.TrimSpace(selected)
		if strings.EqualFold(answer, "Other") || answer == "other" {
			answer = strings.TrimSpace(other)
			if answer == "" {
				answer = "Other"
			}
		}
		out = append(out, map[string]string{
			"id":       q.ID,
			"question": q.Question,
			"answer":   answer,
		})
	}
	return out
}

// permissionOptions lists the choices offered for an Ask decision. Deletes are
// confirmed every time, including when the path escapes the project root (a
// different rule): a session grant would not silence the gate anyway and
// would widen later writes, so it is not offered.
func permissionOptions(toolName string, decision gate.Decision) []acp.PermissionOption {
	options := []acp.PermissionOption{
		{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow once", OptionId: acp.PermissionOptionId(permAllowOnce)},
	}
	if toolName != "delete_path" && !decision.NoSessionGrant {
		options = append(options, acp.PermissionOption{
			Kind: acp.PermissionOptionKindAllowAlways, Name: "Allow for this session", OptionId: acp.PermissionOptionId(permAllowSession),
		})
	}
	return append(options, acp.PermissionOption{
		Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId(permRejectOnce),
	})
}
