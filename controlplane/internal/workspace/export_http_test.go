package workspace_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/export"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/workspace"
)

func exportServer(t *testing.T, exporters *export.Registry) (*httptest.Server, *memFS) {
	t.Helper()
	fsys := newMemFS()
	opener := &mapOpener{envs: map[string]sandbox.Environment{
		"proj_1": &fakeEnv{fs: fsys},
	}}
	var handler http.Handler
	if exporters == nil {
		handler = workspace.Handler(opener)
	} else {
		handler = workspace.HandlerWithExporters(opener, nil, exporters)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, fsys
}

func TestListExportersIncludesDisabledGitHub(t *testing.T) {
	srv, _ := exportServer(t, nil)
	resp, err := http.Get(srv.URL + "/v1/projects/proj_1/exporters")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	var body struct {
		Exporters []export.Method `json:"exporters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	var download, github, netlify *export.Method
	for i := range body.Exporters {
		m := body.Exporters[i]
		switch m.ID {
		case "download":
			download = &m
		case "github":
			github = &m
		case "netlify":
			netlify = &m
		}
	}
	if download == nil || !download.Enabled {
		t.Fatalf("download = %+v", download)
	}
	if github == nil || github.Enabled {
		t.Fatalf("github should be listed but disabled: %+v", github)
	}
	if netlify == nil || !netlify.Enabled {
		t.Fatalf("netlify = %+v", netlify)
	}
}

func TestExportDownloadZip(t *testing.T) {
	srv, fsys := exportServer(t, nil)
	if err := fsys.WriteFile(context.Background(), "index.html", []byte("<h1>hi</h1>")); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{"method":"download"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/zip") {
		t.Fatalf("content-type = %q", resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), ".zip") {
		t.Fatalf("content-disposition = %q", resp.Header.Get("Content-Disposition"))
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"project.json", "threads.json", "workspace/index.html"} {
		if !names[want] {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
}

func TestExportGitHubIsDisabled(t *testing.T) {
	srv, _ := exportServer(t, nil)
	resp, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{"method":"github"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !bytes.Contains(body, []byte(`"error"`)) {
		t.Fatalf("body = %s", body)
	}
}

func TestExportUnknownMethod(t *testing.T) {
	srv, _ := exportServer(t, nil)
	resp, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{"method":"s3"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestExportTooLarge(t *testing.T) {
	reg := export.NewRegistry()
	reg.Register(export.Download{MaxBytes: 64})
	srv, fsys := exportServer(t, reg)
	if err := fsys.WriteFile(context.Background(), "big.bin", bytes.Repeat([]byte("a"), 200)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{"method":"download"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &errBody); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errBody.Error, "50 MiB") {
		t.Fatalf("error = %q", errBody.Error)
	}
}

func TestExportMissingProject(t *testing.T) {
	srv, _ := exportServer(t, nil)
	resp, err := http.Post(srv.URL+"/v1/projects/missing/export", "application/json", strings.NewReader(`{"method":"download"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

type stubHTTPExporter struct {
	method export.Method
	result export.Result
}

func (s stubHTTPExporter) Method() export.Method { return s.method }

func (s stubHTTPExporter) Export(context.Context, export.Request) (export.Result, error) {
	return s.result, nil
}

func TestAddedExporterDoesNotRewriteDownload(t *testing.T) {
	reg := export.DefaultRegistry()
	reg.Register(stubHTTPExporter{
		method: export.Method{ID: "clipboard", Label: "Clipboard", Enabled: true},
		result: export.Result{MediaType: "text/plain", Filename: "note.txt", Body: []byte("copied")},
	})
	srv, _ := exportServer(t, reg)

	list, err := http.Get(srv.URL + "/v1/projects/proj_1/exporters")
	if err != nil {
		t.Fatal(err)
	}
	listBody, _ := io.ReadAll(list.Body)
	list.Body.Close()
	if !bytes.Contains(listBody, []byte(`"clipboard"`)) || !bytes.Contains(listBody, []byte(`"download"`)) {
		t.Fatalf("exporters = %s", listBody)
	}

	clip, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{"method":"clipboard"}`))
	if err != nil {
		t.Fatal(err)
	}
	clipBody, _ := io.ReadAll(clip.Body)
	clip.Body.Close()
	if clip.StatusCode != http.StatusOK || string(clipBody) != "copied" {
		t.Fatalf("clipboard status %d body %s", clip.StatusCode, clipBody)
	}

	dl, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{"method":"download"}`))
	if err != nil {
		t.Fatal(err)
	}
	dlBody, _ := io.ReadAll(dl.Body)
	dl.Body.Close()
	if dl.StatusCode != http.StatusOK || !strings.Contains(dl.Header.Get("Content-Type"), "application/zip") {
		t.Fatalf("download status %d type %s body %s", dl.StatusCode, dl.Header.Get("Content-Type"), dlBody)
	}
	if _, err := zip.NewReader(bytes.NewReader(dlBody), int64(len(dlBody))); err != nil {
		t.Fatalf("download is not a zip: %v", err)
	}
}

func TestExportPublishReturnsJSON(t *testing.T) {
	reg := export.NewRegistry()
	reg.Register(stubHTTPExporter{
		method: export.Method{ID: "netlify", Label: "Netlify", Enabled: true},
		result: export.Result{
			MediaType: "application/json",
			Message:   "Published to Netlify",
			Links: []export.Link{
				{ID: "site", Label: "Site", URL: "https://demo.netlify.app"},
				{ID: "deploy", Label: "This deploy", URL: "https://dep--demo.netlify.app"},
			},
		},
	})
	srv, _ := exportServer(t, reg)
	resp, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{"method":"netlify"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("content-type = %q", resp.Header.Get("Content-Type"))
	}
	var got struct {
		Method  string        `json:"method"`
		Message string        `json:"message"`
		Links   []export.Link `json:"links"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Method != "netlify" || len(got.Links) != 2 {
		t.Fatalf("body = %s", body)
	}
}

func TestExportEmptyMethodDefaultsToDownload(t *testing.T) {
	srv, _ := exportServer(t, nil)
	resp, err := http.Post(srv.URL+"/v1/projects/proj_1/export", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/zip") {
		t.Fatalf("content-type = %q", resp.Header.Get("Content-Type"))
	}
}
