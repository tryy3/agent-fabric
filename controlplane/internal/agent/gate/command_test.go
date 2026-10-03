package gate

import (
	"context"
	"encoding/json"
	"testing"
)

func evalCommand(t *testing.T, argv []string, cwd string, posix bool, grants ...string) Decision {
	t.Helper()
	args := map[string]any{"command": argv}
	if cwd != "" {
		args["cwd"] = cwd
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Rules{}.Evaluate(context.Background(), Request{
		ToolName:      "run_command",
		Args:          raw,
		ProjectRoot:   "/workspace",
		POSIX:         true,
		EnvKind:       kindFor(posix),
		CommandGrants: grants,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCommandTiers(t *testing.T) {
	cases := []struct {
		name      string
		argv      []string
		cwd       string
		kind      DecisionKind
		noSession bool
		key       string
	}{
		{"ls", []string{"ls"}, "", Allow, false, ""},
		{"ls with flags in subdir", []string{"ls", "-la", "src"}, "src", Allow, false, ""},
		{"cat relative", []string{"cat", "a/b.txt"}, "", Allow, false, ""},
		{"absolute system path", []string{"/bin/ls"}, "", Allow, false, ""},
		{"git status", []string{"git", "status"}, "", Allow, false, ""},
		{"git branch list", []string{"git", "branch", "-a"}, "", Allow, false, ""},
		{"version flag", []string{"node", "--version"}, "", Allow, false, ""},
		{"go version", []string{"go", "version"}, "", Allow, false, ""},
		{"find plain", []string{"find", ".", "-name", "*.go"}, "", Allow, false, ""},

		{"cat outside root", []string{"cat", "/var/log/x"}, "", Ask, true, ""},
		{"ls parent", []string{"ls", ".."}, "", Ask, true, ""},
		{"find delete", []string{"find", ".", "-delete"}, "", Ask, true, ""},
		{"find exec", []string{"find", ".", "-exec", "rm", "{}", ";"}, "", Ask, true, ""},
		{"rm", []string{"rm", "-rf", "build"}, "", Ask, true, ""},
		{"mv", []string{"mv", "a", "b"}, "", Ask, true, ""},
		{"git reset hard", []string{"git", "reset", "--hard"}, "", Ask, true, ""},
		{"git push", []string{"git", "push"}, "", Ask, true, ""},
		{"git branch delete", []string{"git", "branch", "-D", "x"}, "", Ask, true, ""},
		{"git -c override", []string{"git", "-c", "core.pager=sh", "log"}, "", Ask, true, ""},
		{"sh -c", []string{"sh", "-c", "echo hi"}, "", Ask, true, ""},
		{"bash -c", []string{"/usr/bin/bash", "-c", "ls | wc"}, "", Ask, true, ""},
		{"python -c", []string{"python3", "-c", "print(1)"}, "", Ask, true, ""},
		{"env wrapper", []string{"env", "FOO=1", "npm", "test"}, "", Ask, true, ""},
		{"xargs", []string{"xargs", "rm"}, "", Ask, true, ""},
		{"rg --pre", []string{"rg", "--pre", "sh", "x"}, "", Ask, false, "rg"},
		{"sort -o", []string{"sort", "-o", "f", "f"}, "", Ask, false, "sort"},

		{"npm test", []string{"npm", "test"}, "", Ask, false, "npm test"},
		{"npm run build", []string{"npm", "run", "build"}, "", Ask, false, "npm run build"},
		{"npm install", []string{"npm", "install", "left-pad"}, "", Ask, false, "npm install"},
		{"go test", []string{"go", "test", "./..."}, "", Ask, false, "go test"},
		{"make target", []string{"make", "-j4"}, "", Ask, false, "make"},
		{"python script", []string{"python3", "-u", "run.py"}, "", Ask, false, "python3 run.py"},
		{"project script", []string{"./scripts/build.sh"}, "", Ask, false, "./scripts/build.sh"},
		{"curl", []string{"curl", "https://example.com"}, "", Ask, false, "curl"},
		{"git commit", []string{"git", "commit", "-m", "x"}, "", Ask, false, "git commit"},

		{"sudo", []string{"sudo", "ls"}, "", Deny, false, ""},
		{"docker", []string{"docker", "ps"}, "", Deny, false, ""},
		{"rm sensitive", []string{"rm", "-rf", "/etc/hosts"}, "", Deny, false, ""},
		{"rm .git", []string{"rm", "-rf", ".git"}, "", Deny, false, ""},
		{"cwd escape", []string{"ls"}, "../other", Deny, false, ""},
		{"cwd absolute outside", []string{"ls"}, "/etc", Deny, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := evalCommand(t, tc.argv, tc.cwd, true)
			if d.Kind != tc.kind {
				t.Fatalf("kind = %s (%s: %s), want %s", d.Kind, d.RuleID, d.Reason, tc.kind)
			}
			if d.Kind == Ask && d.NoSessionGrant != tc.noSession {
				t.Fatalf("NoSessionGrant = %v, want %v (%s)", d.NoSessionGrant, tc.noSession, d.RuleID)
			}
			if tc.key != "" && d.GrantKey != tc.key {
				t.Fatalf("GrantKey = %q, want %q", d.GrantKey, tc.key)
			}
		})
	}
}

func TestCommandLocalEnvironmentDenied(t *testing.T) {
	d := evalCommand(t, []string{"ls"}, "", false)
	if d.Kind != Deny || d.RuleID != RuleCommandLocalDeny {
		t.Fatalf("got %#v", d)
	}
}

func TestCommandSessionGrantAllows(t *testing.T) {
	if d := evalCommand(t, []string{"npm", "test"}, "", true, "npm test"); d.Kind != Allow || d.RuleID != RuleCommandGranted {
		t.Fatalf("granted npm test: %#v", d)
	}
	if d := evalCommand(t, []string{"npm", "install"}, "", true, "npm test"); d.Kind != Ask {
		t.Fatalf("grant must not widen to npm install: %#v", d)
	}
	// A grant never overrides the destructive tier.
	if d := evalCommand(t, []string{"rm", "x"}, "", true, "rm"); d.Kind != Ask || !d.NoSessionGrant {
		t.Fatalf("rm with grant: %#v", d)
	}
}

func TestCommandBadArgsDeny(t *testing.T) {
	for _, raw := range []string{`{}`, `{"command":[]}`, `{"command":[" "]}`, `{"command":"ls -la"}`} {
		d, err := Rules{}.Evaluate(context.Background(), Request{
			ToolName: "run_command", Args: json.RawMessage(raw), ProjectRoot: "/workspace", POSIX: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if d.Kind != Deny || d.RuleID != "rules.bad_args" {
			t.Fatalf("%s: got %#v", raw, d)
		}
	}
}

func kindFor(docker bool) string {
	if docker {
		return "docker"
	}
	return "local"
}
