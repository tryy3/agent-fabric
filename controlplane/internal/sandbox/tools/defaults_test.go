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
		if name == "ask_user" {
			if e.Origin != sandboxtools.OriginControlPlane {
				t.Fatalf("%s: origin = %q, want %q", name, e.Origin, sandboxtools.OriginControlPlane)
			}
			if e.Requires.FS || e.Requires.Exec {
				t.Fatalf("ask_user: expected no requires, got %+v", e.Requires)
			}
			continue
		}
		if e.Origin != sandboxtools.OriginEnvironment {
			t.Fatalf("%s: origin = %q, want %q", name, e.Origin, sandboxtools.OriginEnvironment)
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

func TestCatalogEntriesIncludesWebTools(t *testing.T) {
	entries, err := sandboxtools.CatalogEntries()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]sandboxtools.CatalogEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	cases := map[string]string{
		"web_search": "query",
		"fetch_page": "url",
	}
	for name, prop := range cases {
		e, ok := byName[name]
		if !ok {
			t.Fatalf("missing tool %q in %+v", name, entries)
		}
		if e.Description == "" {
			t.Fatalf("%s: empty description", name)
		}
		if e.Origin != sandboxtools.OriginMCP {
			t.Fatalf("%s: origin = %q, want %q", name, e.Origin, sandboxtools.OriginMCP)
		}
		if e.Requires.FS || e.Requires.Exec {
			t.Fatalf("%s: expected no requires, got %+v", name, e.Requires)
		}
		var params map[string]any
		if err := json.Unmarshal(e.Parameters, &params); err != nil {
			t.Fatalf("%s: parameters: %v", name, err)
		}
		props, ok := params["properties"].(map[string]any)
		if !ok || props[prop] == nil {
			t.Fatalf("%s: expected %s property in %+v", name, prop, params)
		}
	}
}
