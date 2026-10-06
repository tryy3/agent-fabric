package agent_test

import (
	"context"
	"math"
	"os"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type pricedSpecs struct{ cost modelspecs.Cost }

func (p pricedSpecs) ProviderFor(string, string) (modelspecs.Provider, bool) {
	return modelspecs.Provider{
		ID:     "local",
		Models: map[string]modelspecs.Model{"m1": {ID: "m1", Cost: &p.cost}},
	}, true
}

func rate(v float64) *float64 { return &v }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// runTwoRoundTurn runs a prompt whose model calls read_file in round 1 and
// answers in round 2, with the given usage per round.
func runTwoRoundTurn(t *testing.T, specs catalog.SpecsLookup, rounds [2]provider.Usage) (*captureClient, *catalog.Store, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	cat.Specs = specs
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ws := sandbox.ProjectFilesRoot(root, th.ProjectID)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ws+"/a.txt", []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := 0
	fs := &fakeStreamer{streamFn: func(_ context.Context, _ string, _ []runtime.Message, onEvent func(provider.StreamEvent) error) error {
		n++
		u := rounds[n-1]
		if n == 1 {
			return onEvent(provider.StreamEvent{
				Finish:    "tool_calls",
				ToolCalls: []provider.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"a.txt"}`}},
				Usage:     &u,
			})
		}
		return onEvent(provider.StreamEvent{Content: "done", Finish: "stop", Usage: &u})
	}}
	_, csc, client, ctx2, _ := startACPCatalogWithSandbox(t, rt, cat, fs, engineconfig.Engine{DataDir: root})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"assistantId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("read")},
	}); err != nil {
		t.Fatal(err)
	}
	return client, cat, th.ID
}

func ip(n int) *int { return &n }

func TestCostReportedPerRoundAndPersisted(t *testing.T) {
	specs := pricedSpecs{modelspecs.Cost{Input: rate(3), Output: rate(15), CacheRead: rate(0.3)}}
	client, cat, threadID := runTwoRoundTurn(t, specs, [2]provider.Usage{
		{PromptTokens: ip(1000), CompletionTokens: ip(100), TotalTokens: ip(1100)},
		{PromptTokens: ip(2000), CompletionTokens: ip(200), TotalTokens: ip(2200), CachedTokens: ip(1000)},
	})
	const round1 = 1000*3/1e6 + 100*15/1e6
	const round2 = 1000*3/1e6 + 1000*0.3/1e6 + 200*15/1e6

	client.mu.Lock()
	usages := append([]acp.SessionUsageUpdate(nil), client.usages...)
	client.mu.Unlock()
	if len(usages) != 3 {
		t.Fatalf("want an update per round and a final update, got %d", len(usages))
	}

	partial := usages[0].Meta
	if partial["partial"] != true || partial["round"] != float64(0) {
		t.Fatalf("round update meta = %v", partial)
	}
	rc := partial["roundCost"].(map[string]any)
	if !near(rc["total"].(float64), round1) {
		t.Fatalf("round cost = %v, want %v", rc["total"], round1)
	}
	if !near(partial["cost"].(map[string]any)["total"].(float64), round1) {
		t.Fatalf("running cost after round 1 = %v", partial["cost"])
	}

	last := usages[1].Meta
	if last["partial"] != true || last["round"] != float64(1) {
		t.Fatalf("final round update meta = %v", last)
	}
	lrc := last["roundCost"].(map[string]any)
	if !near(lrc["total"].(float64), round2) {
		t.Fatalf("final round cost = %v, want %v", lrc["total"], round2)
	}

	final := usages[2].Meta
	if _, isPartial := final["partial"]; isPartial {
		t.Fatalf("final update must not be partial: %v", final)
	}
	fc := final["cost"].(map[string]any)
	if !near(fc["total"].(float64), round1+round2) || fc["estimated"] != true || fc["currency"] != "USD" {
		t.Fatalf("final cost = %v, want %v", fc, round1+round2)
	}
	if final["cachedTokens"] != float64(1000) {
		t.Fatalf("cachedTokens = %v", final["cachedTokens"])
	}
	if rounds, _ := final["rounds"].([]any); len(rounds) != 2 {
		t.Fatalf("rounds = %v", final["rounds"])
	}

	detail, err := cat.GetThread(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	var usage *catalog.MessagePart
	for i := range detail.Messages[1].Parts {
		if detail.Messages[1].Parts[i].Type == "usage" {
			usage = &detail.Messages[1].Parts[i]
		}
	}
	if usage == nil || usage.Cost == nil || !near(usage.Cost.Total, round1+round2) || len(usage.Rounds) != 2 {
		t.Fatalf("persisted usage = %+v", usage)
	}
	if usage.Rounds[1].Cost == nil || !near(usage.Rounds[1].Cost.Total, round2) {
		t.Fatalf("persisted round 2 = %+v", usage.Rounds[1])
	}
}

func TestNoSpecsMeansNoCostKeys(t *testing.T) {
	client, _, _ := runTwoRoundTurn(t, nil, [2]provider.Usage{
		{PromptTokens: ip(10), CompletionTokens: ip(1), TotalTokens: ip(11)},
		{PromptTokens: ip(20), CompletionTokens: ip(2), TotalTokens: ip(22)},
	})
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, u := range client.usages {
		for _, k := range []string{"cost", "roundCost", "reportedCostUsd"} {
			if _, ok := u.Meta[k]; ok {
				t.Fatalf("unexpected %q without prices: %v", k, u.Meta)
			}
		}
	}
}

func TestProviderReportedCostIsSummed(t *testing.T) {
	a, b := 0.01, 0.02
	client, _, _ := runTwoRoundTurn(t, nil, [2]provider.Usage{
		{PromptTokens: ip(10), TotalTokens: ip(10), ReportedCostUSD: &a},
		{PromptTokens: ip(20), TotalTokens: ip(20), ReportedCostUSD: &b},
	})
	client.mu.Lock()
	defer client.mu.Unlock()
	last := client.usages[len(client.usages)-1].Meta
	if v, _ := last["reportedCostUsd"].(float64); !near(v, 0.03) {
		t.Fatalf("reportedCostUsd = %v", last["reportedCostUsd"])
	}
}
