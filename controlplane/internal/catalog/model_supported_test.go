package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

var googleSpecs = modelspecs.Provider{
	ID: "acme", NPM: "@ai-sdk/google",
	Models: map[string]modelspecs.Model{
		"plain-google":     {ID: "plain-google"},
		"gemini-supported": {ID: "gemini-supported", Provider: &modelspecs.ModelProvider{NPM: "@ai-sdk/openai-compatible"}},
	},
}

func TestModelSupportedUsesSpecsThenPrefixFallback(t *testing.T) {
	const covered = "https://acme.test/zen/v1"
	tests := []struct {
		name     string
		specs    catalog.SpecsLookup
		connType string
		baseURL  string
		id       string
		want     bool
	}{
		{"specs-covered google wire is unsupported", fakeSpecs{googleSpecs}, catalog.TypeOpenCodeZen, covered, "plain-google", false},
		{"specs-covered known wire wins over gemini prefix", fakeSpecs{googleSpecs}, catalog.TypeOpenCodeZen, covered, "gemini-supported", true},
		{"specs-covered unknown model uses prefix rule", fakeSpecs{googleSpecs}, catalog.TypeOpenCodeZen, covered, "gemini-uncovered", false},
		{"specs-covered unknown model without prefix is supported", fakeSpecs{googleSpecs}, catalog.TypeOpenCodeZen, covered, "kimi-k3", true},
		{"specs not synced falls back to prefix rule", nil, catalog.TypeOpenCodeZen, covered, "plain-google", true},
		{"jev prefix unsupported without specs", nil, catalog.TypeOpenCodeGo, covered, "JEV-1", false},
		{"outside specs coverage falls back to prefix rule", fakeSpecs{googleSpecs}, catalog.TypeOpenCodeZen, "https://other.test/v1", "plain-google", true},
		{"non-OpenCode type is never unsupported", fakeSpecs{googleSpecs}, catalog.TypeOpenAICompatible, covered, "gemini-uncovered", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := catalog.Open(dbtest.Open(t))
			if tt.specs != nil {
				store.Specs = tt.specs
			}
			if got := store.ModelSupported(tt.connType, tt.baseURL, tt.id); got != tt.want {
				t.Fatalf("ModelSupported(%s) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestRefreshKeepsAllModelsAndServedConnectionFlagsUnsupported(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"gemini-supported"},{"id":"plain-google"},{"id":"gemini-uncovered"},{"id":"kimi-k3"}]}`))
	}))
	defer upstream.Close()

	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	store.Specs = fakeSpecs{googleSpecs}
	oc, err := store.CreateInferenceConnection(ctx, "Zen", catalog.TypeOpenCodeZen, "", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE inference_connections SET base_url = $1 WHERE id = $2`, upstream.URL+"/acme/v1", oc.ID); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(catalog.HandlerWithHooks(store, catalog.Hooks{Specs: store.Specs}))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/inference/connections/"+oc.ID+"/models/refresh", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %v %v", err, resp)
	}
	var served catalog.InferenceConnection
	_ = json.NewDecoder(resp.Body).Decode(&served)
	resp.Body.Close()

	got := make([]string, 0, len(served.Models))
	for _, m := range served.Models {
		got = append(got, fmt.Sprintf("%s=%v", m.ID, m.Unsupported))
	}
	want := "gemini-supported=false,plain-google=true,gemini-uncovered=true,kimi-k3=false"
	if strings.Join(got, ",") != want {
		t.Fatalf("served models = %v, want %s", got, want)
	}

	stored, err := store.GetInferenceConnection(ctx, oc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Models) != 4 {
		t.Fatalf("stored models = %+v, want all 4 kept", stored.Models)
	}
	for _, m := range stored.Models {
		if m.Unsupported {
			t.Fatalf("unsupported flag leaked into stored catalog: %+v", m)
		}
	}
}

func TestAssistantRejectsUnsupportedDefaultModel(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	oc, err := store.CreateInferenceConnection(ctx, "Zen", catalog.TypeOpenCodeZen, "", "sk")
	if err != nil {
		t.Fatal(err)
	}
	models := []catalog.ModelInfo{{ID: "gemini-x"}, {ID: "kimi-k3"}}
	if _, err := store.ReplaceInferenceConnectionModels(ctx, oc.ID, models, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAssistant(ctx, "A", "", "", oc.ID, "gemini-x"); err == nil {
		t.Fatal("expected unsupported default model to be rejected")
	}
	if _, err := store.CreateAssistant(ctx, "A", "", "", oc.ID, "kimi-k3"); err != nil {
		t.Fatalf("supported model: %v", err)
	}
}
