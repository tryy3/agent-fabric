package catalog_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestProviderCRUDRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := catalog.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	p, err := store.CreateProvider("Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:8888/v1", "sk-test")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if p.ID == "" || p.Type != catalog.TypeOpenAICompatible {
		t.Fatalf("unexpected provider: %+v", p)
	}

	got, ok := store.GetProvider(p.ID)
	if !ok || got.APIKey != "sk-test" {
		t.Fatalf("GetProvider = %+v ok=%v", got, ok)
	}

	name := "Renamed"
	got, err = store.UpdateProvider(p.ID, &name, nil, nil)
	if err != nil || got.Name != "Renamed" {
		t.Fatalf("UpdateProvider: %+v err=%v", got, err)
	}

	now := time.Now().UTC()
	got, err = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, now)
	if err != nil || len(got.Models) != 1 || got.ModelsUpdatedAt == nil {
		t.Fatalf("ReplaceProviderModels: %+v err=%v", got, err)
	}

	// Re-open from disk
	store2, err := catalog.Open(dir)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	list := store2.ListProviders()
	if len(list) != 1 || list[0].Models[0].ID != "m1" {
		t.Fatalf("persisted list = %+v", list)
	}
	if _, err := filepath.Abs(filepath.Join(dir, "providers.json")); err != nil {
		t.Fatal(err)
	}

	if err := store2.DeleteProvider(p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if len(store2.ListProviders()) != 0 {
		t.Fatal("expected empty after delete")
	}
}

func TestCreateProviderRejectsEmptyName(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateProvider("", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateAgentRequiresCachedModel(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, err := store.CreateAgent("A", "", p.ID, "missing")
	if err == nil {
		t.Fatal("expected error when model not cached")
	}
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAgent("A", "desc", p.ID, "m1")
	if err != nil || a.Version != 1 || a.DefaultModel == nil || *a.DefaultModel != "m1" {
		t.Fatalf("CreateAgent: %+v err=%v", a, err)
	}
	name := "B"
	a2, err := store.UpdateAgent(a.ID, &name, nil, nil, nil)
	if err != nil || a2.Version != 2 || a2.Name != "B" {
		t.Fatalf("UpdateAgent: %+v err=%v", a2, err)
	}
}

func TestUpdateProviderRejectsEmptyPointerValues(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.CreateProvider("Local", catalog.TypeOpenAICompatible, "http://x/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	empty := "  "
	if _, err := store.UpdateProvider(p.ID, &empty, nil, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := store.UpdateProvider(p.ID, nil, &empty, nil); err == nil {
		t.Fatal("expected error for empty baseURL")
	}
	if _, err := store.UpdateProvider(p.ID, nil, nil, &empty); err == nil {
		t.Fatal("expected error for empty apiKey")
	}
	got, ok := store.GetProvider(p.ID)
	if !ok || got.Name != "Local" || got.BaseURL != "http://x/v1" || got.APIKey != "sk" {
		t.Fatalf("provider mutated on rejected patch: %+v", got)
	}
}

func TestCreateAndUpdateAgentRejectEmptyName(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if _, err := store.CreateAgent("  ", "", p.ID, "m1"); err == nil {
		t.Fatal("expected error for empty create name")
	}
	a, err := store.CreateAgent("A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := store.UpdateAgent(a.ID, &empty, nil, nil, nil); err == nil {
		t.Fatal("expected error for empty update name")
	}
	got, ok := store.GetAgent(a.ID)
	if !ok || got.Name != "A" {
		t.Fatalf("agent mutated: %+v", got)
	}
}

func TestReplaceProviderModelsRejectsOrphanedAgentDefault(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	now := time.Now().UTC()
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, now)
	a, err := store.CreateAgent("Helper", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m2", Name: "M2"}}, now)
	if err == nil {
		t.Fatal("expected error when refresh drops agent defaultModel")
	}
	if !strings.Contains(err.Error(), a.Name) || !strings.Contains(err.Error(), "m1") {
		t.Fatalf("error = %v, want agent name and model", err)
	}
	got, _ := store.GetProvider(p.ID)
	if len(got.Models) != 1 || got.Models[0].ID != "m1" {
		t.Fatalf("cache mutated: %+v", got)
	}
}

func TestDeleteProviderUnlinksReferencingAgents(t *testing.T) {
	dir := t.TempDir()
	store, _ := catalog.Open(dir)
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAgent("A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if len(store.ListProviders()) != 0 {
		t.Fatal("expected provider gone")
	}
	got, ok := store.GetAgent(a.ID)
	if !ok {
		t.Fatal("agent missing")
	}
	if got.ProviderID != nil || got.DefaultModel != nil {
		t.Fatalf("expected unset ids, got %+v", got)
	}
	if got.Version != a.Version+1 {
		t.Fatalf("version = %d, want %d", got.Version, a.Version+1)
	}

	store2, err := catalog.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got2, ok := store2.GetAgent(a.ID)
	if !ok || got2.ProviderID != nil || got2.DefaultModel != nil {
		t.Fatalf("persisted agent = %+v ok=%v", got2, ok)
	}
	if len(store2.ListProviders()) != 0 {
		t.Fatal("persisted providers not empty")
	}
}

func TestDeleteProviderWithNoAgents(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	q, _ := store.CreateProvider("Q", catalog.TypeOpenAICompatible, "http://y/v1", "k")
	_, _ = store.ReplaceProviderModels(q.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAgent("A", "", q.ID, "m1")

	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatal(err)
	}
	got, ok := store.GetAgent(a.ID)
	if !ok || got.ProviderID == nil || *got.ProviderID != q.ID {
		t.Fatalf("unrelated agent mutated: %+v", got)
	}
}

func TestUpdateAgentNameOnlyOnIncomplete(t *testing.T) {
	dir := t.TempDir()
	store, _ := catalog.Open(dir)
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAgent("A", "", p.ID, "m1")
	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	got, err := store.UpdateAgent(a.ID, &name, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if got.Name != "Renamed" || got.ProviderID != nil || got.DefaultModel != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateAgentRejectsHalfSetPair(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAgent("A", "", p.ID, "m1")
	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatal(err)
	}
	pid := p.ID
	_, err := store.UpdateAgent(a.ID, nil, nil, &pid, nil)
	if err == nil || !strings.Contains(err.Error(), "provider and model must be set together") {
		t.Fatalf("err = %v", err)
	}
}
