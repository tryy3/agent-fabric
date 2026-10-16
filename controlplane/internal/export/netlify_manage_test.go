package export_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/export"
)

func boundProject() catalog.Project {
	return catalog.Project{
		Name:    "Demo",
		Remotes: json.RawMessage(`[{"id":"rmt_netlify","kind":"netlify","path":"site_1","urlOrBucket":"https://demo.netlify.app"}]`),
	}
}

func netlifyAt(srv *httptest.Server) export.Netlify {
	return export.Netlify{Token: "tok", HTTPClient: srv.Client(), APIBase: srv.URL}
}

func deployJSON(id, state, context string, minute int) map[string]any {
	return map[string]any{
		"id": id, "state": state, "context": context,
		"ssl_url":        "https://demo.netlify.app",
		"deploy_ssl_url": "https://" + id + "--demo.netlify.app",
		"created_at":     time.Date(2026, 10, 11, 12, minute, 0, 0, time.UTC).Format(time.RFC3339),
	}
}

func TestNetlifyListDeploysReportsReadyProductionDeploysAndLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sites/site_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "site_1", "ssl_url": "https://demo.netlify.app",
				"published_deploy": map[string]any{"id": "dep_2"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/sites/site_1/deploys":
			if r.URL.Query().Get("page") != "1" {
				t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
			}
			_ = json.NewEncoder(w).Encode([]any{
				deployJSON("dep_3", "error", "production", 3),
				deployJSON("dep_2", "ready", "production", 2),
				deployJSON("dep_pr", "ready", "deploy-preview", 2),
				deployJSON("dep_1", "ready", "production", 1),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	targets, err := netlifyAt(srv).ListDeploys(context.Background(), export.Request{Project: boundProject()})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Target != "site_1" || !targets[0].Complete {
		t.Fatalf("targets = %+v", targets)
	}
	deploys := targets[0].Deploys
	if len(deploys) != 2 || deploys[0].DeployID != "dep_2" || deploys[1].DeployID != "dep_1" {
		t.Fatalf("deploys = %+v", deploys)
	}
	if !deploys[0].Live || deploys[1].Live {
		t.Fatalf("live flags = %+v", deploys)
	}
	if deploys[1].DeployURL != "https://dep_1--demo.netlify.app" || deploys[1].SiteURL != "https://demo.netlify.app" {
		t.Fatalf("links = %+v", deploys[1])
	}
	if !deploys[1].CreatedAt.Equal(time.Date(2026, 10, 11, 12, 1, 0, 0, time.UTC)) {
		t.Fatalf("created at = %v", deploys[1].CreatedAt)
	}
}

func TestNetlifyListDeploysTruncatedKeepsLiveDeploy(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/sites/site_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "site_1", "ssl_url": "https://demo.netlify.app",
				"published_deploy": map[string]any{"id": "dep_old"},
			})
		case r.URL.Path == "/sites/site_1/deploys":
			pages++
			batch := make([]any, 0, 100)
			for i := range 100 {
				batch = append(batch, deployJSON(fmt.Sprintf("dep_%d_%d", pages, i), "ready", "production", 30))
			}
			_ = json.NewEncoder(w).Encode(batch)
		case r.URL.Path == "/deploys/dep_old":
			_ = json.NewEncoder(w).Encode(deployJSON("dep_old", "ready", "production", 0))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	targets, err := netlifyAt(srv).ListDeploys(context.Background(), export.Request{Project: boundProject()})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 10 {
		t.Fatalf("pages fetched = %d, want the 10-page cap", pages)
	}
	got := targets[0]
	if got.Complete {
		t.Fatal("a capped list must be reported incomplete")
	}
	last := got.Deploys[len(got.Deploys)-1]
	if len(got.Deploys) != 1001 || last.DeployID != "dep_old" || !last.Live {
		t.Fatalf("len %d last %+v", len(got.Deploys), last)
	}
}

func TestNetlifyListDeploysUnboundProjectIsEmpty(t *testing.T) {
	targets, err := export.Netlify{}.ListDeploys(context.Background(), export.Request{
		Project: catalog.Project{Remotes: json.RawMessage(`[]`)},
	})
	if err != nil || targets != nil {
		t.Fatalf("targets = %+v err = %v", targets, err)
	}
}

func TestNetlifyDeleteDeploy(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sites/site_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "site_1", "published_deploy": map[string]any{"id": "dep_live"},
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/sites/site_1/deploys/"):
			id := strings.TrimPrefix(r.URL.Path, "/sites/site_1/deploys/")
			if id == "dep_gone" {
				http.NotFound(w, r)
				return
			}
			if id == "dep_locked" {
				http.Error(w, `{"message":"nope"}`, http.StatusUnprocessableEntity)
				return
			}
			deleted = append(deleted, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	n := netlifyAt(srv)
	ctx := context.Background()
	req := export.Request{Project: boundProject()}

	if err := n.DeleteDeploy(ctx, req, "site_1", "dep_old"); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "dep_old" {
		t.Fatalf("deleted = %v", deleted)
	}
	if err := n.DeleteDeploy(ctx, req, "site_1", "dep_live"); !errors.Is(err, export.ErrPublishLive) {
		t.Fatalf("live delete err = %v", err)
	}
	if err := n.DeleteDeploy(ctx, req, "site_1", "dep_gone"); err != nil {
		t.Fatalf("already-deleted deploy err = %v", err)
	}
	if err := n.DeleteDeploy(ctx, req, "site_1", "dep_locked"); !errors.Is(err, export.ErrPublishFailed) {
		t.Fatalf("rejected delete err = %v", err)
	}
	if err := n.DeleteDeploy(ctx, req, "site_missing", "dep_x"); err != nil {
		t.Fatalf("deleted site err = %v", err)
	}
	if err := n.DeleteDeploy(ctx, req, "site_1", ""); !errors.Is(err, export.ErrPublishNotManaged) {
		t.Fatalf("no deploy id err = %v", err)
	}
	if err := (export.Netlify{}).DeleteDeploy(ctx, req, "site_1", "dep_old"); !errors.Is(err, export.ErrMissingCredentials) {
		t.Fatalf("no token err = %v", err)
	}
}

func TestRegistryAdvertisesPublishManagers(t *testing.T) {
	reg := export.DefaultRegistry()
	if got := reg.Managers(); len(got) != 1 || got[0] != export.MethodNetlify {
		t.Fatalf("managers = %v", got)
	}
	if _, ok := reg.Manager(export.MethodDownload); ok {
		t.Fatal("download must not manage publishes")
	}
	for _, m := range reg.Methods() {
		if m.ManagePublishes != (m.ID == export.MethodNetlify) {
			t.Fatalf("method %+v", m)
		}
	}
}
