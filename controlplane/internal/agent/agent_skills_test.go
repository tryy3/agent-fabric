package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/engineconfig"
	"github.com/tryy3/agent-fabric/internal/provider"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

func TestNewSessionPinsSkillsFromProjectFiles(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	root := t.TempDir()
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(sandbox.ProjectFilesRoot(root, th.ProjectID), ".agents", "skills", "hello")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: hello\ndescription: Says hello\n---\nSay hello.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	_, csc, _, ctx2, _ := startACPCatalogWithSandbox(t, rt, cat, &fakeStreamer{deltas: []string{"ok"}}, engineconfig.Engine{DataDir: root})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	newSession := func() runtime.Session {
		t.Helper()
		resp, err := csc.NewSession(ctx2, acp.NewSessionRequest{
			Cwd:        "/",
			McpServers: []acp.McpServer{},
			Meta:       map[string]any{"assistantId": ag.ID, "threadId": th.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		sess, ok := rt.Get(string(resp.SessionId))
		if !ok {
			t.Fatal("session not stored")
		}
		return sess
	}

	first := newSession()
	if len(first.Pin.Skills) != 1 || first.Pin.Skills[0].Name != "hello" ||
		first.Pin.Skills[0].Source != "project" || first.Pin.Skills[0].Hash == "" {
		t.Fatalf("pinned skills = %+v", first.Pin.Skills)
	}

	if _, err := cat.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Skills: json.RawMessage(`{"disabled":["hello"]}`)}); err != nil {
		t.Fatal(err)
	}
	if len(first.Pin.Skills) != 1 {
		t.Fatal("a live session must keep its pin")
	}
	if second := newSession(); len(second.Pin.Skills) != 0 {
		t.Fatalf("disabled skill pinned in a new session: %+v", second.Pin.Skills)
	}
}

type skillTurn struct {
	cat      *catalog.Store
	rt       *runtime.Store
	streamer *fakeStreamer
	client   *captureClient
	csc      *acp.ClientSideConnection
	ctx      context.Context
	session  acp.SessionId
	threadID string
}

const helloSkill = "---\nname: hello\ndescription: Says hello\nargumentHint: <name>\nflags: [depth]\n---\nSay hello.\n"

// newSkillTurn starts a session on a thread whose project has the hello skill
// (with a references/notes.md file) and the given scripted streamer function.
func newSkillTurn(t *testing.T, stream func(context.Context, string, []runtime.Message, func(provider.StreamEvent) error) error) skillTurn {
	t.Helper()
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	root := t.TempDir()
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(sandbox.ProjectFilesRoot(root, th.ProjectID), ".agents", "skills", "hello")
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(helloSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "notes.md"), []byte("extra notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStreamer{streamFn: stream}
	_, csc, client, ctx2, cancel := startACPCatalogWithSandbox(t, rt, cat, fs, engineconfig.Engine{DataDir: root})
	t.Cleanup(cancel)
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
	return skillTurn{cat: cat, rt: rt, streamer: fs, client: client, csc: csc, ctx: ctx2, session: sess.SessionId, threadID: th.ID}
}

func (s skillTurn) prompt(t *testing.T, text string) {
	t.Helper()
	if _, err := s.csc.Prompt(s.ctx, acp.PromptRequest{
		SessionId: s.session,
		Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
	}); err != nil {
		t.Fatal(err)
	}
}

func stopWith(text string) func(context.Context, string, []runtime.Message, func(provider.StreamEvent) error) error {
	return func(_ context.Context, _ string, _ []runtime.Message, onEvent func(provider.StreamEvent) error) error {
		return onEvent(provider.StreamEvent{Content: text, Finish: "stop"})
	}
}

func TestSessionAdvertisesSkillsAsCommandsAndInstructions(t *testing.T) {
	turn := newSkillTurn(t, stopWith("ok"))
	pinned, _ := turn.rt.Get(string(turn.session))
	if !strings.Contains(pinned.Pin.EffectiveInstructions, "<available_skills>") ||
		!strings.Contains(pinned.Pin.EffectiveInstructions, "- hello: Says hello") {
		t.Fatalf("instructions = %q", pinned.Pin.EffectiveInstructions)
	}
	turn.client.mu.Lock()
	defer turn.client.mu.Unlock()
	if len(turn.client.commands) != 1 || turn.client.commands[0].Name != "hello" ||
		turn.client.commands[0].Input == nil || turn.client.commands[0].Input.Unstructured.Hint != "<name>" {
		t.Fatalf("commands = %+v", turn.client.commands)
	}
}

func TestSlashCommandInjectsSkillAndRecordsInvocation(t *testing.T) {
	turn := newSkillTurn(t, stopWith("hello there"))
	turn.prompt(t, "/hello --depth deep Dennis")

	msgs := turn.streamer.snapshotMessages()
	last := msgs[len(msgs)-1].Content
	for _, want := range []string{"Say hello.", "depth: deep", "Dennis", `name="hello"`, `source="project"`} {
		if !strings.Contains(last, want) {
			t.Errorf("model input missing %q:\n%s", want, last)
		}
	}
	if strings.Contains(last, "--depth") {
		t.Errorf("flags must be parsed out of the question: %s", last)
	}

	stored, err := turn.cat.ActiveMessages(context.Background(), turn.threadID)
	if err != nil {
		t.Fatal(err)
	}
	var sawUser, sawPart bool
	for _, m := range stored {
		if m.Role == "user" && m.Content == "/hello --depth deep Dennis" {
			sawUser = true
		}
		for _, p := range m.Parts {
			if p.Type == "skill" && p.Skill != nil && p.Skill.Name == "hello" &&
				p.Skill.Source == "project" && p.Skill.Hash != "" && p.Skill.Flags["depth"] == "deep" {
				sawPart = true
			}
		}
	}
	if !sawUser || !sawPart {
		t.Fatalf("user message kept = %v, skill part recorded = %v, messages = %+v", sawUser, sawPart, stored)
	}
}

func TestSlashCommandBadFlagAnswersWithoutModelCall(t *testing.T) {
	turn := newSkillTurn(t, stopWith("should not run"))
	turn.prompt(t, "/hello --bogus x")
	if len(turn.streamer.options) != 0 {
		t.Fatalf("the model was called %d times", len(turn.streamer.options))
	}
	turn.client.mu.Lock()
	defer turn.client.mu.Unlock()
	if len(turn.client.chunks) != 1 || !strings.Contains(turn.client.chunks[0], "no flag --bogus") {
		t.Fatalf("chunks = %v", turn.client.chunks)
	}
}

func TestUnknownSlashCommandIsPlainText(t *testing.T) {
	turn := newSkillTurn(t, stopWith("ok"))
	turn.prompt(t, "/nothing here")
	msgs := turn.streamer.snapshotMessages()
	if got := msgs[len(msgs)-1].Content; got != "/nothing here" {
		t.Fatalf("model input = %q", got)
	}
}

func TestLoadSkillToolReturnsSkillAndExtraFile(t *testing.T) {
	round := 0
	var turn skillTurn
	turn = newSkillTurn(t, func(_ context.Context, _ string, msgs []runtime.Message, onEvent func(provider.StreamEvent) error) error {
		round++
		if round == 1 {
			return onEvent(provider.StreamEvent{Finish: "tool_calls", ToolCalls: []provider.ToolCall{
				{ID: "c1", Name: "load_skill", Arguments: `{"name":"hello"}`},
				{ID: "c2", Name: "load_skill", Arguments: `{"name":"hello","path":"references/notes.md"}`},
				{ID: "c3", Name: "load_skill", Arguments: `{"name":"hello","path":"../../etc/passwd"}`},
				{ID: "c4", Name: "load_skill", Arguments: `{"name":"missing"}`},
			}})
		}
		var results []string
		for _, m := range msgs {
			if m.Role == "tool" {
				results = append(results, m.Content)
			}
		}
		turn.client.mu.Lock()
		turn.client.toolMessagesSeenByModel = nil
		for _, m := range msgs {
			if m.Role == "tool" {
				turn.client.toolMessagesSeenByModel = append(turn.client.toolMessagesSeenByModel, m)
			}
		}
		turn.client.mu.Unlock()
		return onEvent(provider.StreamEvent{Content: strings.Join(results, "|"), Finish: "stop"})
	})
	turn.prompt(t, "use the skill")

	turn.client.mu.Lock()
	defer turn.client.mu.Unlock()
	seen := turn.client.toolMessagesSeenByModel
	if len(seen) != 4 {
		t.Fatalf("tool results = %+v", seen)
	}
	if !strings.Contains(seen[0].Content, "Say hello.") {
		t.Errorf("SKILL.md result = %q", seen[0].Content)
	}
	if !strings.Contains(seen[1].Content, "extra notes") {
		t.Errorf("extra file result = %q", seen[1].Content)
	}
	if !strings.Contains(seen[2].Content, "has no file") || !strings.Contains(seen[3].Content, "not available") {
		t.Errorf("errors = %q, %q", seen[2].Content, seen[3].Content)
	}
}
