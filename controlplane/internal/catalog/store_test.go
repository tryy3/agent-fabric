package catalog_test

import (
	"errors"
	"path/filepath"
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
	if err != nil || a.Version != 1 || a.DefaultModel != "m1" {
		t.Fatalf("CreateAgent: %+v err=%v", a, err)
	}
	name := "B"
	a2, err := store.UpdateAgent(a.ID, &name, nil, nil, nil)
	if err != nil || a2.Version != 2 || a2.Name != "B" {
		t.Fatalf("UpdateAgent: %+v err=%v", a2, err)
	}
}

func TestDeleteProviderConflictWhenReferenced(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	_, _ = store.CreateAgent("A", "", p.ID, "m1")
	err := store.DeleteProvider(p.ID)
	if err == nil || !errors.Is(err, catalog.ErrProviderInUse) {
		t.Fatalf("err = %v", err)
	}
}
