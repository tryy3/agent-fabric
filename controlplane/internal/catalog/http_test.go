package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestProvidersHTTPCreateListRefresh(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
	}))
	defer upstream.Close()

	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	body := fmt.Sprintf(`{"name":"Local","type":"openai_compatible","baseUrl":%q,"apiKey":"sk"}`, upstream.URL+"/v1")
	resp, err := http.Post(srv.URL+"/v1/providers", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var p catalog.Provider
	_ = json.NewDecoder(resp.Body).Decode(&p)

	resp2, err := http.Post(srv.URL+"/v1/providers/"+p.ID+"/models/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("refresh status %d", resp2.StatusCode)
	}
}

func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if payload.Error == "" {
		t.Fatal("expected error message")
	}
	return payload.Error
}

func TestProvidersHTTPGetPatchDelete(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/providers", "application/json", strings.NewReader(
		`{"name":"Local","type":"openai_compatible","baseUrl":"http://127.0.0.1:9/v1","apiKey":"sk"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	var p catalog.Provider
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	got, err := http.Get(srv.URL + "/v1/providers/" + p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get status %d", got.StatusCode)
	}

	list, err := http.Get(srv.URL + "/v1/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	var providers []catalog.Provider
	if err := json.NewDecoder(list.Body).Decode(&providers); err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 {
		t.Fatalf("list = %+v", providers)
	}

	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/v1/providers/"+p.ID, strings.NewReader(`{"name":"Renamed"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer patch.Body.Close()
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch status %d", patch.StatusCode)
	}
	var updated catalog.Provider
	if err := json.NewDecoder(patch.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed" {
		t.Fatalf("updated = %+v", updated)
	}

	del, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/providers/"+p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	delResp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", delResp.StatusCode)
	}
}

func TestProvidersHTTPErrors(t *testing.T) {
	failUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer failUpstream.Close()

	store := catalog.Open(dbtest.Open(t))
	ctx := context.Background()
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, failUpstream.URL+"/v1", "sk")
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

	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	bad, err := http.Post(srv.URL+"/v1/providers", "application/json", strings.NewReader(`{"name":""}`))
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("create invalid status %d", bad.StatusCode)
	}
	_ = decodeError(t, bad)

	missing, err := http.Get(srv.URL + "/v1/providers/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("get missing status %d", missing.StatusCode)
	}
	_ = decodeError(t, missing)

	del, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/providers/"+p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	defer conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("delete in-use status %d", conflict.StatusCode)
	}
	_ = decodeError(t, conflict)

	refresh, err := http.Post(srv.URL+"/v1/providers/"+p.ID+"/models/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer refresh.Body.Close()
	if refresh.StatusCode != http.StatusBadGateway {
		t.Fatalf("refresh fail status %d", refresh.StatusCode)
	}
	_ = decodeError(t, refresh)
}

func TestAgentsHTTPCreateGetPatchDelete(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	ctx := context.Background()
	p, err := store.CreateProvider(ctx, "P", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	body := fmt.Sprintf(`{"name":"Helper","description":"d","providerId":%q,"defaultModel":"m1"}`, p.ID)
	resp, err := http.Post(srv.URL+"/v1/agents", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create agent status %d", resp.StatusCode)
	}
	var a catalog.Agent
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.Version != 1 {
		t.Fatalf("agent = %+v", a)
	}

	got, err := http.Get(srv.URL + "/v1/agents/" + a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get agent status %d", got.StatusCode)
	}

	list, err := http.Get(srv.URL + "/v1/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	var agents []catalog.Agent
	if err := json.NewDecoder(list.Body).Decode(&agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("agents = %+v", agents)
	}

	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/v1/agents/"+a.ID, strings.NewReader(`{"name":"Other"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer patch.Body.Close()
	if patch.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(patch.Body)
		t.Fatalf("patch agent status %d body %s", patch.StatusCode, b)
	}

	missing, err := http.Get(srv.URL + "/v1/agents/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing agent status %d", missing.StatusCode)
	}

	del, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/agents/"+a.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	delResp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete agent status %d", delResp.StatusCode)
	}
}

func TestProvidersHTTPEmptyListIsJSONArray(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	assertEmptyJSONArray(t, srv.URL+"/v1/providers")
}

func TestAgentsHTTPEmptyListIsJSONArray(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	assertEmptyJSONArray(t, srv.URL+"/v1/agents")
}

func TestProvidersHTTPListGetStoreErrors(t *testing.T) {
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()
	pool.Close()

	list, err := http.Get(srv.URL + "/v1/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	if list.StatusCode != http.StatusInternalServerError {
		t.Fatalf("list status %d", list.StatusCode)
	}
	_ = decodeError(t, list)

	got, err := http.Get(srv.URL + "/v1/providers/prov_closed")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusInternalServerError {
		t.Fatalf("get status %d", got.StatusCode)
	}
	_ = decodeError(t, got)
}

func TestAgentsHTTPListGetStoreErrors(t *testing.T) {
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()
	pool.Close()

	list, err := http.Get(srv.URL + "/v1/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	if list.StatusCode != http.StatusInternalServerError {
		t.Fatalf("list status %d", list.StatusCode)
	}
	_ = decodeError(t, list)

	got, err := http.Get(srv.URL + "/v1/agents/agent_closed")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusInternalServerError {
		t.Fatalf("get status %d", got.StatusCode)
	}
	_ = decodeError(t, got)
}

func assertEmptyJSONArray(t *testing.T, url string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(body))
	if got != "[]" {
		t.Fatalf("body = %q, want []", got)
	}
}
