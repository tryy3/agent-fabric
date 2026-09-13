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

	time.Sleep(2 * time.Millisecond)
	b, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}

	list, err := store.ListThreads(ctx)
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
	updated, err := store.CommitTurn(ctx, th.ID, "How do I pin an agent to a thread please", "You pick the agent first.")
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
	if detail.Messages[0].Role != "user" || detail.Messages[1].Role != "assistant" {
		t.Fatalf("roles = %+v", detail.Messages)
	}
	if detail.Messages[0].Position != 0 || detail.Messages[1].Position != 1 {
		t.Fatalf("positions = %+v", detail.Messages)
	}

	if _, err := store.RenameThread(ctx, th.ID, "Pinned"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitTurn(ctx, th.ID, "second prompt that would retitle", "ok"); err != nil {
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
