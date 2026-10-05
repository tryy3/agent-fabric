package agent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestPromptPermissionRules(t *testing.T) {
	rules := `{"mode":"auto","rules":[
		{"tool":"run_command","match":"ls *","action":"ask"},
		{"tool":"run_command","match":"rm old.txt","action":"allow"},
		{"tool":"write_file","match":"**/.env*","action":"deny"}]}`
	cases := []struct {
		name       string
		call       scriptedCall
		wantPrompt bool
		wantStatus acp.ToolCallStatus
		wantRule   string
	}{
		{"ask rule prompts in a mode that would run it", scriptedCall{"run_command", `{"command":["ls"]}`}, true, acp.ToolCallStatusCompleted, "rules.user_ask"},
		{"allow rule runs a destructive command silently", scriptedCall{"run_command", `{"command":["rm","old.txt"]}`}, false, acp.ToolCallStatusCompleted, "rules.user_allow"},
		{"deny rule refuses without asking", scriptedCall{"write_file", `{"path":"api/.env","content":"x"}`}, false, acp.ToolCallStatusFailed, "rules.user_deny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompted := false
			res := runGatedTurn(t, func(context.Context, *catalog.Store) string { return rules },
				map[string]string{"old.txt": "bye"}, []scriptedCall{tc.call},
				func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
					prompted = true
					return selectOption("allow_once")(req)
				})
			if prompted != tc.wantPrompt {
				t.Fatalf("prompted = %v, want %v", prompted, tc.wantPrompt)
			}
			res.client.mu.Lock()
			defer res.client.mu.Unlock()
			u := res.client.toolCallUpdates[0]
			g := gateMetaOf(t, u)
			if *u.Status != tc.wantStatus || g["ruleId"] != tc.wantRule || g["userRule"] != true {
				t.Fatalf("status = %v, gate meta = %#v", *u.Status, g)
			}
		})
	}
}

func TestPromptPlanePermissionRulesApplyToEveryAssistant(t *testing.T) {
	prompted := false
	res := runGatedTurn(t, func(ctx context.Context, cat *catalog.Store) string {
		if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{
			Permissions: json.RawMessage(`{"rules":[{"tool":"run_command","match":"rm *","action":"deny"}]}`),
		}); err != nil {
			t.Fatal(err)
		}
		// The assistant's own allow does not outrank the plane-wide deny.
		return `{"mode":"full","rules":[{"tool":"run_command","match":"rm *","action":"allow"}]}`
	}, map[string]string{"old.txt": "bye"},
		[]scriptedCall{{"run_command", `{"command":["rm","old.txt"]}`}},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			prompted = true
			return selectOption("allow_once")(req)
		})
	if prompted {
		t.Fatal("a deny rule must not prompt")
	}
	if _, err := os.Stat(filepath.Join(res.ws, "old.txt")); err != nil {
		t.Fatalf("the plane-wide deny must keep the file: %v", err)
	}
}

// systemOneServer is a scripted System One endpoint: it answers every request
// with the given level and confidence and records the request bodies.
type systemOneServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newSystemOneServer(t *testing.T, level, confidence string) *systemOneServer {
	t.Helper()
	s := &systemOneServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(raw))
		s.mu.Unlock()
		_, _ = w.Write([]byte(`{"answers":{"risk":{"score":` + level + `,"confidence":` + confidence + `}},"usage":{"input_tokens":10}}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func fastScorerPermissions(t *testing.T, mode, baseURL string) func(context.Context, *catalog.Store) string {
	return func(ctx context.Context, cat *catalog.Store) string {
		conn, err := cat.CreateInferenceConnection(ctx, "jev", catalog.TypeOpenAICompatible, baseURL, "sk-gate-secret-key")
		if err != nil {
			t.Fatal(err)
		}
		return `{"mode":"` + mode + `","scorers":{"fast":{"connectionId":"` + conn.ID + `","model":"jev-test","strategy":"score"}}}`
	}
}

func TestPromptFastScorerRaisesAndIsCaptured(t *testing.T) {
	srv := newSystemOneServer(t, "8", "0.9") // level 8 = risk 9: cancel
	res := runGatedTurn(t, fastScorerPermissions(t, "auto", srv.URL), nil,
		[]scriptedCall{{"run_command", `{"command":["npm","test"]}`}, {"run_command", `{"command":["ls"]}`}},
		selectOption("allow_once"))

	res.client.mu.Lock()
	first, second := gateMetaOf(t, res.client.toolCallUpdates[0]), gateMetaOf(t, res.client.toolCallUpdates[1])
	res.client.mu.Unlock()
	if first["outcome"] != "cancelled" || first["risk"] != float64(9) || !strings.HasPrefix(first["source"].(string), "systemone:") {
		t.Fatalf("the fast scorer's 9 must cancel npm test: %#v", first)
	}
	if scores, _ := first["scores"].([]any); len(scores) != 2 {
		t.Fatalf("want the rules and the scorer in scores: %#v", first["scores"])
	}
	if second["outcome"] != "allowed" || second["risk"] != float64(1) {
		t.Fatalf("a read the rules call safe never reaches the scorer: %#v", second)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.bodies) != 1 {
		t.Fatalf("scorer calls = %d, want 1 (ls is skipped)", len(srv.bodies))
	}
	if !strings.Contains(srv.bodies[0], "reorganize") {
		t.Errorf("the scorer must see the user's request: %s", srv.bodies[0])
	}

	captures, err := res.cat.ListHopCapturesByThread(context.Background(), res.threadID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range captures {
		if c.Meta["hop"] != "gate_scorer" {
			continue
		}
		found = true
		blob, _ := json.Marshal(c)
		if strings.Contains(string(blob), "sk-gate-secret-key") {
			t.Fatal("scorer capture leaks the API key")
		}
		if c.URL == nil || !strings.HasSuffix(*c.URL, "/v1/systemone") {
			t.Errorf("capture url = %v", c.URL)
		}
	}
	if !found {
		t.Fatalf("no gate_scorer hop capture among %d captures", len(captures))
	}
}
