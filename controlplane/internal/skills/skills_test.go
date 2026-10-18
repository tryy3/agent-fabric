package skills

import (
	"context"
	"reflect"
	"testing"
	"testing/fstest"
)

func skillFS(files map[string]string) FSReader {
	m := fstest.MapFS{}
	for p, c := range files {
		m[p] = &fstest.MapFile{Data: []byte(c)}
	}
	return FSReader{FS: m}
}

const goodSkill = `---
name: deep-research
description: "Research a question and write a cited report"
argumentHint: <question>
flags:
  - depth
  - rounds
maxToolRounds: 80
requires: sandbox
unknownKey: ignored
---
# Body

Do the thing.
`

func TestParseSkillMD(t *testing.T) {
	s, err := parseSkillMD([]byte(goodSkill), "deep-research")
	if err != nil {
		t.Fatal(err)
	}
	if s.Description != "Research a question and write a cited report" || s.ArgumentHint != "<question>" {
		t.Fatalf("bad scalars: %+v", s)
	}
	if !reflect.DeepEqual(s.Flags, []string{"depth", "rounds"}) || s.MaxToolRounds != 80 || s.Requires != RequiresSandbox {
		t.Fatalf("bad fields: %+v", s)
	}
	if s.Body != "# Body\n\nDo the thing." {
		t.Fatalf("body = %q", s.Body)
	}
}

func TestParseSkillMDInlineFlagsAndDefaults(t *testing.T) {
	s, err := parseSkillMD([]byte("---\r\ndescription: x\r\nflags: [a, 'b']\r\n---\r\nhi"), "plain")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "plain" || s.Requires != RequiresNone || !reflect.DeepEqual(s.Flags, []string{"a", "b"}) || s.Body != "hi" {
		t.Fatalf("got %+v", s)
	}
}

func TestParseSkillMDErrors(t *testing.T) {
	cases := map[string]string{
		"no frontmatter":   "hello",
		"unclosed":         "---\ndescription: x\n",
		"missing desc":     "---\nname: a\n---\nbody",
		"name mismatch":    "---\nname: other\ndescription: x\n---\n",
		"bad name":         "---\nname: Bad_Name\ndescription: x\n---\n",
		"bad rounds":       "---\ndescription: x\nmaxToolRounds: lots\n---\n",
		"bad requires":     "---\ndescription: x\nrequires: gpu\n---\n",
		"orphan list item": "---\n- a\ndescription: x\n---\n",
	}
	for name, raw := range cases {
		dir := "a"
		if name == "bad name" {
			dir = "Bad_Name"
		}
		if _, err := parseSkillMD([]byte(raw), dir); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoadDir(t *testing.T) {
	r := skillFS(map[string]string{
		"skills/deep-research/SKILL.md":             goodSkill,
		"skills/deep-research/references/format.md": "x",
		"skills/deep-research/scripts/render.sh":    "y",
		"skills/deep-research/.hidden":              "z",
		"skills/broken/SKILL.md":                    "no frontmatter",
		"skills/not-a-skill/readme.md":              "x",
		"skills/.git/SKILL.md":                      goodSkill,
		"skills/loose-file.md":                      "x",
	})
	got, problems, err := LoadDir(context.Background(), r, "skills", SourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "deep-research" || got[0].Source != SourceProject {
		t.Fatalf("skills = %+v", got)
	}
	if !reflect.DeepEqual(got[0].Files, []string{"references/format.md", "scripts/render.sh"}) {
		t.Fatalf("files = %v", got[0].Files)
	}
	if got[0].Hash == "" || got[0].Path != "skills/deep-research" {
		t.Fatalf("hash/path = %+v", got[0])
	}
	if len(problems) != 1 || problems[0].Path != "skills/broken" {
		t.Fatalf("problems = %+v", problems)
	}
}

func TestLoadDirMissingRootIsEmpty(t *testing.T) {
	got, problems, err := LoadDir(context.Background(), skillFS(nil), "nope", SourceProject)
	if err != nil || len(got) != 0 || len(problems) != 0 {
		t.Fatalf("got %v %v %v", got, problems, err)
	}
}

func TestHashChangesWithContentAndFiles(t *testing.T) {
	a := hashSkill([]byte("a"), []string{"x"})
	if a == hashSkill([]byte("b"), []string{"x"}) || a == hashSkill([]byte("a"), []string{"x", "y"}) {
		t.Fatal("hash must change with content and files")
	}
	if a != hashSkill([]byte("a"), []string{"x"}) {
		t.Fatal("hash must be stable")
	}
}

func TestResolveLayeringAndToggles(t *testing.T) {
	builtin := Layer{Source: SourceBuiltin, Skills: []Skill{
		{Name: "alpha", Description: "built-in alpha"},
		{Name: "beta", Description: "built-in beta"},
	}}
	project := Layer{Source: SourceProject, Skills: []Skill{
		{Name: "beta", Description: "project beta"},
		{Name: "gamma", Description: "project gamma"},
	}}
	got := Resolve([]Layer{builtin, project}, Toggles{Disabled: []string{"gamma"}}, Toggles{Disabled: []string{"alpha"}})
	if len(got) != 3 {
		t.Fatalf("entries = %+v", got)
	}
	if got[0].Name != "alpha" || got[0].Enabled || got[0].Source != SourceBuiltin {
		t.Fatalf("alpha = %+v", got[0])
	}
	if got[1].Name != "beta" || !got[1].Enabled || got[1].Source != SourceProject || got[1].Description != "project beta" ||
		!reflect.DeepEqual(got[1].Overrides, []Source{SourceBuiltin}) {
		t.Fatalf("beta = %+v", got[1])
	}
	if got[2].Name != "gamma" || got[2].Enabled {
		t.Fatalf("gamma = %+v", got[2])
	}
	pins := PinEnabled(got)
	if len(pins) != 1 || pins[0].Name != "beta" || pins[0].Source != SourceProject {
		t.Fatalf("pins = %+v", pins)
	}
}

func TestPinEnabledEmpty(t *testing.T) {
	if pins := PinEnabled(nil); len(pins) != 0 {
		t.Fatalf("pins = %v", pins)
	}
}
