package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
	"github.com/tryy3/agent-fabric/internal/workspace"
)

type fakeEnv struct {
	fs sandbox.FS
}

func (e *fakeEnv) ID() string { return "fake" }
func (e *fakeEnv) Caps() sandbox.Capabilities {
	return sandbox.Capabilities{FS: true}
}
func (e *fakeEnv) FS() (sandbox.FS, bool) { return e.fs, true }
func (e *fakeEnv) Exec() (sandbox.Executor, bool) {
	return nil, false
}
func (e *fakeEnv) Close(context.Context) error { return nil }

type mapOpener struct {
	mu   sync.Mutex
	envs map[string]sandbox.Environment
}

func (o *mapOpener) Open(_ context.Context, projectID string) (sandbox.Environment, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	env, ok := o.envs[projectID]
	if !ok {
		return nil, catalog.ErrProjectNotFound
	}
	return env, nil
}

type memFS struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]struct{}
}

func newMemFS() *memFS {
	return &memFS{
		files: map[string][]byte{},
		dirs:  map[string]struct{}{"/workspace": {}},
	}
}

func (m *memFS) jail(userPath string, access sandboxcore.PathAccess) (string, error) {
	_ = access
	return sandboxcore.ResolvePOSIX("/workspace", userPath, nil, sandboxcore.PathRead)
}

func (m *memFS) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := m.jail(filePath, sandboxcore.PathRead)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[resolved]
	if !ok {
		return nil, fs.ErrNotExist
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (m *memFS) WriteFile(ctx context.Context, filePath string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := m.jail(filePath, sandboxcore.PathWrite)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	parent := path.Dir(resolved)
	m.ensureDirLocked(parent)
	cp := make([]byte, len(data))
	copy(cp, data)
	m.files[resolved] = cp
	return nil
}

func (m *memFS) Stat(ctx context.Context, filePath string) (fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := m.jail(filePath, sandboxcore.PathRead)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.files[resolved]; ok {
		return memInfo{name: path.Base(resolved), size: int64(len(data)), dir: false, mod: time.Unix(1, 0)}, nil
	}
	if _, ok := m.dirs[resolved]; ok {
		return memInfo{name: path.Base(resolved), dir: true, mod: time.Unix(1, 0)}, nil
	}
	return nil, fs.ErrNotExist
}

func (m *memFS) ReadDir(ctx context.Context, filePath string) ([]sandbox.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := m.jail(filePath, sandboxcore.PathRead)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.dirs[resolved]; !ok {
		return nil, fmt.Errorf("not a directory")
	}
	seen := map[string]sandbox.DirEntry{}
	for dir := range m.dirs {
		if dir == "/" || dir == resolved {
			continue
		}
		if path.Dir(dir) == resolved {
			seen[path.Base(dir)] = sandbox.DirEntry{Name: path.Base(dir), IsDir: true, ModTime: time.Unix(1, 0)}
		}
	}
	for file, data := range m.files {
		if path.Dir(file) == resolved {
			seen[path.Base(file)] = sandbox.DirEntry{
				Name:    path.Base(file),
				IsDir:   false,
				Size:    int64(len(data)),
				ModTime: time.Unix(1, 0),
			}
		}
	}
	out := make([]sandbox.DirEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	return out, nil
}

func (m *memFS) Mkdir(ctx context.Context, filePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := m.jail(filePath, sandboxcore.PathWrite)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureDirLocked(resolved)
	return nil
}

func (m *memFS) Remove(ctx context.Context, filePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := m.jail(filePath, sandboxcore.PathWrite)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[resolved]; ok {
		delete(m.files, resolved)
		return nil
	}
	if _, ok := m.dirs[resolved]; ok {
		for dir := range m.dirs {
			if dir != resolved && (path.Dir(dir) == resolved) {
				return fmt.Errorf("directory not empty")
			}
		}
		for file := range m.files {
			if path.Dir(file) == resolved {
				return fmt.Errorf("directory not empty")
			}
		}
		if resolved == "/workspace" {
			return fmt.Errorf("cannot remove root")
		}
		delete(m.dirs, resolved)
		return nil
	}
	return fs.ErrNotExist
}

func (m *memFS) ensureDirLocked(dir string) {
	for dir != "." && dir != "" && dir != "/" {
		m.dirs[dir] = struct{}{}
		if dir == "/workspace" {
			return
		}
		dir = path.Dir(dir)
	}
}

type memInfo struct {
	name string
	size int64
	dir  bool
	mod  time.Time
}

func (i memInfo) Name() string { return i.name }
func (i memInfo) Size() int64  { return i.size }
func (i memInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (i memInfo) ModTime() time.Time { return i.mod }
func (i memInfo) IsDir() bool        { return i.dir }
func (i memInfo) Sys() any           { return nil }

func testServer(t *testing.T) (*httptest.Server, *memFS) {
	t.Helper()
	fsys := newMemFS()
	opener := &mapOpener{envs: map[string]sandbox.Environment{
		"proj_1": &fakeEnv{fs: fsys},
	}}
	srv := httptest.NewServer(workspace.Handler(opener))
	t.Cleanup(srv.Close)
	return srv, fsys
}

func TestListFSMkdirPutGetPreviewAndJail(t *testing.T) {
	srv, _ := testServer(t)

	mkdir, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/proj_1/dirs?path=src", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(mkdir)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("mkdir status %d", resp.StatusCode)
	}

	html := []byte("<html><head></head><body><h1>hi</h1><script src=\"app.js\"></script></body></html>")
	put, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/proj_1/files?path=src/index.html", bytes.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put status %d", resp.StatusCode)
	}

	list, err := http.Get(srv.URL + "/v1/projects/proj_1/fs?path=src")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list status %d", list.StatusCode)
	}
	var listing struct {
		Path    string `json:"path"`
		Entries []struct {
			Name  string `json:"name"`
			IsDir bool   `json:"isDir"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listing); err != nil {
		t.Fatal(err)
	}
	if listing.Path != "src" || len(listing.Entries) != 1 || listing.Entries[0].Name != "index.html" || listing.Entries[0].IsDir {
		t.Fatalf("listing = %+v", listing)
	}

	got, err := http.Get(srv.URL + "/v1/projects/proj_1/files?path=src/index.html")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if got.StatusCode != http.StatusOK || !bytes.Equal(body, html) {
		t.Fatalf("get file status %d body %s", got.StatusCode, body)
	}

	preview, err := http.Get(srv.URL + "/v1/projects/proj_1/preview/src/index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer preview.Body.Close()
	previewBody, _ := io.ReadAll(preview.Body)
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("preview status %d body %s", preview.StatusCode, previewBody)
	}
	if preview.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff = %q", preview.Header.Get("X-Content-Type-Options"))
	}
	if preview.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("cache = %q", preview.Header.Get("Cache-Control"))
	}
	csp := preview.Header.Get("Content-Security-Policy")
	if strings.Contains(csp, "'self'") {
		t.Fatalf("csp used 'self': %s", csp)
	}
	if !strings.Contains(csp, "connect-src 'none'") {
		t.Fatalf("csp missing connect-src none: %s", csp)
	}
	wantPrefix := srv.URL + "/v1/projects/proj_1/preview/"
	if !strings.Contains(csp, wantPrefix) {
		t.Fatalf("csp missing path prefix %q: %s", wantPrefix, csp)
	}
	if !bytes.Contains(previewBody, []byte(`<base href="/v1/projects/proj_1/preview/">`)) {
		t.Fatalf("missing base href in %s", previewBody)
	}
	if preview.Header.Get("Access-Control-Allow-Origin") == "null" || preview.Header.Get("Access-Control-Allow-Origin") == "*" {
		t.Fatalf("unexpected ACAO %q", preview.Header.Get("Access-Control-Allow-Origin"))
	}

	jail, err := http.Get(srv.URL + "/v1/projects/proj_1/files?path=../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	jail.Body.Close()
	if jail.StatusCode != http.StatusForbidden {
		t.Fatalf("jail status %d", jail.StatusCode)
	}

	unknown, err := http.Get(srv.URL + "/v1/projects/missing/fs?path=/")
	if err != nil {
		t.Fatal(err)
	}
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("missing project status %d", unknown.StatusCode)
	}
}

func TestPreviewRejectsDirectoryIndex(t *testing.T) {
	srv, fsys := testServer(t)
	if err := fsys.Mkdir(context.Background(), "src"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/v1/projects/proj_1/preview/src")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("dir preview status %d", resp.StatusCode)
	}
}

func TestDeleteFileAndNonEmptyDir(t *testing.T) {
	srv, _ := testServer(t)
	put, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/proj_1/files?path=src/a.txt", strings.NewReader("x"))
	resp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	delDir, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/projects/proj_1/files?path=src", nil)
	resp, err = http.DefaultClient.Do(delDir)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("non-empty dir status %d", resp.StatusCode)
	}

	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/projects/proj_1/files?path=src/a.txt", nil)
	resp, err = http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete file status %d", resp.StatusCode)
	}
}

func TestGetFileRejectsOversize(t *testing.T) {
	srv, fsys := testServer(t)
	big := bytes.Repeat([]byte("a"), (2<<20)+1)
	if err := fsys.WriteFile(context.Background(), "big.txt", big); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/v1/projects/proj_1/files?path=big.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status %d", resp.StatusCode)
	}
}
