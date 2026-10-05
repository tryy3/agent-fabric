package catalog_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

type fakeSpecs struct{ p modelspecs.Provider }

func (f fakeSpecs) ProviderFor(_, baseURL string) (modelspecs.Provider, bool) {
	return f.p, strings.Contains(baseURL, "acme")
}

func TestConnectionsJoinModelSpecsWithoutStoringThem(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"vendor/big-1"},{"id":"unknown"}]}`))
	}))
	defer upstream.Close()

	store := catalog.Open(dbtest.Open(t))
	in := 3.0
	srv := httptest.NewServer(catalog.HandlerWithHooks(store, catalog.Hooks{Specs: fakeSpecs{modelspecs.Provider{
		ID: "acme", Name: "Acme",
		Models: map[string]modelspecs.Model{"big-1": {ID: "big-1", ToolCall: true, Cost: &modelspecs.Cost{Input: &in}}},
	}}}))
	defer srv.Close()

	body := fmt.Sprintf(`{"name":"Acme","type":"openai_compatible","baseUrl":%q,"apiKey":"k"}`, upstream.URL+"/acme/v1")
	resp, err := http.Post(srv.URL+"/v1/inference/connections", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var created catalog.InferenceConnection
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.SpecsProvider == nil || created.SpecsProvider.ID != "acme" {
		t.Fatalf("created: %+v", created.SpecsProvider)
	}

	resp, err = http.Post(srv.URL+"/v1/inference/connections/"+created.ID+"/models/refresh", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %v %v", err, resp)
	}
	var refreshed catalog.InferenceConnection
	_ = json.NewDecoder(resp.Body).Decode(&refreshed)
	resp.Body.Close()
	if len(refreshed.Models) != 2 {
		t.Fatalf("models: %+v", refreshed.Models)
	}
	if s := refreshed.Models[0].Specs; s == nil || !s.ToolCall || *s.Cost.Input != 3 {
		t.Fatalf("vendor-prefixed id should match big-1: %+v", refreshed.Models[0])
	}
	if refreshed.Models[1].Specs != nil {
		t.Fatalf("unknown model must have no specs")
	}

	stored, err := store.GetInferenceConnection(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range stored.Models {
		if m.Specs != nil {
			t.Fatalf("specs leaked into stored catalog: %+v", m)
		}
	}
}
