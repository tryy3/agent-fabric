package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestSetThreadViewMode(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	ctx := context.Background()
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if th.ViewModeID != nil {
		t.Fatalf("new thread viewModeId=%v, want nil", th.ViewModeID)
	}
	pretty := "pretty"
	updated, err := store.SetThreadViewMode(ctx, th.ID, &pretty)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ViewModeID == nil || *updated.ViewModeID != "pretty" {
		t.Fatalf("got %v", updated.ViewModeID)
	}
	cleared, err := store.SetThreadViewMode(ctx, th.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ViewModeID != nil {
		t.Fatalf("cleared = %v", cleared.ViewModeID)
	}
}

func TestCreateListRenameThread(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))

	a, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "Untitled" || a.TitleSource != catalog.TitleSourceAuto {
		t.Fatalf("create = %+v", a)
	}
	if a.AgentID != nil {
		t.Fatalf("agent_id = %v, want nil", a.AgentID)
	}
	if !strings.HasPrefix(a.ID, "th_") {
		t.Fatalf("id %q", a.ID)
	}
	if a.ProjectID == "" || !strings.HasPrefix(a.ProjectID, "proj_") {
		t.Fatalf("projectId %q", a.ProjectID)
	}

	time.Sleep(2 * time.Millisecond)
	b, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}

	list, err := store.ListThreads(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("list order = %+v", list)
	}
	if list[0].MessageCount != 0 {
		t.Fatalf("messageCount = %d", list[0].MessageCount)
	}

	renamed, err := store.RenameThread(ctx, a.ID, "  My chat  ")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "My chat" || renamed.TitleSource != catalog.TitleSourceUser {
		t.Fatalf("rename = %+v", renamed)
	}

	_, err = store.RenameThread(ctx, a.ID, "   ")
	if err == nil {
		t.Fatal("expected empty title error")
	}

	_, err = store.GetThread(ctx, "th_missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing: %v", err)
	}
}

func TestCommitTurnAutoTitleAndLock(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.CommitTurn(ctx, th.ID, "How do I pin an agent to a thread please", catalog.AssistantTurn{Content: "You pick the agent first."})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "How do I pin an agent to a" {
		t.Fatalf("title = %q", updated.Title)
	}
	if updated.TitleSource != catalog.TitleSourceAuto {
		t.Fatalf("source = %s", updated.TitleSource)
	}
	detail, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d", len(detail.Messages))
	}
	if detail.MessageCount != 2 {
		t.Fatalf("messageCount = %d, want 2", detail.MessageCount)
	}
	if detail.Messages[0].Role != "user" || detail.Messages[1].Role != "assistant" {
		t.Fatalf("roles = %+v", detail.Messages)
	}
	if detail.Messages[0].Position != 0 || detail.Messages[1].Position != 1 {
		t.Fatalf("positions = %+v", detail.Messages)
	}

	if _, err := store.RenameThread(ctx, th.ID, "Pinned"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitTurn(ctx, th.ID, "second prompt that would retitle", catalog.AssistantTurn{Content: "ok"}); err != nil {
		t.Fatal(err)
	}
	again, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != "Pinned" || again.TitleSource != catalog.TitleSourceUser {
		t.Fatalf("kept title = %+v", again.Thread)
	}
	if len(again.Messages) != 4 {
		t.Fatalf("messages = %d", len(again.Messages))
	}
}

func TestCommitTurnKeepsFirstAutoTitle(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitTurn(ctx, th.ID, "How do I pin an agent to a thread please", catalog.AssistantTurn{Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Title != "How do I pin an agent to a" {
		t.Fatalf("first title = %q", first.Title)
	}
	second, err := store.CommitTurn(ctx, th.ID, "this later prompt should not retitle the thread at all", catalog.AssistantTurn{Content: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Title != "How do I pin an agent to a" || second.TitleSource != catalog.TitleSourceAuto {
		t.Fatalf("kept auto title = %+v", second)
	}
}

func TestCommitTurnStoresAssistantPartsAndMetadata(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pt, ps := 3, 40.25
	ttft := int64(12)
	deltas := 2
	_, err = store.CommitTurn(ctx, th.ID, "hi", catalog.AssistantTurn{
		Content:      "hello",
		Model:        "m1",
		ProviderID:   "prov_x",
		ProviderName: "Local",
		StopReason:   "end_turn",
		Parts: []catalog.MessagePart{
			{Type: "thought", Text: "hmm"},
			{Type: "message", Text: "hello"},
			{Type: "usage", PromptTokens: &pt, PredictedPerSecond: &ps, TTFTMs: &ttft, Deltas: &deltas},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	as := detail.Messages[1]
	if as.Role != "assistant" || as.Content != "hello" {
		t.Fatalf("assistant = %+v", as)
	}
	if as.Model == nil || *as.Model != "m1" || as.ProviderName == nil || *as.ProviderName != "Local" {
		t.Fatalf("meta = %+v", as)
	}
	if len(as.Parts) != 3 || as.Parts[0].Type != "thought" || as.Parts[0].Text != "hmm" {
		t.Fatalf("parts = %+v", as.Parts)
	}
	if detail.Messages[0].Parts == nil {
		t.Fatal("user parts must be [] not null")
	}
}

func TestCommitTurnPersistsToolCallParts(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CommitTurn(ctx, th.ID, "read notes.txt", catalog.AssistantTurn{
		Content: "Here is the file.",
		Parts: []catalog.MessagePart{
			{
				Type:       "tool_call",
				ToolCallID: "call_abc",
				Name:       "read_file",
				Title:      "Read file",
				Input:      `{"path":"notes.txt"}`,
				Output:     `{"content":"hello"}`,
				Status:     "completed",
				Text:       "Read notes.txt",
			},
			{Type: "message", Text: "Here is the file."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d", len(detail.Messages))
	}
	parts := detail.Messages[1].Parts
	if len(parts) != 2 {
		t.Fatalf("parts = %+v", parts)
	}
	tc := parts[0]
	if tc.Type != "tool_call" {
		t.Fatalf("type = %q, want tool_call", tc.Type)
	}
	if tc.ToolCallID != "call_abc" || tc.Name != "read_file" || tc.Title != "Read file" {
		t.Fatalf("identity = %+v", tc)
	}
	if tc.Input != `{"path":"notes.txt"}` || tc.Output != `{"content":"hello"}` {
		t.Fatalf("io = %+v", tc)
	}
	if tc.Status != "completed" || tc.Text != "Read notes.txt" {
		t.Fatalf("status/summary = %+v", tc)
	}
	if parts[1].Type != "message" || parts[1].Text != "Here is the file." {
		t.Fatalf("message part = %+v", parts[1])
	}
}

func TestPinThreadAgentLocks(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(ctx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateAgent(ctx, "Other", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, ag.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, ag.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, other.ID); !errors.Is(err, catalog.ErrAgentLocked) {
		t.Fatalf("lock err = %v", err)
	}
	if err := store.SetThreadModel(ctx, th.ID, "m1"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID == nil || *got.AgentID != ag.ID {
		t.Fatalf("agent = %v", got.AgentID)
	}
	if got.CurrentModel == nil || *got.CurrentModel != "m1" {
		t.Fatalf("model = %v", got.CurrentModel)
	}
}

func TestCountThreadsByAgent(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(ctx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	unused, err := store.CreateAgent(ctx, "Other", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, ag.ID); err != nil {
		t.Fatal(err)
	}
	n, err := store.CountThreadsByAgent(ctx, ag.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pinned count = %d", n)
	}
	zero, err := store.CountThreadsByAgent(ctx, unused.ID)
	if err != nil {
		t.Fatal(err)
	}
	if zero != 0 {
		t.Fatalf("unused count = %d", zero)
	}
}
