package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestSessionModesFullAccessIsConfirmedAndSessionOnly(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	if _, err := cat.UpdateAssistant(ctx, ag.ID, nil, nil, nil, nil, nil,
		json.RawMessage(`{"permissions":{"mode":"auto_approve"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{
		Permissions: json.RawMessage(`{"policies":{"auto_approve":{"askAt":4}}}`),
	}); err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, cancel := startACPCatalog(t, rt, cat, &fakeStreamer{})
	t.Cleanup(cancel)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	newSession := func() acp.NewSessionResponse {
		t.Helper()
		sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
			Cwd: "/", McpServers: []acp.McpServer{}, Meta: map[string]any{"assistantId": ag.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		return sess
	}
	sess := newSession()
	if sess.Modes == nil || sess.Modes.CurrentModeId != "auto_approve" || len(sess.Modes.AvailableModes) != 4 {
		t.Fatalf("modes = %+v", sess.Modes)
	}
	for _, m := range sess.Modes.AvailableModes {
		if m.Id == "auto_approve" && (m.Description == nil || *m.Description != "Asks from risk 4, cancels from 9.") {
			t.Fatalf("the description follows the plane thresholds: %+v", m)
		}
	}

	if _, err := csc.SetSessionMode(ctx2, acp.SetSessionModeRequest{SessionId: sess.SessionId, ModeId: "full"}); err == nil {
		t.Fatal("full access without confirmation was accepted")
	}
	if _, err := csc.SetSessionMode(ctx2, acp.SetSessionModeRequest{SessionId: sess.SessionId, ModeId: "yolo"}); err == nil {
		t.Fatal("an unknown mode was accepted")
	}
	if _, err := csc.SetSessionMode(ctx2, acp.SetSessionModeRequest{
		SessionId: sess.SessionId, ModeId: "full", Meta: map[string]any{"confirmFullAccess": true},
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := rt.Get(string(sess.SessionId)); got.Pin.PermissionMode != "full" {
		t.Fatalf("session mode = %q", got.Pin.PermissionMode)
	}
	if _, err := csc.SetSessionMode(ctx2, acp.SetSessionModeRequest{SessionId: sess.SessionId, ModeId: "ask"}); err != nil {
		t.Fatalf("stepping down needs no confirmation: %v", err)
	}

	// The switch is never written back: the assistant keeps its mode and the
	// next session starts in it.
	if _, err := csc.SetSessionMode(ctx2, acp.SetSessionModeRequest{
		SessionId: sess.SessionId, ModeId: "full", Meta: map[string]any{"confirmFullAccess": true},
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := cat.GetAssistant(ctx, ag.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := catalog.PermissionsFromSettings(stored.Settings); p.Mode != "auto_approve" {
		t.Fatalf("assistant mode = %q", p.Mode)
	}
	if next := newSession(); next.Modes.CurrentModeId != "auto_approve" {
		t.Fatalf("a new session restored %q", next.Modes.CurrentModeId)
	}
}

func TestSessionModeSwitchAppliesToTheTurn(t *testing.T) {
	// The helper switches to full over session/set_mode: rm -rf runs unasked.
	prompted := false
	res := runGatedTurn(t, func(context.Context, *catalog.Store) string { return `{"mode":"full"}` },
		map[string]string{"old.txt": "bye"},
		[]scriptedCall{{"run_command", `{"command":["rm","-rf","old.txt"]}`}},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			prompted = true
			return selectOption("reject_once")(req)
		})
	if prompted {
		t.Fatal("full access must not prompt")
	}
	res.client.mu.Lock()
	defer res.client.mu.Unlock()
	if g := gateMetaOf(t, res.client.toolCallUpdates[0]); g["mode"] != "full" {
		t.Fatalf("gate meta mode = %v", g["mode"])
	}
}

func TestPlanePolicyChangesModeThreshold(t *testing.T) {
	// rm scores 6: Run automatically runs it by default, but asks once the
	// plane lowers its ask threshold to 5.
	prompted := false
	runGatedTurn(t, func(ctx context.Context, cat *catalog.Store) string {
		if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{
			Permissions: json.RawMessage(`{"policies":{"auto":{"askAt":5}}}`),
		}); err != nil {
			t.Fatal(err)
		}
		return `{"mode":"auto"}`
	}, map[string]string{"old.txt": "bye"},
		[]scriptedCall{{"run_command", `{"command":["rm","old.txt"]}`}},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			prompted = true
			return selectOption("allow_once")(req)
		})
	if !prompted {
		t.Fatal("the plane threshold did not apply")
	}
}
