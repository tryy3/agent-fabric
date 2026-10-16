package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// managedStub is a publish provider that can list and delete deploys.
type managedStub struct {
	stubHTTPExporter
	targets []export.TargetDeploys
	listErr error
	deleted []string
}

func (m *managedStub) ListDeploys(context.Context, export.Request) ([]export.TargetDeploys, error) {
	return m.targets, m.listErr
}

func (m *managedStub) DeleteDeploy(_ context.Context, _ export.Request, target, deployID string) error {
	if deployID == "dep_live" {
		return export.ErrPublishLive
	}
	m.deleted = append(m.deleted, target+"/"+deployID)
	return nil
}

type syncResponse struct {
	Providers []struct {
		Provider string `json:"provider"`
		Added    int    `json:"added"`
		Removed  int    `json:"removed"`
		Updated  int    `json:"updated"`
		Error    string `json:"error"`
	} `json:"providers"`
	Publishes []struct {
		ID        string `json:"id"`
		Provider  string `json:"provider"`
		DeployID  string `json:"deployId"`
		Live      bool   `json:"live"`
		CanDelete bool   `json:"canDelete"`
	} `json:"publishes"`
}

func TestSyncAndDeletePublishes(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	ctx := context.Background()
	project, err := store.CreateProject(ctx, "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	// Recorded before the sync: a publish of a provider that can't manage
	// deploys, and a Netlify deploy that was deleted on Netlify since.
	if _, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{Provider: "internal", SiteURL: "https://pages.local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertPublish(ctx, project.ID, catalog.PublishInput{
		Provider: "netlify", Target: "site_1", DeployID: "dep_deleted", SiteURL: "https://demo.netlify.app",
	}); err != nil {
		t.Fatal(err)
	}
	when := func(minute int) time.Time { return time.Date(2026, 10, 11, 12, minute, 0, 0, time.UTC) }
	netlify := &managedStub{
		stubHTTPExporter: stubHTTPExporter{method: export.Method{ID: "netlify", Label: "Netlify", Enabled: true}},
		targets: []export.TargetDeploys{{
			Target:   "site_1",
			Complete: true,
			Deploys: []catalog.PublishInput{
				{DeployID: "dep_live", SiteURL: "https://demo.netlify.app", DeployURL: "https://dep_live--demo.netlify.app", Live: true, CreatedAt: when(2)},
				{DeployID: "dep_old", SiteURL: "https://demo.netlify.app", DeployURL: "https://dep_old--demo.netlify.app", CreatedAt: when(1)},
			},
		}},
	}
	broken := &managedStub{
		stubHTTPExporter: stubHTTPExporter{method: export.Method{ID: "broken", Label: "Broken", Enabled: true}},
		listErr:          errors.New("provider down"),
	}
	unbound := &managedStub{
		stubHTTPExporter: stubHTTPExporter{method: export.Method{ID: "unbound", Label: "Unbound", Enabled: true}},
	}
	reg := export.NewRegistry()
	reg.Register(netlify)
	reg.Register(broken)
	reg.Register(unbound)
	reg.Register(stubHTTPExporter{method: export.Method{ID: "internal", Label: "Internal", Enabled: true}})
	opener := &mapOpener{envs: map[string]sandbox.Environment{project.ID: &fakeEnv{fs: newMemFS()}}}
	srv := httptest.NewServer(workspace.HandlerWithExporters(opener, store, reg))
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/v1/projects/"+project.ID+"/publishes/sync", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync status %d body %s", resp.StatusCode, body)
	}
	var synced syncResponse
	if err := json.Unmarshal(body, &synced); err != nil {
		t.Fatal(err)
	}
	if len(synced.Providers) != 2 {
		t.Fatalf("providers (unbound one omitted) = %s", body)
	}
	if p := synced.Providers[0]; p.Provider != "netlify" || p.Added != 2 || p.Removed != 1 || p.Error != "" {
		t.Fatalf("netlify = %+v", p)
	}
	if p := synced.Providers[1]; p.Provider != "broken" || p.Error != "provider down" {
		t.Fatalf("broken = %+v", p)
	}
	byDeploy := map[string]struct {
		id        string
		live      bool
		canDelete bool
	}{}
	for _, p := range synced.Publishes {
		byDeploy[p.Provider+"/"+p.DeployID] = struct {
			id        string
			live      bool
			canDelete bool
		}{p.ID, p.Live, p.CanDelete}
	}
	if len(synced.Publishes) != 3 {
		t.Fatalf("publishes = %s", body)
	}
	if p := byDeploy["netlify/dep_live"]; !p.live || p.canDelete {
		t.Fatalf("live deploy = %+v", p)
	}
	if p := byDeploy["netlify/dep_old"]; p.live || !p.canDelete {
		t.Fatalf("old deploy = %+v", p)
	}
	if p := byDeploy["internal/"]; p.id == "" || p.canDelete {
		t.Fatalf("unmanaged provider publish = %+v", p)
	}

	del := func(id string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/projects/"+project.ID+"/publishes/"+id, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, b := del(byDeploy["netlify/dep_live"].id); code != http.StatusConflict {
		t.Fatalf("delete live: %d %s", code, b)
	}
	if code, b := del(byDeploy["internal/"].id); code != http.StatusBadRequest {
		t.Fatalf("delete unmanaged: %d %s", code, b)
	}
	if code, b := del("pub_missing"); code != http.StatusNotFound {
		t.Fatalf("delete missing: %d %s", code, b)
	}
	if code, b := del(byDeploy["netlify/dep_old"].id); code != http.StatusNoContent {
		t.Fatalf("delete old: %d %s", code, b)
	}
	if len(netlify.deleted) != 1 || netlify.deleted[0] != "site_1/dep_old" {
		t.Fatalf("provider deletes = %v", netlify.deleted)
	}
	left, err := store.ListPublishes(ctx, project.ID, "netlify", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].DeployID != "dep_live" {
		t.Fatalf("netlify publishes left = %+v", left)
	}
}
