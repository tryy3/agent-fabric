package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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

	store2 := catalog.Open(pool) // same DB; proves rows survive a new Store handle
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
		t.Fatalf("ListProviders after delete: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("expected empty non-nil list after delete, got %#v", list)
	}

	_, err = store2.GetProvider(ctx, p.ID)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetProvider missing: %v", err)
	}
}

func TestCreateProviderRejectsEmptyName(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	_, err := store.CreateProvider(ctx, "", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateAgentRequiresCachedModel(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAgent(ctx, "A", "", p.ID, "missing")
	if err == nil {
		t.Fatal("expected error when model not cached")
	}
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
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
	store := catalog.Open(dbtest.Open(t))
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
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.Name != "Local" || got.BaseURL != "http://x/v1" || got.APIKey != "sk" {
		t.Fatalf("provider mutated on rejected patch: %+v", got)
	}
}

func TestCreateAndUpdateAgentRejectEmptyName(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.Name != "A" {
		t.Fatalf("agent mutated: %+v", got)
	}
	_, err = store.GetAgent(ctx, "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetAgent missing: %v", err)
	}
}

func TestReplaceProviderModelsRejectsOrphanedAgentDefault(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, now)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "m1" {
		t.Fatalf("cache mutated: %+v", got)
	}
}

func TestDeleteProviderConflictWhenReferenced(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAgent(ctx, "A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	err = store.DeleteProvider(ctx, p.ID)
	if err == nil || !errors.Is(err, catalog.ErrProviderInUse) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteAgentConflictWhenThreadPinned(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(ctx, "A", "", p.ID, "m1")
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
	err = store.DeleteAgent(ctx, ag.ID)
	if err == nil || !errors.Is(err, catalog.ErrAgentInUse) {
		t.Fatalf("err = %v", err)
	}
}

func TestConcurrentUpdateAgentIncrementsVersion(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.CreateAgent(ctx, "A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}

	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("A-%d", i)
			_, err := store.UpdateAgent(ctx, a.ID, &name, nil, nil, nil)
			errCh <- err
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("UpdateAgent: %v", err)
		}
	}

	got, err := store.GetAgent(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.Version != 1+n {
		t.Fatalf("version = %d, want %d (lost concurrent increments)", got.Version, 1+n)
	}
}

func TestConcurrentReplaceModelsAndCreateAgentDoesNotOrphanDefault(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, now)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m2", Name: "M2"}}, now)
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		_, err := store.CreateAgent(ctx, "Helper", "", p.ID, "m1")
		errCh <- err
	}()
	wg.Wait()
	close(errCh)
	for range errCh {
		// create may fail if refresh committed first; refresh may fail if the agent landed on m1
	}

	prov, err := store.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	ids := make(map[string]struct{}, len(prov.Models))
	for _, m := range prov.Models {
		ids[m.ID] = struct{}{}
	}
	agents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	for _, agent := range agents {
		if _, ok := ids[agent.DefaultModel]; !ok {
			t.Fatalf("orphaned default_model %q on agent %q; provider models=%v", agent.DefaultModel, agent.ID, prov.Models)
		}
	}
}
