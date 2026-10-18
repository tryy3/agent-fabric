package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func limitsPatch(raw string) func(*catalog.Store) {
	return func(cat *catalog.Store) {
		if _, err := cat.PatchPlaneSettingsFull(context.Background(), catalog.PlaneSettingsPatch{Limits: json.RawMessage(raw)}); err != nil {
			panic(err)
		}
	}
}

// budgetTurn builds a turn whose model keeps calling load_skill while it is
// offered tools, and answers "best answer" once it is not.
func budgetTurn(t *testing.T, setup ...func(*catalog.Store)) (skillTurn, *[]bool, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var withTools []bool
	var turn skillTurn
	turn = newSkillTurn(t, func(_ context.Context, _ string, _ []runtime.Message, onEvent func(provider.StreamEvent) error) error {
		turn.streamer.mu.Lock()
		offered := len(turn.streamer.options[len(turn.streamer.options)-1].Tools) > 0
		turn.streamer.mu.Unlock()
		mu.Lock()
		withTools = append(withTools, offered)
		mu.Unlock()
		if offered {
			return onEvent(provider.StreamEvent{Finish: "tool_calls", ToolCalls: []provider.ToolCall{
				{ID: "c", Name: "load_skill", Arguments: `{"name":"hello"}`},
			}})
		}
		return onEvent(provider.StreamEvent{Content: "best answer", Finish: "stop"})
	}, setup...)
	return turn, &withTools, &mu
}

func TestNormalPromptEndsWithAToollessRoundWhenRoundsRunOut(t *testing.T) {
	turn, withTools, mu := budgetTurn(t, limitsPatch(`{"defaultToolRounds":2}`))
	turn.prompt(t, "hi")
	mu.Lock()
	defer mu.Unlock()
	if got := *withTools; len(got) != 3 || !got[0] || !got[1] || got[2] {
		t.Fatalf("rounds offered tools = %v, want [true true false]", got)
	}
	turn.client.mu.Lock()
	defer turn.client.mu.Unlock()
	if !strings.Contains(strings.Join(turn.client.chunks, ""), "best answer") {
		t.Fatalf("chunks = %v", turn.client.chunks)
	}
}

func TestSkillRunUsesItsCeilingAndDepthShare(t *testing.T) {
	cases := map[string]int{
		"/deep q":                10,
		"/deep --depth quick q":  3,
		"/deep --depth normal q": 6,
		"/deep --depth deep q":   10,
	}
	for prompt, tool := range cases {
		turn, withTools, mu := budgetTurn(t)
		turn.prompt(t, prompt)
		mu.Lock()
		got := len(*withTools)
		mu.Unlock()
		if got != tool+1 {
			t.Errorf("%s: %d rounds, want %d tool rounds plus the final one", prompt, got, tool)
		}
	}
}

func TestPlaneMaximumCapsSkillCeiling(t *testing.T) {
	turn, withTools, mu := budgetTurn(t, limitsPatch(`{"maxToolRounds":4}`))
	turn.prompt(t, "/deep q")
	mu.Lock()
	defer mu.Unlock()
	if len(*withTools) != 5 {
		t.Fatalf("rounds = %d, want 4 plus the final one", len(*withTools))
	}
}

func TestWallClockLimitEndsWithAToollessRound(t *testing.T) {
	turn, withTools, mu := budgetTurn(t, limitsPatch(`{"maxTurnSeconds":10}`))
	var tick int64
	turn.agent.SetTestTurnClock(func() time.Time {
		tick++
		return time.Unix(1_700_000_000+tick*60, 0)
	})
	turn.prompt(t, "hi")
	mu.Lock()
	defer mu.Unlock()
	if got := *withTools; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("rounds offered tools = %v, want [true false]", got)
	}
}

func TestTimeFlagAboveTheLimitIsClampedAndSaid(t *testing.T) {
	turn, _, _ := budgetTurn(t, limitsPatch(`{"maxTurnSeconds":600}`))
	turn.prompt(t, "/deep --time 2h q")
	turn.client.mu.Lock()
	defer turn.client.mu.Unlock()
	if !strings.Contains(strings.Join(turn.client.chunks, ""), "--time 2h0m0s is above this assistant's limit; using 10m0s") {
		t.Fatalf("chunks = %v", turn.client.chunks)
	}
}
