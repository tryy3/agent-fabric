package export_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/export"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

func TestDefaultRegistryListsDownloadAndDisabledGitHub(t *testing.T) {
	reg := export.DefaultRegistry()
	methods := reg.Methods()
	if len(methods) < 2 {
		t.Fatalf("methods = %+v", methods)
	}
	var download, github *export.Method
	for i := range methods {
		m := methods[i]
		switch m.ID {
		case "download":
			download = &m
		case "github":
			github = &m
		}
	}
	if download == nil || !download.Enabled {
		t.Fatalf("download = %+v", download)
	}
	if github == nil || github.Enabled {
		t.Fatalf("github should be visible but disabled: %+v", github)
	}
}

func TestDownloadZipOmitsSecretsAndIncludesWorkspaceThreads(t *testing.T) {
	ctx := context.Background()
	fsys := newMemFS()
	if err := fsys.WriteFile(ctx, "index.html", []byte("<h1>hi</h1>")); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(ctx, ".git/HEAD", []byte("ref: refs/heads/main\n")); err != nil {
		t.Fatal(err)
	}
	reg := export.DefaultRegistry()
	result, err := reg.Export(ctx, "download", export.Request{
		Project: catalog.Project{
			Name:        "Landing",
			Description: "prototype",
			Settings:    json.RawMessage(`{"sandbox":{"image":"alpine"},"apiKey":"sk-secret"}`),
		},
		Threads: []catalog.ThreadDetail{{
			Thread: catalog.Thread{ID: "th_1", Title: "hero", ProjectID: "proj_1"},
			Messages: []catalog.ThreadMessage{{
				ID: "msg_1", Role: "user", Content: "write index.html",
			}},
		}},
		Agents: []catalog.Agent{{
			ID:           "agent_1",
			Name:         "Coder",
			DefaultModel: strPtr("m1"),
			Settings:     json.RawMessage(`{"apiKey":"nope"}`),
		}},
		FS: fsys,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaType != "application/zip" || !strings.HasSuffix(result.Filename, ".zip") {
		t.Fatalf("result %+v", result)
	}
	files := zipNames(t, result.Body)
	for _, want := range []string{"project.json", "threads.json", "workspace/index.html", "workspace/.git/HEAD"} {
		if !files[want] {
			t.Fatalf("missing %s in %v", want, keys(files))
		}
	}
	raw := zipFile(t, result.Body, "project.json")
	if bytes.Contains(raw, []byte("sk-secret")) || bytes.Contains(raw, []byte("apiKey")) {
		t.Fatalf("project.json leaked secrets: %s", raw)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["name"] != "Landing" {
		t.Fatalf("name = %v", meta["name"])
	}
	threads := zipFile(t, result.Body, "threads.json")
	if !bytes.Contains(threads, []byte("write index.html")) {
		t.Fatalf("threads.json missing messages: %s", threads)
	}
	if bytes.Contains(result.Body, []byte("sk-secret")) || bytes.Contains(threads, []byte("nope")) {
		t.Fatalf("archive leaked a secret")
	}
}

func TestGitHubExportIsDisabled(t *testing.T) {
	_, err := export.DefaultRegistry().Export(context.Background(), "github", export.Request{
		Project: catalog.Project{Name: "Landing"},
	})
	if !errors.Is(err, export.ErrDisabled) {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadRespectsSizeCap(t *testing.T) {
	ctx := context.Background()
	fsys := newMemFS()
	if err := fsys.WriteFile(ctx, "big.bin", bytes.Repeat([]byte("a"), 200)); err != nil {
		t.Fatal(err)
	}
	reg := export.NewRegistry()
	reg.Register(export.Download{MaxBytes: 64})
	_, err := reg.Export(ctx, "download", export.Request{
		Project: catalog.Project{Name: "Huge"},
		FS:      fsys,
	})
	if !errors.Is(err, export.ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownMethod(t *testing.T) {
	_, err := export.DefaultRegistry().Export(context.Background(), "s3", export.Request{})
	if !errors.Is(err, export.ErrUnknownMethod) {
		t.Fatalf("err = %v", err)
	}
}

type stubExporter struct {
	method export.Method
	result export.Result
}

func (s stubExporter) Method() export.Method { return s.method }

func (s stubExporter) Export(context.Context, export.Request) (export.Result, error) {
	return s.result, nil
}

func TestRegisterAdditionalExporterKeepsDownload(t *testing.T) {
	ctx := context.Background()
	reg := export.DefaultRegistry()
	reg.Register(stubExporter{
		method: export.Method{ID: "clipboard", Label: "Clipboard", Enabled: true},
		result: export.Result{MediaType: "text/plain", Filename: "note.txt", Body: []byte("hi")},
	})
	var ids []string
	for _, m := range reg.Methods() {
		ids = append(ids, m.ID)
	}
	if !strings.Contains(strings.Join(ids, ","), "download") || !strings.Contains(strings.Join(ids, ","), "clipboard") {
		t.Fatalf("methods = %v", ids)
	}
	got, err := reg.Export(ctx, "clipboard", export.Request{Project: catalog.Project{Name: "Landing"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Body) != "hi" {
		t.Fatalf("clipboard body = %q", got.Body)
	}
	zip, err := reg.Export(ctx, "download", export.Request{Project: catalog.Project{Name: "Landing"}})
	if err != nil {
		t.Fatal(err)
	}
	if zip.MediaType != "application/zip" {
		t.Fatalf("download still must be zip: %+v", zip)
	}
}

func strPtr(s string) *string { return &s }

func zipNames(t *testing.T, body []byte) map[string]bool {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, f := range r.File {
		out[f.Name] = true
	}
	return out
}

func zipFile(t *testing.T, body []byte, name string) []byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("missing zip entry %s", name)
	return nil
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type memFS struct {
	files map[string][]byte
	dirs  map[string]struct{}
}

func newMemFS() *memFS {
	return &memFS{
		files: map[string][]byte{},
		dirs:  map[string]struct{}{"/workspace": {}},
	}
}

func (m *memFS) jail(p string) (string, error) {
	return sandboxcore.ResolvePOSIX("/workspace", p, nil, sandboxcore.PathRead)
}

func (m *memFS) ReadFile(_ context.Context, filePath string) ([]byte, error) {
	resolved, err := m.jail(filePath)
	if err != nil {
		return nil, err
	}
	data, ok := m.files[resolved]
	if !ok {
		return nil, fs.ErrNotExist
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (m *memFS) WriteFile(_ context.Context, filePath string, data []byte) error {
	resolved, err := m.jail(filePath)
	if err != nil {
		return err
	}
	m.ensureDir(path.Dir(resolved))
	cp := make([]byte, len(data))
	copy(cp, data)
	m.files[resolved] = cp
	return nil
}

func (m *memFS) Stat(context.Context, string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (m *memFS) ReadDir(_ context.Context, filePath string) ([]sandbox.DirEntry, error) {
	resolved, err := m.jail(filePath)
	if err != nil {
		return nil, err
	}
	if _, ok := m.dirs[resolved]; !ok {
		return nil, fmt.Errorf("not a directory")
	}
	seen := map[string]sandbox.DirEntry{}
	for dir := range m.dirs {
		if dir != resolved && path.Dir(dir) == resolved {
			seen[path.Base(dir)] = sandbox.DirEntry{Name: path.Base(dir), IsDir: true, ModTime: time.Unix(1, 0)}
		}
	}
	for file, data := range m.files {
		if path.Dir(file) == resolved {
			seen[path.Base(file)] = sandbox.DirEntry{Name: path.Base(file), Size: int64(len(data)), ModTime: time.Unix(1, 0)}
		}
	}
	out := make([]sandbox.DirEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	return out, nil
}

func (m *memFS) Mkdir(context.Context, string) error  { return nil }
func (m *memFS) Remove(context.Context, string) error { return nil }

func (m *memFS) ensureDir(dir string) {
	for dir != "." && dir != "" && dir != "/" {
		m.dirs[dir] = struct{}{}
		if dir == "/workspace" {
			return
		}
		dir = path.Dir(dir)
	}
}
