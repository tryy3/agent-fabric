package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/scrub"
)

func TestHopCaptureInsertLinkAndListHTTP(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var th catalog.Thread
	_ = json.NewDecoder(resp.Body).Decode(&th)
	resp.Body.Close()

	ctx := context.Background()
	h := http.Header{}
	h.Set("Authorization", "Bearer sk-secret-key")
	h.Set("Content-Type", "application/json")
	_, err = store.InsertLLMHopCapture(ctx, catalog.InsertLLMHopCaptureParams{
		ThreadID:   th.ID,
		SessionID:  "sess_1",
		RoundIndex: 0,
		Method:     http.MethodPost,
		URL:        "http://llm.test/v1/chat/completions",
		StatusCode: 200,
		ReqHeaders: h,
		RespHeaders: http.Header{"Content-Type": []string{"text/event-stream"}},
		ReqBody:    `{"model":"m","messages":[{"role":"user","content":"hi alice@acme.com"}]}`,
		RespBody:   `{"content":"ok"}`,
		Meta:       map[string]any{"model": "m"},
		Pipeline: scrub.Pipeline{
			Headers: scrub.DefaultHeaders{},
			Body:    scrub.Fake{Replacements: map[string]string{"alice@acme.com": "«Email_1»"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.CommitTurn(ctx, th.ID, "hi", catalog.AssistantTurn{
		Content:          "ok",
		CaptureSessionID: "sess_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetThread(ctx, updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	var assistantID string
	for _, m := range detail.Messages {
		if m.Role == "assistant" {
			assistantID = m.ID
			break
		}
	}
	if assistantID == "" {
		t.Fatal("no assistant message")
	}

	listResp, err := http.Get(srv.URL + "/v1/threads/" + th.ID + "/messages/" + assistantID + "/captures")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", listResp.StatusCode)
	}
	var captures []catalog.HopCapture
	if err := json.NewDecoder(listResp.Body).Decode(&captures); err != nil {
		t.Fatal(err)
	}
	if len(captures) != 1 {
		t.Fatalf("captures = %+v", captures)
	}
	auth, _ := captures[0].Headers["request"].(map[string]any)
	if auth == nil {
		t.Fatalf("headers %+v", captures[0].Headers)
	}
	if auth["Authorization"] != "[REDACTED]" {
		t.Fatalf("Authorization = %v", auth["Authorization"])
	}
	if !strings.Contains(captures[0].BodyText, "«Email_1»") {
		t.Fatalf("body not scrubbed: %q", captures[0].BodyText)
	}
	if strings.Contains(captures[0].BodyText, "alice@acme.com") {
		t.Fatalf("raw email leaked: %q", captures[0].BodyText)
	}

	// Second turn keeps its own Round 0; thread list returns both hops.
	_, err = store.InsertLLMHopCapture(ctx, catalog.InsertLLMHopCaptureParams{
		ThreadID:    th.ID,
		SessionID:   "sess_2",
		RoundIndex:  0,
		Method:      http.MethodPost,
		URL:         "http://llm.test/v1/chat/completions",
		StatusCode:  200,
		ReqHeaders:  h,
		RespHeaders: http.Header{"Content-Type": []string{"text/event-stream"}},
		ReqBody:     `{"model":"m","messages":[{"role":"user","content":"again"}]}`,
		RespBody:    `{"content":"yo"}`,
		Meta:        map[string]any{"model": "m"},
		Pipeline: scrub.Pipeline{
			Headers: scrub.DefaultHeaders{},
			Body:    scrub.Identity{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitTurn(ctx, th.ID, "again", catalog.AssistantTurn{
		Content:          "yo",
		CaptureSessionID: "sess_2",
	}); err != nil {
		t.Fatal(err)
	}
	threadListResp, err := http.Get(srv.URL + "/v1/threads/" + th.ID + "/captures")
	if err != nil {
		t.Fatal(err)
	}
	defer threadListResp.Body.Close()
	if threadListResp.StatusCode != http.StatusOK {
		t.Fatalf("thread captures status %d", threadListResp.StatusCode)
	}
	var threadCaptures []catalog.HopCapture
	if err := json.NewDecoder(threadListResp.Body).Decode(&threadCaptures); err != nil {
		t.Fatal(err)
	}
	if len(threadCaptures) != 2 {
		t.Fatalf("thread captures = %+v", threadCaptures)
	}
	if threadCaptures[0].RoundIndex != 0 || threadCaptures[1].RoundIndex != 0 {
		t.Fatalf("expected per-turn round 0s, got %+v", threadCaptures)
	}
	if threadCaptures[0].MessageID == nil || threadCaptures[1].MessageID == nil ||
		*threadCaptures[0].MessageID == *threadCaptures[1].MessageID {
		t.Fatalf("expected distinct message ids, got %+v %+v", threadCaptures[0].MessageID, threadCaptures[1].MessageID)
	}
}

func TestPatchViewModeRawAllowed(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var created catalog.Thread
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID,
		strings.NewReader(`{"viewModeId":"raw"}`))
	req.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", patchResp.StatusCode)
	}
	var patched catalog.Thread
	_ = json.NewDecoder(patchResp.Body).Decode(&patched)
	if patched.ViewModeID == nil || *patched.ViewModeID != "raw" {
		t.Fatalf("patched %+v", patched)
	}
}
