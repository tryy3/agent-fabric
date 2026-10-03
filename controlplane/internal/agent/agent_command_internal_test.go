package agent

import (
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/agent/gate"
)

func TestPermissionOptionsForCommands(t *testing.T) {
	ids := func(opts []acp.PermissionOption) string {
		var out []string
		for _, o := range opts {
			out = append(out, string(o.OptionId))
		}
		return strings.Join(out, ",")
	}
	if got := ids(permissionOptions("run_command", gate.Decision{})); got != "allow_once,allow_session,reject_once" {
		t.Fatalf("grantable = %s", got)
	}
	if got := ids(permissionOptions("run_command", gate.Decision{NoSessionGrant: true})); got != "allow_once,reject_once" {
		t.Fatalf("hard ask = %s", got)
	}
}

func TestPermissionRawInputIncludesCommand(t *testing.T) {
	raw := permissionRawInput(gate.Decision{
		Reason: "run npm test", RuleID: gate.RuleCommandAsk,
		Command: []string{"npm", "test"}, Cwd: "web", GrantKey: "npm test",
	})
	if cmd, _ := raw["command"].([]string); strings.Join(cmd, " ") != "npm test" || raw["cwd"] != "web" || raw["grantKey"] != "npm test" {
		t.Fatalf("raw = %#v", raw)
	}
	if _, ok := permissionRawInput(gate.Decision{Path: "a"})["command"]; ok {
		t.Fatal("non-command decisions must not carry command")
	}
}

func TestSessionCommandGrants(t *testing.T) {
	a := &Agent{}
	a.addSessionCommandGrant("s1", "npm test")
	a.addSessionCommandGrant("s1", "npm test")
	a.addSessionCommandGrant("s1", "")
	if got := a.sessionCommandGrants("s1"); len(got) != 1 || got[0] != "npm test" {
		t.Fatalf("grants = %v", got)
	}
	if got := a.sessionCommandGrants("s2"); len(got) != 0 {
		t.Fatalf("other session leaked grants: %v", got)
	}
}
