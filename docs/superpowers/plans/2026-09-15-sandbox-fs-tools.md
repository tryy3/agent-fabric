# Sandbox FS Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a standalone `sandbox` Go package with capability-shaped environments (local + docker/podman), `read_file` / `write_file` tools with OpenAI-shaped schemas, a JSON config loader outside the core package, and tests that do not require the ACP agent loop.

**Architecture:** `Open(OpenOptions) → Environment` with optional `FS` and `Executor`. Local uses native rooted I/O; Docker uses a container manager (scope `shared`|`session`, idle TTL) plus an exec-backed FS. Tools register once and filter by capabilities. Config JSON maps three layers (global / environment / engine) into `OpenOptions` without the sandbox package reading files.

**Tech Stack:** Go 1.22+, stdlib only for V1 (`os`, `os/exec`, `io/fs`, `encoding/json`, `path/filepath`, `testing`). Module: `github.com/tryy3/agent-fabric` under `controlplane/`.

## Global Constraints

- Module root: `controlplane/`; run unit tests with `go -C controlplane test ./internal/sandbox/... ./internal/sandboxconfig/...`.
- Package `sandbox` must not import `agent`, `transport`, `catalog`, or `github.com/coder/acp-go-sdk`.
- Package `sandbox` must not read config files; only accept `OpenOptions`.
- `workspaceRoot` is always a path **inside** the environment (host for local, container for docker).
- Docker auto runtime: `BinPath` → named `Runtime` → else first of `podman`, then `docker` on `PATH`.
- Docker scope default: `session`; `IdleTTL == 0` → 10 minutes.
- Exactly one of `image` or `dockerfile` when `kind=docker`.
- Follow TDD: failing test → implement → pass → commit per task.
- Integration tests: `//go:build integration`; skip if no runtime binary.

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/sandbox/capabilities.go` | `Capabilities` type + `Satisfies` helper |
| `controlplane/internal/sandbox/path.go` | Resolve/jail paths under workspace root |
| `controlplane/internal/sandbox/path_test.go` | Escape / symlink / relative cases |
| `controlplane/internal/sandbox/fs.go` | `FS` interface |
| `controlplane/internal/sandbox/exec.go` | `Executor`, `ExecRequest`, `ExecResult` |
| `controlplane/internal/sandbox/env.go` | `Environment`, `OpenOptions`, `Scope`, `Open` |
| `controlplane/internal/sandbox/tool.go` | `Tool`, `Registry`, OpenAI definition shape |
| `controlplane/internal/sandbox/local/fs.go` | Native rooted FS |
| `controlplane/internal/sandbox/local/env.go` | Local environment + local executor |
| `controlplane/internal/sandbox/local/fs_test.go` | Jail + read/write |
| `controlplane/internal/sandbox/tools/file/file.go` | `read_file` / `write_file` tools |
| `controlplane/internal/sandbox/tools/file/file_test.go` | Tool round-trip via local env |
| `controlplane/internal/sandbox/execfs/fs.go` | Exec-backed FS (shared by docker) |
| `controlplane/internal/sandbox/execfs/fs_test.go` | Fake executor tests |
| `controlplane/internal/sandbox/container/manager.go` | Scope acquire, idle reclaim, runtime CLI |
| `controlplane/internal/sandbox/container/manager_test.go` | Fake CLI runner tests |
| `controlplane/internal/sandbox/docker/runtime.go` | Resolve binary / build image |
| `controlplane/internal/sandbox/docker/env.go` | Docker environment wiring |
| `controlplane/internal/sandboxconfig/config.go` | JSON → `OpenOptions` |
| `controlplane/internal/sandboxconfig/config_test.go` | Layer validation + path resolve |
| `controlplane/internal/sandbox/docker/integration_test.go` | Real podman/docker (build tag) |

---

### Task 1: Path jail helpers

**Files:**
- Create: `controlplane/internal/sandbox/path.go`
- Create: `controlplane/internal/sandbox/path_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  - `func ResolveUnderRoot(root, userPath string) (abs string, err error)` — cleans `userPath`, joins with `root` if relative, requires result stays under cleaned `root` after `filepath.EvalSymlinks` on existing parents where possible; returns error containing `escapes workspace root` on escape

- [ ] **Step 1: Write the failing test**

```go
package sandbox_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

func TestResolveUnderRootAcceptsRelative(t *testing.T) {
	root := t.TempDir()
	got, err := sandbox.ResolveUnderRoot(root, "a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "a", "b.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveUnderRootRejectsDotDot(t *testing.T) {
	root := t.TempDir()
	_, err := sandbox.ResolveUnderRoot(root, "../outside.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveUnderRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "leak")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	_, err := sandbox.ResolveUnderRoot(root, "leak/secret.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/sandbox/ -run TestResolveUnderRoot -count=1`

Expected: FAIL (package/function undefined)

- [ ] **Step 3: Write minimal implementation**

```go
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveUnderRoot(root, userPath string) (string, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	rootResolved, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		rootResolved = cleanRoot
	}

	var candidate string
	if filepath.IsAbs(userPath) {
		candidate = filepath.Clean(userPath)
	} else {
		candidate = filepath.Join(cleanRoot, userPath)
		candidate = filepath.Clean(candidate)
	}

	resolved, err := evalExistingPrefix(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace root %q", userPath, cleanRoot)
	}
	return candidate, nil
}

// evalExistingPrefix EvalSymlinks the longest existing prefix, then rejoins the tail.
func evalExistingPrefix(path string) (string, error) {
	path = filepath.Clean(path)
	cur := path
	var tail []string
	for {
		if _, err := os.Lstat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		dir, base := filepath.Dir(cur), filepath.Base(cur)
		if dir == cur {
			return path, nil
		}
		tail = append(tail, base)
		cur = dir
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/sandbox/ -run TestResolveUnderRoot -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/sandbox/path.go controlplane/internal/sandbox/path_test.go
git commit -m "$(cat <<'EOF'
feat(sandbox): add workspace path jail helper

EOF
)"
```

---

### Task 2: Capabilities, FS, Executor, Environment interfaces + Open local

**Files:**
- Create: `controlplane/internal/sandbox/capabilities.go`
- Create: `controlplane/internal/sandbox/fs.go`
- Create: `controlplane/internal/sandbox/exec.go`
- Create: `controlplane/internal/sandbox/env.go`
- Create: `controlplane/internal/sandbox/local/fs.go`
- Create: `controlplane/internal/sandbox/local/env.go`
- Create: `controlplane/internal/sandbox/local/fs_test.go`
- Create: `controlplane/internal/sandbox/open_test.go`

**Interfaces:**
- Consumes: `ResolveUnderRoot`
- Produces:
  - `type Capabilities struct { FS, Exec bool }`
  - `func (c Capabilities) Satisfies(need Capabilities) bool`
  - `type FS interface { ReadFile(ctx, path) ([]byte, error); WriteFile(ctx, path string, data []byte) error; Stat(ctx, path) (fs.FileInfo, error) }`
  - `type Executor interface { Run(ctx context.Context, req ExecRequest) (ExecResult, error) }`
  - `type Environment interface { ID() string; Caps() Capabilities; FS() (FS, bool); Exec() (Executor, bool); Close(ctx context.Context) error }`
  - `type OpenOptions struct { Kind string; WorkspaceRoot string; Docker *DockerOptions }`
  - `type DockerOptions` / `Scope` / `Mount` as in spec
  - `func Open(ctx context.Context, opts OpenOptions) (Environment, error)` — V1: `kind=local` only in this task; `kind=docker` returns `fmt.Errorf("docker not implemented")`

- [ ] **Step 1: Write the failing test**

`local/fs_test.go`:

```go
package local_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
)

func TestLocalFSReadWriteAndJail(t *testing.T) {
	root := t.TempDir()
	env, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	fsys, ok := env.FS()
	if !ok {
		t.Fatal("expected FS")
	}
	ctx := context.Background()
	if err := fsys.WriteFile(ctx, "hello.txt", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	b, err := fsys.ReadFile(ctx, "hello.txt")
	if err != nil || string(b) != "hi" {
		t.Fatalf("read = %q err=%v", b, err)
	}
	if _, err := fsys.ReadFile(ctx, "../nope.txt"); err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
		t.Fatalf("jail err = %v", err)
	}
	if filepath.Base(env.ID()) == "" {
		t.Fatal("empty id")
	}
}
```

`open_test.go`:

```go
func TestOpenLocal(t *testing.T) {
	root := t.TempDir()
	env, err := sandbox.Open(context.Background(), sandbox.OpenOptions{
		Kind: "local", WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())
	if !env.Caps().FS || !env.Caps().Exec {
		t.Fatalf("caps = %+v", env.Caps())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/sandbox/... -count=1`

Expected: FAIL

- [ ] **Step 3: Implement interfaces + local backend**

- `local.New(root)` returns `Environment` with native FS using `ResolveUnderRoot`, `os.ReadFile` / `os.WriteFile` (mkdir parents on write), and a local `Executor` wrapping `os/exec` with `Dir` under root.
- `sandbox.Open` switches on `opts.Kind`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/sandbox/... -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/sandbox/
git commit -m "$(cat <<'EOF'
feat(sandbox): add Environment contracts and local backend

EOF
)"
```

---

### Task 3: Tool registry + read_file / write_file

**Files:**
- Create: `controlplane/internal/sandbox/tool.go`
- Create: `controlplane/internal/sandbox/tool_test.go`
- Create: `controlplane/internal/sandbox/tools/file/file.go`
- Create: `controlplane/internal/sandbox/tools/file/file_test.go`

**Interfaces:**
- Consumes: `Environment`, `FS`, `Capabilities`
- Produces:
  - `type Tool struct { Name, Description string; Parameters json.RawMessage; Requires Capabilities; Run func(ctx context.Context, env Environment, args json.RawMessage) (string, error) }`
  - `type Definition struct { Type string; Function FunctionDef }` with `FunctionDef{Name, Description string; Parameters json.RawMessage}` — OpenAI chat-completions nested shape (`type: "function"`)
  - `type Registry struct{...}` with `Register(Tool)`, `Definitions(env Environment) []Definition`, `Call(ctx, env, name string, args json.RawMessage) (string, error)`
  - `file.Tools() []sandbox.Tool` returning `read_file` and `write_file`
  - Args: `read_file` → `{"path":"..."}`; `write_file` → `{"path":"...","content":"..."}`
  - Results: JSON object strings; errors as `{"error":"..."}` without failing `Call` (or `Call` returns error only for unknown tool)

- [ ] **Step 1: Write the failing test**

```go
func TestFileToolsRoundTrip(t *testing.T) {
	root := t.TempDir()
	env, err := sandbox.Open(context.Background(), sandbox.OpenOptions{Kind: "local", WorkspaceRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(context.Background())

	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}
	defs := reg.Definitions(env)
	if len(defs) != 2 {
		t.Fatalf("defs = %d", len(defs))
	}
	if defs[0].Type != "function" || defs[0].Function.Name == "" {
		t.Fatalf("bad def: %+v", defs[0])
	}

	out, err := reg.Call(context.Background(), env, "write_file", json.RawMessage(`{"path":"a.txt","content":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"error"`) {
		t.Fatal(out)
	}
	out, err = reg.Call(context.Background(), env, "read_file", json.RawMessage(`{"path":"a.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("out = %s", out)
	}
}

func TestDefinitionsEmptyWithoutFS(t *testing.T) {
	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}
	env := &capsEnv{caps: sandbox.Capabilities{}} // tiny test double: Caps only, FS() false
	if len(reg.Definitions(env)) != 0 {
		t.Fatal("expected no defs")
	}
}
```

Provide a small `capsEnv` test double in `tool_test.go` implementing `Environment` with configurable caps.

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/sandbox/... -count=1`

Expected: FAIL

- [ ] **Step 3: Implement registry + file tools**

`read_file` description must mention reading workspace files.  
`write_file` creates parent directories via FS write helper.

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/sandbox/... -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/sandbox/
git commit -m "$(cat <<'EOF'
feat(sandbox): add tool registry and read_file/write_file

EOF
)"
```

---

### Task 4: JSON config loader (three layers)

**Files:**
- Create: `controlplane/internal/sandboxconfig/config.go`
- Create: `controlplane/internal/sandboxconfig/config_test.go`

**Interfaces:**
- Consumes: `sandbox.OpenOptions` types
- Produces:
  - `func Load(configDir string, data []byte) (sandbox.OpenOptions, error)`
  - Relative `dockerfile` and mount `source` resolved against `configDir`
  - Validate: kind required; docker requires scope; image xor dockerfile; session scope does **not** require SessionID in JSON (caller fills SessionID when opening)

JSON shape from spec (global + `docker` env/engine fields).

- [ ] **Step 1: Write the failing test**

```go
func TestLoadDockerDockerfile(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
	  "kind": "docker",
	  "workspaceRoot": "/workspace",
	  "docker": {
	    "containerScope": "session",
	    "idleTTLSeconds": 600,
	    "runtime": "auto",
	    "dockerfile": "./Dockerfile",
	    "mounts": [{"source": "./data", "target": "/workspace"}]
	  }
	}`)
	opts, err := sandboxconfig.Load(dir, data)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Kind != "docker" || opts.WorkspaceRoot != "/workspace" {
		t.Fatalf("%+v", opts)
	}
	if opts.Docker.Dockerfile != filepath.Join(dir, "Dockerfile") {
		t.Fatalf("dockerfile = %q", opts.Docker.Dockerfile)
	}
	if opts.Docker.IdleTTL != 600*time.Second {
		t.Fatalf("ttl = %v", opts.Docker.IdleTTL)
	}
	if opts.Docker.Mounts[0].Source != filepath.Join(dir, "data") {
		t.Fatalf("mount = %+v", opts.Docker.Mounts[0])
	}
}

func TestLoadRejectsImageAndDockerfile(t *testing.T) {
	_, err := sandboxconfig.Load(t.TempDir(), []byte(`{
	  "kind":"docker","workspaceRoot":"/w",
	  "docker":{"containerScope":"shared","image":"alpine","dockerfile":"./Dockerfile"}
	}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadLocal(t *testing.T) {
	opts, err := sandboxconfig.Load(t.TempDir(), []byte(`{"kind":"local","workspaceRoot":"/tmp/ws"}`))
	if err != nil {
		t.Fatal(err)
	}
	if opts.Docker != nil {
		t.Fatal("docker should be nil")
	}
}
```

Fix the IdleTTL assertion to only expect `600 * time.Second` (remove the confusing comment branch in the real file).

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/sandboxconfig/ -count=1`

Expected: FAIL

- [ ] **Step 3: Implement Load**

Map `containerScope` → `Scope.Kind`; default scope `session` if omitted for docker; default `idleTTLSeconds` omitted → leave `IdleTTL=0` (manager applies 10m later).

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/sandboxconfig/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/sandboxconfig/
git commit -m "$(cat <<'EOF'
feat(sandboxconfig): load layered JSON into OpenOptions

EOF
)"
```

---

### Task 5: Exec-backed FS

**Files:**
- Create: `controlplane/internal/sandbox/execfs/fs.go`
- Create: `controlplane/internal/sandbox/execfs/fs_test.go`

**Interfaces:**
- Consumes: `sandbox.Executor`, workspace root string (in-env path)
- Produces: `func New(exec sandbox.Executor, workspaceRoot string) sandbox.FS`
- `ReadFile`: run `cat` / `dd` via executor with path under root (validate with string prefix jail on cleaned paths — no host EvalSymlinks; reject `..` segments)
- `WriteFile`: `mkdir -p` parent + atomic temp write via stdin + `mv` (Hermes-style), using `ExecRequest.Stdin`

- [ ] **Step 1: Write the failing test**

```go
type memExec struct {
	files map[string][]byte
}

func (m *memExec) Run(ctx context.Context, req sandbox.ExecRequest) (sandbox.ExecResult, error) {
	// Interpret a tiny command vocabulary used by execfs in tests:
	// or simpler: record last req and return scripted results per test case.
}
```

Prefer a **scripted fake** that maps exact argv prefixes:

```go
type fakeExec struct {
	run func(sandbox.ExecRequest) (sandbox.ExecResult, error)
}

func TestExecFSReadWrite(t *testing.T) {
	store := map[string][]byte{}
	exec := &fakeExec{run: func(req sandbox.ExecRequest) (sandbox.ExecResult, error) {
		// Implement enough to satisfy whatever commands execfs emits.
		// Keep commands stable: ["sh","-c", script] with documented scripts.
		return handle(store, req)
	}}
	fsys := execfs.New(exec, "/workspace")
	ctx := context.Background()
	if err := fsys.WriteFile(ctx, "f.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	b, err := fsys.ReadFile(ctx, "f.txt")
	if err != nil || string(b) != "x" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := fsys.ReadFile(ctx, "../etc/passwd"); err == nil {
		t.Fatal("expected jail error")
	}
}
```

Document the exact shell scripts `execfs` uses in a comment at the top of `fs.go` so the fake can match them.

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/sandbox/execfs/ -count=1`

Expected: FAIL

- [ ] **Step 3: Implement execfs**

Path jail: clean path, reject `..`, ensure joined path has prefix `workspaceRoot + "/"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/sandbox/execfs/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/sandbox/execfs/
git commit -m "$(cat <<'EOF'
feat(sandbox): add exec-backed filesystem adapter

EOF
)"
```

---

### Task 6: Container manager (fake CLI)

**Files:**
- Create: `controlplane/internal/sandbox/container/manager.go`
- Create: `controlplane/internal/sandbox/container/manager_test.go`

**Interfaces:**
- Consumes: none from docker package (keep CLI abstract)
- Produces:
  - `type Runner interface { LookPath(name string) (string, error); CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) }`
  - `type Manager struct` with `NewManager(runner Runner, opts ManagerOptions)`
  - `ManagerOptions` includes default idle TTL (10m), label prefix `agent-fabric.sandbox`
  - `func (m *Manager) ResolveBinary(binPath, runtime string) (string, error)` — podman-then-docker for auto
  - `func (m *Manager) Acquire(ctx context.Context, key string, spec ContainerSpec) (containerID string, err error)`
  - `ContainerSpec`: Image, Mounts, WorkspaceRoot (workdir), labels including scope key
  - Reuse: `ps -q -f label=...` then create `run -d --workdir ... image sleep infinity`
  - `Touch(key)` updates last activity; `Reap(ctx)` removes idle containers with zero busy refs
  - `Release` / refcount: `Acquire` increments; `Done(key)` decrements

- [ ] **Step 1: Write the failing test**

```go
func TestAcquireReusesSameKey(t *testing.T) {
	r := newFakeRunner() // in-memory map of running containers by label key
	m := container.NewManager(r, container.ManagerOptions{IdleTTL: time.Minute})
	id1, err := m.Acquire(context.Background(), "session:s1", container.ContainerSpec{
		Image: "alpine:3.20", WorkspaceRoot: "/workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := m.Acquire(context.Background(), "session:s1", container.ContainerSpec{
		Image: "alpine:3.20", WorkspaceRoot: "/workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("%s vs %s", id1, id2)
	}
	id3, err := m.Acquire(context.Background(), "session:s2", container.ContainerSpec{
		Image: "alpine:3.20", WorkspaceRoot: "/workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id3 == id1 {
		t.Fatal("expected different container for other session")
	}
}

func TestResolveBinaryPrefersPodman(t *testing.T) {
	r := &fakeRunner{paths: map[string]string{"podman": "/usr/bin/podman", "docker": "/usr/bin/docker"}}
	m := container.NewManager(r, container.ManagerOptions{})
	bin, err := m.ResolveBinary("", "auto")
	if err != nil || bin != "/usr/bin/podman" {
		t.Fatalf("%q %v", bin, err)
	}
}
```

`fakeRunner` must simulate `run`, `ps`, `rm`/`stop` enough for reuse logic.

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/sandbox/container/ -count=1`

Expected: FAIL

- [ ] **Step 3: Implement manager**

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/sandbox/container/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/sandbox/container/
git commit -m "$(cat <<'EOF'
feat(sandbox): add scoped container manager with idle TTL hooks

EOF
)"
```

---

### Task 7: Docker image resolve (pull tag vs Dockerfile build) + Open(docker)

**Files:**
- Create: `controlplane/internal/sandbox/docker/runtime.go`
- Create: `controlplane/internal/sandbox/docker/runtime_test.go`
- Create: `controlplane/internal/sandbox/docker/env.go`
- Create: `controlplane/internal/sandbox/docker/exec.go`
- Modify: `controlplane/internal/sandbox/env.go` — implement `kind=docker` in `Open`
- Create: `controlplane/internal/sandbox/docker/env_test.go` (unit with fake runner; no real containers)

**Interfaces:**
- Consumes: `container.Manager`, `execfs`, `sandbox.OpenOptions.Docker`
- Produces:
  - `func ResolveImage(ctx, runner, bin, opts) (imageRef string, err error)` — if Dockerfile: `build -t agent-fabric-sandbox:<hash> -f Dockerfile context`; hash = sha256 of Dockerfile bytes (+ optional context file list later); reuse tag if already present (`images -q`)
  - Docker `Environment`: `Exec()` runs `bin exec -w workspaceRoot containerID ...`; `FS()` returns `execfs.New(...)`
  - `Open` for docker: validate options, resolve binary, resolve image, acquire container with scope key `shared` or `session:<id>`, return env that `Close`s by `Done` on manager (does not force-remove container — idle reaper owns removal)

- [ ] **Step 1: Write the failing test**

```go
func TestResolveImageBuildsDockerfile(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	os.WriteFile(df, []byte("FROM alpine:3.20\n"), 0o644)
	r := newRecordingRunner() // succeeds build, returns image id queries
	ref, err := docker.ResolveImage(context.Background(), r, "/usr/bin/podman", sandbox.DockerOptions{
		Dockerfile: df, BuildContext: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "agent-fabric-sandbox:") {
		t.Fatalf("ref = %q", ref)
	}
	if !r.sawSubstr("build") {
		t.Fatal("expected build")
	}
}

func TestOpenDockerRequiresSessionID(t *testing.T) {
	_, err := sandbox.Open(context.Background(), sandbox.OpenOptions{
		Kind: "docker", WorkspaceRoot: "/workspace",
		Docker: &sandbox.DockerOptions{
			Scope: sandbox.Scope{Kind: sandbox.ScopeSession},
			Image: "alpine:3.20",
		},
	})
	if err == nil {
		t.Fatal("expected session id error")
	}
}
```

For `Open` success path in unit tests, inject a test hook **or** keep `Open` using a package-level runner only in tests via `docker.SetRunnerForTest` — prefer constructor:

`docker.Open(ctx, opts, manager)` called from `sandbox.Open` with default OS runner.

Use `docker.OpenForTest(ctx, opts, manager)` from tests with fake manager already holding a container — simplest path: test `docker.NewEnv(containerID, bin, root, exec)` directly for FS/Exec wiring; leave full `Open` covered by integration test in Task 8.

Still implement `sandbox.Open` docker branch for real use.

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/sandbox/docker/ -count=1`

Expected: FAIL

- [ ] **Step 3: Implement docker package + wire Open**

- [ ] **Step 4: Run unit tests**

Run: `go -C controlplane test ./internal/sandbox/... ./internal/sandboxconfig/... -count=1`

Expected: PASS (no integration tag)

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/sandbox/
git commit -m "$(cat <<'EOF'
feat(sandbox): wire docker/podman environments with image build

EOF
)"
```

---

### Task 8: Integration test (real podman/docker)

**Files:**
- Create: `controlplane/internal/sandbox/docker/integration_test.go`

**Interfaces:**
- Consumes: full `sandbox.Open` + file tools
- Produces: none

- [ ] **Step 1: Write integration test**

```go
//go:build integration

package docker_test

func TestDockerFileToolsIntegration(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		if _, err2 := exec.LookPath("docker"); err2 != nil {
			t.Skip("no podman/docker")
		}
	}
	ctx := context.Background()
	env, err := sandbox.Open(ctx, sandbox.OpenOptions{
		Kind: "docker", WorkspaceRoot: "/workspace",
		Docker: &sandbox.DockerOptions{
			Scope:   sandbox.Scope{Kind: sandbox.ScopeSession, SessionID: "itest-1"},
			Runtime: "auto",
			Image:   "alpine:3.20",
			IdleTTL: time.Minute,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close(ctx)

	reg := sandbox.NewRegistry()
	for _, tool := range file.Tools() {
		reg.Register(tool)
	}
	if _, err := reg.Call(ctx, env, "write_file", json.RawMessage(`{"path":"x.txt","content":"pod"}`)); err != nil {
		t.Fatal(err)
	}
	out, err := reg.Call(ctx, env, "read_file", json.RawMessage(`{"path":"x.txt"}`))
	if err != nil || !strings.Contains(out, "pod") {
		t.Fatalf("%s %v", out, err)
	}
}
```

- [ ] **Step 2: Ensure unit tests still ignore it**

Run: `go -C controlplane test ./internal/sandbox/... -count=1`

Expected: PASS (integration file excluded)

- [ ] **Step 3: Run integration when runtime available**

Run: `go -C controlplane test -tags=integration ./internal/sandbox/docker/ -count=1 -v`

Expected: PASS or Skip

- [ ] **Step 4: Commit**

```bash
git add controlplane/internal/sandbox/docker/integration_test.go
git commit -m "$(cat <<'EOF'
test(sandbox): add docker/podman file tools integration test

EOF
)"
```

---

### Task 9: README note for sandbox POC

**Files:**
- Modify: `README.md` (short subsection under control plane / development)

- [ ] **Step 1: Add docs**

Document:
- Package location and that it is not wired to ACP yet
- Example local vs docker JSON for `sandboxconfig`
- Unit test command
- Integration: `go -C controlplane test -tags=integration ./internal/sandbox/docker/`

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "$(cat <<'EOF'
docs: note sandbox FS tools POC and how to test it

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Standalone package, no ACP | Tasks 1–8 (import constraint) |
| Capabilities Environment | Task 2 |
| Local native FS + jail | Tasks 1–2 |
| read_file / write_file + OpenAI schemas | Task 3 |
| Config three layers, package unaware | Task 4 |
| Exec-backed FS | Task 5 |
| Container scope shared/session + idle TTL | Task 6 |
| Podman-first auto runtime | Task 6–7 |
| image xor dockerfile build | Task 4 + 7 |
| workspaceRoot env-relative; mounts engine-only | Task 4 + 7 |
| Hot-reload ready (no watcher) | Task 4 Load pure; Close/re-Open semantics in Task 7 |
| Integration docker/podman | Task 8 |
| README | Task 9 |

## Placeholder / consistency notes

- Idle TTL JSON: `idleTTLSeconds: 600` → `600 * time.Second` in loader; manager default applies only when `IdleTTL == 0`.
- OpenAI definition shape: nested `type` + `function.{name,description,parameters}` (Chat Completions), not the flattened Responses API form — document in `tool.go` comment.
- `SessionID` is supplied at `Open` time by the caller, not from JSON.
