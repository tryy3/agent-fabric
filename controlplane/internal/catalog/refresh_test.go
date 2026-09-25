package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestRefreshModelsCachesList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"alpha"},{"id":"beta"}]}`))
	}))
	defer upstream.Close()

	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, _ := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	got, err := store.RefreshModels(ctx, p.ID, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 2 || got.Models[0].ID != "alpha" || got.ModelsUpdatedAt == nil {
		t.Fatalf("got = %+v", got)
	}
}

func TestRefreshModelsKeepsCacheOnFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer upstream.Close()

	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, _ := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	now := time.Now().UTC()
	_, _ = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "old", Name: "old"}}, now)
	_, err := store.RefreshModels(ctx, p.ID, upstream.Client())
	if err == nil {
		t.Fatal("expected error")
	}
	got, err := store.GetProvider(ctx, p.ID)
	if err != nil || len(got.Models) != 1 || got.Models[0].ID != "old" {
		t.Fatalf("cache cleared: %+v err=%v", got, err)
	}
}

func TestRefreshModelsFiltersOpenCodeUnsupported(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-5"},{"id":"gemini-3-flash"},{"id":"jev-1.13"},{"id":"kimi-k3"}]}`))
	}))
	defer upstream.Close()

	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)

	custom, err := store.CreateProvider(ctx, "Custom", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	customGot, err := store.RefreshModels(ctx, custom.ID, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(customGot.Models) != 4 {
		t.Fatalf("custom should keep all models, got %+v", customGot.Models)
	}

	oc, err := store.CreateProvider(ctx, "Zen", catalog.TypeOpenCodeZen, "", "sk")
	if err != nil {
		t.Fatal(err)
	}
	// Point refresh at httptest while keeping the OpenCode type (base URL is locked on update).
	if _, err := pool.Exec(ctx, `UPDATE providers SET base_url = $1 WHERE id = $2`, upstream.URL+"/v1", oc.ID); err != nil {
		t.Fatalf("update base_url: %v", err)
	}
	got, err := store.RefreshModels(ctx, oc.ID, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 2 {
		t.Fatalf("expected 2 models after filter, got %+v", got.Models)
	}
	if got.Models[0].ID != "claude-sonnet-5" || got.Models[1].ID != "kimi-k3" {
		t.Fatalf("models = %+v", got.Models)
	}
}

func TestRefreshModelsKeepsCacheWhenAgentDefaultWouldOrphan(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"other"}]}`))
	}))
	defer upstream.Close()

	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, _ := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	now := time.Now().UTC()
	_, _ = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "old", Name: "old"}}, now)
	_, err := store.CreateAgent(ctx, "Helper", "", p.ID, "old")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RefreshModels(ctx, p.ID, upstream.Client())
	if err == nil {
		t.Fatal("expected error")
	}
	got, err := store.GetProvider(ctx, p.ID)
	if err != nil || len(got.Models) != 1 || got.Models[0].ID != "old" {
		t.Fatalf("cache cleared: %+v err=%v", got, err)
	}
}
