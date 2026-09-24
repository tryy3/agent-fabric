package agent_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/gitrepo"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
	"github.com/tryy3/agent-fabric/internal/sandboxconfig"
)

func TestPromptWriteFileAutoCommitsAndRestoreKeepsThread(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	root := t.TempDir()
	rt := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	project, err := cat.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := cat.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, messages []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			last := messages[len(messages)-1]
			if last.Role == "tool" {
				return onEvent(provider.StreamEvent{Content: "rewrote index.html", Finish: "stop"})
			}
			content := "<h1>one</h1>"
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "user" && strings.Contains(messages[i].Content, "again") {
					content = "<h1>rewrite</h1>"
					break
				}
			}
			return onEvent(provider.StreamEvent{
				Finish: "tool_calls",
				ToolCalls: []provider.ToolCall{{
					ID:        "call_write",
					Name:      "write_file",
					Arguments: `{"path":"index.html","content":"` + content + `"}`,
				}},
			})
		},
	}
	_, csc, _, ctx2, _ := startACPCatalogWithSandbox(
		t,
		rt,
		cat,
		fs,
		sandboxconfig.Engine{DataDir: root},
	)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": thread.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("write index.html")},
	}); err != nil {
		t.Fatal(err)
	}

	ws := sandbox.ProjectWorkspaceRoot(root, project.ID)
	first, err := os.ReadFile(filepath.Join(ws, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "<h1>one</h1>" {
		t.Fatalf("first write = %q", first)
	}
	logOut, err := exec.Command("git", "-C", ws, "log", "--pretty=%s").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logOut), "agent:") {
		t.Fatalf("missing auto-commit:\n%s", logOut)
	}
	if strings.Contains(string(logOut), `"path"`) || strings.Contains(string(logOut), "rewrote index.html") {
		t.Fatalf("commit must not embed tool JSON or transcripts:\n%s", logOut)
	}
	sha1, err := exec.Command("git", "-C", ws, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("rewrite it again")},
	}); err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile(filepath.Join(ws, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rewritten) != "<h1>rewrite</h1>" {
		t.Fatalf("rewrite = %q", rewritten)
	}

	env, err := local.New(ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Close(context.Background()) })
	execu, _ := env.Exec()
	if err := gitrepo.CheckoutForce(context.Background(), execu, strings.TrimSpace(string(sha1))); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(ws, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "<h1>one</h1>" {
		t.Fatalf("restored = %q", restored)
	}

	detail, err := cat.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) < 4 {
		t.Fatalf("thread messages were lost: %+v", detail.Messages)
	}
}
