package file_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/execfs"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
	"github.com/tryy3/agent-fabric/internal/sandbox/tools/file"
)

// execEnv is an environment whose filesystem goes through execfs's sh scripts,
// the same path Docker-backed environments use, so both backends are held to
// one contract.
type execEnv struct {
	sandbox.Environment
	fsys sandboxcore.FS
}

func (e execEnv) FS() (sandboxcore.FS, bool) { return e.fsys, true }

type harness struct {
	t    *testing.T
	root string
	env  sandbox.Environment
	reg  *sandbox.Registry
}

func eachBackend(t *testing.T, run func(t *testing.T, h *harness)) {
	for _, backend := range []string{"local", "exec"} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			env, err := local.New(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = env.Close(context.Background()) })
			var use sandbox.Environment = env
			if backend == "exec" {
				exec, _ := env.Exec()
				use = execEnv{Environment: env, fsys: execfs.New(exec, root, nil)}
			}
			reg := sandbox.NewRegistry()
			for _, tool := range file.Tools() {
				reg.Register(tool)
			}
			run(t, &harness{t: t, root: root, env: use, reg: reg})
		})
	}
}

// call runs a tool and decodes its JSON result.
func (h *harness) call(name string, args map[string]any) map[string]any {
	h.t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		h.t.Fatal(err)
	}
	out, err := h.reg.Call(context.Background(), h.env, name, raw)
	if err != nil {
		h.t.Fatalf("%s: %v", name, err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		h.t.Fatalf("%s: bad result %q: %v", name, out, err)
	}
	return res
}

func (h *harness) ok(name string, args map[string]any) map[string]any {
	h.t.Helper()
	res := h.call(name, args)
	if res["error"] != nil {
		h.t.Fatalf("%s failed: %v", name, res)
	}
	return res
}

func (h *harness) wantCode(name string, args map[string]any, code string) {
	h.t.Helper()
	res := h.call(name, args)
	if res["code"] != code || res["error"] == nil {
		h.t.Fatalf("%s result = %v, want code %q", name, res, code)
	}
}

func (h *harness) put(rel, content string) {
	h.t.Helper()
	full := filepath.Join(h.root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) read(rel string) string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.root, rel))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

func (h *harness) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(h.root, rel))
	return err == nil
}

func paths(res map[string]any, key, field string) []string {
	var out []string
	for _, item := range res[key].([]any) {
		out = append(out, item.(map[string]any)[field].(string))
	}
	return out
}

func equal(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func TestListFiles(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("a.txt", "a")
		h.put("src/main.go", "m")
		h.put("src/util/x.go", "x")
		h.put("node_modules/pkg/index.js", "i")
		h.put(".git/HEAD", "ref")

		res := h.ok("list_files", map[string]any{})
		if got := paths(res, "entries", "path"); !equal(got, []string{"a.txt", "src"}) {
			t.Fatalf("shallow = %v", got)
		}

		res = h.ok("list_files", map[string]any{"recursive": true})
		want := []string{"a.txt", "src", "src/main.go", "src/util", "src/util/x.go"}
		if got := paths(res, "entries", "path"); !equal(got, want) {
			t.Fatalf("recursive = %v", got)
		}

		res = h.ok("list_files", map[string]any{"recursive": true, "max_depth": 1})
		if got := paths(res, "entries", "path"); !equal(got, []string{"a.txt", "src"}) {
			t.Fatalf("depth 1 = %v", got)
		}

		res = h.ok("list_files", map[string]any{"path": "src", "recursive": true, "glob": "*.go"})
		if got := paths(res, "entries", "path"); !equal(got, []string{"src/main.go", "src/util/x.go"}) {
			t.Fatalf("glob = %v", got)
		}

		res = h.ok("list_files", map[string]any{"recursive": true, "glob": "src/**/x.go"})
		if got := paths(res, "entries", "path"); !equal(got, []string{"src/util/x.go"}) {
			t.Fatalf("path glob = %v", got)
		}

		res = h.ok("list_files", map[string]any{"include_ignored": true})
		if got := paths(res, "entries", "path"); !equal(got, []string{".git", "a.txt", "node_modules", "src"}) {
			t.Fatalf("include_ignored = %v", got)
		}

		h.wantCode("list_files", map[string]any{"path": "missing"}, "not_found")
		h.wantCode("list_files", map[string]any{"path": "a.txt"}, "not_dir")
	})
}

func TestListFilesTruncates(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		for i := 0; i < file.MaxListEntries+5; i++ {
			h.put(fmt.Sprintf("many/f%04d.txt", i), "x")
		}
		res := h.ok("list_files", map[string]any{"path": "many"})
		if res["truncated"] != true || res["truncated_reason"] != "max_entries" {
			t.Fatalf("truncated = %v", res["truncated"])
		}
		if int(res["count"].(float64)) != file.MaxListEntries {
			t.Fatalf("count = %v", res["count"])
		}
	})
}

func TestSearchText(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("a.txt", "alpha\nNeedle one\nbeta\n")
		h.put("src/b.go", "needle two\nNEEDLE three\n")
		h.put("node_modules/n.js", "needle ignored\n")
		h.put("bin.dat", "needle\x00binary")

		res := h.ok("search_text", map[string]any{"query": "needle"})
		got := paths(res, "matches", "path")
		if !equal(got, []string{"src/b.go"}) {
			t.Fatalf("case-sensitive matches = %v (%v)", got, res)
		}
		if res["skipped_binary"].(float64) != 1 {
			t.Fatalf("skipped_binary = %v", res["skipped_binary"])
		}

		res = h.ok("search_text", map[string]any{"query": "needle", "case_sensitive": false})
		if got := paths(res, "matches", "path"); !equal(got, []string{"a.txt", "src/b.go", "src/b.go"}) {
			t.Fatalf("insensitive = %v", got)
		}

		res = h.ok("search_text", map[string]any{"query": `^n\w+e (one|two)$`, "regex": true, "case_sensitive": false})
		if got := paths(res, "matches", "path"); len(got) != 2 {
			t.Fatalf("regex = %v", got)
		}

		res = h.ok("search_text", map[string]any{"query": "needle", "glob": "*.txt", "case_sensitive": false})
		if got := paths(res, "matches", "path"); !equal(got, []string{"a.txt"}) {
			t.Fatalf("glob = %v", got)
		}

		res = h.ok("search_text", map[string]any{"query": "one", "path": "a.txt"})
		if got := paths(res, "matches", "path"); !equal(got, []string{"a.txt"}) {
			t.Fatalf("single file = %v", got)
		}

		h.wantCode("search_text", map[string]any{"query": "(", "regex": true}, "invalid_args")
		h.wantCode("search_text", map[string]any{"query": ""}, "invalid_args")
		h.wantCode("search_text", map[string]any{"query": "x", "path": "nope"}, "not_found")
	})
}

func TestSearchTextCaps(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		var perFile strings.Builder
		for i := 0; i < file.MaxSearchMatchesFile+10; i++ {
			perFile.WriteString("hit\n")
		}
		h.put("one.txt", perFile.String())
		res := h.ok("search_text", map[string]any{"query": "hit"})
		if n := len(res["matches"].([]any)); n != file.MaxSearchMatchesFile {
			t.Fatalf("per-file matches = %d", n)
		}
		if res["files_with_more_matches"].(float64) != 1 || res["truncated"] != false {
			t.Fatalf("result = %v", res)
		}

		for i := 0; i < 10; i++ {
			h.put(fmt.Sprintf("f%d.txt", i), perFile.String())
		}
		res = h.ok("search_text", map[string]any{"query": "hit"})
		if n := len(res["matches"].([]any)); n != file.MaxSearchMatchesTotal {
			t.Fatalf("total matches = %d", n)
		}
		if res["truncated"] != true || res["truncated_reason"] != "max_matches" {
			t.Fatalf("result = %v", res)
		}
	})
}

func TestApplyPatch(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("a.txt", "one\ntwo\nthree\n")
		h.put("sub/b.txt", "x\ny\n")

		diff := "--- a/a.txt\n+++ b/a.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n" +
			"--- /dev/null\n+++ b/new/c.txt\n@@ -0,0 +1,2 @@\n+hello\n+world\n"
		res := h.ok("apply_patch", map[string]any{"diff": diff})
		if got := paths(res, "files", "path"); !equal(got, []string{"a.txt", "new/c.txt"}) {
			t.Fatalf("files = %v", got)
		}
		if h.read("a.txt") != "one\nTWO\nthree\n" || h.read("new/c.txt") != "hello\nworld\n" {
			t.Fatalf("contents: %q %q", h.read("a.txt"), h.read("new/c.txt"))
		}

		// A mismatch in the second file leaves the first untouched.
		bad := "--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-one\n+ONE\n" +
			"--- a/sub/b.txt\n+++ b/sub/b.txt\n@@ -1,1 +1,1 @@\n-nope\n+zzz\n"
		h.wantCode("apply_patch", map[string]any{"diff": bad}, "mismatch")
		if h.read("a.txt") != "one\nTWO\nthree\n" || h.read("sub/b.txt") != "x\ny\n" {
			t.Fatal("mismatch changed files")
		}

		// Wrong line number is a mismatch, not a fuzzy relocation.
		offset := "--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-three\n+3\n"
		h.wantCode("apply_patch", map[string]any{"diff": offset}, "mismatch")

		// Creating over an existing file collides.
		clash := "--- /dev/null\n+++ b/a.txt\n@@ -0,0 +1 @@\n+x\n"
		h.wantCode("apply_patch", map[string]any{"diff": clash}, "exists")

		h.wantCode("apply_patch", map[string]any{"diff": "--- a/missing\n+++ b/missing\n@@ -1 +1 @@\n-a\n+b\n"}, "not_found")
		h.wantCode("apply_patch", map[string]any{"diff": "not a diff"}, "invalid_args")
		h.wantCode("apply_patch", map[string]any{"diff": "--- a/a.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-one\n"}, "unsupported")
		h.wantCode("apply_patch", map[string]any{"diff": "--- /dev/null\n+++ b/.git/config\n@@ -0,0 +1 @@\n+x\n"}, "protected")
	})
}

func TestApplyPatchLimits(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("bin.dat", "a\x00b\n")
		h.put("big.txt", strings.Repeat("x", file.MaxEditFileBytes+1))
		patch := func(p string) map[string]any {
			return map[string]any{"diff": "--- a/" + p + "\n+++ b/" + p + "\n@@ -1 +1 @@\n-a\n+b\n"}
		}
		h.wantCode("apply_patch", patch("bin.dat"), "binary")
		h.wantCode("apply_patch", patch("big.txt"), "too_large")
	})
}

func TestAppendFile(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("nonl.txt", "abc")
		h.put("nl.txt", "abc\n")
		h.ok("append_file", map[string]any{"path": "nonl.txt", "content": "def"})
		if got := h.read("nonl.txt"); got != "abc\ndef" {
			t.Fatalf("ensure_newline default = %q", got)
		}
		h.ok("append_file", map[string]any{"path": "nl.txt", "content": "def\n"})
		if got := h.read("nl.txt"); got != "abc\ndef\n" {
			t.Fatalf("existing newline = %q", got)
		}
		h.put("raw.txt", "abc")
		h.ok("append_file", map[string]any{"path": "raw.txt", "content": "def", "ensure_newline": false})
		if got := h.read("raw.txt"); got != "abcdef" {
			t.Fatalf("raw = %q", got)
		}

		h.wantCode("append_file", map[string]any{"path": "none.txt", "content": "x"}, "not_found")
		res := h.ok("append_file", map[string]any{"path": "d/none.txt", "content": "x", "create": true})
		if res["created"] != true || h.read("d/none.txt") != "x" {
			t.Fatalf("create = %v", res)
		}

		h.put("big.txt", strings.Repeat("x", file.MaxEditFileBytes))
		h.wantCode("append_file", map[string]any{"path": "big.txt", "content": "y"}, "too_large")
		h.put("bin.dat", "a\x00")
		h.wantCode("append_file", map[string]any{"path": "bin.dat", "content": "y"}, "binary")
		h.wantCode("append_file", map[string]any{"path": ".git/config", "content": "y"}, "protected")
	})
}

func TestCreateDirectory(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		res := h.ok("create_directory", map[string]any{"path": "a/b/c"})
		if res["created"] != true || !h.exists("a/b/c") {
			t.Fatalf("create = %v", res)
		}
		res = h.ok("create_directory", map[string]any{"path": "a/b/c"})
		if res["created"] != false {
			t.Fatalf("idempotent = %v", res)
		}
		h.put("f.txt", "x")
		h.wantCode("create_directory", map[string]any{"path": "f.txt"}, "exists")
		h.wantCode("create_directory", map[string]any{"path": ".git/x"}, "protected")
	})
}

func TestMovePath(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("a.txt", "A")
		h.put("b.txt", "B")
		h.put("dir/inner.txt", "I")
		h.put(".git/HEAD", "ref")
		if err := os.MkdirAll(filepath.Join(h.root, "dest"), 0o755); err != nil {
			t.Fatal(err)
		}

		h.ok("move_path", map[string]any{"from": "a.txt", "to": "dest/a.txt"})
		if h.exists("a.txt") || h.read("dest/a.txt") != "A" {
			t.Fatal("file not moved")
		}
		h.ok("move_path", map[string]any{"from": "dir", "to": "renamed"})
		if h.read("renamed/inner.txt") != "I" {
			t.Fatal("dir not moved")
		}

		// Collision leaves both sides unchanged.
		h.wantCode("move_path", map[string]any{"from": "b.txt", "to": "dest/a.txt"}, "exists")
		if h.read("b.txt") != "B" || h.read("dest/a.txt") != "A" {
			t.Fatal("collision changed files")
		}
		h.wantCode("move_path", map[string]any{"from": "missing", "to": "x"}, "not_found")
		h.wantCode("move_path", map[string]any{"from": "b.txt", "to": "nodir/b.txt"}, "not_found")
		h.wantCode("move_path", map[string]any{"from": "renamed", "to": "renamed/sub"}, "invalid_args")
		h.wantCode("move_path", map[string]any{"from": "b.txt", "to": "../escape.txt"}, "invalid_args")
		h.wantCode("move_path", map[string]any{"from": ".git", "to": "git2"}, "protected")
		h.wantCode("move_path", map[string]any{"from": "b.txt", "to": ".git/b.txt"}, "protected")
	})
}

func TestDeletePath(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("f.txt", "x")
		h.put("full/inner.txt", "x")
		h.put(".git/HEAD", "ref")
		if err := os.MkdirAll(filepath.Join(h.root, "empty"), 0o755); err != nil {
			t.Fatal(err)
		}

		h.ok("delete_path", map[string]any{"path": "f.txt"})
		if h.exists("f.txt") {
			t.Fatal("file not deleted")
		}
		h.ok("delete_path", map[string]any{"path": "empty"})
		if h.exists("empty") {
			t.Fatal("empty dir not deleted")
		}

		h.wantCode("delete_path", map[string]any{"path": "full"}, "not_empty")
		if !h.exists("full/inner.txt") {
			t.Fatal("non-empty directory was erased")
		}
		h.wantCode("delete_path", map[string]any{"path": "missing"}, "not_found")
		h.wantCode("delete_path", map[string]any{"path": "."}, "invalid_args")
		h.wantCode("delete_path", map[string]any{"path": "/"}, "invalid_args")
		h.wantCode("delete_path", map[string]any{"path": ".git"}, "protected")
		h.wantCode("delete_path", map[string]any{"path": ".git/HEAD"}, "protected")
	})
}

func TestReadFileLimits(t *testing.T) {
	eachBackend(t, func(t *testing.T, h *harness) {
		h.put("ok.txt", "héllo")
		h.put("bin.dat", "a\x00b")
		h.put("big.txt", strings.Repeat("x", file.MaxEditFileBytes+1))
		res := h.ok("read_file", map[string]any{"path": "ok.txt"})
		if res["content"] != "héllo" {
			t.Fatalf("content = %v", res["content"])
		}
		h.wantCode("read_file", map[string]any{"path": "bin.dat"}, "binary")
		h.wantCode("read_file", map[string]any{"path": "big.txt"}, "too_large")
		h.wantCode("read_file", map[string]any{"path": "nope"}, "not_found")
		h.wantCode("write_file", map[string]any{"path": ".git/config", "content": "x"}, "protected")
	})
}

func TestIsMutating(t *testing.T) {
	for _, name := range []string{"write_file", "apply_patch", "append_file", "create_directory", "move_path", "delete_path"} {
		if !file.IsMutating(name) {
			t.Errorf("%s should be mutating", name)
		}
	}
	for _, name := range []string{"read_file", "list_files", "search_text", "ask_user"} {
		if file.IsMutating(name) {
			t.Errorf("%s should not be mutating", name)
		}
	}
}
