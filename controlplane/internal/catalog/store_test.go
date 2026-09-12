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

func TestProviderCRUDRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)

	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:8888/v1", "sk-test")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if p.ID == "" || p.Type != catalog.TypeOpenAICompatible {
		t.Fatalf("unexpected provider: %+v", p)
	}

	got, err := store.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.APIKey != "sk-test" {
		t.Fatalf("GetProvider = %+v", got)
	}

	name := "Renamed"
	got, err = store.UpdateProvider(ctx, p.ID, &name, nil, nil)
	if err != nil || got.Name != "Renamed" {
		t.Fatalf("UpdateProvider: %+v err=%v", got, err)
	}

	now := time.Now().UTC()
	got, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, now)
	if err != nil || len(got.Models) != 1 || got.ModelsUpdatedAt == nil {
		t.Fatalf("ReplaceProviderModels: %+v err=%v", got, err)
	}

	store2 := catalog.Open(pool)
	list, err := store2.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(list) != 1 || list[0].Models[0].ID != "m1" {
		t.Fatalf("persisted list = %+v", list)
	}

	if err := store2.DeleteProvider(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	list, err = store2.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(list) != 0 {
		t.Fatal("expected empty after delete")
	}
}

func TestCreateProviderRejectsEmptyName(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	_, err := store.CreateProvider(ctx, "", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateAgentRequiresCachedModel(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, err := store.CreateAgent(ctx, "A", "", p.ID, "missing")
	if err == nil {
		t.Fatal("expected error when model not cached")
	}
	_, _ = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAgent(ctx, "A", "desc", p.ID, "m1")
	if err != nil || a.Version != 1 || a.DefaultModel != "m1" {
		t.Fatalf("CreateAgent: %+v err=%v", a, err)
	}
	name := "B"
	a2, err := store.UpdateAgent(ctx, a.ID, &name, nil, nil, nil)
	if err != nil || a2.Version != 2 || a2.Name != "B" {
		t.Fatalf("UpdateAgent: %+v err=%v", a2, err)
	}
}

func TestUpdateProviderRejectsEmptyPointerValues(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://x/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	empty := "  "
	if _, err := store.UpdateProvider(ctx, p.ID, &empty, nil, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := store.UpdateProvider(ctx, p.ID, nil, &empty, nil); err == nil {
		t.Fatal("expected error for empty baseURL")
	}
	if _, err := store.UpdateProvider(ctx, p.ID, nil, nil, &empty); err == nil {
		t.Fatal("expected error for empty apiKey")
	}
	got, err := store.GetProvider(ctx, p.ID)
	if err != nil || got.Name != "Local" || got.BaseURL != "http://x/v1" || got.APIKey != "sk" {
		t.Fatalf("provider mutated on rejected patch: %+v err=%v", got, err)
	}
}

func TestCreateAndUpdateAgentRejectEmptyName(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if _, err := store.CreateAgent(ctx, "  ", "", p.ID, "m1"); err == nil {
		t.Fatal("expected error for empty create name")
	}
	a, err := store.CreateAgent(ctx, "A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := store.UpdateAgent(ctx, a.ID, &empty, nil, nil, nil); err == nil {
		t.Fatal("expected error for empty update name")
	}
	got, err := store.GetAgent(ctx, a.ID)
	if err != nil || got.Name != "A" {
		t.Fatalf("agent mutated: %+v err=%v", got, err)
	}
}

func TestReplaceProviderModelsRejectsOrphanedAgentDefault(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	now := time.Now().UTC()
	_, _ = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, now)
	a, err := store.CreateAgent(ctx, "Helper", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m2", Name: "M2"}}, now)
	if err == nil {
		t.Fatal("expected error when refresh drops agent defaultModel")
	}
	if !strings.Contains(err.Error(), a.Name) || !strings.Contains(err.Error(), "m1") {
		t.Fatalf("error = %v, want agent name and model", err)
	}
	got, err := store.GetProvider(ctx, p.ID)
	if err != nil || len(got.Models) != 1 || got.Models[0].ID != "m1" {
		t.Fatalf("cache mutated: %+v err=%v", got, err)
	}
}

func TestDeleteProviderConflictWhenReferenced(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	_, _ = store.CreateAgent(ctx, "A", "", p.ID, "m1")
	err := store.DeleteProvider(ctx, p.ID)
	if err == nil || !errors.Is(err, catalog.ErrProviderInUse) {
		t.Fatalf("err = %v", err)
	}
}
