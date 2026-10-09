package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

type googleWireSpecs struct{ id string }

func (g googleWireSpecs) ProviderFor(string, string) (modelspecs.Provider, bool) {
	return modelspecs.Provider{
		ID: "opencode", NPM: "@ai-sdk/openai-compatible",
		Models: map[string]modelspecs.Model{
			g.id: {ID: g.id, Provider: &modelspecs.ModelProvider{NPM: "@ai-sdk/google"}},
		},
	}, true
}

func seedOpenCode(t *testing.T) (*catalog.Store, catalog.Assistant) {
	t.Helper()
	ctx := context.Background()
	cat := catalog.Open(dbtest.Open(t))
	p, err := cat.CreateInferenceConnection(ctx, "Zen", catalog.TypeOpenCodeZen, "", "sk")
	if err != nil {
		t.Fatal(err)
	}
	models := []catalog.ModelInfo{{ID: "kimi-k3", Name: "Kimi"}, {ID: "gemini-x", Name: "Gemini"}}
	if _, err := cat.ReplaceInferenceConnectionModels(ctx, p.ID, models, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := cat.CreateAssistant(ctx, "Coder", "", "", p.ID, "kimi-k3")
	if err != nil {
		t.Fatal(err)
	}
	return cat, ag
}

func TestNewSessionPinSkipsUnsupportedModels(t *testing.T) {
	store := runtime.NewStore()
	cat, ag := seedOpenCode(t)
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{})
	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess := mustNewSession(t, ctx, csc, ag.ID)
	pinned, ok := store.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("session not stored")
	}
	if len(pinned.Pin.Models) != 1 || pinned.Pin.Models[0].ID != "kimi-k3" {
		t.Fatalf("pin models = %+v, want only kimi-k3", pinned.Pin.Models)
	}
	if opts := *sess.ConfigOptions[0].Select.Options.Ungrouped; len(opts) != 1 {
		t.Fatalf("config options = %+v, want 1", opts)
	}
}

func TestNewSessionFailsClearlyWhenDefaultModelBecomesUnsupported(t *testing.T) {
	store := runtime.NewStore()
	cat, ag := seedOpenCode(t)
	cat.Specs = googleWireSpecs{id: "kimi-k3"}
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{})
	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	_, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"assistantId": ag.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("NewSession err = %v, want not supported", err)
	}
}
