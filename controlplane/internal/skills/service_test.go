package skills_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/skills"
)

type memFS struct {
	sandbox.FS
	files map[string]string
}

func (m memFS) ReadFile(_ context.Context, p string) ([]byte, error) {
	if c, ok := m.files[path.Clean(p)]; ok {
		return []byte(c), nil
	}
	return nil, errors.New("no such file: " + p)
}

func (m memFS) ReadDir(_ context.Context, p string) ([]sandbox.DirEntry, error) {
	p = path.Clean(p)
	seen := map[string]bool{}
	var out []sandbox.DirEntry
	for f := range m.files {
		rest, ok := strings.CutPrefix(f, p+"/")
		if !ok {
			continue
		}
		name, _, nested := strings.Cut(rest, "/")
		if !seen[name] {
			seen[name] = true
			out = append(out, sandbox.DirEntry{Name: name, IsDir: nested})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("open " + p + ": no such file or directory")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type memEnv struct {
	sandbox.Environment
	fs memFS
}

func (e memEnv) FS() (sandbox.FS, bool)      { return e.fs, true }
func (e memEnv) Close(context.Context) error { return nil }

type memOpener struct {
	files map[string]string
	err   error
}

func (o memOpener) Open(context.Context, string) (sandbox.Environment, error) {
	if o.err != nil {
		return nil, o.err
	}
	return memEnv{fs: memFS{files: o.files}}, nil
}

func skillMD(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\nBody of " + name + "\n"
}

func openStore(t *testing.T) *catalog.Store {
	t.Helper()
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return catalog.Open(pool)
}

func newProject(t *testing.T, store *catalog.Store, settings string) catalog.Project {
	t.Helper()
	p, err := store.CreateProject(context.Background(), "p", "")
	if err != nil {
		t.Fatal(err)
	}
	if settings != "" {
		p, err = store.UpdateProject(context.Background(), p.ID, nil, nil, json.RawMessage(settings))
		if err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func names(entries []skills.Entry) map[string]skills.Entry {
	m := map[string]skills.Entry{}
	for _, e := range entries {
		m[e.Name] = e
	}
	return m
}

func TestServiceResolveLayersProjectSkillsAndToggles(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	project := newProject(t, store, `{"skills":{"disabled":["gamma"]}}`)
	if _, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Skills: json.RawMessage(`{"disabled":["delta"]}`)}); err != nil {
		t.Fatal(err)
	}
	opener := memOpener{files: map[string]string{
		".agents/skills/alpha/SKILL.md":       skillMD("alpha", "from .agents"),
		".agent-fabric/skills/alpha/SKILL.md": skillMD("alpha", "from .agent-fabric"),
		".agents/skills/gamma/SKILL.md":       skillMD("gamma", "gamma"),
		".agents/skills/delta/SKILL.md":       skillMD("delta", "delta"),
		".agents/skills/bad/SKILL.md":         "oops",
	}}
	svc := skills.NewService(store, opener)

	res, err := svc.Resolve(ctx, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	got := names(res.Entries)
	if len(got) != 3 {
		t.Fatalf("entries = %+v", res.Entries)
	}
	if a := got["alpha"]; !a.Enabled || a.Description != "from .agent-fabric" || a.Source != skills.SourceProject {
		t.Fatalf("alpha = %+v", a)
	}
	if got["gamma"].Enabled || got["delta"].Enabled {
		t.Fatalf("project and plane toggles not applied: %+v", got)
	}
	if len(res.Problems) != 1 || res.Problems[0].Path != ".agents/skills/bad" {
		t.Fatalf("problems = %+v", res.Problems)
	}
	if pins := skills.PinEnabled(res.Entries); len(pins) != 1 || pins[0].Name != "alpha" {
		t.Fatalf("pins = %+v", pins)
	}
}

func TestServiceResolveOpenFailureKeepsBuiltins(t *testing.T) {
	store := openStore(t)
	project := newProject(t, store, "")
	svc := skills.NewService(store, memOpener{err: errors.New("docker down")})
	res, err := svc.Resolve(context.Background(), project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 1 || !strings.Contains(res.Problems[0].Error, "docker down") {
		t.Fatalf("problems = %+v", res.Problems)
	}
}

func TestServiceResolveUnknownProject(t *testing.T) {
	store := openStore(t)
	svc := skills.NewService(store, nil)
	if _, err := svc.Resolve(context.Background(), "missing", ""); !errors.Is(err, catalog.ErrProjectNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPListAndGet(t *testing.T) {
	store := openStore(t)
	project := newProject(t, store, "")
	svc := skills.NewService(store, memOpener{files: map[string]string{
		".agents/skills/alpha/SKILL.md":            skillMD("alpha", "an alpha skill"),
		".agents/skills/alpha/references/notes.md": "x",
	}})
	mux := http.NewServeMux()
	skills.Mount(mux, svc)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/skills?projectId=" + project.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list skills.Resolution
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d err %v", resp.StatusCode, err)
	}
	if len(list.Entries) != 1 || list.Entries[0].Name != "alpha" || len(list.Entries[0].Files) != 1 {
		t.Fatalf("list = %+v", list)
	}

	detail, err := http.Get(srv.URL + "/v1/skills/alpha?projectId=" + project.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer detail.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(detail.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["body"] != "Body of alpha" || body["enabled"] != true || body["source"] != "project" {
		t.Fatalf("detail = %+v", body)
	}

	for url, want := range map[string]int{
		"/v1/skills/nope?projectId=" + project.ID: http.StatusNotFound,
		"/v1/skills?projectId=missing":            http.StatusNotFound,
		"/v1/skills?assistantId=missing":          http.StatusNotFound,
	} {
		r, err := http.Get(srv.URL + url)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("%s: status %d, want %d", url, r.StatusCode, want)
		}
	}
}

func TestServiceLoadSkillAndExtraFiles(t *testing.T) {
	store := openStore(t)
	project := newProject(t, store, "")
	opener := memOpener{files: map[string]string{
		".agents/skills/alpha/SKILL.md":            skillMD("alpha", "an alpha skill"),
		".agents/skills/alpha/references/notes.md": "the notes",
	}}
	svc := skills.NewService(store, opener)
	ctx := context.Background()

	entry, body, err := svc.Load(ctx, project.ID, "", "alpha", "", "")
	if err != nil || body != "Body of alpha" {
		t.Fatalf("body = %q, %v", body, err)
	}
	_, extra, err := svc.Load(ctx, project.ID, "", "alpha", "references/notes.md", entry.Hash)
	if err != nil || extra != "the notes" {
		t.Fatalf("extra = %q, %v", extra, err)
	}
	for name, args := range map[string][]string{
		"unknown skill":   {"missing", "", ""},
		"unlisted file":   {"alpha", "../secret", ""},
		"changed hash":    {"alpha", "", "stale"},
		"absolute escape": {"alpha", "/etc/passwd", ""},
	} {
		if _, _, err := svc.Load(ctx, project.ID, "", args[0], args[1], args[2]); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	if _, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{Skills: json.RawMessage(`{"disabled":["alpha"]}`)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Load(ctx, project.ID, "", "alpha", "", ""); err == nil {
		t.Error("a disabled skill must not load")
	}
}
