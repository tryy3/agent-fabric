package agent_test

import (
	"context"
	"os"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/gateinfo"
)

func TestReadGateInstructionsReturnsThePin(t *testing.T) {
	for name, tc := range map[string]struct {
		plane string
		want  string
	}{
		"set":   {"Never push to main.", "Never push to main."},
		"unset": {"", gateinfo.NoneText},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			cat, agDef := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
			th, err := cat.CreateThread(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(sandbox.ProjectFilesRoot(root, th.ProjectID), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.plane != "" {
				if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{GateInstructions: &tc.plane}); err != nil {
					t.Fatal(err)
				}
			}
			var seen string
			round := 0
			fs := &fakeStreamer{
				streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
					round++
					if round == 1 {
						return onEvent(provider.StreamEvent{
							Finish:    "tool_calls",
							ToolCalls: []provider.ToolCall{{ID: "call_gi", Name: gateinfo.Name, Arguments: `{}`}},
						})
					}
					seen = messages[len(messages)-1].Content
					return onEvent(provider.StreamEvent{Content: "ok", Finish: "stop"})
				},
			}
			_, csc, _, ctx2, cancel := startACPCatalogWithSandbox(t, runtime.NewStore(), cat, fs, engineconfig.Engine{DataDir: root})
			defer cancel()
			if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
				t.Fatal(err)
			}
			sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
				Cwd: "/", McpServers: []acp.McpServer{},
				Meta: map[string]any{"assistantId": agDef.ID, "threadId": th.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			// A later change does not reach the live session.
			later := "changed after the session started"
			if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{GateInstructions: &later}); err != nil {
				t.Fatal(err)
			}
			if _, err := csc.Prompt(ctx2, acp.PromptRequest{
				SessionId: sess.SessionId,
				Prompt:    []acp.ContentBlock{acp.TextBlock("what are the rules?")},
			}); err != nil {
				t.Fatal(err)
			}
			if seen != tc.want {
				t.Fatalf("tool result = %q, want %q", seen, tc.want)
			}
		})
	}
}
