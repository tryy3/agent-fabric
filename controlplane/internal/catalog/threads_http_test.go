package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestThreadsHTTPCreateListGetRename(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var created catalog.Thread
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if created.Title != "Untitled" {
		t.Fatalf("title %q", created.Title)
	}

	listResp, err := http.Get(srv.URL + "/v1/threads")
	if err != nil {
		t.Fatal(err)
	}
	var list []catalog.ThreadListItem
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if len(list) != 1 || list[0].ID != created.ID || list[0].MessageCount != 0 {
		t.Fatalf("list %+v", list)
	}

	got, err := http.Get(srv.URL + "/v1/threads/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var detail catalog.ThreadDetail
	if err := json.NewDecoder(got.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	got.Body.Close()
	if detail.Messages == nil {
		t.Fatal("messages must be [] not null")
	}
	if detail.MessageCount != 0 {
		t.Fatalf("empty messageCount = %d", detail.MessageCount)
	}

	if _, err := store.CommitTurn(context.Background(), created.ID, "hi", catalog.AssistantTurn{Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	got, err = http.Get(srv.URL + "/v1/threads/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(got.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	got.Body.Close()
	if detail.MessageCount != 2 {
		t.Fatalf("messageCount = %d, want 2", detail.MessageCount)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d", len(detail.Messages))
	}
	as := detail.Messages[1]
	if len(as.Parts) != 1 || as.Parts[0].Type != "message" || as.Parts[0].Text != "hello" {
		t.Fatalf("assistant parts = %+v", as.Parts)
	}
	if detail.Messages[0].Parts == nil {
		t.Fatal("user parts must be [] not null")
	}

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID, strings.NewReader(`{"title":"Renamed"}`))
	req.Header.Set("content-type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch %d", patch.StatusCode)
	}
	patch.Body.Close()

	bad, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID, strings.NewReader(`{"title":"  "}`))
	bad.Header.Set("content-type", "application/json")
	badResp, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty title status %d", badResp.StatusCode)
	}
	badResp.Body.Close()

	missing, err := http.Get(srv.URL + "/v1/threads/th_nope")
	if err != nil {
		t.Fatal(err)
	}
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status %d", missing.StatusCode)
	}
	missing.Body.Close()
}

func TestDeleteAgentWithThreadConflict(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(ctx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, ag.ID); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/agents/"+ag.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
