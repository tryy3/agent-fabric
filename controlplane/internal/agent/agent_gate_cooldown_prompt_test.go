package agent_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

// The agent's blocked calls of one prompt must not lock the user's next
// request: each prompt starts without a cooldown.
func TestCooldownResetsOnANewPrompt(t *testing.T) {
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
	read := provider.ToolCall{ID: "call_read", Name: "list_files", Arguments: `{"path":"."}`}
	steps := []provider.StreamEvent{
		{Finish: "tool_calls", ToolCalls: []provider.ToolCall{{ID: "call_git", Name: "write_file", Arguments: `{"path":".git/config","content":"x"}`}}},
		{Finish: "tool_calls", ToolCalls: []provider.ToolCall{read}},
		{Content: "done", Finish: "stop"},
		{Finish: "tool_calls", ToolCalls: []provider.ToolCall{read}},
		{Content: "done", Finish: "stop"},
	}
	var results []string
	round := 0
	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			if last := messages[len(messages)-1]; last.Role == "tool" {
				results = append(results, last.Content)
			}
			round++
			return onEvent(steps[round-1])
		},
	}
	ag, csc, _, ctx2, cancel := startACPCatalogWithSandbox(t, runtime.NewStore(), cat, fs, engineconfig.Engine{DataDir: root})
	defer cancel()
	stopped := time.Unix(1_700_000_000, 0)
	ag.SetTestCooldownClock(func() time.Time { return stopped })
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
	for _, text := range []string{"edit the git config", "list the files"} {
		if _, err := csc.Prompt(ctx2, acp.PromptRequest{SessionId: sess.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock(text)}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(results) != 3 {
		t.Fatalf("tool results = %q", results)
	}
	if !strings.Contains(results[1], "cooling down") {
		t.Fatalf("within the prompt the block cools the agent down: %q", results[1])
	}
	if strings.Contains(results[2], "cooling down") {
		t.Fatalf("a new prompt must start without the earlier cooldown: %q", results[2])
	}
}
