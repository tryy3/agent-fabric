package skills

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		in, name, rest string
		ok             bool
	}{
		{"/deep-research what is X", "deep-research", "what is X", true},
		{"  /a", "a", "", true},
		{"/a\n  multi line", "a", "multi line", true},
		{"hello /a", "", "", false},
		{"/", "", "", false},
		{"/Bad_Name x", "", "", false},
		{"/path/to/file", "", "", false},
	}
	for _, c := range cases {
		name, rest, ok := SplitCommand(c.in)
		if name != c.name || rest != c.rest || ok != c.ok {
			t.Errorf("SplitCommand(%q) = %q, %q, %v; want %q, %q, %v", c.in, name, rest, ok, c.name, c.rest, c.ok)
		}
	}
}

func TestParseInvocation(t *testing.T) {
	declared := []string{"depth", "rounds", "time", "save", "custom"}
	inv, err := ParseInvocation("r", "--depth deep --rounds=6 --time 10m --save project --custom anything  the real question", declared)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"depth": "deep", "rounds": "6", "time": "10m", "save": "project", "custom": "anything"}
	if !reflect.DeepEqual(inv.Flags, want) || inv.Question != "the real question" {
		t.Fatalf("inv = %+v", inv)
	}

	plain, err := ParseInvocation("r", "just a question with --dashes inside", nil)
	if err != nil || plain.Flags != nil || plain.Question != "just a question with --dashes inside" {
		t.Fatalf("plain = %+v, %v", plain, err)
	}
}

func TestParseInvocationErrors(t *testing.T) {
	declared := []string{"depth", "rounds", "time", "save"}
	cases := map[string]string{
		"--bogus x q":     "no flag --bogus",
		"--depth":         "needs a value",
		"--depth=":        "needs a value",
		"--depth huge q":  "must be one of quick, normal, deep",
		"--rounds zero q": "whole number",
		"--rounds 0 q":    "whole number",
		"--time soon q":   "duration",
		"--save cloud q":  "must be one of project, scratch",
	}
	for args, want := range cases {
		_, err := ParseInvocation("r", args, declared)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want containing %q", args, err, want)
		}
	}
	if _, err := ParseInvocation("r", "--depth deep", nil); err == nil || !strings.Contains(err.Error(), "takes no flags") {
		t.Errorf("no declared flags: err = %v", err)
	}
}

func TestRenderInvocation(t *testing.T) {
	got := RenderInvocation(Invocation{Name: "r", Flags: map[string]string{"b": "2", "a": "1"}, Question: "why?"}, SourceBuiltin, "abc", "BODY", map[string]string{"scratch_dir": ".agent-fabric/scratch/th_1"})
	for _, want := range []string{`<skill_invocation name="r" source="builtin" hash="abc">`, "BODY", "a: 1\nb: 2\n", "scratch_dir: .agent-fabric/scratch/th_1", "</skill_invocation>\n\nwhy?"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if empty := RenderInvocation(Invocation{Name: "r"}, SourceBuiltin, "h", "B", nil); !strings.Contains(empty, "no question") || strings.Contains(empty, "skill_flags") {
		t.Errorf("empty = %s", empty)
	}
}
