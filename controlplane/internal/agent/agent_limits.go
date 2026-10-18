package agent

import (
	"context"
	"log/slog"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

// limitsToPin and limitsFromPin convert between catalog.Limits (nil = unset)
// and the runtime pin (0 = unset).
func limitsToPin(l catalog.Limits) runtime.LimitsPin {
	val := func(v *int) int {
		if v == nil {
			return 0
		}
		return *v
	}
	return runtime.LimitsPin{
		DefaultToolRounds: val(l.DefaultToolRounds),
		MaxToolRounds:     val(l.MaxToolRounds),
		MaxTurnSeconds:    val(l.MaxTurnSeconds),
	}
}

func limitsFromPin(p runtime.LimitsPin) catalog.Limits {
	ptr := func(v int) *int {
		if v == 0 {
			return nil
		}
		return &v
	}
	return catalog.Limits{
		DefaultToolRounds: ptr(p.DefaultToolRounds),
		MaxToolRounds:     ptr(p.MaxToolRounds),
		MaxTurnSeconds:    ptr(p.MaxTurnSeconds),
	}
}

// pinLimits stores the plane and assistant budget limits on the pin. A failure
// leaves them unset, which means the built-in defaults.
func (a *Agent) pinLimits(ctx context.Context, pin *runtime.SessionPin) {
	if a.catalog == nil || pin == nil {
		return
	}
	pin.PlaneLimits = limitsToPin(a.catalog.PlaneLimits(ctx))
	ag, err := a.catalog.GetAssistant(ctx, pin.AssistantID)
	if err != nil {
		slog.Warn("limits: load assistant", "assistant", pin.AssistantID, "err", err)
		return
	}
	pin.AssistantLimits = limitsToPin(catalog.LimitsFromSettings(ag.Settings))
}

// promptLimits resolves the budget of one Prompt from the pinned limits and,
// for a skill run, the skill's ceiling and its --depth and --time flags.
func promptLimits(pin runtime.SessionPin, skill *skillPrompt) catalog.ResolvedLimits {
	run := catalog.RunRequest{}
	if skill != nil {
		run.Skill = true
		run.SkillMaxToolRounds = skill.MaxToolRounds
		run.Depth = skill.Inv.Flags["depth"]
		if t, err := time.ParseDuration(skill.Inv.Flags["time"]); err == nil {
			run.Seconds = int(t.Seconds())
		}
	}
	return catalog.ResolveLimits(limitsFromPin(pin.PlaneLimits), limitsFromPin(pin.AssistantLimits), run)
}

// budgetReachedNote is shown to the model, not stored, when a run used its
// tool rounds or wall-clock time.
const budgetReachedNote = "The tool budget for this run is used up, so no more tools are available. " +
	"Write the best answer you can now from what you have gathered, and say what you could not finish."

// sendBudgetUpdate reports tool-round use ("round 14 of 40") to the client in
// session_info_update meta, next to the other agentFabric updates.
func (a *Agent) sendBudgetUpdate(ctx context.Context, sessionID acp.SessionId, info map[string]any) {
	conn := a.connection()
	if conn == nil {
		return
	}
	if err := conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sessionID,
		Update: acp.SessionUpdate{SessionInfoUpdate: &acp.SessionSessionInfoUpdate{
			SessionUpdate: "session_info_update",
			Meta:          map[string]any{"agentFabric": info},
		}},
	}); err != nil {
		slog.Debug("budget update failed", "session", sessionID, "err", err)
	}
}

// startNotes writes clamp notes to the turn before the model's first round.
func (a *Agent) startNotes(ctx context.Context, sessionID acp.SessionId, notes []string, content *strings.Builder) {
	conn := a.connection()
	if conn == nil || len(notes) == 0 {
		return
	}
	text := strings.Join(notes, "\n") + "\n\n"
	content.WriteString(text)
	if err := conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sessionID,
		Update:    acp.UpdateAgentMessageText(text),
	}); err != nil {
		slog.Debug("limit note failed", "session", sessionID, "err", err)
	}
}

// SetTestTurnClock replaces the clock the per-Prompt wall-clock limit reads; tests only.
func (a *Agent) SetTestTurnClock(now func() time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.turnClock = now
}

func (a *Agent) turnNow() time.Time {
	a.mu.Lock()
	clock := a.turnClock
	a.mu.Unlock()
	if clock != nil {
		return clock()
	}
	return time.Now()
}
