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
