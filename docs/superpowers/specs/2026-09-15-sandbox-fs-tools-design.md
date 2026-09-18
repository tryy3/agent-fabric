# Sandbox environments + file tools (V1 POC)

**Date:** 2026-09-15  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Inspiration:** NousResearch Hermes Agent terminal backends / file tools (execute-backed remote FS; config-selected backend)

## Problem

Agent Fabric’s control plane has no sandbox or tool runtime yet. Architecture already defines execution origins (`sandbox` / `mcp` / `client`) and defers Docker, but we need a concrete, testable foundation for:

- Running file operations in an isolated environment (local jail vs container)
- Exposing tools to models in an OpenAI-compatible shape (`name` / `description` / `parameters`)
- Supporting Docker **and** Podman from day one
- Keeping the core package free of process-wide config coupling so hot-reload can land later

## Goals

- Ship a **standalone Go package** under the control plane module that can be unit/integration tested without ACP, catalog, or the agent loop.
- Capability-shaped **`Environment`**: optional `FS` + optional `Executor` (approach C).
- V1 agent-facing tools: **`read_file`** and **`write_file`**, gated by capabilities.
- **Local** backend: native OS I/O under a rooted workspace; reject path escapes (including symlink escape).
- **Docker/Podman** backend: managed long-lived containers; file ops via an **exec-backed FS** over the same executor.
- **Container manager** with pluggable scope: at least `shared` and `session` (default `session`).
- Package accepts all behavior via **`Open` / options structs** — no config file reads inside the package.
- Separate **JSON config loader** (thin) that maps three setting layers → `OpenOptions`.
- Tool definitions carry OpenAI-shaped metadata so a future harness can put them on Chat Completions `tools`.
- Design for **hot-reload**: new opens use new options; live environments keep the options they were opened with until `Close`.
- Support **`image` xor `dockerfile`** for container images (build from Dockerfile beside server config when set).

## Non-goals (V1)

- Wiring into ACP `session/prompt`, provider tool-calling, or Flutter UI
- MCP host tools (may stay host-side later; security TBD)
- SSH / Modal / Daytona / S3 / in-memory backends (interfaces should not block them)
- Agent-facing `terminal` / `execute` tool (Executor exists for Docker FS and future tools)
- Real config file watcher / hot-reload implementation
- Approval UX for dangerous writes
- Mapping Docker to ACP `fs/*` (explicitly forbidden per architecture)

## Approach

**Chosen: capability environment (C)**

| Approach | Summary | Why not / why |
| --- | --- | --- |
| A — Hermes execute-only | Everything is shell | Fine for remote; fights Go/`io/fs`; blocks S3/memory later |
| B — FS-only | Exec bolted on later | Docker file tools still need exec-backed FS |
| **C — capabilities** | `Environment` offers `FS` and/or `Exec` | Local native FS; Docker exec-backed FS; tools declare requirements |

Hermes insight we keep: **one configured backend per open**; model does not choose local vs docker per call. File tools share the same environment instance (cwd / root consistency).

Hermes insight we generalize: container ownership is a **scope key**, not a single process-global container.

## Architecture

```text
                    ┌─────────────────────────────┐
                    │ sandboxconfig (JSON loader) │  ← outside core package
                    │  global / env / engine      │
                    └──────────────┬──────────────┘
                                   │ OpenOptions
                                   ▼
┌──────────────────────────────────────────────────────────────┐
│ sandbox (standalone package)                                 │
│                                                              │
│  Open(ctx, OpenOptions) → Environment                        │
│       │                                                      │
│       ├─ local:  native FS under WorkspaceRoot               │
│       └─ docker: ContainerManager → Executor → exec FS       │
│                                                              │
│  tools/file: read_file, write_file                           │
│       Schema (OpenAI) + Run(ctx, env, args) → JSON string    │
│                                                              │
│  Registry: filter tools by env.Caps()                        │
└──────────────────────────────────────────────────────────────┘

Future harness (out of scope):
  Registry.Definitions() → provider tools[]
  Run → ACP session/update tool_call_update
```

### Package placement

Prefer `controlplane/internal/sandbox/` until the API stabilizes (same module `github.com/tryy3/agent-fabric`). Optional thin `sandboxconfig` sibling package or `sandbox/config` for JSON decode only.

Suggested layout:

```text
controlplane/internal/sandbox/
  env.go              # Environment, Capabilities, Open, OpenOptions
  fs.go               # FS contract + io/fs bridge helpers
  exec.go             # Executor, ExecRequest/Result
  tool.go             # Tool, Registry
  path.go             # root jail / resolve helpers (shared)
  local/
    env.go
    fs.go
  docker/
    env.go
    fs.go             # exec-backed FS
    runtime.go        # docker|podman CLI discovery
    build.go          # dockerfile → image
  container/
    manager.go        # acquire/release by scope, idle TTL
  tools/file/
    read.go
    write.go
    schemas.go
controlplane/internal/sandboxconfig/   # optional separate
  config.go           # Load([]byte) → OpenOptions
```

## Core contracts

### Capabilities and Environment

```go
type Capabilities struct {
    FS   bool
    Exec bool
}

type Environment interface {
    ID() string
    Caps() Capabilities
    FS() (FS, bool)
    Exec() (Executor, bool)
    Close(ctx context.Context) error
}
```

Local V1: `FS=true`, `Exec=true` (even if no terminal tool yet — useful for tests / future).  
Docker V1: `FS=true`, `Exec=true`.  
Future S3: `FS=true`, `Exec=false` → file tools only.

### FS

Own write-capable contract; stay compatible with `io/fs` for the read side so third-party backends (memfs, S3, gophercloud, etc.) can adapt later.

Minimum V1 methods:

- `Open(name) (io/fs.File, error)` or embed/`AsIOFS() io/fs.FS`
- `ReadFile(ctx, path) ([]byte, error)`
- `WriteFile(ctx, path, data []byte, perm fs.FileMode) error`
- `Stat(ctx, path) (fs.FileInfo, error)`
- `ReadDir(ctx, path) ([]fs.DirEntry, error)` as needed for tests

**Path rules:**

- `WorkspaceRoot` is always interpreted **inside the environment** (host path for local; container path for docker).
- Tool args use paths relative to `WorkspaceRoot` (or absolute paths that still resolve under that root after cleaning).
- Reject escapes: `..`, absolute paths outside root, symlink resolution that leaves root (`EvalSymlinks` / equivalent for local; careful command construction for docker).

### Executor

```go
type ExecRequest struct {
    Cmd     []string // preferred: argv form for local; docker may join carefully
    WorkDir string   // relative to WorkspaceRoot or absolute under root
    Stdin   []byte
    Timeout time.Duration
}

type ExecResult struct {
    ExitCode int
    Stdout   []byte
    Stderr   []byte
}

type Executor interface {
    Run(ctx context.Context, req ExecRequest) (ExecResult, error)
}
```

Docker FS uses `Run` with shell-safe scripts for read/write (temp + rename for atomic-ish writes), similar to Hermes’ portable file ops — but behind our `FS` interface so local does not shell out.

### OpenOptions (package input — no JSON here)

```go
type ScopeKind string // "shared" | "session"  (+ later "project")

type Scope struct {
    Kind      ScopeKind
    SessionID string // required when Kind == session
}

type Mount struct {
    Source   string // host path (absolute after loader resolve)
    Target   string // path inside container
    ReadOnly bool
}

type DockerOptions struct {
    // Environment-layer (lifecycle / isolation)
    Scope   Scope
    IdleTTL time.Duration // idle reclaim; 0 → default 10m

    // Engine-layer (how Docker/Podman is invoked)
    Runtime      string // "auto" | "docker" | "podman"
    BinPath      string // optional explicit binary
    Image        string // xor Dockerfile
    Dockerfile   string // path to Dockerfile (absolute after loader)
    BuildContext string // default: dir of Dockerfile
    Mounts       []Mount
}

type OpenOptions struct {
    // Global
    Kind          string // "local" | "docker"
    WorkspaceRoot string

    // Kind-specific (local has no extra fields in V1)
    Docker *DockerOptions // required when Kind == docker
}
```

Validation: `kind=docker` requires `Docker`; exactly one of `Image` or `Dockerfile`; `session` scope requires `SessionID`.

## Tools (model-facing)

### How the model learns about tools

**OpenAI Chat Completions / Responses:** the harness sends a `tools` array. Each function tool needs roughly:

- `type: "function"`
- `name`
- `description` (when to use it)
- `parameters` (JSON Schema object)

That is what steers “user asked to read a file → call `read_file`.”

**ACP:** clients learn about **invocations** via `session/update` / `tool_call_update` (title, kind, status, content). ACP does **not** define the schema sent to the model. V1 package does not import ACP; a future harness maps `Run` results into ACP updates.

### Tool interface

```go
type Tool struct {
    Name        string
    Description string
    Parameters  json.RawMessage // JSON Schema
    Requires    Capabilities    // e.g. FS: true
    Run         func(ctx context.Context, env Environment, args json.RawMessage) (string, error)
}
```

`Registry.Definitions(env)` returns only tools whose `Requires` ⊆ `env.Caps()`.  
Handlers return **JSON strings** (success or `{"error":"..."}`) for model consumption.

### V1 tools

| Name | Behavior |
| --- | --- |
| `read_file` | Read text file under workspace; optional offset/limit later — V1 can be full-file with size cap |
| `write_file` | Create/overwrite file under workspace; create parents as needed |

Descriptions should tell the model to prefer these over shell `cat`/`echo` once a terminal tool exists.

## Container manager (docker)

Responsibilities:

1. Resolve runtime binary: `BinPath` if set → else if `Runtime` is `docker`/`podman` look up that name on `PATH` → else **auto**: first existing of `podman`, then `docker` (Podman-first for this project’s hosts).
2. Resolve image: if `Dockerfile` set, `build` with tag derived from content hash (rebuild when hash changes); else use `Image`.
3. **Acquire(scopeKey)**: if a running container exists for that key, reuse; else `run -d` (long-lived, e.g. `sleep infinity`), apply mounts, set workdir semantics via `WorkspaceRoot`.
4. **Idle TTL**: track last activity; background reaper stops/removes containers idle longer than `IdleTTL` (only when no in-flight ops — V1 can use a simple refcount or “busy” flag). If `IdleTTL == 0`, use default **10 minutes**.
5. Scope keys: `shared` → fixed key; `session` → `session:<SessionID>`.

Default scope for docker config: **`session`**.

This is **not** Hermes’ single process-wide container; shared mode is available when configured.

## Config layer (three setting types)

The JSON file is **not** read by `sandbox`. A loader owns decode + path resolution (Dockerfile / mount sources relative to **config file directory**).

### Layers

| Layer | Examples | Notes |
| --- | --- | --- |
| **Global** | `workspaceRoot`, `kind` | Same meaning for every environment kind |
| **Environment** | docker: `containerScope`, `idleTTLSeconds` | Lifecycle/isolation of the env instance; omit for local |
| **Engine** | docker: `runtime`, `binPath`, `image` / `dockerfile`, `mounts`, `buildContext` | How the docker/podman engine is invoked |

### Example JSON

```json
{
  "kind": "docker",
  "workspaceRoot": "/workspace",
  "docker": {
    "containerScope": "session",
    "idleTTLSeconds": 600,
    "runtime": "auto",
    "dockerfile": "./Dockerfile",
    "mounts": [
      {
        "source": "./data",
        "target": "/workspace",
        "readOnly": false
      }
    ]
  }
}
```

Local example:

```json
{
  "kind": "local",
  "workspaceRoot": "/home/user/project"
}
```

### Hot-reload readiness (V1 behavior, later watcher)

- Loader is a pure function: `Load(path or bytes) (OpenOptions, error)`.
- Callers may reload and call `Open` again with new options.
- Existing `Environment` instances **do not** mutate mid-flight when config changes.
- Idle TTL / image / mounts changes apply to **newly acquired** containers only (document clearly).

## Security (V1)

### Local

- All paths cleaned and constrained under `WorkspaceRoot`.
- Symlinks that resolve outside root → error.
- No following `..` out of jail.

### Docker

- Commands run inside the container; host FS only via explicit mounts.
- Prefer argv / carefully quoted scripts; avoid interpolating unsanitized paths into shell without escaping.
- V1 hardening stretch (nice-to-have, not blockers): `--cap-drop ALL`, `--security-opt no-new-privileges`, PID limits — document as follow-ups if not in first cut.
- Secrets: do not forward host env wholesale into containers in V1.

### Host-side MCP / tools

Out of scope; future work must revisit isolation when host MCP coexists with sandboxed file tools.

## Testing

| Layer | What |
| --- | --- |
| Unit | Path jail escapes; tool registry capability filtering; config Load validation (`image` xor `dockerfile`) |
| Unit | Exec-backed FS against a **fake Executor** |
| Integration (`//go:build integration`) | Real podman or docker: acquire container, read/write file, session vs shared scope keys |
| Skip | Integration tests skip cleanly when no runtime binary is available |

## Alignment with architecture

- Sandbox file tools are origin **`sandbox`**, never ACP client `fs/*`.
- Docker/Podman isolation is a property of the environment definition / open options, not of the Flutter client.
- Future session pin can store sandbox identity; this POC only needs `Scope.SessionID` when opening.

## Success criteria

1. Tests open a **local** env, `write_file` + `read_file` round-trip under root; escape attempts fail.
2. With Podman or Docker available, integration test opens **docker** env (image or Dockerfile), round-trips file tools inside `workspaceRoot`.
3. `Registry.Definitions(env)` returns OpenAI-shaped schemas for both tools when `FS` is available.
4. Two session scopes get two containers; `shared` reuses one (integration or manager unit with fake runtime).
5. No import cycle from `sandbox` → `agent` / `transport` / ACP SDK.

## Open follow-ups (post-V1)

- Wire definitions into `ChatStreamer` tool-calling + ACP tool updates
- Terminal tool on `Executor`
- Config watcher hot-reload
- Project-scoped containers
- Stronger container security flags
- Host MCP isolation policy
