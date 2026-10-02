package workspace_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func seedFile(t *testing.T, srv *httptest.Server, p, body string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/proj_1/files?path="+p, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("seed %s status %d", p, resp.StatusCode)
	}
}

func postPathOp(t *testing.T, srv *httptest.Server, op, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/v1/projects/proj_1/fs/"+op, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Path != "" {
		return resp.StatusCode, out.Path
	}
	return resp.StatusCode, out.Error
}

func TestMovePathEndpoint(t *testing.T) {
	srv, fsys := testServer(t)
	seedFile(t, srv, "src/a.txt", "alpha")
	seedFile(t, srv, "taken.txt", "keep")
	mkdir, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/proj_1/dirs?path=dest", nil)
	if resp, err := http.DefaultClient.Do(mkdir); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
	}

	cases := []struct {
		name   string
		body   string
		status int
	}{
		{"ok", `{"from":"src/a.txt","to":"dest/a.txt"}`, http.StatusOK},
		{"missing source", `{"from":"nope.txt","to":"x.txt"}`, http.StatusNotFound},
		{"destination exists", `{"from":"dest/a.txt","to":"taken.txt"}`, http.StatusConflict},
		{"escape", `{"from":"dest/a.txt","to":"../x.txt"}`, http.StatusForbidden},
		{"into itself", `{"from":"src","to":"src/sub"}`, http.StatusBadRequest},
		{"same path", `{"from":"taken.txt","to":"taken.txt"}`, http.StatusBadRequest},
		{"missing to", `{"from":"taken.txt"}`, http.StatusBadRequest},
		{"bad json", `nope`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if status, msg := postPathOp(t, srv, "move", tc.body); status != tc.status {
			t.Errorf("%s: status %d (%s), want %d", tc.name, status, msg, tc.status)
		}
	}
	if string(fsys.files["/workspace/dest/a.txt"]) != "alpha" {
		t.Fatal("file was not moved")
	}
	if _, ok := fsys.files["/workspace/src/a.txt"]; ok {
		t.Fatal("source still present")
	}
	if string(fsys.files["/workspace/taken.txt"]) != "keep" {
		t.Fatal("existing destination was overwritten")
	}
}

func TestCopyPathEndpointAndDuplicate(t *testing.T) {
	srv, fsys := testServer(t)
	seedFile(t, srv, "index.html", "<h1>")

	if status, got := postPathOp(t, srv, "copy", `{"from":"index.html"}`); status != http.StatusOK || got != "index copy.html" {
		t.Fatalf("duplicate = %d %q", status, got)
	}
	if status, got := postPathOp(t, srv, "copy", `{"from":"index.html"}`); status != http.StatusOK || got != "index copy 2.html" {
		t.Fatalf("second duplicate = %d %q", status, got)
	}
	if status, got := postPathOp(t, srv, "copy", `{"from":"index.html","to":"explicit.html"}`); status != http.StatusOK || got != "explicit.html" {
		t.Fatalf("explicit copy = %d %q", status, got)
	}
	if status, _ := postPathOp(t, srv, "copy", `{"from":"index.html","to":"explicit.html"}`); status != http.StatusConflict {
		t.Fatalf("collision status %d", status)
	}
	if status, _ := postPathOp(t, srv, "copy", `{"from":"missing.html"}`); status != http.StatusNotFound {
		t.Fatalf("missing status %d", status)
	}
	if string(fsys.files["/workspace/index copy.html"]) != "<h1>" || string(fsys.files["/workspace/index.html"]) != "<h1>" {
		t.Fatal("copy content wrong")
	}
}
