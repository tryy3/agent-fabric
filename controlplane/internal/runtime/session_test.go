package runtime_test

import (
	"testing"

	"github.com/tryy3/agent-fabric/internal/runtime"
)

func TestCreatePinsEchoDefinition(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty session id")
	}
	sess, ok := store.Get(id)
	if !ok {
		t.Fatal("session not found")
	}
	if sess.Definition.ID != "echo" {
		t.Fatalf("pinned id = %q, want echo", sess.Definition.ID)
	}
	if sess.Definition.Name != "Echo" {
		t.Fatalf("name = %q, want Echo", sess.Definition.Name)
	}
	if sess.Definition.Version != "1" {
		t.Fatalf("version = %q, want 1", sess.Definition.Version)
	}
}

func TestDeleteRemovesSession(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.Delete(id)
	if _, ok := store.Get(id); ok {
		t.Fatal("expected session gone after Delete")
	}
}

func TestAppendBuildsTranscript(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(id, runtime.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if err := store.Append(id, runtime.Message{Role: "assistant", Content: "hello"}); err != nil {
		t.Fatalf("Append assistant: %v", err)
	}
	msgs, ok := store.Messages(id)
	if !ok {
		t.Fatal("session missing")
	}
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hi" {
		t.Fatalf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hello" {
		t.Fatalf("msgs[1] = %+v", msgs[1])
	}
}

func TestAppendUnknownSession(t *testing.T) {
	store := runtime.NewStore()
	err := store.Append("missing", runtime.Message{Role: "user", Content: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMessagesReturnsCopy(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.EchoDefinition())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(id, runtime.Message{Role: "user", Content: "a"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	msgs, ok := store.Messages(id)
	if !ok {
		t.Fatal("missing")
	}
	msgs[0].Content = "mutated"
	again, _ := store.Messages(id)
	if again[0].Content != "a" {
		t.Fatalf("store mutated via returned slice: %q", again[0].Content)
	}
}
