package runtime_test

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/runtime"
)

func testPin() runtime.SessionPin {
	return runtime.SessionPin{
		AgentID:      "ag1",
		AgentName:    "Coder",
		AgentVersion: 1,
		ProviderID:   "p1",
		ProviderType: "openai_compatible",
		BaseURL:      "http://127.0.0.1:8888/v1",
		APIKey:       "sk-test",
		Models: []runtime.ModelRef{
			{ID: "m1", Name: "Model 1"},
			{ID: "m2", Name: "Model 2"},
		},
		CurrentModel: "m1",
	}
}

func TestCreatePinsSession(t *testing.T) {
	store := runtime.NewStore()
	pin := testPin()
	id, err := store.Create(pin)
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
	if sess.ID != id {
		t.Fatalf("sess.ID = %q, want %q", sess.ID, id)
	}
	if sess.Pin.AgentID != "ag1" || sess.Pin.AgentName != "Coder" || sess.Pin.AgentVersion != 1 {
		t.Fatalf("agent pin = %+v", sess.Pin)
	}
	if sess.Pin.ProviderID != "p1" || sess.Pin.ProviderType != "openai_compatible" {
		t.Fatalf("provider pin = %+v", sess.Pin)
	}
	if sess.Pin.BaseURL != pin.BaseURL || sess.Pin.APIKey != "sk-test" {
		t.Fatalf("endpoint pin = %+v", sess.Pin)
	}
	if sess.Pin.CurrentModel != "m1" {
		t.Fatalf("CurrentModel = %q, want m1", sess.Pin.CurrentModel)
	}
	if len(sess.Pin.Models) != 2 || sess.Pin.Models[0].ID != "m1" || sess.Pin.Models[1].Name != "Model 2" {
		t.Fatalf("Models = %+v", sess.Pin.Models)
	}
}

func TestCreateCopiesModels(t *testing.T) {
	store := runtime.NewStore()
	pin := testPin()
	id, err := store.Create(pin)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pin.Models[0].ID = "mutated"
	sess, ok := store.Get(id)
	if !ok {
		t.Fatal("session not found")
	}
	if sess.Pin.Models[0].ID != "m1" {
		t.Fatalf("store mutated via input slice: %q", sess.Pin.Models[0].ID)
	}
}

func TestSetCurrentModelUpdatesWhenInPin(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(testPin())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.SetCurrentModel(id, "m2"); err != nil {
		t.Fatalf("SetCurrentModel: %v", err)
	}
	sess, ok := store.Get(id)
	if !ok {
		t.Fatal("session not found")
	}
	if sess.Pin.CurrentModel != "m2" {
		t.Fatalf("CurrentModel = %q, want m2", sess.Pin.CurrentModel)
	}
}

func TestSetCurrentModelRejectsUnknownModel(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(testPin())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.SetCurrentModel(id, "m3"); err == nil {
		t.Fatal("expected error for model not in pin")
	}
	sess, _ := store.Get(id)
	if sess.Pin.CurrentModel != "m1" {
		t.Fatalf("CurrentModel = %q, want unchanged m1", sess.Pin.CurrentModel)
	}
}

func TestSetCurrentModelUnknownSession(t *testing.T) {
	store := runtime.NewStore()
	if err := store.SetCurrentModel("missing", "m1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteRemovesSession(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(testPin())
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
	id, err := store.Create(testPin())
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

func TestCreateHydratedCopiesMessagesAndThreadID(t *testing.T) {
	store := runtime.NewStore()
	pin := runtime.SessionPin{AgentID: "ag", CurrentModel: "m1", Models: []runtime.ModelRef{{ID: "m1", Name: "M"}}}
	msgs := []runtime.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}}
	id, err := store.CreateHydrated(pin, "th_abc", msgs)
	if err != nil {
		t.Fatal(err)
	}
	sess, ok := store.Get(id)
	if !ok {
		t.Fatal("missing")
	}
	if sess.ThreadID != "th_abc" {
		t.Fatalf("ThreadID = %q", sess.ThreadID)
	}
	if !strings.HasPrefix(id, "sess_") {
		t.Fatalf("id %q", id)
	}
	got, ok := store.Messages(id)
	if !ok || len(got) != 2 || got[0].Content != "hi" {
		t.Fatalf("messages %+v", got)
	}
	got[0].Content = "mutated"
	again, _ := store.Messages(id)
	if again[0].Content != "hi" {
		t.Fatal("stored slice must be copied")
	}
}

func TestCreateLeavesThreadIDEmpty(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(runtime.SessionPin{AgentID: "ag", CurrentModel: "m1", Models: []runtime.ModelRef{{ID: "m1"}}})
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := store.Get(id)
	if sess.ThreadID != "" {
		t.Fatalf("ThreadID = %q", sess.ThreadID)
	}
}

func TestMessagesReturnsCopy(t *testing.T) {
	store := runtime.NewStore()
	id, err := store.Create(testPin())
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
