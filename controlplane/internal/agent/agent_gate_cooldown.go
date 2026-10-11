package agent

import (
	"fmt"
	"time"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
)

// outcomeCooldown is reported when the session is cooling down and the gate
// did not evaluate the call.
const outcomeCooldown = "cooldown"

// sessionCooldown returns the session's gate cooldown, created on first use.
func (a *Agent) sessionCooldown(sessionID string) *gate.Cooldown {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cooldowns == nil {
		a.cooldowns = make(map[string]*gate.Cooldown)
	}
	cd, ok := a.cooldowns[sessionID]
	if !ok {
		cd = gate.NewCooldown(gate.CooldownConfig{}, a.cooldownClock)
		a.cooldowns[sessionID] = cd
	}
	return cd
}

// withCooldown runs a gated call unless the session is cooling down. A call
// the gate itself denied or cancelled starts or lengthens the cooldown; a
// rejected prompt, an allowed ask and an unscored structural refusal do not.
func (a *Agent) withCooldown(sessionID string, tr *gateTrace, run func() (string, error)) (string, error) {
	cd := a.sessionCooldown(sessionID)
	if blocks, remaining, active := cd.Active(); active {
		reason := fmt.Sprintf("cooling down after %d blocked calls, next attempt in ~%s s", blocks, cooldownSeconds(remaining))
		tr.Decision = gate.Decision{Kind: gate.Deny, RuleID: "gate.cooldown", Reason: reason}
		tr.Evaluated, tr.Outcome = true, outcomeCooldown
		return "", fmt.Errorf("denied: %s", reason)
	}
	result, err := run()
	if tr.Evaluated && tr.Decision.Kind == gate.Deny && tr.Decision.Risk > 0 &&
		(tr.Outcome == outcomeDenied || tr.Outcome == outcomeCancelled) {
		cd.Block()
	}
	return result, err
}

// SetTestCooldownClock replaces the clock gate cooldowns read; tests only.
func (a *Agent) SetTestCooldownClock(now func() time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cooldownClock = now
}

// cooldownSeconds renders a wait rounded up: whole seconds from one second,
// hundredths below.
func cooldownSeconds(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%d", int((d+time.Second-1)/time.Second))
	}
	return fmt.Sprintf("%.2f", float64((d+10*time.Millisecond-1)/(10*time.Millisecond))/100)
}
