package export_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/export"
)

func TestWorkspaceZipIsRootRelative(t *testing.T) {
	ctx := context.Background()
	fsys := newMemFS()
	if err := fsys.WriteFile(ctx, "index.html", []byte("<h1>hi</h1>")); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(ctx, "css/app.css", []byte("body{}")); err != nil {
		t.Fatal(err)
	}
	reg := export.NewRegistry()
	reg.Register(export.Netlify{
		Token:      "tok",
		HTTPClient: http.DefaultClient,
		APIBase:    "http://127.0.0.1:1", // unused; we only test zip via download path
	})
	// Exercise buildWorkspaceZip indirectly through a scripted Netlify server.
	var gotZip []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sites"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "site_1", "ssl_url": "https://demo.netlify.app",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deploys"):
			gotZip, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_1", "state": "uploaded",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/deploys/dep_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_1", "state": "ready",
				"ssl_url":        "https://demo.netlify.app",
				"deploy_ssl_url": "https://dep_1--demo.netlify.app",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	result, err := export.Netlify{
		Token:      "tok",
		HTTPClient: srv.Client(),
		APIBase:    srv.URL,
		PollEvery:  time.Millisecond,
		PollLimit:  time.Second,
	}.Export(ctx, export.Request{
		Project: catalog.Project{Name: "Demo Site", Remotes: json.RawMessage(`[]`)},
		FS:      fsys,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Links) != 2 {
		t.Fatalf("links = %+v", result.Links)
	}
	files := zipNames(t, gotZip)
	// Netlify mishandles a lone file at zip root (path becomes "/"); keep a
	// single top-level folder that Netlify strips on publish.
	if files["index.html"] || files["workspace/index.html"] {
		t.Fatalf("workspace zip must not place files at archive root: %v", keys(files))
	}
	if !files["site/index.html"] || !files["site/css/app.css"] {
		t.Fatalf("workspace zip entries = %v", keys(files))
	}
	if !strings.Contains(string(result.RemotesPatch), `"netlify"`) {
		t.Fatalf("remotes patch = %s", result.RemotesPatch)
	}
}

func TestWorkspaceZipWrapsSingleFile(t *testing.T) {
	ctx := context.Background()
	fsys := newMemFS()
	if err := fsys.WriteFile(ctx, "index.html", []byte("<h1>solo</h1>")); err != nil {
		t.Fatal(err)
	}
	var gotZip []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sites"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "site_1", "ssl_url": "https://solo.netlify.app",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deploys"):
			gotZip, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_1", "state": "uploaded",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/deploys/dep_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_1", "state": "ready",
				"ssl_url": "https://solo.netlify.app", "deploy_ssl_url": "https://dep--solo.netlify.app",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	_, err := export.Netlify{
		Token: "tok", HTTPClient: srv.Client(), APIBase: srv.URL,
		PollEvery: time.Millisecond, PollLimit: time.Second,
	}.Export(ctx, export.Request{
		Project: catalog.Project{Name: "Solo", Remotes: json.RawMessage(`[]`)},
		FS:      fsys,
	})
	if err != nil {
		t.Fatal(err)
	}
	files := zipNames(t, gotZip)
	if files["index.html"] || !files["site/index.html"] || len(files) != 1 {
		t.Fatalf("single-file zip must be site/index.html only, got %v", keys(files))
	}
}

func TestNetlifyReusesExistingSite(t *testing.T) {
	ctx := context.Background()
	fsys := newMemFS()
	_ = fsys.WriteFile(ctx, "index.html", []byte("ok"))
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sites":
			creates.Add(1)
			http.Error(w, "should not create", http.StatusInternalServerError)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/sites/site_existing/deploys"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_2", "state": "building",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/deploys/dep_2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_2", "state": "ready",
				"ssl_url":        "https://existing.netlify.app",
				"deploy_ssl_url": "https://dep_2--existing.netlify.app",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	result, err := export.Netlify{
		Token:      "tok",
		HTTPClient: srv.Client(),
		APIBase:    srv.URL,
		PollEvery:  time.Millisecond,
		PollLimit:  time.Second,
	}.Export(ctx, export.Request{
		Project: catalog.Project{
			Name: "Existing",
			Remotes: json.RawMessage(`[{
				"id":"rmt_netlify","kind":"netlify",
				"path":"site_existing","urlOrBucket":"https://existing.netlify.app"
			}]`),
		},
		FS: fsys,
	})
	if err != nil {
		t.Fatal(err)
	}
	if creates.Load() != 0 {
		t.Fatalf("unexpected site create")
	}
	if result.Links[0].URL != "https://existing.netlify.app" {
		t.Fatalf("links = %+v", result.Links)
	}
}

func TestNetlifyMissingCredentials(t *testing.T) {
	_, err := export.Netlify{}.Export(context.Background(), export.Request{
		Project: catalog.Project{Name: "X"},
	})
	if !errors.Is(err, export.ErrMissingCredentials) {
		t.Fatalf("err = %v", err)
	}
}

func TestNetlifyReadsIntegrationsBag(t *testing.T) {
	ctx := context.Background()
	fsys := newMemFS()
	_ = fsys.WriteFile(ctx, "index.html", []byte("ok"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/acct_1/sites"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "site_a", "ssl_url": "https://a.netlify.app",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deploys"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_a", "state": "uploaded",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/deploys/dep_a":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dep_a", "state": "ready",
				"ssl_url": "https://a.netlify.app", "deploy_ssl_url": "https://dep--a.netlify.app",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	_, err := export.Netlify{
		HTTPClient: srv.Client(),
		APIBase:    srv.URL,
		PollEvery:  time.Millisecond,
		PollLimit:  time.Second,
	}.Export(ctx, export.Request{
		Project: catalog.Project{Name: "From Settings", Remotes: json.RawMessage(`[]`)},
		FS:      fsys,
		Integrations: json.RawMessage(`{
			"netlify":{"apiKey":"secret-token","accountId":"acct_1"}
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNetlifyDeployError(t *testing.T) {
	ctx := context.Background()
	fsys := newMemFS()
	_ = fsys.WriteFile(ctx, "index.html", []byte("ok"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sites":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "site_1", "ssl_url": "https://x.netlify.app"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deploys"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "dep_err", "state": "error", "error_message": "build failed"})
		case r.Method == http.MethodGet && r.URL.Path == "/deploys/dep_err":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "dep_err", "state": "error", "error_message": "build failed"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	_, err := export.Netlify{
		Token: "tok", HTTPClient: srv.Client(), APIBase: srv.URL,
		PollEvery: time.Millisecond, PollLimit: time.Second,
	}.Export(ctx, export.Request{
		Project: catalog.Project{Name: "Bad", Remotes: json.RawMessage(`[]`)},
		FS:      fsys,
	})
	if !errors.Is(err, export.ErrPublishFailed) || !strings.Contains(err.Error(), "build failed") {
		t.Fatalf("err = %v", err)
	}
}
