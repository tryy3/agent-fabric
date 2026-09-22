package catalog_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestProjectsHTTPCreateListGetPatchDelete(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	listResp, err := http.Get(srv.URL + "/v1/projects")
	if err != nil {
		t.Fatal(err)
	}
	var seeded []catalog.Project
	if err := json.NewDecoder(listResp.Body).Decode(&seeded); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK || len(seeded) != 1 || seeded[0].Name != catalog.DefaultProjectName {
		t.Fatalf("seeded status %d list %+v", listResp.StatusCode, seeded)
	}

	resp, err := http.Post(srv.URL+"/v1/projects", "application/json", strings.NewReader(`{"name":"Landing page"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create status %d body %s", resp.StatusCode, body)
	}
	var created catalog.Project
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.HasPrefix(created.ID, "proj_") || created.Name != "Landing page" {
		t.Fatalf("created %+v", created)
	}

	got, err := http.Get(srv.URL + "/v1/projects/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get %d", got.StatusCode)
	}
	got.Body.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/projects/"+created.ID, strings.NewReader(`{"name":"Renamed"}`))
	req.Header.Set("Content-Type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch %d", patch.StatusCode)
	}
	var renamed catalog.Project
	_ = json.NewDecoder(patch.Body).Decode(&renamed)
	patch.Body.Close()
	if renamed.Name != "Renamed" {
		t.Fatalf("renamed %+v", renamed)
	}

	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/projects/"+created.ID, nil)
	delResp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	missing, err := http.Get(srv.URL + "/v1/projects/proj_nope")
	if err != nil {
		t.Fatal(err)
	}
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing %d", missing.StatusCode)
	}
	msg := decodeError(t, missing)
	if !strings.Contains(msg, "not found") {
		t.Fatalf("error %q", msg)
	}

	bad, err := http.Post(srv.URL+"/v1/projects", "application/json", strings.NewReader(`{"name":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name %d", bad.StatusCode)
	}
	bad.Body.Close()
}

func TestProjectsHTTPDeleteConflictWhenThreadsRemain(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/projects", "application/json", strings.NewReader(`{"name":"Busy"}`))
	if err != nil {
		t.Fatal(err)
	}
	var p catalog.Project
	_ = json.NewDecoder(resp.Body).Decode(&p)
	resp.Body.Close()

	threadBody := `{"projectId":"` + p.ID + `"}`
	thResp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(threadBody))
	if err != nil {
		t.Fatal(err)
	}
	if thResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(thResp.Body)
		thResp.Body.Close()
		t.Fatalf("create thread %d %s", thResp.StatusCode, body)
	}
	thResp.Body.Close()

	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/projects/"+p.ID, nil)
	delResp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusConflict {
		t.Fatalf("status %d", delResp.StatusCode)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(delResp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "project in use" {
		t.Fatalf("error %q", payload.Error)
	}
}

func TestProjectsHTTPPatchMergesSettingsAndListsResolved(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	if _, err := store.EnsurePlaneSettings(t.Context(), catalog.DeprecatedSandbox{}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/projects", "application/json", strings.NewReader(`{"name":"Landing"}`))
	if err != nil {
		t.Fatal(err)
	}
	var p catalog.Project
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/projects/"+p.ID, strings.NewReader(`{
		"settings":{"sandbox":{"image":"golang:1.23"},"allowedAgents":["agent_1"],"memory":{"enabled":false}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if patch.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(patch.Body)
		patch.Body.Close()
		t.Fatalf("patch %d %s", patch.StatusCode, body)
	}
	patch.Body.Close()

	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/v1/projects/"+p.ID, strings.NewReader(`{
		"settings":{"sandbox":{"kind":"local"},"mcp":{"servers":[]}},
		"remotes":[{"id":"rmt_gh","kind":"github","urlOrBucket":"https://github.com/acme/landing"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	again, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if again.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(again.Body)
		again.Body.Close()
		t.Fatalf("second patch %d %s", again.StatusCode, body)
	}
	var updated catalog.Project
	if err := json.NewDecoder(again.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	again.Body.Close()
	if !strings.Contains(string(updated.Settings), `"golang:1.23"`) || !strings.Contains(string(updated.Settings), `"local"`) {
		t.Fatalf("sandbox replaced: %s", updated.Settings)
	}
	if !strings.Contains(string(updated.Settings), `"agent_1"`) || !strings.Contains(string(updated.Settings), `"mcp"`) {
		t.Fatalf("groups lost: %s", updated.Settings)
	}
	if !strings.Contains(string(updated.Remotes), `"rmt_gh"`) {
		t.Fatalf("remotes = %s", updated.Remotes)
	}

	resolved, err := http.Get(srv.URL + "/v1/projects/" + p.ID + "/sandbox/resolved")
	if err != nil {
		t.Fatal(err)
	}
	defer resolved.Body.Close()
	if resolved.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resolved.Body)
		t.Fatalf("resolved %d %s", resolved.StatusCode, body)
	}
	var overlay catalog.Overlay
	if err := json.NewDecoder(resolved.Body).Decode(&overlay); err != nil {
		t.Fatal(err)
	}
	if overlay.Image == nil || *overlay.Image != "golang:1.23" || overlay.Kind == nil || *overlay.Kind != "local" {
		t.Fatalf("resolved overlay %+v", overlay)
	}
}

func TestThreadsHTTPCreateUnderProjectAndFilter(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	projResp, err := http.Post(srv.URL+"/v1/projects", "application/json", strings.NewReader(`{"name":"Landing"}`))
	if err != nil {
		t.Fatal(err)
	}
	var proj catalog.Project
	_ = json.NewDecoder(projResp.Body).Decode(&proj)
	projResp.Body.Close()

	personalResp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if personalResp.StatusCode != http.StatusCreated {
		t.Fatalf("personal thread %d", personalResp.StatusCode)
	}
	var personal catalog.Thread
	_ = json.NewDecoder(personalResp.Body).Decode(&personal)
	personalResp.Body.Close()
	if personal.ProjectID == "" || personal.ProjectID == proj.ID {
		t.Fatalf("personal thread project %+v", personal)
	}

	landingResp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{"projectId":"`+proj.ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if landingResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(landingResp.Body)
		landingResp.Body.Close()
		t.Fatalf("landing thread %d %s", landingResp.StatusCode, body)
	}
	var landing catalog.Thread
	_ = json.NewDecoder(landingResp.Body).Decode(&landing)
	landingResp.Body.Close()
	if landing.ProjectID != proj.ID {
		t.Fatalf("landing project %q want %q", landing.ProjectID, proj.ID)
	}

	filtered, err := http.Get(srv.URL + "/v1/threads?projectId=" + proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	var list []catalog.ThreadListItem
	if err := json.NewDecoder(filtered.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	filtered.Body.Close()
	if len(list) != 1 || list[0].ID != landing.ID {
		t.Fatalf("filtered %+v", list)
	}

	allResp, err := http.Get(srv.URL + "/v1/threads")
	if err != nil {
		t.Fatal(err)
	}
	var all []catalog.ThreadListItem
	if err := json.NewDecoder(allResp.Body).Decode(&all); err != nil {
		t.Fatal(err)
	}
	allResp.Body.Close()
	if len(all) != 2 {
		t.Fatalf("unfiltered %+v", all)
	}

	missing, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{"projectId":"proj_nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing project status %d", missing.StatusCode)
	}
	missing.Body.Close()
}
