package agent

import (
	"context"
	"fmt"
	"log/slog"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/agent/gate"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

// MetaConfirmFullAccess is the session/set_mode _meta key a client sets to
// true once the user has confirmed switching a session to full access.
const MetaConfirmFullAccess = "confirmFullAccess"

// modeNames are the user-facing names of the permission modes.
var modeNames = map[gate.Mode]string{
	gate.ModeAsk:         "Ask for approval",
	gate.ModeAutoApprove: "Approve for me",
	gate.ModeAuto:        "Run automatically",
	gate.ModeFull:        "Full access",
}

// sessionModes advertises the permission modes as ACP session modes, each
// described by the thresholds pinned for this session.
func (a *Agent) sessionModes(pin runtime.SessionPin) *acp.SessionModeState {
	policies := pinnedPolicies(pin)
	modes := make([]acp.SessionMode, 0, len(gate.Modes))
	for _, m := range gate.Modes {
		desc := modeDescription(m, policies.PolicyFor(m))
		modes = append(modes, acp.SessionMode{
			Id:          acp.SessionModeId(m),
			Name:        modeNames[m],
			Description: &desc,
			Meta:        map[string]any{"askAt": askAtMeta(policies.PolicyFor(m)), "cancelAt": int(policies.PolicyFor(m).CancelAt)},
		})
	}
	return &acp.SessionModeState{
		CurrentModeId:  acp.SessionModeId(modeFor(pin)),
		AvailableModes: modes,
	}
}

// askAtMeta is the ask threshold, or nil for a policy that never asks.
func askAtMeta(p gate.Policy) any {
	if !p.Asks() {
		return nil
	}
	return int(p.AskAt)
}

func modeDescription(m gate.Mode, p gate.Policy) string {
	var s string
	if p.Asks() {
		s = fmt.Sprintf("Asks from risk %d, cancels from %d.", p.AskAt, p.CancelAt)
	} else {
		s = fmt.Sprintf("Never asks; cancels from risk %d.", p.CancelAt)
	}
	if m == gate.ModeFull {
		s += " Paths outside the sandbox policy are opened without asking. This session only."
	}
	return s
}

// SetSessionMode switches a live session's permission mode. The change stays
// in this session: it is not written to the assistant, and a new session
// starts in the assistant's mode again. Full access needs the client to
// confirm it in _meta (MetaConfirmFullAccess), so a client cannot select it
// by accident.
func (a *Agent) SetSessionMode(ctx context.Context, params acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	sid := string(params.SessionId)
	mode, err := gate.ParseMode(string(params.ModeId))
	if err != nil || params.ModeId == "" {
		if err == nil {
			err = fmt.Errorf("modeId is required")
		}
		return acp.SetSessionModeResponse{}, acp.NewInvalidParams(map[string]any{"error": err.Error()})
	}
	if mode == gate.ModeFull {
		if confirmed, _ := params.Meta[MetaConfirmFullAccess].(bool); !confirmed {
			return acp.SetSessionModeResponse{}, acp.NewInvalidParams(map[string]any{
				"error": "full access needs _meta." + MetaConfirmFullAccess + " = true after the user confirms it",
			})
		}
	}
	if err := a.store.SetPermissionMode(sid, string(mode)); err != nil {
		slog.Error("session/set_mode failed", "session", sid, "err", err)
		return acp.SetSessionModeResponse{}, err
	}
	slog.Info("session/set_mode", "session", sid, "mode", mode)
	return acp.SetSessionModeResponse{}, nil
}
