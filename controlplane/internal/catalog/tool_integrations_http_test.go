package catalog_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestToolIntegrationsCRUDAndSecretsRedaction(t *testing.T) {
	store := openGooseStore(t)
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	body := `{"name":"Linkup","kind":"linkup","mode":"external","secrets":{"apiKey":"secret-key"}}`
	resp, err := http.Post(srv.URL+"/v1/tool-integrations", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status %d body %s", resp.StatusCode, b)
	}
	var created catalog.ToolIntegration
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.SecretsConfigured["apiKey"] != true {
		t.Fatalf("secretsConfigured = %#v", created.SecretsConfigured)
	}
	if created.Endpoint != catalog.HostedDefaultEndpoints[catalog.KindLinkup] {
		t.Fatalf("linkup default endpoint = %q", created.Endpoint)
	}
	if len(created.Capabilities) != 2 ||
		created.Capabilities[0] != catalog.CapabilityWebSearch ||
		created.Capabilities[1] != catalog.CapabilityFetchPage {
		t.Fatalf("linkup capabilities = %#v", created.Capabilities)
	}
	raw, _ := json.Marshal(created)
	if strings.Contains(string(raw), "secret-key") {
		t.Fatalf("secret leaked in response: %s", raw)
	}

	get, err := http.Get(srv.URL + "/v1/tool-integrations/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var got catalog.ToolIntegration
	if err := json.NewDecoder(get.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.SecretsConfigured["apiKey"] {
		t.Fatal("expected apiKey configured")
	}

	// Preserve secret when omitted from PATCH.
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/tool-integrations/"+created.ID, strings.NewReader(`{"name":"Linkup Prod"}`))
	req.Header.Set("Content-Type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer patch.Body.Close()
	var renamed catalog.ToolIntegration
	if err := json.NewDecoder(patch.Body).Decode(&renamed); err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Linkup Prod" || !renamed.SecretsConfigured["apiKey"] {
		t.Fatalf("renamed = %+v", renamed)
	}
	secrets, err := store.GetToolIntegrationSecrets(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secrets["apiKey"] != "secret-key" {
		t.Fatalf("secret not preserved: %#v", secrets)
	}

	// Explicit null clears secret.
	clearReq, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/tool-integrations/"+created.ID, strings.NewReader(`{"secrets":{"apiKey":null}}`))
	clearReq.Header.Set("Content-Type", "application/json")
	clear, err := http.DefaultClient.Do(clearReq)
	if err != nil {
		t.Fatal(err)
	}
	defer clear.Body.Close()
	var cleared catalog.ToolIntegration
	if err := json.NewDecoder(clear.Body).Decode(&cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.SecretsConfigured["apiKey"] {
		t.Fatal("expected apiKey cleared")
	}

	// Plane defaults
	if _, err := store.EnsurePlaneSettings(t.Context(), catalog.DeprecatedSandbox{}); err != nil {
		t.Fatal(err)
	}
	searx, err := store.CreateToolIntegration(t.Context(), catalog.CreateToolIntegrationParams{
		Name: "SearXNG",
		Kind: catalog.KindSearXNG,
		Mode: catalog.ModeBundled,
	})
	if err != nil {
		t.Fatal(err)
	}
	defReq, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/settings", strings.NewReader(`{"webSearchIntegrationId":"`+searx.ID+`"}`))
	defReq.Header.Set("Content-Type", "application/json")
	defResp, err := http.DefaultClient.Do(defReq)
	if err != nil {
		t.Fatal(err)
	}
	defer defResp.Body.Close()
	if defResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(defResp.Body)
		t.Fatalf("defaults status %d body %s", defResp.StatusCode, b)
	}
	var settings catalog.PlaneSettings
	if err := json.NewDecoder(defResp.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.WebSearchIntegrationID == nil || *settings.WebSearchIntegrationID != searx.ID {
		t.Fatalf("webSearchIntegrationId = %v", settings.WebSearchIntegrationID)
	}

	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/tool-integrations/"+created.ID, nil)
	del, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", del.StatusCode)
	}
}
