package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

// The prompt-test harness reports a docker sandbox kind to the Gate but runs
// commands through the local executor, so these exercise the whole Gate →
// permission → tool path.

func commandUpdateStatuses(client *captureClient) []acp.ToolCallStatus {
	var out []acp.ToolCallStatus
	for _, u := range client.toolCallUpdates {
		out = append(out, *u.Status)
	}
	return out
}

func TestPromptRunCommandReadOnlyRunsSilently(t *testing.T) {
	asked := false
	_, client := runFileToolTurn(t, map[string]string{"a.txt": "x"},
		[]scriptedCall{{"run_command", `{"command":["ls"]}`}},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			asked = true
			return selectOption("allow_once")(req)
		},
	)
	if asked {
		t.Fatal("ls must not prompt")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.toolCalls) != 1 || client.toolCalls[0].Title != "Run command" || client.toolCalls[0].Kind != acp.ToolKindExecute {
		t.Fatalf("tool calls = %+v", client.toolCalls)
	}
	if got := commandUpdateStatuses(client); len(got) != 1 || got[0] != acp.ToolCallStatusCompleted {
		t.Fatalf("statuses = %v", got)
	}
	out := client.toolCallUpdates[0].RawOutput
	if m, _ := out.(map[string]any); m == nil || !strings.Contains(m["stdout"].(string), "a.txt") {
		t.Fatalf("output = %#v", out)
	}
}

func TestPromptRunCommandSessionGrantAsksOnce(t *testing.T) {
	prompts := 0
	var rawInput map[string]any
	var offered []string
	_, client := runFileToolTurn(t, map[string]string{"build.sh": "echo built\n"},
		[]scriptedCall{
			{"run_command", `{"command":["sh","build.sh"]}`},
			{"run_command", `{"command":["sh","build.sh"]}`},
		},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			prompts++
			rawInput, _ = req.ToolCall.RawInput.(map[string]any)
			offered = nil
			for _, o := range req.Options {
				offered = append(offered, string(o.OptionId))
			}
			return selectOption("allow_session")(req)
		},
	)
	if prompts != 1 {
		t.Fatalf("prompts = %d, want 1 (second call covered by the session grant)", prompts)
	}
	if strings.Join(offered, ",") != "allow_once,allow_session,reject_once" {
		t.Fatalf("options = %v", offered)
	}
	if rawInput["grantKey"] != "sh build.sh" || rawInput["command"] == nil {
		t.Fatalf("rawInput = %#v", rawInput)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, s := range commandUpdateStatuses(client) {
		if s != acp.ToolCallStatusCompleted {
			t.Fatalf("statuses = %v", commandUpdateStatuses(client))
		}
	}
}

func TestPromptRunCommandRejectFails(t *testing.T) {
	ws, client := runFileToolTurn(t, map[string]string{"touch.sh": "touch ran\n"},
		[]scriptedCall{{"run_command", `{"command":["sh","touch.sh"]}`}},
		selectOption("reject_once"),
	)
	if _, err := os.Stat(filepath.Join(ws, "ran")); !os.IsNotExist(err) {
		t.Fatalf("rejected command ran: %v", err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if got := commandUpdateStatuses(client); len(got) != 1 || got[0] != acp.ToolCallStatusFailed {
		t.Fatalf("statuses = %v", got)
	}
}

func TestPromptRunCommandDestructiveHasNoSessionGrant(t *testing.T) {
	var offered []string
	ws, _ := runFileToolTurn(t, map[string]string{"old.txt": "bye"},
		[]scriptedCall{{"run_command", `{"command":["rm","old.txt"]}`}},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			for _, o := range req.Options {
				offered = append(offered, string(o.OptionId))
			}
			return selectOption("reject_once")(req)
		},
	)
	if strings.Join(offered, ",") != "allow_once,reject_once" {
		t.Fatalf("options = %v", offered)
	}
	if _, err := os.Stat(filepath.Join(ws, "old.txt")); err != nil {
		t.Fatalf("file removed despite reject: %v", err)
	}
}

func TestPromptRunCommandNonZeroExitIsUsableResult(t *testing.T) {
	_, client := runFileToolTurn(t, map[string]string{"fail.sh": "echo oops >&2\nexit 3\n"},
		[]scriptedCall{{"run_command", `{"command":["sh","fail.sh"]}`}},
		selectOption("allow_once"),
	)
	client.mu.Lock()
	defer client.mu.Unlock()
	if got := commandUpdateStatuses(client); len(got) != 1 || got[0] != acp.ToolCallStatusCompleted {
		t.Fatalf("statuses = %v", got)
	}
	m, _ := client.toolCallUpdates[0].RawOutput.(map[string]any)
	if m == nil || m["exit_code"] != float64(3) || !strings.Contains(m["stderr"].(string), "oops") {
		t.Fatalf("output = %#v", client.toolCallUpdates[0].RawOutput)
	}
}

func TestPromptRunCommandCwdEscapeDenied(t *testing.T) {
	asked := false
	_, client := runFileToolTurn(t, nil,
		[]scriptedCall{{"run_command", `{"command":["ls"],"cwd":"../.."}`}},
		func(req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
			asked = true
			return selectOption("allow_once")(req)
		},
	)
	if asked {
		t.Fatal("cwd escape must be denied without prompting")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if got := commandUpdateStatuses(client); len(got) != 1 || got[0] != acp.ToolCallStatusFailed {
		t.Fatalf("statuses = %v", got)
	}
}
