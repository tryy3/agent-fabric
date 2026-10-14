package agent_test

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestNewSessionPinsGateInstructions(t *testing.T) {
	store := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	ctx := context.Background()
	plane := "Never push to main."
	if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{GateInstructions: &plane}); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStreamer{deltas: []string{"ok"}}
	_, csc, _, acpCtx, _ := startACPCatalog(t, store, cat, fs)
	if _, err := csc.Initialize(acpCtx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	pinnedOf := func(id acp.SessionId) runtime.GatePin {
		t.Helper()
		s, ok := store.Get(string(id))
		if !ok {
			t.Fatal("session not stored")
		}
		return s.Pin.Gate
	}

	inherits := mustNewSession(t, acpCtx, csc, ag.ID)
	if got := pinnedOf(inherits.SessionId).Instructions; got != plane {
		t.Fatalf("an assistant without its own rules inherits the plane default, got %q", got)
	}

	own := "  Only edit files under src/.  "
	if _, err := cat.PatchAssistant(ctx, ag.ID, catalog.AssistantPatch{SetGateInstructions: true, GateInstructions: &own}); err != nil {
		t.Fatal(err)
	}
	overridden := mustNewSession(t, acpCtx, csc, ag.ID)
	if got := pinnedOf(overridden.SessionId).Instructions; got != "Only edit files under src/." {
		t.Fatalf("the assistant's rules replace the plane's, got %q", got)
	}

	none := ""
	if _, err := cat.PatchAssistant(ctx, ag.ID, catalog.AssistantPatch{SetGateInstructions: true, GateInstructions: &none}); err != nil {
		t.Fatal(err)
	}
	if got := pinnedOf(mustNewSession(t, acpCtx, csc, ag.ID).SessionId).Instructions; got != "" {
		t.Fatalf("an empty override means no rules, got %q", got)
	}

	changed := "CHANGED"
	if _, err := cat.PatchAssistant(ctx, ag.ID, catalog.AssistantPatch{SetGateInstructions: true, GateInstructions: &changed}); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{GateInstructions: &changed}); err != nil {
		t.Fatal(err)
	}
	if got := pinnedOf(inherits.SessionId).Instructions; got != plane {
		t.Fatalf("a live session keeps its pin after a plane change, got %q", got)
	}
	if got := pinnedOf(overridden.SessionId).Instructions; got != "Only edit files under src/." {
		t.Fatalf("a live session keeps its pin after an assistant change, got %q", got)
	}
	if got := pinnedOf(mustNewSession(t, acpCtx, csc, ag.ID).SessionId).Instructions; got != changed {
		t.Fatalf("a new session picks up the change, got %q", got)
	}
}
