# Dynamic Agents: Providers Catalog + Agent Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Catalog HTTP API (JSON-backed providers + agents), wire ACP `session/new` / `set_config_option` to those definitions with mid-session model switching, and give Flutter a Settings (Providers | Agents) shell plus chat agent/model pickers.

**Architecture:** Control plane serves Catalog REST and ACP WebSocket on one port. Providers/agents persist under `{dataDir}/*.json`. `session/new` reads `_meta.agentId`, pins agent+provider+model list, returns ACP `configOptions` (`category: "model"`). Prompts resolve an `openai_compatible` streamer from the pin and pass the session’s current model. Flutter sidebar: Chat · Settings; Settings tabs Providers | Agents.

**Tech Stack:** Go 1.22+, stdlib `encoding/json` + `net/http`, existing `acp-go-sdk`, Flutter/`acpd`. No new Go dependencies.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-09-12-dynamic-agents-providers-settings-design.md`
- Module root: `controlplane/` (`github.com/tryy3/agent-fabric`); tests: `go -C controlplane test ./...`
- Flutter tests: `cd client && flutter test`
- No `OPENAI_*` required at startup; empty catalog is valid
- Stdlib JSON only for persistence; atomic write (temp + rename)
- API keys plaintext in JSON (local-dev)
- Provider type `openai_compatible` only; registry must accept future types
- ACP agent id via `_meta.agentId`; model via `configOptions` id `"model"`
- CI offline: fakes / `httptest`; never call real Unsloth in tests
- TDD: failing test → implement → pass → commit per task
- Placeholders for MCP/tools/sandbox/memory: store empty / UI disabled only

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/catalog/types.go` | Provider, ModelInfo, Agent structs + constants |
| `controlplane/internal/catalog/store.go` | JSON file store, CRUD, validation, refresh lock |
| `controlplane/internal/catalog/store_test.go` | Store unit tests |
| `controlplane/internal/catalog/refresh.go` | `GET {baseUrl}/models` → cache |
| `controlplane/internal/catalog/refresh_test.go` | `httptest` refresh tests |
| `controlplane/internal/catalog/http.go` | `/v1/providers`, `/v1/agents` handlers |
| `controlplane/internal/catalog/http_test.go` | HTTP handler tests |
| `controlplane/internal/provider/streamer.go` | `ChatStreamer` gains `model string` |
| `controlplane/internal/provider/openai.go` | Model per `StreamChat` call (not constructor-only) |
| `controlplane/internal/provider/registry.go` | `NewStreamer(type, baseURL, apiKey, client)` |
| `controlplane/internal/runtime/definition.go` | Expand pin fields used by sessions |
| `controlplane/internal/runtime/session.go` | Pin snapshot + `CurrentModel` + `SetModel` |
| `controlplane/internal/agent/agent.go` | Catalog-aware NewSession / Prompt |
| `controlplane/internal/agent/config_options.go` | Build/apply model `configOptions` |
| `controlplane/internal/agent/stubs.go` | Remove `SetSessionConfigOption` stub |
| `controlplane/internal/server/server.go` | Mount catalog + pass catalog into WS |
| `controlplane/internal/transport/ws/handler.go` | Pass `*catalog.Store` into `agent.New` |
| `controlplane/cmd/controlplane/main.go` | `-data-dir`, no env OpenAI load |
| `controlplane/cmd/acp-cli/main.go` | Pass `_meta.agentId` (flag) |
| `client/lib/catalog/catalog_client.dart` | HTTP catalog client |
| `client/lib/catalog/models.dart` | Dart DTOs |
| `client/lib/app_shell.dart` | Sidebar Chat / Settings |
| `client/lib/settings/settings_page.dart` | Tabs Providers \| Agents |
| `client/lib/settings/providers_tab.dart` | Provider CRUD + refresh |
| `client/lib/settings/agents_tab.dart` | Agent CRUD + placeholders |
| `client/lib/acp/agent_connection.dart` | `agentId` on new session; model config API |
| `client/lib/chat/chat_controller.dart` | Agent/model selection |
| `client/lib/chat/chat_screen.dart` | Pickers in AppBar/header |
| `client/lib/main.dart` | Host `AppShell` |
| `README.md` | Catalog + settings flow; drop required env |

---

### Task 1: Catalog types + provider JSON store

**Files:**
- Create: `controlplane/internal/catalog/types.go`
- Create: `controlplane/internal/catalog/store.go`
- Create: `controlplane/internal/catalog/store_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  - `const TypeOpenAICompatible = "openai_compatible"`
  - `type ModelInfo struct { ID, Name string }`
  - `type Provider struct { ID, Name, Type, BaseURL, APIKey string; Models []ModelInfo; ModelsUpdatedAt *time.Time; CreatedAt, UpdatedAt time.Time }`
  - `func Open(dataDir string) (*Store, error)` — creates dir; loads or inits empty `providers.json` / `agents.json`
  - `func (s *Store) ListProviders() []Provider`
  - `func (s *Store) GetProvider(id string) (Provider, bool)`
  - `func (s *Store) CreateProvider(name, typ, baseURL, apiKey string) (Provider, error)`
  - `func (s *Store) UpdateProvider(id string, name, baseURL, apiKey *string) (Provider, error)`
  - `func (s *Store) DeleteProvider(id string) error` — for now allow delete (agent refs added in Task 2)
  - `func (s *Store) ReplaceProviderModels(id string, models []ModelInfo, updatedAt time.Time) (Provider, error)`

- [ ] **Step 1: Write the failing test**

Create `controlplane/internal/catalog/store_test.go`:

```go
package catalog_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestProviderCRUDRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := catalog.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	p, err := store.CreateProvider("Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:8888/v1", "sk-test")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if p.ID == "" || p.Type != catalog.TypeOpenAICompatible {
		t.Fatalf("unexpected provider: %+v", p)
	}

	got, ok := store.GetProvider(p.ID)
	if !ok || got.APIKey != "sk-test" {
		t.Fatalf("GetProvider = %+v ok=%v", got, ok)
	}

	name := "Renamed"
	got, err = store.UpdateProvider(p.ID, &name, nil, nil)
	if err != nil || got.Name != "Renamed" {
		t.Fatalf("UpdateProvider: %+v err=%v", got, err)
	}

	now := time.Now().UTC()
	got, err = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, now)
	if err != nil || len(got.Models) != 1 || got.ModelsUpdatedAt == nil {
		t.Fatalf("ReplaceProviderModels: %+v err=%v", got, err)
	}

	// Re-open from disk
	store2, err := catalog.Open(dir)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	list := store2.ListProviders()
	if len(list) != 1 || list[0].Models[0].ID != "m1" {
		t.Fatalf("persisted list = %+v", list)
	}
	if _, err := filepath.Abs(filepath.Join(dir, "providers.json")); err != nil {
		t.Fatal(err)
	}

	if err := store2.DeleteProvider(p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if len(store2.ListProviders()) != 0 {
		t.Fatal("expected empty after delete")
	}
}

func TestCreateProviderRejectsEmptyName(t *testing.T) {
	store, err := catalog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateProvider("", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/catalog/ -count=1`

Expected: FAIL (package not found / undefined)

- [ ] **Step 3: Write minimal implementation**

`types.go`:

```go
package catalog

import "time"

const TypeOpenAICompatible = "openai_compatible"

type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Provider struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Type             string      `json:"type"`
	BaseURL          string      `json:"baseUrl"`
	APIKey           string      `json:"apiKey"`
	Models           []ModelInfo `json:"models"`
	ModelsUpdatedAt  *time.Time  `json:"modelsUpdatedAt,omitempty"`
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Version      int       `json:"version"`
	ProviderID   string    `json:"providerId"`
	DefaultModel string    `json:"defaultModel"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
```

`store.go`: mutex; load/save `providers.json` / `agents.json` as `{"providers":[...]}` / `{"agents":[...]}`; `Open` mkdirall; ids `prov_` / `agent_` + hex; trim trailing `/` on BaseURL; `CreateProvider` rejects empty name/baseURL/apiKey and unknown type; atomic write via `os.CreateTemp` in same dir + `os.Rename`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/catalog/ -count=1`

Expected: PASS (agent methods may be stubs returning empty until Task 2)

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/types.go controlplane/internal/catalog/store.go controlplane/internal/catalog/store_test.go
git commit -m "feat(catalog): add provider JSON store"
```

---

### Task 2: Agent store + cross-validation

**Files:**
- Modify: `controlplane/internal/catalog/store.go`
- Modify: `controlplane/internal/catalog/store_test.go`

**Interfaces:**
- Consumes: Provider store from Task 1
- Produces:
  - `func (s *Store) ListAgents() []Agent`
  - `func (s *Store) GetAgent(id string) (Agent, bool)`
  - `func (s *Store) CreateAgent(name, description, providerID, defaultModel string) (Agent, error)`
  - `func (s *Store) UpdateAgent(id string, name, description, providerID, defaultModel *string) (Agent, error)` — bumps `Version`
  - `func (s *Store) DeleteAgent(id string) error`
  - `DeleteProvider` returns error containing `conflict` / use `var ErrProviderInUse = errors.New("provider in use")` when any agent references it
  - Create/Update agent: provider must exist; `defaultModel` must be in that provider’s `Models`

- [ ] **Step 1: Write the failing test**

Append to `store_test.go`:

```go
func TestCreateAgentRequiresCachedModel(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, err := store.CreateAgent("A", "", p.ID, "missing")
	if err == nil {
		t.Fatal("expected error when model not cached")
	}
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAgent("A", "desc", p.ID, "m1")
	if err != nil || a.Version != 1 || a.DefaultModel != "m1" {
		t.Fatalf("CreateAgent: %+v err=%v", a, err)
	}
	name := "B"
	a2, err := store.UpdateAgent(a.ID, &name, nil, nil, nil)
	if err != nil || a2.Version != 2 || a2.Name != "B" {
		t.Fatalf("UpdateAgent: %+v err=%v", a2, err)
	}
}

func TestDeleteProviderConflictWhenReferenced(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	_, _ = store.CreateAgent("A", "", p.ID, "m1")
	err := store.DeleteProvider(p.ID)
	if err == nil || !errors.Is(err, catalog.ErrProviderInUse) {
		t.Fatalf("err = %v", err)
	}
}
```

Add `"errors"` import.

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/catalog/ -count=1 -run 'TestCreateAgent|TestDeleteProviderConflict'`

Expected: FAIL

- [ ] **Step 3: Implement agent CRUD + `ErrProviderInUse`**

- [ ] **Step 4: Run tests — expect PASS**

Run: `go -C controlplane test ./internal/catalog/ -count=1`

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/
git commit -m "feat(catalog): add agents with provider/model validation"
```

---

### Task 3: Refresh provider models

**Files:**
- Create: `controlplane/internal/catalog/refresh.go`
- Create: `controlplane/internal/catalog/refresh_test.go`

**Interfaces:**
- Consumes: `Store.GetProvider`, `Store.ReplaceProviderModels`
- Produces: `func (s *Store) RefreshModels(ctx context.Context, id string, client *http.Client) (Provider, error)`
  - `GET {baseUrl}/models` with `Authorization: Bearer {apiKey}`
  - Parse OpenAI list shape `{"data":[{"id":"...","object":"model"},...]}`; `Name` defaults to `id` if no display name
  - On HTTP error: return error wrapping status + truncated body; **do not** clear existing cache
  - Unknown provider → error

- [ ] **Step 1: Write the failing test**

```go
package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestRefreshModelsCachesList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"alpha"},{"id":"beta"}]}`))
	}))
	defer upstream.Close()

	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	got, err := store.RefreshModels(context.Background(), p.ID, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 2 || got.Models[0].ID != "alpha" || got.ModelsUpdatedAt == nil {
		t.Fatalf("got = %+v", got)
	}
}

func TestRefreshModelsKeepsCacheOnFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer upstream.Close()

	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, upstream.URL+"/v1", "sk-test")
	now := time.Now().UTC()
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "old", Name: "old"}}, now)
	_, err := store.RefreshModels(context.Background(), p.ID, upstream.Client())
	if err == nil {
		t.Fatal("expected error")
	}
	got, _ := store.GetProvider(p.ID)
	if len(got.Models) != 1 || got.Models[0].ID != "old" {
		t.Fatalf("cache cleared: %+v", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `RefreshModels`**

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/refresh.go controlplane/internal/catalog/refresh_test.go
git commit -m "feat(catalog): refresh and cache provider /models"
```

---

### Task 4: Catalog HTTP API

**Files:**
- Create: `controlplane/internal/catalog/http.go`
- Create: `controlplane/internal/catalog/http_test.go`
- Modify: `controlplane/internal/server/server.go`
- Modify: `controlplane/internal/server/server_test.go` (compile fixes only if needed)

**Interfaces:**
- Consumes: Store methods from Tasks 1–3
- Produces: `func Handler(store *Store) http.Handler` mounted at `/` with routes:
  - `GET/POST /v1/providers`
  - `GET/PATCH/DELETE /v1/providers/{id}`
  - `POST /v1/providers/{id}/models/refresh`
  - `GET/POST /v1/agents`
  - `GET/PATCH/DELETE /v1/agents/{id}`
- JSON errors: `{"error":"..."}` with 400/404/409/502 as appropriate (`ErrProviderInUse` → 409; refresh upstream fail → 502)
- `POST` bodies: providers `{name,type,baseUrl,apiKey}`; agents `{name,description,providerId,defaultModel}`
- `PATCH` bodies: partial fields as pointers / omitempty maps

- [ ] **Step 1: Write failing HTTP test**

```go
func TestProvidersHTTPCreateListRefresh(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m1"}]}`))
	}))
	defer upstream.Close()

	store, _ := catalog.Open(t.TempDir())
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	body := fmt.Sprintf(`{"name":"Local","type":"openai_compatible","baseUrl":%q,"apiKey":"sk"}`, upstream.URL+"/v1")
	resp, err := http.Post(srv.URL+"/v1/providers", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var p catalog.Provider
	_ = json.NewDecoder(resp.Body).Decode(&p)

	resp2, err := http.Post(srv.URL+"/v1/providers/"+p.ID+"/models/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("refresh status %d", resp2.StatusCode)
	}
}
```

Use `StatusOK` for create if you prefer one convention — pick **201 Created** for POST create and document it in the handler.

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `Handler` with `http.NewServeMux` Go 1.22 patterns** (`GET /v1/providers/{id}`, etc.)

- [ ] **Step 4: Update `server.NewMux`**

```go
func NewMux(store *runtime.Store, catalogStore *catalog.Store) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/acp", wstransport.Handler(store, catalogStore))
	// catalog.Handler registers full paths: GET /v1/providers, GET /v1/providers/{id}, …
	mux.Handle("/v1/", catalog.Handler(catalogStore))
	return mux
}
```

Implement `catalog.Handler` as an `http.ServeMux` whose patterns are the full `/v1/...` paths (Go 1.22+). Do **not** mount a catch-all at `/`. Update `New`, WS handler, and all call sites to compile (temporary: pass catalog even if agent ignores it until Task 8). Drop the unused process-global `streamer` argument from `NewMux` once Task 10 lands; until then keep a stub parameter only if needed for compile.

- [ ] **Step 5: Run `go -C controlplane test ./... -count=1` — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add controlplane/internal/catalog/ controlplane/internal/server/ controlplane/internal/transport/ws/ controlplane/cmd/
git commit -m "feat(catalog): expose providers and agents HTTP API"
```

---

### Task 5: Controlplane main — data dir, drop env OpenAI

**Files:**
- Modify: `controlplane/cmd/controlplane/main.go`
- Modify: `controlplane/internal/config/config.go` (optional: deprecate `Load` or leave unused)
- Modify: `README.md` (partial — full rewrite in Task 16)

**Interfaces:**
- Consumes: `catalog.Open`, `server.New` with catalog
- Produces: flags `-addr`, `-data-dir` (default `./data`); no `config.Load()`; live path does not construct process-global OpenAI (pass `nil` streamer until Task 10, or a registry helper)

- [ ] **Step 1: Write a smoke compile check by adjusting `main.go` and running**

```bash
go -C controlplane test ./... -count=1
```

(No new unit test required if Task 4 already covers wiring; ensure `main` builds:)

```bash
go -C controlplane build -o /tmp/controlplane ./cmd/controlplane
```

Expected: build succeeds without `OPENAI_*`.

- [ ] **Step 2: Implement `main.go`**

```go
addr := flag.String("addr", ":8080", "HTTP listen address")
dataDir := flag.String("data-dir", "./data", "catalog JSON directory")
flag.Parse()

cat, err := catalog.Open(*dataDir)
if err != nil {
	log.Fatal(err)
}
store := runtime.NewStore()
srv := server.New(*addr, store, cat, nil) // streamer unused once agent resolves from catalog
slog.Info("controlplane listening", "addr", *addr, "data_dir", *dataDir, "acp", "/acp", "catalog", "/v1")
log.Fatal(srv.ListenAndServe())
```

Update `server.New` signature accordingly. Keep agent tests injecting an explicit fake streamer **or** catalog-backed pins (Tasks 8–10).

- [ ] **Step 3: Build + test — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add controlplane/cmd/controlplane/main.go controlplane/internal/server/
git commit -m "feat(controlplane): serve catalog from data-dir without OPENAI env"
```

---

### Task 6: `ChatStreamer` takes model per call + registry

**Files:**
- Modify: `controlplane/internal/provider/streamer.go`
- Modify: `controlplane/internal/provider/openai.go`
- Modify: `controlplane/internal/provider/openai_test.go`
- Create: `controlplane/internal/provider/registry.go`
- Create: `controlplane/internal/provider/registry_test.go`
- Modify: all fakes implementing `ChatStreamer` (agent tests, server tests)

**Interfaces:**
- Consumes: none
- Produces:
  - `type ChatStreamer interface { StreamChat(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error }`
  - `func NewOpenAI(baseURL, apiKey string, httpClient *http.Client) *OpenAI` — **no model field**
  - `func NewStreamer(typ, baseURL, apiKey string, httpClient *http.Client) (ChatStreamer, error)` — `openai_compatible` → `NewOpenAI`; unknown type → error

- [ ] **Step 1: Update openai tests to pass model into `StreamChat` and assert JSON `model` field**

- [ ] **Step 2: Run — expect FAIL (signature mismatch)**

- [ ] **Step 3: Implement signature change + registry**

- [ ] **Step 4: Fix all compile breaks in fakes; run `go -C controlplane test ./... -count=1` — PASS**

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/provider/ controlplane/internal/agent/ controlplane/internal/server/ controlplane/internal/transport/
git commit -m "feat(provider): per-call model and type registry"
```

---

### Task 7: Runtime session pin for provider + model

**Files:**
- Modify: `controlplane/internal/runtime/definition.go`
- Modify: `controlplane/internal/runtime/session.go`
- Modify: `controlplane/internal/runtime/session_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  - Expand `Definition` **or** add `SessionPin` embedded on `Session`:

```go
type SessionPin struct {
	AgentID      string
	AgentName    string
	AgentVersion int
	ProviderID   string
	ProviderType string
	BaseURL      string
	APIKey       string
	Models       []catalog.ModelInfo // copy at pin time — avoid catalog import cycle by duplicating a small ModelRef type in runtime
	CurrentModel string
}

type Session struct {
	ID       string
	Pin      SessionPin
	Messages []Message
}
```

Prefer `runtime.ModelRef {ID, Name string}` to avoid `runtime` → `catalog` import.

  - `Create(pin SessionPin) (string, error)` replaces `Create(def Definition)`
  - `SetCurrentModel(id, model string) error` — validates model ∈ pin.Models
  - Keep `EchoDefinition` only if tests still need it; otherwise delete and update tests

- [ ] **Step 1: Write failing tests for Create pin + SetCurrentModel validation**

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement; update agent temporarily if needed to compile**

- [ ] **Step 4: PASS `go -C controlplane test ./internal/runtime/ -count=1`**

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/runtime/
git commit -m "feat(runtime): pin provider and model options on session"
```

---

### Task 8: ACP `session/new` from catalog + model configOptions

**Files:**
- Modify: `controlplane/internal/agent/agent.go`
- Create: `controlplane/internal/agent/config_options.go`
- Modify: `controlplane/internal/agent/agent_test.go`
- Modify: `controlplane/internal/transport/ws/handler.go`

**Interfaces:**
- Consumes: `*catalog.Store`, runtime pin APIs
- Produces:
  - `func New(store *runtime.Store, cat *catalog.Store) *Agent` (streamer resolved later from pin)
  - `NewSession` reads `params.Meta["agentId"]` as string; loads agent+provider; errors if missing/empty models/default not in cache; pins session; returns `ConfigOptions` with one select:

```go
cat := acp.SessionConfigOptionCategoryModel
opt := acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
	Type:         "select",
	Id:           acp.SessionConfigId("model"),
	Name:         "Model",
	Category:     &cat,
	CurrentValue: acp.SessionConfigValueId(pin.CurrentModel),
	Options: acp.SessionConfigSelectOptions{Ungrouped: &options},
}}
```

Helper `modelConfigOptions(pin runtime.SessionPin) []acp.SessionConfigOption` in `config_options.go`.

- [ ] **Step 1: Write failing agent test** using temp catalog with provider+models+agent; in-process ACP connection; `NewSession` with `Meta: map[string]any{"agentId": agent.ID}`; assert `resp.ConfigOptions[0].Select.CurrentValue` and options length

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement NewSession; remove `EchoDefinition` live path**

- [ ] **Step 4: PASS agent tests (prompt tests may still use fake until Task 10 — inject pin via catalog or test helper that seeds catalog)**

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/agent/ controlplane/internal/transport/ws/
git commit -m "feat(acp): session/new pins catalog agent and model options"
```

---

### Task 9: `session/set_config_option` for model

**Files:**
- Modify: `controlplane/internal/agent/agent.go` (or move impl out of stubs)
- Modify: `controlplane/internal/agent/stubs.go` — delete SetSessionConfigOption stub
- Modify: `controlplane/internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `runtime.Store.SetCurrentModel`, `modelConfigOptions`
- Produces: `SetSessionConfigOption` handles `ValueId` variant with `ConfigId == "model"`; updates pin; returns full `ConfigOptions`; unknown config/model → error; leave other stubs as MethodNotFound

- [ ] **Step 1: Failing test — new session, set model to second option, assert response currentValue**

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

- [ ] **Step 4: PASS**

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/agent/
git commit -m "feat(acp): set_config_option switches session model"
```

---

### Task 10: Prompt uses pinned provider + current model

**Files:**
- Modify: `controlplane/internal/agent/agent.go`
- Modify: `controlplane/internal/agent/agent_test.go`
- Modify: `controlplane/internal/server/server_test.go`
- Modify: `controlplane/internal/transport/ws/handler_test.go`

**Interfaces:**
- Consumes: `provider.NewStreamer(pin.ProviderType, pin.BaseURL, pin.APIKey, nil)`, `StreamChat(ctx, pin.CurrentModel, ...)`
- Produces: live Prompt path without injected process-global streamer; tests use catalog + `httptest` OpenAI **or** a test-only provider type — **prefer** `httptest` OpenAI with real `openai_compatible` path for one integration test, and a fake by registering optional test hook **only if needed**. Simplest approach for unit tests: keep optional `Agent` test field `streamerForTest provider.ChatStreamer` that overrides registry when non-nil.

```go
func (a *Agent) streamerFor(pin runtime.SessionPin) (provider.ChatStreamer, error) {
	if a.testStreamer != nil {
		return a.testStreamer, nil
	}
	return provider.NewStreamer(pin.ProviderType, pin.BaseURL, pin.APIKey, nil)
}
```

- [ ] **Step 1: Test that after set_config_option, fake streamer observes the new model string on Prompt**

```go
type recordingStreamer struct {
	lastModel string
	chunks    []string
}

func (r *recordingStreamer) StreamChat(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
	r.lastModel = model
	for _, c := range r.chunks {
		if err := onDelta(c); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: FAIL until Prompt reads CurrentModel**

- [ ] **Step 3: Implement Prompt resolution**

- [ ] **Step 4: `go -C controlplane test ./... -count=1` PASS**

- [ ] **Step 5: Commit**

```bash
git add controlplane/
git commit -m "feat(acp): prompt streams via pinned provider and model"
```

---

### Task 11: Flutter catalog client

**Files:**
- Create: `client/lib/catalog/models.dart`
- Create: `client/lib/catalog/catalog_client.dart`
- Create: `client/test/catalog/catalog_client_test.dart`

**Interfaces:**
- Consumes: HTTP JSON from Task 4
- Produces:
  - `class CatalogClient { CatalogClient({required Uri baseUri, http.Client? httpClient}); ... }`
  - Methods: `listProviders`, `createProvider`, `updateProvider`, `deleteProvider`, `refreshModels`, `listAgents`, `createAgent`, `updateAgent`, `deleteAgent`
  - `final defaultCatalogBase = Uri.parse('http://localhost:8080');`
  - Use `package:http` — add to `pubspec.yaml` if missing

- [ ] **Step 1: Failing test with `MockClient` returning a providers list JSON**

- [ ] **Step 2: `cd client && flutter test test/catalog/catalog_client_test.dart` — FAIL**

- [ ] **Step 3: Implement DTOs + client**

- [ ] **Step 4: PASS**

- [ ] **Step 5: Commit**

```bash
git add client/pubspec.yaml client/pubspec.lock client/lib/catalog/ client/test/catalog/
git commit -m "feat(client): add catalog HTTP client"
```

---

### Task 12: App shell — sidebar Chat / Settings

**Files:**
- Create: `client/lib/app_shell.dart`
- Modify: `client/lib/main.dart`
- Create: `client/test/app_shell_test.dart` (smoke: finds Chat and Settings destinations)

**Interfaces:**
- Consumes: existing `ChatScreen`
- Produces: `AppShell` with `NavigationRail` (or equivalent) destinations Chat + Settings; Settings body placeholder `Text('Settings')` until Task 13

- [ ] **Step 1: Widget test pumps AppShell, taps Settings, expects Settings text**

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement shell; `MaterialApp(home: AppShell(...))`**

- [ ] **Step 4: PASS `flutter test`**

- [ ] **Step 5: Commit**

```bash
git add client/lib/app_shell.dart client/lib/main.dart client/test/app_shell_test.dart
git commit -m "feat(client): add Chat/Settings sidebar shell"
```

---

### Task 13: Settings — Providers tab

**Files:**
- Create: `client/lib/settings/settings_page.dart`
- Create: `client/lib/settings/providers_tab.dart`
- Create: `client/test/settings/providers_tab_test.dart`
- Modify: `client/lib/app_shell.dart` — Settings → `SettingsPage`

**Interfaces:**
- Consumes: `CatalogClient`
- Produces: TabBar **Providers** | **Agents** (Agents tab placeholder in this task); Providers: list, create dialog (name, baseUrl, apiKey), refresh models button, show cached models

- [ ] **Step 1: Test with fake CatalogClient / mocked client showing one provider after load**

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement UI (keep styling minimal Material)**

- [ ] **Step 4: PASS**

- [ ] **Step 5: Commit**

```bash
git add client/lib/settings/ client/lib/app_shell.dart client/test/settings/
git commit -m "feat(client): providers settings tab"
```

---

### Task 14: Settings — Agents tab + placeholders

**Files:**
- Create: `client/lib/settings/agents_tab.dart`
- Modify: `client/lib/settings/settings_page.dart`
- Create: `client/test/settings/agents_tab_test.dart`

**Interfaces:**
- Consumes: `CatalogClient`
- Produces: Agent list/create/edit: name, description, provider dropdown, default model dropdown (from selected provider’s cached models); disabled ExpansionTiles or sections labeled Tools / MCP / Sandbox / Memory with subtitle “Coming soon”

- [ ] **Step 1: Widget test — create form requires provider+model from fake data**

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

- [ ] **Step 4: PASS**

- [ ] **Step 5: Commit**

```bash
git add client/lib/settings/ client/test/settings/
git commit -m "feat(client): agents settings tab with placeholders"
```

---

### Task 15: Chat agent picker + ACP model configOptions

**Files:**
- Modify: `client/lib/acp/agent_connection.dart`
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/test/chat/chat_controller_test.dart`
- Create: `client/test/acp/agent_connection_test.dart` if useful

**Interfaces:**
- Consumes: catalog agents list; ACP `NewSessionRequest(meta: {'agentId': id})`; session `configOptions`; `session.setConfigOption` / whatever `acpd` exposes on `Session`
- Produces:
  - `AgentConnection.connect` does **not** auto-create session; add `Future<List<ConfigOption>> startSession(String agentId)`
  - Or `connect` + `newSession(agentId)` clearing transcript
  - `Future<void> setModel(String modelId)` calling ACP set config option
  - `ChatController`: load agents; `selectAgent` clears messages and starts new session; expose `modelOptions` + `currentModel` + `selectModel`
  - UI: Dropdowns in chat header

Inspect `acpd` `Session` API for setConfigOption naming while implementing; match the package’s method (likely `setConfigOption` / `setSessionConfigOption`).

- [ ] **Step 1: Controller test with fake `AgentSessionApi` asserting `newSession` receives agentId and setModel forwarded**

Extend fake API:

```dart
abstract class AgentSessionApi {
  Stream<void> get closed;
  Future<void> connect({Transport? transport});
  Future<void> startSession(String agentId);
  Future<void> setModel(String modelId);
  List<ModelOption> get modelOptions;
  String? get currentModel;
  Future<void> sendPrompt(String text, {required AgentChunkHandler onChunk});
  Future<void> close();
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement connection + controller + screen pickers**

- [ ] **Step 4: `cd client && flutter test` PASS**

- [ ] **Step 5: Commit**

```bash
git add client/
git commit -m "feat(client): chat agent picker and ACP model selector"
```

---

### Task 16: README + acp-cli agentId

**Files:**
- Modify: `README.md`
- Modify: `controlplane/cmd/acp-cli/main.go`

**Interfaces:**
- Consumes: finished APIs
- Produces: docs for `-data-dir`, create provider via UI or curl, refresh models, create agent, run Flutter; `acp-cli` flag `-agent-id` passed as `_meta.agentId`

- [ ] **Step 1: Update README run section (replace OPENAI env requirements)**

Include example curl:

```bash
curl -s localhost:8080/v1/providers -H 'content-type: application/json' \
  -d '{"name":"Unsloth","type":"openai_compatible","baseUrl":"http://127.0.0.1:8888/v1","apiKey":"sk-unsloth-…"}'
```

- [ ] **Step 2: Add `-agent-id` to acp-cli `NewSessionRequest.Meta`**

- [ ] **Step 3: `go -C controlplane test ./...` && `cd client && flutter test`**

- [ ] **Step 4: Commit**

```bash
git add README.md controlplane/cmd/acp-cli/
git commit -m "docs: catalog settings workflow; acp-cli agent id"
```

---

## Self-review (plan vs spec)

| Spec requirement | Task |
| --- | --- |
| Providers CRUD + JSON | 1, 4 |
| Refresh `/models` + cache | 3, 4, 13 |
| Agents CRUD + provider/model link | 2, 4, 14 |
| Placeholders MCP/tools/sandbox/memory | 14 |
| Provider type registry | 6 |
| Drop OPENAI env | 5, 16 |
| `session/new` `_meta.agentId` + pin + configOptions | 8 |
| `set_config_option` model | 9 |
| Prompt via pinned provider/model | 10 |
| Flutter sidebar + Settings tabs | 12–14 |
| Chat agent + model pickers | 15 |
| Offline tests | throughout |
| 409 provider in use | 2, 4 |
| No catalog auth / no encryption | respected (non-goals) |

**Type consistency notes for implementers:**
- HTTP field names: `baseUrl`, `apiKey`, `providerId`, `defaultModel` (camelCase JSON)
- ACP meta key: `agentId` (string)
- Config option id: `model`
- `ChatStreamer.StreamChat(ctx, model, messages, onDelta)`
- Runtime pin uses `ModelRef`, not importing `catalog` into `runtime`
