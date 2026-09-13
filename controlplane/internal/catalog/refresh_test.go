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
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
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
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "old", Name: "old"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RefreshModels(ctx, p.ID, upstream.Client())
	if err == nil {
		t.Fatal("expected error")
	}
	got, err := store.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "old" {
		t.Fatalf("cache cleared: %+v", got)
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
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "old", Name: "old"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAgent(ctx, "Helper", "", p.ID, "old")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RefreshModels(ctx, p.ID, upstream.Client())
	if err == nil {
		t.Fatal("expected error")
	}
	got, err := store.GetProvider(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "old" {
		t.Fatalf("cache mutated: %+v", got)
	}
}

func TestRefreshModelsUnknownProvider(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	_, err := store.RefreshModels(ctx, "prov_missing", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	want := `provider "prov_missing" not found`
	if err.Error() != want {
		t.Fatalf("err = %v, want %s", err, want)
	}
}
