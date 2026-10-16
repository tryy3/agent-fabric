package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/askuser"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/command"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/web"
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

// gateContext is what the gate is told about the turn besides the call itself.
type gateContext struct {
	// Intent is the user's prompt of this turn.
	Intent string
	// Tainted is set once the session has read web content.
	Tainted bool
	// Instructions are the session's pinned gate instructions.
	Instructions string
}

// maxIntentChars bounds the user request shown to gate scorers.
const maxIntentChars = 2000

func gateRequest(
	toolName string,
	args json.RawMessage,
	opts sandbox.OpenOptions,
	commandGrants []string,
	gc gateContext,
) gate.Request {
	intent := strings.TrimSpace(gc.Intent)
	if len(intent) > maxIntentChars {
		intent = strings.ToValidUTF8(intent[:maxIntentChars], "") + "…"
	}
	posix := opts.Kind != "local"
	return gate.Request{
		ToolName:         toolName,
		Args:             args,
		ProjectRoot:      opts.ProjectRoot,
		POSIX:            posix,
		EnvKind:          opts.Kind,
		PathPolicy:       opts.PathPolicy,
		CommandGrants:    commandGrants,
		UserIntent:       intent,
		Tainted:          gc.Tainted,
		GateInstructions: gc.Instructions,
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
	if decision.Risk > 0 {
		raw["risk"] = decision.Risk
		raw["band"] = string(gate.BandOf(decision.Risk))
	}
	if decision.Rationale != "" {
		raw["rationale"] = decision.Rationale
	}
	if decision.Source != "" {
		raw["source"] = decision.Source
	}
	if decision.Floor > 0 {
		raw["floor"] = decision.Floor
	}
	if scores := scoresMeta(decision.Scores); scores != nil {
		raw["scores"] = scores
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
	ev gate.Evaluator,
	policy gate.Policy,
	gc gateContext,
	tr *gateTrace,
) (string, error) {
	req := gateRequest(toolName, args, opts, a.sessionCommandGrants(string(sessionID)), gc)
	if toolName == command.Name {
		req.ReferencedLoader = func(ctx context.Context) []gate.FileExcerpt {
			tr.Referenced = gate.LoadReferenced(ctx, req, env)
			return tr.Referenced
		}
	}
	decision, err := ev.Evaluate(ctx, req)
	if err != nil {
		return "", err
	}
	decision = policy.Resolve(decision)
	tr.Decision, tr.Evaluated = decision, true
	switch decision.Kind {
	case gate.Deny:
		tr.Outcome = outcomeDenied
		if decision.Risk >= policy.CancelAt {
			tr.Outcome = outcomeCancelled
		}
		return "", fmt.Errorf("denied: %s", decision.Reason)
	case gate.Ask:
		outcome, permErr := requestToolPermission(ctx, conn, sessionID, callID, toolName, decision)
		if permErr != nil {
			return "", permErr
		}
		if outcome == nil || outcome.Cancelled != nil {
			tr.Outcome = outcomeDismissed
			return "", fmt.Errorf("permission cancelled")
		}
		if outcome.Selected == nil {
			tr.Outcome = outcomeRejected
			return "", fmt.Errorf("permission rejected")
		}
		switch string(outcome.Selected.OptionId) {
		case permRejectOnce:
			tr.Outcome = outcomeRejected
			return "", fmt.Errorf("permission rejected: %s", decision.Reason)
		case permAllowOnce, permAllowSession:
			tr.Outcome = outcomeApproved
			if string(outcome.Selected.OptionId) == permAllowSession {
				tr.Outcome = outcomeApprovedSession
			}
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
		tr.Outcome = outcomeAllowed
		if decision.Overridden && decision.Path != "" && toolName != command.Name {
			// The mode runs a call the jail would still block: elevate for
			// this call only, without prompting or remembering a grant.
			grantPath := decision.Resolved
			if grantPath == "" {
				grantPath = decision.Path
			}
			grant := sandbox.GrantForResolved(grantPath, decision.Access)
			elevEnv, openErr := open(ctx, mergeOpenPolicy(opts, []sandbox.PathGrant{grant}))
			if openErr != nil {
				return "", fmt.Errorf("elevate sandbox: %w", openErr)
			}
			defer func() { _ = elevEnv.Close(context.Background()) }()
			return registry.Call(ctx, elevEnv, toolName, args)
		}
		return registry.Call(ctx, env, toolName, args)
	}
}

// policyFor returns the gate policy of a permission mode under the session's
// pinned thresholds, tightened once the session has read web content.
func (a *Agent) policyFor(pin runtime.SessionPin, mode gate.Mode, tainted bool) gate.Policy {
	policy := pinnedPolicies(pin).PolicyFor(mode)
	if tainted {
		policy = policy.Tainted()
	}
	return policy
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

// Gate outcomes reported to clients on a tool call.
const (
	outcomeAllowed         = "allowed"          // ran without asking
	outcomeApproved        = "approved"         // the user allowed it once
	outcomeApprovedSession = "approved_session" // the user allowed it for the session
	outcomeRejected        = "rejected"         // the user said no
	outcomeDismissed       = "dismissed"        // the prompt was cancelled
	outcomeDenied          = "denied"           // a hard rule refused it
	outcomeCancelled       = "cancelled"        // the risk score reached the cancel threshold
)

// gateTrace records what the gate decided for one tool call, for the client.
// It is shown to the user and stored with the transcript but never sent to the
// model: only the tool result text is.
type gateTrace struct {
	Mode      gate.Mode
	Decision  gate.Decision
	Evaluated bool
	Outcome   string
	// Tainted records that the session had read web content when the call was gated.
	Tainted bool
	// Referenced are the files shown to the scorers next to a command.
	Referenced []gate.FileExcerpt
}

// meta is the ACP `_meta.gate` object of the tool call update (also stored on
// the persisted tool_call part). Nil when the gate did not evaluate the call.
func (t gateTrace) meta() map[string]any {
	if !t.Evaluated {
		return nil
	}
	d := t.Decision
	m := map[string]any{
		"mode":    string(t.Mode),
		"outcome": t.Outcome,
		"verdict": string(d.Kind),
		"ruleId":  d.RuleID,
	}
	if d.Risk > 0 {
		m["risk"] = d.Risk
		m["band"] = string(gate.BandOf(d.Risk))
	}
	if d.Reason != "" {
		m["reason"] = d.Reason
	}
	if d.Rationale != "" {
		m["rationale"] = d.Rationale
	}
	if d.Source != "" {
		m["source"] = d.Source
	}
	if d.Floor > 0 {
		m["floor"] = d.Floor
	}
	if scores := scoresMeta(d.Scores); scores != nil {
		m["scores"] = scores
	}
	if d.Pinned {
		m["userRule"] = true
	}
	if t.Tainted {
		m["tainted"] = true
	}
	// Paths only: the content went to the scorers, not into the transcript.
	if shown, withheld := gate.ReferencedPaths(t.Referenced); len(shown)+len(withheld) > 0 {
		if len(shown) > 0 {
			m["referenced"] = shown
		}
		if len(withheld) > 0 {
			m["referencedWithheld"] = withheld
		}
	}
	if omitted := gate.OmittedPaths(t.Referenced); len(omitted) > 0 {
		m["referencedOmitted"] = len(omitted)
	}
	return m
}

// scoresMeta lists every gate tier's score in evaluation order, or nil.
func scoresMeta(in []gate.Score) []map[string]any {
	if len(in) == 0 {
		return nil
	}
	scores := make([]map[string]any, 0, len(in))
	for _, s := range in {
		e := map[string]any{"source": s.Source, "risk": s.Risk}
		if s.RuleID != "" {
			e["ruleId"] = s.RuleID
		}
		if s.Rationale != "" {
			e["rationale"] = s.Rationale
		}
		if s.Confidence > 0 {
			e["confidence"] = s.Confidence
		}
		if s.Floor > 0 {
			e["floor"] = s.Floor
		}
		if s.Undecided {
			e["undecided"] = true
		}
		scores = append(scores, e)
	}
	return scores
}

// pinnedPolicies are the built-in mode thresholds with the session's pinned
// plane-wide overrides.
func pinnedPolicies(pin runtime.SessionPin) gate.Policies {
	if len(pin.Gate.Policies) == 0 {
		return gate.DefaultPolicies
	}
	over := make(map[gate.Mode]gate.Policy, len(pin.Gate.Policies))
	for name, t := range pin.Gate.Policies {
		over[gate.Mode(name)] = gate.Policy{AskAt: gate.Risk(t.AskAt), CancelAt: gate.Risk(t.CancelAt)}
	}
	return gate.DefaultPolicies.With(over)
}

// modeFor returns the permission mode of a pin.
func modeFor(pin runtime.SessionPin) gate.Mode {
	mode, err := gate.ParseMode(pin.PermissionMode)
	if err != nil {
		return gate.DefaultMode
	}
	return mode
}

// currentMode returns the session's permission mode as it is now, which
// session/set_mode may have changed since pin was read.
func (a *Agent) currentMode(sessionID string, pin runtime.SessionPin) gate.Mode {
	if sess, ok := a.store.Get(sessionID); ok {
		return modeFor(sess.Pin)
	}
	return modeFor(pin)
}

// gatePin resolves the effective permissions (plane-wide default, then the
// assistant's) into the session's gate configuration, looking up the scorers'
// inference connections so keys stay in the catalog.
func (a *Agent) gatePin(ctx context.Context, ag catalog.Assistant, assistant catalog.Permissions) (runtime.GatePin, error) {
	plane, err := a.catalog.PlanePermissions(ctx)
	if err != nil {
		return runtime.GatePin{}, fmt.Errorf("plane permissions: %w", err)
	}
	planeInstructions, err := a.catalog.PlaneGateInstructions(ctx)
	if err != nil {
		return runtime.GatePin{}, err
	}
	eff := catalog.EffectivePermissions(plane, assistant)
	pin := runtime.GatePin{Instructions: catalog.EffectiveGateInstructions(planeInstructions, ag.GateInstructions)}
	for mode, p := range eff.Policies {
		if pin.Policies == nil {
			pin.Policies = map[string]runtime.PolicyThresholds{}
		}
		pin.Policies[mode] = runtime.PolicyThresholds{AskAt: p.AskAt, CancelAt: p.CancelAt}
	}
	for _, r := range eff.Rules {
		pin.Rules = append(pin.Rules, runtime.PermissionRule{Tool: r.Tool, Match: r.Match, Action: r.Action, Risk: r.Risk, Consult: r.Consult})
	}
	for id, b := range eff.Builtins {
		if pin.Builtins == nil {
			pin.Builtins = map[string]runtime.BuiltinOverride{}
		}
		pin.Builtins[id] = runtime.BuiltinOverride{Risk: b.Risk, Consult: b.Consult, Add: b.Add, Remove: b.Remove}
	}
	if q := eff.Questions; q != nil {
		for id, o := range q.Builtins {
			if pin.Questions.Builtins == nil {
				pin.Questions.Builtins = map[string]runtime.QuestionOverride{}
			}
			pin.Questions.Builtins[id] = runtime.QuestionOverride{Enabled: o.Enabled, Risk: o.Risk, Text: o.Text}
		}
		for _, c := range q.Custom {
			pin.Questions.Custom = append(pin.Questions.Custom, runtime.CustomQuestion{
				ID: c.ID, Tools: c.Tools, Kind: c.Kind, Text: c.Text, Risk: c.Risk, Enabled: c.Enabled,
			})
		}
	}
	if eff.Scorers == nil {
		return pin, nil
	}
	pin.InstructionsMaxChars = eff.Scorers.InstructionsMaxChars
	resolve := func(name string, sc *catalog.PermissionScorer) (*runtime.GateScorer, error) {
		if sc == nil {
			return nil, nil
		}
		conn, err := a.catalog.GetInferenceConnection(ctx, sc.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("gate %s scorer connection %q: %w", name, sc.ConnectionID, err)
		}
		baseURL := conn.BaseURL
		if baseURL == "" {
			baseURL = catalog.FixedBaseURL(conn.Type)
		}
		return &runtime.GateScorer{
			ConnectionType: conn.Type, BaseURL: baseURL, APIKey: conn.APIKey, Model: sc.Model, Strategy: sc.Strategy,
			Style: sc.Style, EnableThinking: sc.EnableThinking, ReasoningEffort: sc.ReasoningEffort,
			MaxTokens: sc.MaxTokens, StructuredOutput: sc.StructuredOutput,
		}, nil
	}
	if pin.Fast, err = resolve("fast", eff.Scorers.Fast); err != nil {
		return runtime.GatePin{}, err
	}
	if pin.Deep, err = resolve("deep", eff.Scorers.Deep); err != nil {
		return runtime.GatePin{}, err
	}
	pin.MinConfidence, pin.MaxLower, pin.SkipAtOrBelow = eff.Scorers.MinConfidence, eff.Scorers.MaxLower, eff.Scorers.SkipAtOrBelow
	return pin, nil
}

// gateFor builds the session's tool gate from its pin: the rules with the
// user's permission rules, then the optional fast (System One) and deep (chat
// model) scorers as a cascade. With no scorer configured it is rules only.
// onCapture persists each scorer exchange.
func (a *Agent) gateFor(pin runtime.SessionPin, sessionID string, onCapture func(provider.HopCapture)) gate.Evaluator {
	g := pin.Gate
	rules := gate.Rules{}
	for _, r := range g.Rules {
		rules.User = append(rules.User, gate.UserRule{Tool: r.Tool, Match: r.Match, Action: r.Action, Risk: r.Risk, Consult: r.Consult})
	}
	for id, b := range g.Builtins {
		if rules.Builtins == nil {
			rules.Builtins = map[string]gate.TierOverride{}
		}
		rules.Builtins[id] = gate.TierOverride{Risk: b.Risk, Consult: b.Consult, Add: b.Add, Remove: b.Remove}
	}
	c := gate.Cascade{Rules: rules, MinConfidence: g.MinConfidence, SkipAtOrBelow: gate.DefaultSkipAtOrBelow}
	if g.SkipAtOrBelow != nil {
		c.SkipAtOrBelow = *g.SkipAtOrBelow
	}
	if g.MaxLower != nil {
		if c.MaxLower = *g.MaxLower; c.MaxLower <= 0 {
			c.MaxLower = -1
		}
	}
	questions := gate.ResolveQuestions(gateQuestions(g.Questions))
	if g.Fast != nil {
		c.Fast = gate.SystemOneScorer{
			BaseURL: g.Fast.BaseURL, APIKey: g.Fast.APIKey, Model: g.Fast.Model,
			Strategy: g.Fast.Strategy, OnCapture: onCapture, InstructionsMaxChars: g.InstructionsMaxChars,
			Questions: questions,
		}
	}
	if g.Deep != nil {
		st, err := provider.NewStreamer(g.Deep.ConnectionType, g.Deep.BaseURL, g.Deep.APIKey, provider.StreamerOpts{SessionID: sessionID})
		if err != nil {
			slog.Error("gate deep scorer unavailable; continuing without it", "session", sessionID, "err", err)
		} else {
			deep := gate.LLMScorer{
				Streamer: st, Model: g.Deep.Model, OnCapture: onCapture, InstructionsMaxChars: g.InstructionsMaxChars,
				Style: g.Deep.Style, MaxTokens: g.Deep.MaxTokens, Structured: g.Deep.StructuredOutput,
				EnableThinking: g.Deep.EnableThinking, Questions: questions,
			}
			if deep.Style == "" {
				deep.Style = gate.StyleBands
			}
			if g.Deep.ReasoningEffort != "" {
				deep.ReasoningEffort = &g.Deep.ReasoningEffort
			}
			// Placing a call in a band needs no reasoning pass, and one costs
			// seconds per gated call: off unless the settings say otherwise.
			if deep.EnableThinking == nil && g.Deep.ConnectionType == catalog.TypeUnslothStudio {
				off := false
				deep.EnableThinking = &off
			}
			c.Deep = deep
		}
	}
	return c
}

func gateQuestions(in runtime.GateQuestions) gate.QuestionSettings {
	out := gate.QuestionSettings{}
	for id, o := range in.Builtins {
		if out.Builtins == nil {
			out.Builtins = map[string]gate.QuestionOverride{}
		}
		out.Builtins[id] = gate.QuestionOverride{Enabled: o.Enabled, Risk: o.Risk, Text: o.Text}
	}
	for _, c := range in.Custom {
		out.Custom = append(out.Custom, gate.CustomQuestion{
			ID: c.ID, Tools: c.Tools, Kind: gate.QuestionKind(c.Kind), Text: c.Text, Risk: c.Risk, Enabled: c.Enabled,
		})
	}
	return out
}

// readsWebContent reports tools whose results come from the open web.
func readsWebContent(toolName string) bool {
	return toolName == web.SearchName || toolName == web.FetchName
}

// historyTainted reports whether an earlier turn of the session read web content.
func historyTainted(msgs []runtime.Message) bool {
	for _, m := range msgs {
		if m.Role == "tool" && readsWebContent(m.Name) {
			return true
		}
	}
	return false
}
