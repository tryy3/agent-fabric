package modelspecs_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
)

const sampleSpecs = `{
  "acme": {"id":"acme","name":"Acme","api":"https://api.acme.test/v1","models":{
    "big-1":{"id":"big-1","name":"Big 1","tool_call":true,"reasoning":true,
      "limit":{"context":200000,"output":8000},
      "cost":{"input":3,"output":15,"cache_read":0.3},
      "modalities":{"input":["text","image"],"output":["text"]}},
    "free-1":{"name":"Free 1","cost":{"input":0,"output":0}}}}
}`

func newService(t *testing.T) *modelspecs.Service {
	t.Helper()
	svc, err := modelspecs.New(context.Background(), db.New(dbtest.Open(t)), nil)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func setSource(t *testing.T, svc *modelspecs.Service, url string) {
	t.Helper()
	if _, err := svc.UpdateSettings(context.Background(), modelspecs.Settings{SourceURL: url, Enabled: true}); err != nil {
		t.Fatal(err)
	}
}

func TestSyncStoresSnapshotAndServesProviders(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(sampleSpecs))
	}))
	defer up.Close()
	svc := newService(t)
	setSource(t, svc, up.URL)

	st, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.ProviderCount != 1 || st.ModelCount != 2 || st.LastSyncedAt == nil || st.LastError != "" {
		t.Fatalf("status: %+v", st)
	}
	p, ok := svc.Provider("acme")
	if !ok {
		t.Fatal("provider missing")
	}
	m := p.Models["big-1"]
	if !m.ToolCall || m.Limit.Context != 200000 || *m.Cost.Input != 3 || *m.Cost.CacheRead != 0.3 {
		t.Fatalf("model: %+v", m)
	}
	if free := p.Models["free-1"]; free.ID != "free-1" || free.Cost.Input == nil || *free.Cost.Input != 0 {
		t.Fatalf("free model id/cost: %+v", free)
	}
	// Snapshot survives a restart.
	svc2, err := modelspecs.New(context.Background(), svc.QueriesForTest(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if svc2.Status().ModelCount != 2 {
		t.Fatalf("reloaded status: %+v", svc2.Status())
	}
}

func TestSyncETagNotModifiedKeepsSnapshot(t *testing.T) {
	var conditional atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(sampleSpecs))
	}))
	defer up.Close()
	svc := newService(t)
	setSource(t, svc, up.URL)
	for range 2 {
		if _, err := svc.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if conditional.Load() != 1 || svc.Status().ModelCount != 2 {
		t.Fatalf("conditional=%d status=%+v", conditional.Load(), svc.Status())
	}
}

func TestSyncFailureKeepsSnapshotAndRecordsError(t *testing.T) {
	var fail atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(sampleSpecs))
	}))
	defer up.Close()
	svc := newService(t)
	setSource(t, svc, up.URL)
	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	st, err := svc.Sync(context.Background())
	if err == nil || !strings.Contains(st.LastError, "500") {
		t.Fatalf("want error, got err=%v status=%+v", err, st)
	}
	if st.ModelCount != 2 || st.LastSyncedAt == nil {
		t.Fatalf("snapshot lost: %+v", st)
	}
}

func TestSyncRejectsInvalidDocuments(t *testing.T) {
	for name, body := range map[string]string{"not json": "<html>", "empty": "{}"} {
		t.Run(name, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer up.Close()
			svc := newService(t)
			setSource(t, svc, up.URL)
			if _, err := svc.Sync(context.Background()); err == nil {
				t.Fatal("expected error")
			}
			if svc.Status().ProviderCount != 0 {
				t.Fatal("invalid document must not replace snapshot")
			}
		})
	}
}

func TestUpdateSettingsValidation(t *testing.T) {
	svc := newService(t)
	if _, err := svc.UpdateSettings(context.Background(), modelspecs.Settings{SourceURL: "ftp://x"}); err == nil {
		t.Fatal("want error for non-http source")
	}
	st := svc.Status()
	if st.EffectiveSourceURL != modelspecs.DefaultSourceURL || st.SyncIntervalHours != 24 || !st.Enabled {
		t.Fatalf("defaults: %+v", st)
	}
}

func TestHTTPSyncAndSettings(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleSpecs))
	}))
	defer up.Close()
	svc := newService(t)
	api := httptest.NewServer(modelspecs.Handler(svc))
	defer api.Close()

	req, _ := http.NewRequest(http.MethodPatch, api.URL+"/v1/model-specs/settings",
		strings.NewReader(`{"sourceUrl":"`+up.URL+`"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("patch: %v %v", err, resp)
	}
	resp, err = http.Post(api.URL+"/v1/model-specs/sync", "application/json", nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("sync: %v %v", err, resp)
	}
	var st modelspecs.Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil || st.ModelCount != 2 {
		t.Fatalf("decode: %v %+v", err, st)
	}
}

func TestProviderForAndLookup(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleSpecs))
	}))
	defer up.Close()
	svc := newService(t)
	setSource(t, svc, up.URL)
	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, ok := svc.ProviderFor("openai_compatible", "https://API.acme.test/v1/")
	if !ok || p.ID != "acme" {
		t.Fatalf("baseUrl match: %v %v", p.ID, ok)
	}
	if _, ok := svc.ProviderFor("openai_compatible", ""); ok {
		t.Fatal("empty baseUrl must not match")
	}
	for _, id := range []string{"big-1", "acme/big-1", "BIG-1"} {
		if _, ok := p.Lookup(id); !ok {
			t.Fatalf("lookup %q failed", id)
		}
	}
	if _, ok := p.Lookup("nope"); ok {
		t.Fatal("unexpected match")
	}
}

func TestLogoProxyServesFromSourceAndCachesMiss(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api.json", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(sampleSpecs)) })
	mux.HandleFunc("/logos/acme.svg", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("<svg/>"))
	})
	up := httptest.NewServer(mux)
	defer up.Close()
	svc := newService(t)
	setSource(t, svc, up.URL+"/api.json")
	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(modelspecs.Handler(svc))
	defer api.Close()

	for range 2 {
		resp, err := http.Get(api.URL + "/v1/model-specs/providers/acme/logo")
		if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" {
			t.Fatalf("logo: %v %v", err, resp)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("logo fetched %d times, want cached", hits.Load())
	}
	for _, id := range []string{"missing", "..%2Fapi.json"} {
		resp, err := http.Get(api.URL + "/v1/model-specs/providers/" + id + "/logo")
		if err != nil || resp.StatusCode != 404 {
			t.Fatalf("%s: %v %v", id, err, resp)
		}
	}
	resp, err := http.Get(api.URL + "/v1/model-specs/providers")
	if err != nil || resp.StatusCode != 200 {
		t.Fatal(err)
	}
	var list []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list) != 1 || list[0]["logoUrl"] != "/v1/model-specs/providers/acme/logo" {
		t.Fatalf("providers: %v", list)
	}
}
