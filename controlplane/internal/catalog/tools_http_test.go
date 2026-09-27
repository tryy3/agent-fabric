package catalog_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestToolsHTTPList(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	type toolEntry struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
		Requires    struct {
			FS   bool `json:"fs"`
			Exec bool `json:"exec"`
		} `json:"requires"`
		Origin string `json:"origin"`
	}
	var payload struct {
		Tools []toolEntry `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]toolEntry, len(payload.Tools))
	for _, tool := range payload.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"ask_user", "read_file", "write_file"} {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("missing %q in %+v", name, payload.Tools)
		}
		if tool.Description == "" {
			t.Fatalf("%s: empty description", name)
		}
		if tool.Origin != "sandbox" {
			t.Fatalf("%s: origin = %q", name, tool.Origin)
		}
		if name == "ask_user" {
			if tool.Requires.FS || tool.Requires.Exec {
				t.Fatalf("ask_user: expected no requires")
			}
			continue
		}
		if !tool.Requires.FS {
			t.Fatalf("%s: expected requires.fs", name)
		}
		var params map[string]any
		if err := json.Unmarshal(tool.Parameters, &params); err != nil {
			t.Fatalf("%s: parameters: %v", name, err)
		}
		if params["type"] != "object" {
			t.Fatalf("%s: type = %v", name, params["type"])
		}
		props, _ := params["properties"].(map[string]any)
		if props["path"] == nil {
			t.Fatalf("%s: expected path property", name)
		}
	}
}
