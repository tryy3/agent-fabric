package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
)

func call(t *testing.T, ctx context.Context, args string) (map[string]any, error) {
	t.Helper()
	env, err := local.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := sandbox.NewRegistry()
	for _, tool := range Tools() {
		reg.Register(tool)
	}
	out, err := reg.Call(ctx, env, Name, json.RawMessage(args))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("result %q: %v", out, err)
	}
	return m, nil
}

func TestRunCommandOutputAndExit(t *testing.T) {
	m, err := call(t, context.Background(), `{"command":["sh","-c","echo out; echo err >&2; exit 4"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if m["exit_code"] != float64(4) || m["stdout"] != "out\n" || m["stderr"] != "err\n" || m["timed_out"] != false {
		t.Fatalf("result = %#v", m)
	}
	if _, ok := m["error"]; ok {
		t.Fatalf("non-zero exit must not be an error envelope: %#v", m)
	}
}

func TestRunCommandStdin(t *testing.T) {
	m, err := call(t, context.Background(), `{"command":["cat"],"stdin":"hello"}`)
	if err != nil || m["stdout"] != "hello" {
		t.Fatalf("result = %#v, %v", m, err)
	}
}

func TestRunCommandTruncatesOutput(t *testing.T) {
	n := MaxOutputBytes * 3
	m, err := call(t, context.Background(), `{"command":["head","-c","`+itoa(n)+`","/dev/zero"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(m["stdout"].(string)); got != MaxOutputBytes {
		t.Fatalf("stdout len = %d, want %d", got, MaxOutputBytes)
	}
	if m["stdout_truncated"] != true || m["stderr_truncated"] != false {
		t.Fatalf("truncation flags = %#v", m)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	start := time.Now()
	m, err := call(t, context.Background(), `{"command":["sleep","30"],"timeout_seconds":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if m["timed_out"] != true {
		t.Fatalf("result = %#v", m)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("timeout did not stop the command")
	}
}

func TestRunCommandCancelStopsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	m, _ := call(t, ctx, `{"command":["sleep","30"]}`)
	if time.Since(start) > 10*time.Second {
		t.Fatal("cancel did not stop the command")
	}
	if m["error"] == nil {
		t.Fatalf("cancelled call should not look like success: %#v", m)
	}
}

func TestRunCommandRejectsBadInput(t *testing.T) {
	for name, args := range map[string]string{
		"missing command": `{}`,
		"string command":  `{"command":"ls -la"}`,
		"empty program":   `{"command":[" "]}`,
		"negative timout": `{"command":["ls"],"timeout_seconds":-1}`,
		"cwd escape":      `{"command":["ls"],"cwd":"../.."}`,
	} {
		t.Run(name, func(t *testing.T) {
			m, err := call(t, context.Background(), args)
			if err != nil {
				t.Fatal(err)
			}
			if m["error"] == nil {
				t.Fatalf("want error envelope, got %#v", m)
			}
		})
	}
}

func TestTimeoutClamped(t *testing.T) {
	if got := (Args{}).Timeout(); got != DefaultTimeout {
		t.Fatalf("default = %v", got)
	}
	if got := (Args{TimeoutSeconds: 999999}).Timeout(); got != MaxTimeout {
		t.Fatalf("max = %v", got)
	}
}

func TestSchemaDeclaresArrayItems(t *testing.T) {
	b, err := json.Marshal(Tools()[0].Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"command":{"type":"array"`) || !strings.Contains(string(b), `"items":{"type":"string"}`) {
		t.Fatalf("schema = %s", b)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
