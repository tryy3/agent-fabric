package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

// researchScript plays a model that follows the deep-research skill: it writes
// a plan, then keeps rewriting the report until it is offered no more tools,
// and answers with the report path.
func researchScript(mu *sync.Mutex, rounds *int, sawInjected *string) func(context.Context, string, []runtime.Message, func(provider.StreamEvent) error) error {
	return func(_ context.Context, _ string, msgs []runtime.Message, onEvent func(provider.StreamEvent) error) error {
		mu.Lock()
		*rounds++
		n := *rounds
		if n == 1 && len(msgs) > 0 {
			*sawInjected = msgs[len(msgs)-1].Content
		}
		mu.Unlock()
		offered := false
		for _, m := range msgs {
			if m.Role == "user" && strings.Contains(m.Content, "tool budget for this run is used up") {
				offered = true
			}
		}
		if offered {
			return onEvent(provider.StreamEvent{Content: "Report written to research/solar/report.md", Finish: "stop"})
		}
		return onEvent(provider.StreamEvent{Finish: "tool_calls", ToolCalls: []provider.ToolCall{{
			ID:        "w",
			Name:      "write_file",
			Arguments: `{"path":"research/solar/report.md","content":"# Plan\nround ` + string(rune('A'+n%26)) + `\n"}`,
		}}})
	}
}

func TestDeepResearchSkillRunsWithItsFlagsBudgetAndFiles(t *testing.T) {
	var mu sync.Mutex
	rounds := 0
	injected := ""
	turn := newSkillTurn(t, researchScript(&mu, &rounds, &injected))
	turn.client.permissionFn = func(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		return selectOption("allow_session")(req), nil
	}
	turn.prompt(t, "/deep-research --depth quick --save project is solar worth it?")

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{
		"# Deep research", "depth: quick", "save: project", "is solar worth it?",
		`name="deep-research"`, `source="builtin"`, "references/report-format.md",
	} {
		if !strings.Contains(injected, want) {
			t.Errorf("model input missing %q", want)
		}
	}
	// quick = 25% of the skill's 80-round ceiling: 20 tool rounds and the final one.
	if rounds != 21 {
		t.Errorf("rounds = %d, want 21", rounds)
	}

	root := sandbox.ProjectFilesRoot(turn.dataRoot, mustProjectID(t, turn))
	data, err := os.ReadFile(filepath.Join(root, "research", "solar", "report.md"))
	if err != nil || !strings.HasPrefix(string(data), "# Plan") {
		t.Fatalf("report.md = %q, %v", data, err)
	}
}

func mustProjectID(t *testing.T, turn skillTurn) string {
	t.Helper()
	th, err := turn.cat.GetThread(context.Background(), turn.threadID)
	if err != nil {
		t.Fatal(err)
	}
	return th.ProjectID
}

func TestDeepResearchHiddenWithoutAProject(t *testing.T) {
	// A skill that needs a sandbox is not pinned for a session with no thread.
	turn := newSkillTurn(t, stopWith("ok"))
	resp, err := turn.csc.NewSession(turn.ctx, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"assistantId": mustAssistantID(t, turn)},
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := turn.rt.Get(string(resp.SessionId))
	if len(sess.Pin.Skills) != 0 {
		t.Fatalf("pinned skills without a project = %+v", sess.Pin.Skills)
	}
}

func mustAssistantID(t *testing.T, turn skillTurn) string {
	t.Helper()
	pinned, ok := turn.rt.Get(string(turn.session))
	if !ok {
		t.Fatal("session not stored")
	}
	return pinned.Pin.AssistantID
}
