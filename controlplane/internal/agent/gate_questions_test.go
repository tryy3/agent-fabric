package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestNewSessionPinsGateQuestions(t *testing.T) {
	store := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	ctx := context.Background()
	plane := json.RawMessage(`{"questions":{"builtins":{"data_leak":{"risk":9},"destructive":{"text":"Does the plane think this destroys data?"}},
		"custom":[{"id":"prod_cluster","tools":["command"],"text":"Does this touch the production cluster?","risk":9}]}}`)
	if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Permissions: plane}); err != nil {
		t.Fatal(err)
	}
	own := json.RawMessage(`{"permissions":{"questions":{"builtins":{"destructive":{"enabled":false}}}}}`)
	if _, err := cat.UpdateAssistant(ctx, ag.ID, nil, nil, nil, nil, nil, own); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStreamer{deltas: []string{"ok"}}
	_, csc, _, acpCtx, _ := startACPCatalog(t, store, cat, fs)
	if _, err := csc.Initialize(acpCtx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	pinnedOf := func(id acp.SessionId) runtime.GateQuestions {
		t.Helper()
		s, ok := store.Get(string(id))
		if !ok {
			t.Fatal("session not stored")
		}
		return s.Pin.Gate.Questions
	}
	check := func(q runtime.GateQuestions, what string) {
		t.Helper()
		d := q.Builtins["destructive"]
		if q.Builtins["data_leak"].Risk != 9 || d.Enabled == nil || *d.Enabled || d.Text != "" || len(q.Custom) != 1 || q.Custom[0].ID != "prod_cluster" {
			t.Fatalf("%s: %+v", what, q)
		}
	}

	live := mustNewSession(t, acpCtx, csc, ag.ID)
	check(pinnedOf(live.SessionId), "plane default with the assistant's override per id")

	if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Permissions: json.RawMessage(`null`)}); err != nil {
		t.Fatal(err)
	}
	check(pinnedOf(live.SessionId), "a live session keeps its pin after a plane change")
	if q := pinnedOf(mustNewSession(t, acpCtx, csc, ag.ID).SessionId); q.Builtins["data_leak"].Risk != 0 || len(q.Custom) != 0 || q.Builtins["destructive"].Enabled == nil {
		t.Fatalf("a new session picks up the change: %+v", q)
	}
}
