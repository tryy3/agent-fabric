package catalog_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestSettingsHTTPGetSeedsAndPatchMerges(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GET status %d body %s", resp.StatusCode, body)
	}
	var seeded catalog.PlaneSettings
	if err := json.NewDecoder(resp.Body).Decode(&seeded); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	overlay, err := catalog.DecodeOverlay(seeded.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Image == nil || *overlay.Image != catalog.DefaultSandboxImage {
		t.Fatalf("seeded image = %v", overlay.Image)
	}

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/settings", strings.NewReader(`{"sandbox":{"image":"golang:1.23"}}`))
	req.Header.Set("Content-Type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if patch.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(patch.Body)
		patch.Body.Close()
		t.Fatalf("PATCH status %d body %s", patch.StatusCode, body)
	}
	var updated catalog.PlaneSettings
	if err := json.NewDecoder(patch.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	patch.Body.Close()
	got, err := catalog.DecodeOverlay(updated.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if got.Image == nil || *got.Image != "golang:1.23" {
		t.Fatalf("patched image = %v", got.Image)
	}
	if got.Kind == nil || *got.Kind != catalog.DefaultSandboxKind {
		t.Fatalf("kind should remain after image patch: %v", got.Kind)
	}
}

func TestSettingsHTTPPatchNullDeletesKey(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/settings", strings.NewReader(`{"sandbox":{"kind":null}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	var settings catalog.PlaneSettings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	overlay, err := catalog.DecodeOverlay(settings.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Kind != nil {
		t.Fatalf("kind should be deleted, got %q", *overlay.Kind)
	}
}

func TestAgentHTTPPatchMergesSettingsSandbox(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	p, err := store.CreateProvider(t.Context(), "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(t.Context(), p.ID, []catalog.ModelInfo{{ID: "m1", Name: "m1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(t.Context(), "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/agents/"+ag.ID, strings.NewReader(`{"settings":{"sandbox":{"image":"golang:1.23"}}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	var got catalog.Agent
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(string(got.Settings), `"golang:1.23"`) {
		t.Fatalf("settings = %s", got.Settings)
	}
}
