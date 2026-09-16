package agent

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

func TestCommitNewSessionDeletesWhenClosed(t *testing.T) {
	store := runtime.NewStore()
	a := New(store, nil, sandbox.OpenOptions{})
	id, err := store.Create(runtime.SessionPin{AgentID: "ag1", CurrentModel: "m1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	a.CloseConnectionSessions()
	if store.Len() != 1 {
		t.Fatalf("store.Len() = %d, want 1 before commit", store.Len())
	}
	err = a.commitNewSession(id)
	if err == nil || !strings.Contains(err.Error(), "connection closed") {
		t.Fatalf("err = %v, want connection closed", err)
	}
	if store.Len() != 0 {
		t.Fatalf("orphaned session remained, len=%d", store.Len())
	}
}
