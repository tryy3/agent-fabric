package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type scriptedCall struct{ name, args string }

// runFileToolTurn seeds a project workspace, then runs one prompt in which the
// scripted provider issues the given tool calls one per round before stopping.
// It returns the workspace path and the recording client.
func runFileToolTurn(
	t *testing.T,
	seed map[string]string,
	calls []scriptedCall,
	permission func(acp.RequestPermissionRequest) acp.RequestPermissionResponse,
) (string, *captureClient) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	project, err := cat.CreateProject(ctx, "Site", "")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := cat.CreateThreadForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	ws := sandbox.ProjectFilesRoot(root, project.ID)
	for rel, content := range seed {
		full := filepath.Join(ws, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	round := 0
	fs := &fakeStreamer{
		streamFn: func(_ context.Context, _ string, _ []runtime.Message, onEvent func(provider.StreamEvent) error) error {
			mu.Lock()
			i := round
			round++
			mu.Unlock()
			if i >= len(calls) {
				return onEvent(provider.StreamEvent{Content: "done", Finish: "stop"})
			}
			return onEvent(provider.StreamEvent{
				Finish: "tool_calls",
				ToolCalls: []provider.ToolCall{{
					ID: "call_" + calls[i].name, Name: calls[i].name, Arguments: calls[i].args,
				}},
			})
		},
	}
	_, csc, client, ctx2, cancel := startACPCatalogWithSandbox(t, rt, cat, fs, engineconfig.Engine{DataDir: root})
	t.Cleanup(cancel)
	if permission != nil {
		client.permissionFn = func(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
			return permission(req), nil
		}
	}
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"assistantId": ag.ID, "threadId": thread.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("reorganize")},
	}); err != nil {
		t.Fatal(err)
	}
	return ws, client
}

func selectOption(id string) func(acp.RequestPermissionRequest) acp.RequestPermissionResponse {
	return func(acp.RequestPermissionRequest) acp.RequestPermissionResponse {
		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(acp.PermissionOptionId(id)),
		}
	}
}

func TestPromptFileToolsEditCommitAndReport(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	patch := "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n"
	ws, client := runFileToolTurn(t,
		map[string]string{"a.txt": "one\ntwo\n", "src/keep.txt": "keep\n"},
		[]scriptedCall{
			{"list_files", `{"recursive":true}`},
			{"search_text", `{"query":"two"}`},
			{"apply_patch", `{"diff":` + jsonQuote(patch) + `}`},
			{"append_file", `{"path":"a.txt","content":"three"}`},
			{"create_directory", `{"path":"docs"}`},
			{"move_path", `{"from":"a.txt","to":"docs/a.txt"}`},
		},
		nil,
	)

	got, err := os.ReadFile(filepath.Join(ws, "docs", "a.txt"))
	if err != nil || string(got) != "one\nTWO\nthree" {
		t.Fatalf("docs/a.txt = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(ws, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("a.txt should have moved: %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	wantTitles := []string{"List files", "Search text", "Apply patch", "Append to file", "Create directory", "Move path"}
	if len(client.toolCalls) != len(wantTitles) {
		t.Fatalf("tool calls = %d", len(client.toolCalls))
	}
	for i, title := range wantTitles {
		if client.toolCalls[i].Title != title {
			t.Errorf("call %d title = %q, want %q", i, client.toolCalls[i].Title, title)
		}
	}
	if client.toolCalls[5].Kind != acp.ToolKindMove {
		t.Errorf("move kind = %v", client.toolCalls[5].Kind)
	}
	for i, u := range client.toolCallUpdates {
		if u.Status == nil || *u.Status != acp.ToolCallStatusCompleted {
			t.Errorf("update %d = %+v", i, u)
		}
	}

	// No write_file ran, so the auto-commit proves the new tools mark the turn mutated.
	logOut, err := exec.Command("git", "-C", ws, "log", "--pretty=%s").Output()
	if err != nil || !strings.Contains(string(logOut), "agent:") {
		t.Fatalf("missing auto-commit: %s %v", logOut, err)
	}
	tracked, err := exec.Command("git", "-C", ws, "ls-files").Output()
	if err != nil || !strings.Contains(string(tracked), "docs/a.txt") {
		t.Fatalf("committed files = %q %v", tracked, err)
	}
}

func TestPromptFailedFileToolDoesNotCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ws, client := runFileToolTurn(t,
		map[string]string{"a.txt": "one\n"},
		[]scriptedCall{{"apply_patch", `{"diff":"--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-nope\n+x\n"}`}},
		nil,
	)
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.toolCallUpdates) != 1 || *client.toolCallUpdates[0].Status != acp.ToolCallStatusFailed {
		t.Fatalf("updates = %+v", client.toolCallUpdates)
	}
	if got, _ := os.ReadFile(filepath.Join(ws, "a.txt")); string(got) != "one\n" {
		t.Fatalf("a.txt changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(ws, ".git")); err == nil {
		t.Fatal("a failed mutation must not create a commit")
	}
}

func TestPromptDeletePathRequiresConfirmation(t *testing.T) {
	var offered []string
	record := func(id string) func(acp.RequestPermissionRequest) acp.RequestPermissionResponse {
		return func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			offered = nil
			for _, o := range req.Options {
				offered = append(offered, string(o.OptionId))
			}
			return selectOption(id)(req)
		}
	}
	call := []scriptedCall{{"delete_path", `{"path":"old.txt"}`}}
	seed := map[string]string{"old.txt": "bye"}

	t.Run("reject keeps file", func(t *testing.T) {
		ws, client := runFileToolTurn(t, seed, call, record("reject_once"))
		if _, err := os.Stat(filepath.Join(ws, "old.txt")); err != nil {
			t.Fatalf("file was deleted despite reject: %v", err)
		}
		client.mu.Lock()
		defer client.mu.Unlock()
		if *client.toolCallUpdates[0].Status != acp.ToolCallStatusFailed {
			t.Fatalf("status = %v", *client.toolCallUpdates[0].Status)
		}
		if strings.Join(offered, ",") != "allow_once,reject_once" {
			t.Fatalf("options = %v, want no session grant", offered)
		}
	})
	t.Run("allow once deletes", func(t *testing.T) {
		ws, client := runFileToolTurn(t, seed, call, record("allow_once"))
		if _, err := os.Stat(filepath.Join(ws, "old.txt")); !os.IsNotExist(err) {
			t.Fatalf("file still exists: %v", err)
		}
		client.mu.Lock()
		defer client.mu.Unlock()
		if *client.toolCallUpdates[0].Status != acp.ToolCallStatusCompleted {
			t.Fatalf("status = %v", *client.toolCallUpdates[0].Status)
		}
	})
}

func TestPromptFileToolsRefuseGitMetadata(t *testing.T) {
	ws, client := runFileToolTurn(t,
		map[string]string{"a.txt": "x"},
		[]scriptedCall{
			{"write_file", `{"path":".git/config","content":"x"}`},
			{"delete_path", `{"path":".git"}`},
		},
		selectOption("allow_once"),
	)
	if _, err := os.Stat(filepath.Join(ws, ".git", "config")); err == nil {
		t.Fatal(".git/config was written")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.toolCallUpdates) != 2 {
		t.Fatalf("updates = %d", len(client.toolCallUpdates))
	}
	for i, u := range client.toolCallUpdates {
		if *u.Status != acp.ToolCallStatusFailed {
			t.Errorf("update %d status = %v, want failed", i, *u.Status)
		}
	}
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
