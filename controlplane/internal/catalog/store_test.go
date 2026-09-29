package catalog_test

import (
	"context"
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

	p, err := store.CreateInferenceConnection(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:8888/v1", "sk-test")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if p.ID == "" || !strings.HasPrefix(p.ID, "prov_") || p.Type != catalog.TypeOpenAICompatible {
		t.Fatalf("unexpected provider: %+v", p)
	}

	got, err := store.GetInferenceConnection(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.APIKey != "sk-test" {
		t.Fatalf("GetProvider = %+v", got)
	}

	name := "Renamed"
	got, err = store.UpdateInferenceConnection(ctx, p.ID, &name, nil, nil)
	if err != nil || got.Name != "Renamed" {
		t.Fatalf("UpdateProvider: %+v err=%v", got, err)
	}

	now := time.Now().UTC()
	got, err = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, now)
	if err != nil || len(got.Models) != 1 || got.ModelsUpdatedAt == nil {
		t.Fatalf("ReplaceInferenceConnectionModels: %+v err=%v", got, err)
	}

	store2 := catalog.Open(pool)
	list, err := store2.ListInferenceConnections(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(list) != 1 || list[0].Models[0].ID != "m1" {
		t.Fatalf("persisted list = %+v", list)
	}

	if err := store2.DeleteInferenceConnection(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	list, err = store2.ListInferenceConnections(ctx)
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
	_, err := store.CreateInferenceConnection(ctx, "", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateOpenCodeProviderForcesBaseURLAndDefaultName(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))

	zen, err := store.CreateInferenceConnection(ctx, "", catalog.TypeOpenCodeZen, "http://evil.example/v1", "sk-zen")
	if err != nil {
		t.Fatalf("CreateProvider zen: %v", err)
	}
	if zen.Name != "OpenCode Zen" || zen.Type != catalog.TypeOpenCodeZen || zen.BaseURL != catalog.OpenCodeZenBaseURL {
		t.Fatalf("zen = %+v", zen)
	}

	goProv, err := store.CreateInferenceConnection(ctx, "My Go", catalog.TypeOpenCodeGo, "", "sk-go")
	if err != nil {
		t.Fatalf("CreateProvider go: %v", err)
	}
	if goProv.Name != "My Go" || goProv.BaseURL != catalog.OpenCodeGoBaseURL {
		t.Fatalf("go = %+v", goProv)
	}

	base := "https://attacker.example/v1"
	updated, err := store.UpdateInferenceConnection(ctx, zen.ID, nil, &base, nil)
	if err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if updated.BaseURL != catalog.OpenCodeZenBaseURL {
		t.Fatalf("base URL changed to %q", updated.BaseURL)
	}
}

func TestCreateProviderRejectsUnknownType(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	_, err := store.CreateInferenceConnection(ctx, "X", "not_a_type", "http://x/v1", "k")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateAgentRequiresCachedModel(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, err := store.CreateAssistant(ctx, "A", "", p.ID, "missing")
	if err == nil {
		t.Fatal("expected error when model not cached")
	}
	_, _ = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAssistant(ctx, "A", "desc", p.ID, "m1")
	if err != nil || !strings.HasPrefix(a.ID, "agent_") || a.Version != 1 || a.DefaultModel == nil || *a.DefaultModel != "m1" {
		t.Fatalf("CreateAgent: %+v err=%v", a, err)
	}
	name := "B"
	a2, err := store.UpdateAssistant(ctx, a.ID, &name, nil, nil, nil, nil)
	if err != nil || a2.Version != 2 || a2.Name != "B" {
		t.Fatalf("UpdateAgent: %+v err=%v", a2, err)
	}
}

func TestUpdateProviderRejectsEmptyPointerValues(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, err := store.CreateInferenceConnection(ctx, "Local", catalog.TypeOpenAICompatible, "http://x/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	empty := "  "
	if _, err := store.UpdateInferenceConnection(ctx, p.ID, &empty, nil, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := store.UpdateInferenceConnection(ctx, p.ID, nil, &empty, nil); err == nil {
		t.Fatal("expected error for empty baseURL")
	}
	if _, err := store.UpdateInferenceConnection(ctx, p.ID, nil, nil, &empty); err == nil {
		t.Fatal("expected error for empty apiKey")
	}
	got, err := store.GetInferenceConnection(ctx, p.ID)
	if err != nil || got.Name != "Local" || got.BaseURL != "http://x/v1" || got.APIKey != "sk" {
		t.Fatalf("provider mutated on rejected patch: %+v err=%v", got, err)
	}
}

func TestGetAndListAgentsIncludeInferenceConnectionName(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateInferenceConnection(ctx, "Local", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	a, err := store.CreateAssistant(ctx, "A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAssistant(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.InferenceConnectionName == nil || *got.InferenceConnectionName != "Local" {
		t.Fatalf("GetAgent inferenceConnectionName = %+v", got)
	}
	listed, err := store.ListAssistants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].InferenceConnectionName == nil || *listed[0].InferenceConnectionName != "Local" {
		t.Fatalf("ListAgents inferenceConnectionName = %+v", listed)
	}
}

func TestCreateProviderUnslothStudio(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateInferenceConnection(ctx, "", catalog.TypeUnslothStudio, "http://127.0.0.1:8888/v1", "sk-unsloth")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if p.Name != "Unsloth Studio" || p.Type != catalog.TypeUnslothStudio {
		t.Fatalf("provider = %+v", p)
	}
	if p.BaseURL != "http://127.0.0.1:8888/v1" {
		t.Fatalf("baseURL = %q", p.BaseURL)
	}
}

func TestCreateAndUpdateAgentRejectEmptyName(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if _, err := store.CreateAssistant(ctx, "  ", "", p.ID, "m1"); err == nil {
		t.Fatal("expected error for empty create name")
	}
	a, err := store.CreateAssistant(ctx, "A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := store.UpdateAssistant(ctx, a.ID, &empty, nil, nil, nil, nil); err == nil {
		t.Fatal("expected error for empty update name")
	}
	got, err := store.GetAssistant(ctx, a.ID)
	if err != nil || got.Name != "A" {
		t.Fatalf("agent mutated: %+v err=%v", got, err)
	}
}

func TestReplaceInferenceConnectionModelsRejectsOrphanedAgentDefault(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	now := time.Now().UTC()
	_, _ = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, now)
	a, err := store.CreateAssistant(ctx, "Helper", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m2", Name: "M2"}}, now)
	if err == nil {
		t.Fatal("expected error when refresh drops agent defaultModel")
	}
	if !strings.Contains(err.Error(), a.Name) || !strings.Contains(err.Error(), "m1") {
		t.Fatalf("error = %v, want agent name and model", err)
	}
	got, err := store.GetInferenceConnection(ctx, p.ID)
	if err != nil || len(got.Models) != 1 || got.Models[0].ID != "m1" {
		t.Fatalf("cache mutated: %+v err=%v", got, err)
	}
}

func TestDeleteProviderUnlinksReferencingAgents(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAssistant(ctx, "A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteInferenceConnection(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	list, err := store.ListInferenceConnections(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("expected provider gone, list=%+v err=%v", list, err)
	}
	got, err := store.GetAssistant(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.InferenceConnectionID != nil || got.DefaultModel != nil {
		t.Fatalf("expected unset ids, got %+v", got)
	}
	if got.Version != a.Version+1 {
		t.Fatalf("version = %d, want %d", got.Version, a.Version+1)
	}

	store2 := catalog.Open(pool)
	got2, err := store2.GetAssistant(ctx, a.ID)
	if err != nil || got2.InferenceConnectionID != nil || got2.DefaultModel != nil {
		t.Fatalf("persisted agent = %+v err=%v", got2, err)
	}
	list, err = store2.ListInferenceConnections(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("persisted providers not empty: %+v err=%v", list, err)
	}
}

func TestDeleteProviderWithNoAgents(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	q, _ := store.CreateInferenceConnection(ctx, "Q", catalog.TypeOpenAICompatible, "http://y/v1", "k")
	_, _ = store.ReplaceInferenceConnectionModels(ctx, q.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAssistant(ctx, "A", "", q.ID, "m1")

	if err := store.DeleteInferenceConnection(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAssistant(ctx, a.ID)
	if err != nil || got.InferenceConnectionID == nil || *got.InferenceConnectionID != q.ID {
		t.Fatalf("unrelated agent mutated: %+v err=%v", got, err)
	}
}

func TestUpdateAgentNameOnlyOnIncomplete(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAssistant(ctx, "A", "", p.ID, "m1")
	if err := store.DeleteInferenceConnection(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	got, err := store.UpdateAssistant(ctx, a.ID, &name, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if got.Name != "Renamed" || got.InferenceConnectionID != nil || got.DefaultModel != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateAgentRejectsHalfSetPair(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, _ := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAssistant(ctx, "A", "", p.ID, "m1")
	if err := store.DeleteInferenceConnection(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	pid := p.ID
	_, err := store.UpdateAssistant(ctx, a.ID, nil, nil, &pid, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "provider and model must be set together") {
		t.Fatalf("err = %v", err)
	}
}
