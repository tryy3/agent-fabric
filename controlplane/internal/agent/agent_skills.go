package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/skilltool"
	"github.com/tryy3/agent-fabric/internal/skills"
)

// pinSkills resolves the enabled skills for the bound thread's project and the
// session's assistant and stores them on the pin. A failure leaves the session
// without skills instead of failing session/new.
func (a *Agent) pinSkills(ctx context.Context, pin *runtime.SessionPin, threadID string) map[string]skills.Entry {
	if a.catalog == nil || pin == nil {
		return nil
	}
	projectID := ""
	if threadID != "" {
		th, err := a.catalog.GetThread(ctx, threadID)
		if err != nil {
			slog.Warn("skills: load thread", "thread", threadID, "err", err)
			return nil
		}
		projectID = th.ProjectID
	}
	res, err := skills.NewService(a.catalog, agentProjectOpener{a: a}).Resolve(ctx, projectID, pin.AssistantID)
	if err != nil {
		slog.Warn("skills: resolve", "assistant", pin.AssistantID, "project", projectID, "err", err)
		return nil
	}
	for _, p := range res.Problems {
		slog.Warn("skills: skipped", "source", p.Source, "path", p.Path, "err", p.Error)
	}
	pin.Skills = nil
	for _, p := range skills.PinEnabled(res.Entries) {
		pin.Skills = append(pin.Skills, runtime.SkillPin{
			Name:        p.Name,
			Description: p.Description,
			Source:      string(p.Source),
			Hash:        p.Hash,
		})
	}
	appendSkillsInstructions(pin)
	entries := make(map[string]skills.Entry, len(res.Entries))
	for _, e := range res.Entries {
		entries[e.Name] = e
	}
	return entries
}

// agentProjectOpener opens a project's execution environment the way a prompt
// does, so skill discovery sees the files the agent sees.
type agentProjectOpener struct{ a *Agent }

func (o agentProjectOpener) Open(ctx context.Context, projectID string) (sandbox.Environment, error) {
	project, err := o.a.catalog.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	opts, err := openPromptSandbox(ctx, o.a.catalog, o.a.engine, project)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.Kind) == "" {
		return nil, fmt.Errorf("project %q has no execution environment", projectID)
	}
	open := sandbox.Open
	if o.a.testEnvironment != nil {
		open = o.a.testEnvironment
	}
	return open(ctx, opts)
}

const availableSkillsHeader = "Skills are folders of instructions you can load with the load_skill tool. " +
	"When a task matches a skill's description, load it first and follow it with your normal tools."

// skillsInstructionBlock lists the pinned skills (name and description only)
// for the effective instructions, or "" when none are enabled.
func skillsInstructionBlock(pins []runtime.SkillPin) string {
	if len(pins) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	b.WriteString(availableSkillsHeader)
	b.WriteString("\n")
	for _, p := range pins {
		fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Description)
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func appendSkillsInstructions(pin *runtime.SessionPin) {
	block := skillsInstructionBlock(pin.Skills)
	if block == "" {
		return
	}
	if strings.TrimSpace(pin.EffectiveInstructions) == "" {
		pin.EffectiveInstructions = block
		return
	}
	pin.EffectiveInstructions = pin.EffectiveInstructions + "\n\n" + block
}

// withoutLoadSkill drops the load_skill tool definition from a tool list.
func withoutLoadSkill(tools []provider.ToolDefinition) []provider.ToolDefinition {
	out := tools[:0:0]
	for _, t := range tools {
		if t.Function.Name != skilltool.Name {
			out = append(out, t)
		}
	}
	return out
}

func (a *Agent) skillProjectID(ctx context.Context, threadID string) string {
	if threadID == "" || a.catalog == nil {
		return ""
	}
	th, err := a.catalog.GetThread(ctx, threadID)
	if err != nil {
		return ""
	}
	return th.ProjectID
}

// runLoadSkill answers a load_skill call from the session's pinned skills.
func (a *Agent) runLoadSkill(ctx context.Context, sess runtime.Session, raw json.RawMessage) (string, error) {
	args, err := skilltool.ParseArgs(raw)
	if err != nil {
		return "", err
	}
	var pinned *runtime.SkillPin
	for i := range sess.Pin.Skills {
		if sess.Pin.Skills[i].Name == args.Name {
			pinned = &sess.Pin.Skills[i]
			break
		}
	}
	if pinned == nil {
		return "", fmt.Errorf("skill %q is not available in this session", args.Name)
	}
	_, text, err := a.skillService().Load(ctx, a.skillProjectID(ctx, sess.ThreadID), sess.Pin.AssistantID, args.Name, args.Path, pinned.Hash)
	return text, err
}

func (a *Agent) skillService() *skills.Service {
	return skills.NewService(a.catalog, agentProjectOpener{a: a})
}

// skillPrompt is a prompt that starts with /name of a pinned skill.
type skillPrompt struct {
	Inv      skills.Invocation
	Pin      runtime.SkillPin
	Injected string
	// MaxToolRounds is the skill's own ceiling from its frontmatter (0 = none).
	MaxToolRounds int
}

// detectSkillPrompt recognises `/name args` for a skill pinned in the session.
// A text that does not start with an enabled skill's name is a normal prompt
// (nil, nil). A recognised command with bad flags returns an error whose text
// is written for the user.
func (a *Agent) detectSkillPrompt(ctx context.Context, sess runtime.Session, text string) (*skillPrompt, error) {
	name, rest, ok := skills.SplitCommand(text)
	if !ok {
		return nil, nil
	}
	var pin *runtime.SkillPin
	for i := range sess.Pin.Skills {
		if sess.Pin.Skills[i].Name == name {
			pin = &sess.Pin.Skills[i]
			break
		}
	}
	if pin == nil {
		return nil, nil
	}
	entry, body, err := a.skillService().Load(ctx, a.skillProjectID(ctx, sess.ThreadID), sess.Pin.AssistantID, name, "", pin.Hash)
	if err != nil {
		return nil, err
	}
	inv, err := skills.ParseInvocation(name, rest, entry.Flags)
	if err != nil {
		return nil, err
	}
	return &skillPrompt{
		Inv:           inv,
		Pin:           *pin,
		Injected:      skills.RenderInvocation(inv, entry.Source, entry.Hash, body),
		MaxToolRounds: entry.MaxToolRounds,
	}, nil
}

// skillPart is the turn part that records a skill invocation.
func (p *skillPrompt) skillPart() catalog.MessagePart {
	return catalog.MessagePart{
		Type: "skill",
		Name: p.Inv.Name,
		Skill: &catalog.SkillInvocationPart{
			Name:   p.Inv.Name,
			Source: p.Pin.Source,
			Hash:   p.Pin.Hash,
			Flags:  p.Inv.Flags,
		},
	}
}

// sendAvailableCommands advertises the pinned skills as ACP slash commands.
func (a *Agent) sendAvailableCommands(ctx context.Context, sessionID string, pins []runtime.SkillPin, entries map[string]skills.Entry) {
	conn := a.connection()
	if conn == nil {
		return
	}
	commands := make([]acp.AvailableCommand, 0, len(pins))
	for _, p := range pins {
		cmd := acp.AvailableCommand{Name: p.Name, Description: p.Description}
		if e, ok := entries[p.Name]; ok && e.ArgumentHint != "" {
			cmd.Input = &acp.AvailableCommandInput{Unstructured: &acp.UnstructuredCommandInput{Hint: e.ArgumentHint}}
		}
		commands = append(commands, cmd)
	}
	if err := conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: acp.SessionId(sessionID),
		Update: acp.SessionUpdate{AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
			AvailableCommands: commands,
		}},
	}); err != nil {
		slog.Warn("skills: available_commands_update failed", "session", sessionID, "err", err)
	}
}
