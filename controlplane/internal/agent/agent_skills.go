package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/skills"
)

// pinSkills resolves the enabled skills for the bound thread's project and the
// session's assistant and stores them on the pin. A failure leaves the session
// without skills instead of failing session/new.
func (a *Agent) pinSkills(ctx context.Context, pin *runtime.SessionPin, threadID string) {
	if a.catalog == nil || pin == nil {
		return
	}
	projectID := ""
	if threadID != "" {
		th, err := a.catalog.GetThread(ctx, threadID)
		if err != nil {
			slog.Warn("skills: load thread", "thread", threadID, "err", err)
			return
		}
		projectID = th.ProjectID
	}
	res, err := skills.NewService(a.catalog, agentProjectOpener{a: a}).Resolve(ctx, projectID, pin.AssistantID)
	if err != nil {
		slog.Warn("skills: resolve", "assistant", pin.AssistantID, "project", projectID, "err", err)
		return
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
