package catalog_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func gateTestAssistant(t *testing.T, store *catalog.Store) catalog.Assistant {
	t.Helper()
	ctx := context.Background()
	p, err := store.CreateInferenceConnection(ctx, "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceInferenceConnectionModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	a, err := store.CreateAssistant(ctx, "A", "", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func gateHTTP(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestGateInstructionsPlaneAndAssistantRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openGooseStore(t)
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()
	a := gateTestAssistant(t, store)

	if a.GateInstructions != nil {
		t.Fatalf("new assistant gateInstructions = %q, want null", *a.GateInstructions)
	}
	if got := store.PlaneGateInstructions(ctx); got != "" {
		t.Fatalf("seeded plane gate instructions = %q", got)
	}

	status, body := gateHTTP(t, http.MethodPatch, srv.URL+"/v1/settings", `{"gateInstructions":"Never push to main."}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH settings = %d %s", status, body)
	}
	var plane catalog.PlaneSettings
	if err := json.Unmarshal(body, &plane); err != nil || plane.GateInstructions != "Never push to main." {
		t.Fatalf("plane after patch = %+v err=%v", plane, err)
	}
	status, body = gateHTTP(t, http.MethodGet, srv.URL+"/v1/settings", "")
	var reloaded map[string]any
	if err := json.Unmarshal(body, &reloaded); err != nil || status != http.StatusOK || reloaded["gateInstructions"] != "Never push to main." {
		t.Fatalf("GET settings = %d %s", status, body)
	}

	status, body = gateHTTP(t, http.MethodPatch, srv.URL+"/v1/assistants/"+a.ID, `{"gateInstructions":"Only edit files under src/."}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH assistant = %d %s", status, body)
	}
	var patched catalog.Assistant
	if err := json.Unmarshal(body, &patched); err != nil || patched.GateInstructions == nil || *patched.GateInstructions != "Only edit files under src/." {
		t.Fatalf("assistant after patch: %s", body)
	}
	if patched.Version != a.Version+1 {
		t.Fatalf("version %d, want %d", patched.Version, a.Version+1)
	}

	// An unrelated patch keeps the override; null clears it back to inherit.
	status, body = gateHTTP(t, http.MethodPatch, srv.URL+"/v1/assistants/"+a.ID, `{"description":"d"}`)
	if status != http.StatusOK || !strings.Contains(string(body), `"gateInstructions":"Only edit files under src/."`) {
		t.Fatalf("unrelated patch = %d %s", status, body)
	}
	status, body = gateHTTP(t, http.MethodPatch, srv.URL+"/v1/assistants/"+a.ID, `{"gateInstructions":null}`)
	if status != http.StatusOK || !strings.Contains(string(body), `"gateInstructions":null`) {
		t.Fatalf("null patch = %d %s", status, body)
	}

	// An empty string is an explicit "none", distinct from inherit.
	status, body = gateHTTP(t, http.MethodPatch, srv.URL+"/v1/assistants/"+a.ID, `{"gateInstructions":""}`)
	if status != http.StatusOK || !strings.Contains(string(body), `"gateInstructions":""`) {
		t.Fatalf("empty patch = %d %s", status, body)
	}
	got, err := store.GetAssistant(ctx, a.ID)
	if err != nil || got.GateInstructions == nil || *got.GateInstructions != "" {
		t.Fatalf("stored empty override: %+v err=%v", got.GateInstructions, err)
	}
}

func TestGateInstructionsValidation(t *testing.T) {
	store := openGooseStore(t)
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()
	a := gateTestAssistant(t, store)

	tooLong, _ := json.Marshal(map[string]string{"gateInstructions": strings.Repeat("x", catalog.MaxGateInstructionsChars+1)})
	atLimit, _ := json.Marshal(map[string]string{"gateInstructions": strings.Repeat("é", catalog.MaxGateInstructionsChars)})
	for name, target := range map[string]string{"settings": "/v1/settings", "assistant": "/v1/assistants/" + a.ID} {
		if status, body := gateHTTP(t, http.MethodPatch, srv.URL+target, string(tooLong)); status != http.StatusBadRequest || !strings.Contains(string(body), "gateInstructions") {
			t.Errorf("%s: too long = %d %s", name, status, body)
		}
		if status, body := gateHTTP(t, http.MethodPatch, srv.URL+target, `{"gateInstructions":7}`); status != http.StatusBadRequest {
			t.Errorf("%s: non-string = %d %s", name, status, body)
		}
		if status, body := gateHTTP(t, http.MethodPatch, srv.URL+target, string(atLimit)); status != http.StatusOK {
			t.Errorf("%s: at the limit counts characters, not bytes: %d %s", name, status, body)
		}
	}
	if got := store.PlaneGateInstructions(context.Background()); len([]rune(got)) != catalog.MaxGateInstructionsChars {
		t.Fatalf("stored %d characters", len([]rune(got)))
	}
}

func TestEffectiveGateInstructions(t *testing.T) {
	own, none := "  assistant rules ", ""
	for name, tc := range map[string]struct {
		plane     string
		assistant *string
		want      string
	}{
		"plane default":      {"plane rules", nil, "plane rules"},
		"assistant replaces": {"plane rules", &own, "assistant rules"},
		"empty override":     {"plane rules", &none, ""},
		"neither":            {"", nil, ""},
		"plane is trimmed":   {" plane rules\n", nil, "plane rules"},
	} {
		if got := catalog.EffectiveGateInstructions(tc.plane, tc.assistant); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

func TestInstructionsMaxCharsValidation(t *testing.T) {
	for raw, ok := range map[string]bool{
		`{"scorers":{"instructionsMaxChars":4000}}`:  true,
		`{"scorers":{"instructionsMaxChars":0}}`:     true,
		`{"scorers":{"instructionsMaxChars":50}}`:    false,
		`{"scorers":{"instructionsMaxChars":20001}}`: false,
	} {
		_, err := catalog.DecodePermissions(json.RawMessage(raw))
		if (err == nil) != ok {
			t.Errorf("%s: err=%v, want ok=%v", raw, err, ok)
		}
	}
}
