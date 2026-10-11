package workspace_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/export"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/workspace"
)

type publishesResponse struct {
	Publishes []struct {
		ID            string `json:"id"`
		Provider      string `json:"provider"`
		ProviderLabel string `json:"providerLabel"`
		Target        string `json:"target"`
		SiteURL       string `json:"siteUrl"`
		DeployURL     string `json:"deployUrl"`
	} `json:"publishes"`
}

func publishStub(id, label, target, siteURL, deployURL string) stubHTTPExporter {
	return stubHTTPExporter{
		method: export.Method{ID: id, Label: label, Enabled: true},
		result: export.Result{
			Links: []export.Link{
				{ID: "site", Label: "Site", URL: siteURL},
				{ID: "deploy", Label: "This deploy", URL: deployURL},
			},
			Publish: &catalog.PublishInput{Target: target, SiteURL: siteURL, DeployURL: deployURL},
		},
	}
}

func TestPublishIsRecordedAndListedPerProvider(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(context.Background(), "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	opener := &mapOpener{envs: map[string]sandbox.Environment{project.ID: &fakeEnv{fs: newMemFS()}}}
	reg := export.NewRegistry()
	reg.Register(publishStub("netlify", "Netlify", "site_1", "https://demo.netlify.app", "https://dep--demo.netlify.app"))
	reg.Register(publishStub("internal", "Internal pages", "", "https://pages.local/demo", "https://pages.local/demo/v1"))
	srv := httptest.NewServer(workspace.HandlerWithExporters(opener, store, reg))
	t.Cleanup(srv.Close)

	publish := func(method string) {
		t.Helper()
		resp, err := http.Post(srv.URL+"/v1/projects/"+project.ID+"/export", "application/json",
			strings.NewReader(`{"method":"`+method+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("publish %s: status %d body %s", method, resp.StatusCode, body)
		}
		var got struct {
			Publish *struct {
				ID       string `json:"id"`
				Provider string `json:"provider"`
			} `json:"publish"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Publish == nil || got.Publish.ID == "" || got.Publish.Provider != method {
			t.Fatalf("publish %s: response = %s", method, body)
		}
	}
	list := func(query string) publishesResponse {
		t.Helper()
		resp, err := http.Get(srv.URL + "/v1/projects/" + project.ID + "/publishes" + query)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list: status %d body %s", resp.StatusCode, body)
		}
		var out publishesResponse
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	publish("netlify")
	publish("internal")
	publish("netlify")

	all := list("")
	if len(all.Publishes) != 3 {
		t.Fatalf("all = %+v", all)
	}
	if p := all.Publishes[0]; p.Provider != "netlify" || p.ProviderLabel != "Netlify" ||
		p.Target != "site_1" || p.DeployURL != "https://dep--demo.netlify.app" {
		t.Fatalf("newest = %+v", p)
	}
	if p := all.Publishes[1]; p.Provider != "internal" || p.ProviderLabel != "Internal pages" {
		t.Fatalf("second = %+v", p)
	}

	netlify := list("?provider=netlify")
	if len(netlify.Publishes) != 2 {
		t.Fatalf("netlify = %+v", netlify)
	}
	for _, p := range netlify.Publishes {
		if p.Provider != "netlify" {
			t.Fatalf("filter leaked %+v", p)
		}
	}
	// The filter applies before the limit: the newest internal publish is not
	// the newest overall.
	if internal := list("?provider=internal&limit=1"); len(internal.Publishes) != 1 ||
		internal.Publishes[0].Provider != "internal" {
		t.Fatalf("internal = %+v", internal)
	}
	if limited := list("?limit=1"); len(limited.Publishes) != 1 {
		t.Fatalf("limited = %+v", limited)
	}

	resp, err := http.Get(srv.URL + "/v1/projects/proj_missing/publishes")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing project status = %d", resp.StatusCode)
	}
	bad, err := http.Get(srv.URL + "/v1/projects/" + project.ID + "/publishes?limit=zero")
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d", bad.StatusCode)
	}
}
