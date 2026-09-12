package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
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

	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	got, err := store.RefreshModels(context.Background(), p.ID, upstream.Client())
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

	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	now := time.Now().UTC()
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "old", Name: "old"}}, now)
	_, err := store.RefreshModels(context.Background(), p.ID, upstream.Client())
	if err == nil {
		t.Fatal("expected error")
	}
	got, _ := store.GetProvider(p.ID)
	if len(got.Models) != 1 || got.Models[0].ID != "old" {
		t.Fatalf("cache cleared: %+v", got)
	}
}
