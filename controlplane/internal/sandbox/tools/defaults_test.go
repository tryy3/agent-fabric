package tools_test

import (
	"encoding/json"
	"testing"

	sandboxtools "github.com/tryy3/agent-fabric/internal/sandbox/tools"
)

func TestCatalogEntriesIncludesFileTools(t *testing.T) {
	entries, err := sandboxtools.CatalogEntries()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]sandboxtools.CatalogEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	for _, name := range []string{"ask_user", "read_file", "write_file"} {
		e, ok := byName[name]
		if !ok {
			t.Fatalf("missing tool %q in %+v", name, entries)
		}
		if e.Description == "" {
			t.Fatalf("%s: empty description", name)
		}
		if e.Origin != sandboxtools.OriginSandbox {
			t.Fatalf("%s: origin = %q", name, e.Origin)
		}
		if name == "ask_user" {
			if e.Requires.FS || e.Requires.Exec {
				t.Fatalf("ask_user: expected no requires, got %+v", e.Requires)
			}
			continue
		}
		if !e.Requires.FS {
			t.Fatalf("%s: expected requires.fs", name)
		}
		var params map[string]any
		if err := json.Unmarshal(e.Parameters, &params); err != nil {
			t.Fatalf("%s: parameters: %v", name, err)
		}
		if params["type"] != "object" {
			t.Fatalf("%s: type = %v", name, params["type"])
		}
		props, ok := params["properties"].(map[string]any)
		if !ok || props["path"] == nil {
			t.Fatalf("%s: expected path property in %+v", name, params)
		}
	}
}
