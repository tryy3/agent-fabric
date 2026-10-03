package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestPromptPermissionModes(t *testing.T) {
	cases := []struct {
		mode       string
		argv       string
		wantPrompt bool
		wantRisk   float64
		wantRan    bool // the file is gone afterwards
	}{
		{"ask", `["rm","old.txt"]`, true, 6, true},
		{"auto_approve", `["rm","old.txt"]`, true, 6, true},
		{"auto", `["rm","old.txt"]`, false, 0, true},
		{"auto", `["rm","-rf","old.txt"]`, true, 7, true},
		{"full", `["rm","-rf","old.txt"]`, false, 0, true},
		{"full", `["sudo","ls"]`, false, 0, false},
		{"ask", `["sudo","ls"]`, false, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.mode+tc.argv, func(t *testing.T) {
			var risk any
			prompted := false
			ws, _ := runFileToolTurnMode(t, tc.mode, map[string]string{"old.txt": "bye"},
				[]scriptedCall{{"run_command", `{"command":` + tc.argv + `}`}},
				func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
					prompted = true
					raw, _ := req.ToolCall.RawInput.(map[string]any)
					risk = raw["risk"]
					return selectOption("allow_once")(req)
				},
			)
			if prompted != tc.wantPrompt {
				t.Fatalf("prompted = %v, want %v", prompted, tc.wantPrompt)
			}
			if tc.wantPrompt && risk != tc.wantRisk {
				t.Fatalf("risk = %v, want %v", risk, tc.wantRisk)
			}
			_, err := os.Stat(filepath.Join(ws, "old.txt"))
			if gone := os.IsNotExist(err); tc.argv != `["sudo","ls"]` && gone != tc.wantRan {
				t.Fatalf("file removed = %v, want %v", gone, tc.wantRan)
			}
		})
	}
}

func TestPromptPermissionModeFullElevatesPathEscape(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompted := false
	_, client := runFileToolTurnMode(t, "full", nil,
		[]scriptedCall{{"read_file", `{"path":` + jsonQuote(filepath.Join(outside, "x.txt")) + `}`}},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			prompted = true
			return selectOption("reject_once")(req)
		},
	)
	if prompted {
		t.Fatal("full mode must not prompt")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if got := commandUpdateStatuses(client); len(got) != 1 || got[0] != acp.ToolCallStatusCompleted {
		t.Fatalf("statuses = %v", got)
	}
}

func gateMetaOf(t *testing.T, u acp.SessionToolCallUpdate) map[string]any {
	t.Helper()
	g, _ := u.Meta["gate"].(map[string]any)
	if g == nil {
		t.Fatalf("tool call update has no _meta.gate: %+v", u.Meta)
	}
	return g
}

func TestPromptToolCallCarriesGateMetaNotSeenByModel(t *testing.T) {
	cases := []struct {
		name, mode, argv, answer string
		wantOutcome              string
		wantRisk                 float64
		wantBand                 string
	}{
		{"read-only runs", "auto_approve", `["ls"]`, "allow_once", "allowed", 1, "safe"},
		{"approved", "ask", `["rm","old.txt"]`, "allow_once", "approved", 6, "elevated"},
		{"rejected", "ask", `["rm","old.txt"]`, "reject_once", "rejected", 6, "elevated"},
		{"hard deny", "auto", `["sudo","ls"]`, "allow_once", "cancelled", 10, "cancel"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, client := runFileToolTurnMode(t, tc.mode, map[string]string{"old.txt": "x"},
				[]scriptedCall{{"run_command", `{"command":` + tc.argv + `}`}},
				selectOption(tc.answer),
			)
			client.mu.Lock()
			defer client.mu.Unlock()
			if len(client.toolCallUpdates) != 1 {
				t.Fatalf("updates = %d", len(client.toolCallUpdates))
			}
			g := gateMetaOf(t, client.toolCallUpdates[0])
			if g["outcome"] != tc.wantOutcome || g["risk"] != tc.wantRisk || g["band"] != tc.wantBand || g["mode"] != tc.mode {
				t.Fatalf("gate meta = %#v", g)
			}
			scores, _ := g["scores"].([]any)
			if len(scores) == 0 {
				t.Fatalf("no per-evaluator scores: %#v", g)
			}
			if len(client.toolMessagesSeenByModel) != 1 {
				t.Fatalf("model saw %d tool messages", len(client.toolMessagesSeenByModel))
			}
			seen := client.toolMessagesSeenByModel[0].Content
			for _, leak := range []string{"elevated", "rules.", `"mode"`, "gate"} {
				if strings.Contains(seen, leak) {
					t.Errorf("model-visible tool result leaks %q: %s", leak, seen)
				}
			}
		})
	}
}
