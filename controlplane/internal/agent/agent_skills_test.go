package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

func TestNewSessionPinsSkillsFromProjectFiles(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	root := t.TempDir()
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(sandbox.ProjectFilesRoot(root, th.ProjectID), ".agents", "skills", "hello")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: hello\ndescription: Says hello\n---\nSay hello.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	_, csc, _, ctx2, _ := startACPCatalogWithSandbox(t, rt, cat, &fakeStreamer{deltas: []string{"ok"}}, engineconfig.Engine{DataDir: root})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	newSession := func() runtime.Session {
		t.Helper()
		resp, err := csc.NewSession(ctx2, acp.NewSessionRequest{
			Cwd:        "/",
			McpServers: []acp.McpServer{},
			Meta:       map[string]any{"assistantId": ag.ID, "threadId": th.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		sess, ok := rt.Get(string(resp.SessionId))
		if !ok {
			t.Fatal("session not stored")
		}
		return sess
	}

	first := newSession()
	if len(first.Pin.Skills) != 1 || first.Pin.Skills[0].Name != "hello" ||
		first.Pin.Skills[0].Source != "project" || first.Pin.Skills[0].Hash == "" {
		t.Fatalf("pinned skills = %+v", first.Pin.Skills)
	}

	if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Skills: json.RawMessage(`{"disabled":["hello"]}`)}); err != nil {
		t.Fatal(err)
	}
	if len(first.Pin.Skills) != 1 {
		t.Fatal("a live session must keep its pin")
	}
	if second := newSession(); len(second.Pin.Skills) != 0 {
		t.Fatalf("disabled skill pinned in a new session: %+v", second.Pin.Skills)
	}
}
