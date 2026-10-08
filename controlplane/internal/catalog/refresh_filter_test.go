package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

func TestRefreshModelsOpenCodeFilterUsesSpecsThenPrefixFallback(t *testing.T) {
	googleSpecs := modelspecs.Provider{
		ID: "acme", NPM: "@ai-sdk/google",
		Models: map[string]modelspecs.Model{
			"plain-google":     {ID: "plain-google"},
			"gemini-supported": {ID: "gemini-supported", Provider: &modelspecs.ModelProvider{NPM: "@ai-sdk/openai-compatible"}},
		},
	}
	tests := []struct {
		name  string
		specs catalog.SpecsLookup
		path  string
		want  string
	}{
		{
			name:  "specs-covered unsupported dropped, supported kept despite odd name, uncovered uses prefix rule",
			specs: fakeSpecs{googleSpecs},
			path:  "/acme/v1",
			want:  "gemini-supported,kimi-k3",
		},
		{
			name:  "specs not synced falls back to prefix rule",
			specs: nil,
			path:  "/acme/v1",
			want:  "plain-google,kimi-k3",
		},
		{
			name:  "connection outside specs coverage falls back to prefix rule",
			specs: fakeSpecs{googleSpecs},
			path:  "/other/v1",
			want:  "plain-google,kimi-k3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"id":"gemini-supported"},{"id":"plain-google"},{"id":"gemini-uncovered"},{"id":"jev-uncovered"},{"id":"kimi-k3"}]}`))
			}))
			defer upstream.Close()

			ctx := context.Background()
			pool := dbtest.Open(t)
			store := catalog.Open(pool)
			if tt.specs != nil {
				store.Specs = tt.specs
			}
			oc, err := store.CreateInferenceConnection(ctx, "Zen", catalog.TypeOpenCodeZen, "", "sk")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE inference_connections SET base_url = $1 WHERE id = $2`, upstream.URL+tt.path, oc.ID); err != nil {
				t.Fatal(err)
			}
			got, err := store.RefreshModels(ctx, oc.ID, upstream.Client())
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(got.Models))
			for _, m := range got.Models {
				ids = append(ids, m.ID)
			}
			if strings.Join(ids, ",") != tt.want {
				t.Fatalf("models = %v, want %s", ids, tt.want)
			}
		})
	}
}
