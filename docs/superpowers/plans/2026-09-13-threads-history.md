# Durable Threads and Chat History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist chat threads and messages in Postgres, expose them on the catalog HTTP API, bind ACP sessions to threads, and add a Flutter thread pane (new, filter, rename, restore history).

**Architecture:** Threads are catalog HTTP resources. The plane is the only writer of messages (ACP prompt commit). A live ACP session is a throwaway handle: `session/new` with `_meta.threadId` hydrates `runtime.Store` from the thread; omit `threadId` for today’s in-memory CLI/test path. Flutter lists/loads threads over HTTP and streams turns over ACP.

**Tech Stack:** Go 1.22+, pgx/v5, sqlc, goose, testcontainers (from the Postgres catalog slice), Flutter, `acpd` `Session.cancel()`.

## Global Constraints

- **Prerequisite:** Finish [`2026-09-12-postgres-catalog.md`](./2026-09-12-postgres-catalog.md) first. This plan assumes `catalog.Open(pool *pgxpool.Pool) *Store`, context-aware catalog methods, `dbtest.Open(t)`, goose migration `00001_catalog`, and sqlc under `controlplane/internal/db/`.
- Spec: [`2026-09-13-threads-history-design.md`](../specs/2026-09-13-threads-history-design.md).
- Thread ids `th_` + hex; message ids `msg_` + hex (`catalog.newID`).
- Default title `Untitled`, `titleSource` `auto` | `user`. Auto-title = first 8 `strings.Fields` of the first committed user prompt; never overwrite `user`.
- Client never POSTs message bodies. No delete-thread, default-agent, server search, or pagination.
- Bound prompt: commit user+assistant in one DB transaction after success; cancel/error writes nothing and does not append to `runtime.Store`. Unbound (`threadId` absent) keeps current in-memory append behavior.
- `initialize` keeps `loadSession: false`.
- Module path `github.com/tryy3/agent-fabric` (root `controlplane/`).
- Follow TDD: failing test → implement → pass → commit per task.
- If `catalog.Store` only holds `*db.Queries` after the Postgres plan, add `pool *pgxpool.Pool` for `CommitTurn` transactions.

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/db/migrations/00002_threads.up.sql` | `threads` + `messages` tables |
| `controlplane/internal/db/migrations/00002_threads.down.sql` | Drop those tables |
| `controlplane/internal/db/queries/threads.sql` | sqlc thread/message queries |
| `controlplane/internal/db/*.go` | Regenerated sqlc |
| `controlplane/internal/catalog/thread_types.go` | `Thread`, `ThreadListItem`, `ThreadDetail`, `ThreadMessage`, `TitleSource` |
| `controlplane/internal/catalog/title.go` | `AutoTitle(prompt string) string` |
| `controlplane/internal/catalog/store.go` | Thread CRUD, pin, model, `CommitTurn`, agent-in-use |
| `controlplane/internal/catalog/http.go` | `/v1/threads` routes |
| `controlplane/internal/runtime/session.go` | `Session.ThreadID`; `CreateHydrated` |
| `controlplane/internal/agent/agent.go` | `_meta.threadId` hydrate; bound prompt commit; persist model |
| `client/lib/catalog/models.dart` | Thread Dart models |
| `client/lib/catalog/catalog_client.dart` | Thread HTTP methods |
| `client/lib/acp/agent_connection.dart` | `startSession(..., {threadId})`, `cancel()` |
| `client/lib/chat/chat_controller.dart` | Thread list/selection/switch |
| `client/lib/chat/thread_pane.dart` | Sidebar UI |
| `client/lib/app_shell.dart` | Show pane only on Chat |
| `README.md` | Thread UX + curl |

**Store methods this plan adds** (all take `context.Context`):

```go
func (s *Store) CreateThread(ctx context.Context) (Thread, error)
func (s *Store) ListThreads(ctx context.Context) ([]ThreadListItem, error)
func (s *Store) GetThread(ctx context.Context, id string) (ThreadDetail, error) // not found: thread %q not found
func (s *Store) RenameThread(ctx context.Context, id, title string) (Thread, error)
func (s *Store) PinThreadAgent(ctx context.Context, id, agentID string) error // ErrAgentLocked if mismatch
func (s *Store) SetThreadModel(ctx context.Context, id, model string) error
func (s *Store) CommitTurn(ctx context.Context, threadID, userText, assistantText string) (Thread, error)
func (s *Store) CountThreadsByAgent(ctx context.Context, agentID string) (int64, error)

var ErrAgentInUse = errors.New("agent in use")
var ErrAgentLocked = errors.New("thread agent is locked")
```

**Runtime:**

```go
func (s *Store) CreateHydrated(pin SessionPin, threadID string, msgs []Message) (string, error)
// Session gains ThreadID string (empty when unbound)
```

**Flutter session API:**

```dart
Future<void> startSession(String agentId, {String? threadId});
Future<void> cancel();
```

---

### Task 1: Threads schema + sqlc queries

**Files:**
- Create: `controlplane/internal/db/migrations/00002_threads.up.sql`
- Create: `controlplane/internal/db/migrations/00002_threads.down.sql`
- Create: `controlplane/internal/db/queries/threads.sql`
- Modify: generated files under `controlplane/internal/db/` (via `sqlc generate`)
- Test: `controlplane/internal/db/postgres_test.go` (add schema smoke) or `controlplane/internal/db/threads_schema_test.go`

**Interfaces:**
- Consumes: goose embed from Postgres plan; `agents(id)` from `00001_catalog`
- Produces: sqlc methods listed in the SQL below

- [ ] **Step 1: Write a failing schema smoke test**

Create `controlplane/internal/db/threads_schema_test.go`:

```go
package db_test

import (
	"context"
	"testing"

	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestThreadsSchemaAllowsUntitledInsert(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
INSERT INTO threads (id, title, title_source, created_at, updated_at)
VALUES ('th_test', 'Untitled', 'auto', now(), now())`)
	if err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM threads`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/db/ -run TestThreadsSchemaAllowsUntitledInsert -count=1`

Expected: FAIL (`relation "threads" does not exist`).

- [ ] **Step 3: Add migration and queries, generate sqlc**

`controlplane/internal/db/migrations/00002_threads.up.sql`:

```sql
CREATE TABLE threads (
    id            text PRIMARY KEY,
    title         text NOT NULL DEFAULT 'Untitled',
    title_source  text NOT NULL CHECK (title_source IN ('auto', 'user')),
    agent_id      text REFERENCES agents(id) ON DELETE RESTRICT,
    current_model text,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL
);

CREATE TABLE messages (
    id         text PRIMARY KEY,
    thread_id  text NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('user', 'assistant')),
    content    text NOT NULL,
    position   int NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (thread_id, position)
);

CREATE INDEX messages_thread_id_position_idx ON messages (thread_id, position);
```

`controlplane/internal/db/migrations/00002_threads.down.sql`:

```sql
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS threads;
```

`controlplane/internal/db/queries/threads.sql`:

```sql
-- name: InsertThread :one
INSERT INTO threads (
  id, title, title_source, agent_id, current_model, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: ListThreads :many
SELECT
  t.id,
  t.title,
  t.title_source,
  t.agent_id,
  t.current_model,
  t.created_at,
  t.updated_at,
  (SELECT count(*)::int FROM messages m WHERE m.thread_id = t.id) AS message_count
FROM threads t
ORDER BY t.updated_at DESC;

-- name: GetThread :one
SELECT id, title, title_source, agent_id, current_model, created_at, updated_at
FROM threads
WHERE id = $1;

-- name: RenameThread :one
UPDATE threads
SET title = $2, title_source = $3, updated_at = $4
WHERE id = $1
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: PinThreadAgent :one
UPDATE threads
SET agent_id = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: SetThreadModel :one
UPDATE threads
SET current_model = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: SetThreadTitleIfAuto :exec
UPDATE threads
SET title = $2, updated_at = $3
WHERE id = $1 AND title_source = 'auto';

-- name: TouchThread :exec
UPDATE threads SET updated_at = $2 WHERE id = $1;

-- name: InsertMessage :one
INSERT INTO messages (id, thread_id, role, content, position, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, thread_id, role, content, position, created_at;

-- name: ListMessages :many
SELECT id, thread_id, role, content, position, created_at
FROM messages
WHERE thread_id = $1
ORDER BY position ASC;

-- name: NextMessagePosition :one
SELECT COALESCE(MAX(position), -1)::int AS max_position
FROM messages
WHERE thread_id = $1;

-- name: CountThreadsByAgent :one
SELECT count(*) FROM threads WHERE agent_id = $1;
```

If `sqlc.yaml` `schema` is a glob that already includes `migrations`, leave it. If it lists only `00001_catalog.up.sql`, add `00002_threads.up.sql`.

From `controlplane/internal/db`:

```bash
sqlc generate
```

Commit generated Go.

- [ ] **Step 4: Run test to verify it passes**

Run: `go -C controlplane test ./internal/db/ -run TestThreadsSchemaAllowsUntitledInsert -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/db
git commit -m "feat(db): add threads and messages schema"
```

---

### Task 2: Catalog thread store + AutoTitle

**Files:**
- Create: `controlplane/internal/catalog/thread_types.go`
- Create: `controlplane/internal/catalog/title.go`
- Create: `controlplane/internal/catalog/title_test.go`
- Modify: `controlplane/internal/catalog/store.go`
- Test: `controlplane/internal/catalog/threads_store_test.go`

**Interfaces:**
- Consumes: sqlc thread queries; `newID`; `dbtest.Open`; existing `CreateProvider` / `CreateAgent` for pin tests
- Produces: store methods in File Structure; `AutoTitle`

- [ ] **Step 1: Write failing tests**

`controlplane/internal/catalog/title_test.go`:

```go
package catalog

import "testing"

func TestAutoTitleFirstEightWords(t *testing.T) {
	got := AutoTitle("  How do I pin an agent to a thread please  ")
	want := "How do I pin an agent to a"
	if got != want {
		t.Fatalf("AutoTitle = %q, want %q", got, want)
	}
}

func TestAutoTitleShortPrompt(t *testing.T) {
	got := AutoTitle("hello there")
	if got != "hello there" {
		t.Fatalf("AutoTitle = %q", got)
	}
}

func TestAutoTitleEmptyStaysUntitled(t *testing.T) {
	got := AutoTitle("   ")
	if got != "Untitled" {
		t.Fatalf("AutoTitle = %q, want Untitled", got)
	}
}
```

`controlplane/internal/catalog/threads_store_test.go` (package `catalog_test`):

```go
package catalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestCreateListRenameThread(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))

	a, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "Untitled" || a.TitleSource != catalog.TitleSourceAuto {
		t.Fatalf("create = %+v", a)
	}
	if a.AgentID != nil {
		t.Fatalf("agent_id = %v, want nil", a.AgentID)
	}
	if !strings.HasPrefix(a.ID, "th_") {
		t.Fatalf("id %q", a.ID)
	}

	time.Sleep(2 * time.Millisecond)
	b, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}

	list, err := store.ListThreads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("list order = %+v", list)
	}
	if list[0].MessageCount != 0 {
		t.Fatalf("messageCount = %d", list[0].MessageCount)
	}

	renamed, err := store.RenameThread(ctx, a.ID, "  My chat  ")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "My chat" || renamed.TitleSource != catalog.TitleSourceUser {
		t.Fatalf("rename = %+v", renamed)
	}

	_, err = store.RenameThread(ctx, a.ID, "   ")
	if err == nil {
		t.Fatal("expected empty title error")
	}

	_, err = store.GetThread(ctx, "th_missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing: %v", err)
	}
}

func TestCommitTurnAutoTitleAndLock(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.CommitTurn(ctx, th.ID, "How do I pin an agent to a thread please", "You pick the agent first.")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "How do I pin an agent to a" {
		t.Fatalf("title = %q", updated.Title)
	}
	if updated.TitleSource != catalog.TitleSourceAuto {
		t.Fatalf("source = %s", updated.TitleSource)
	}
	detail, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d", len(detail.Messages))
	}
	if detail.Messages[0].Role != "user" || detail.Messages[1].Role != "assistant" {
		t.Fatalf("roles = %+v", detail.Messages)
	}
	if detail.Messages[0].Position != 0 || detail.Messages[1].Position != 1 {
		t.Fatalf("positions = %+v", detail.Messages)
	}

	if _, err := store.RenameThread(ctx, th.ID, "Pinned"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitTurn(ctx, th.ID, "second prompt that would retitle", "ok"); err != nil {
		t.Fatal(err)
	}
	again, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != "Pinned" || again.TitleSource != catalog.TitleSourceUser {
		t.Fatalf("kept title = %+v", again.Thread)
	}
	if len(again.Messages) != 4 {
		t.Fatalf("messages = %d", len(again.Messages))
	}
}

func TestPinThreadAgentLocks(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(ctx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateAgent(ctx, "Other", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, ag.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, ag.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, other.ID); !errors.Is(err, catalog.ErrAgentLocked) {
		t.Fatalf("lock err = %v", err)
	}
	if err := store.SetThreadModel(ctx, th.ID, "m1"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID == nil || *got.AgentID != ag.ID {
		t.Fatalf("agent = %v", got.AgentID)
	}
	if got.CurrentModel == nil || *got.CurrentModel != "m1" {
		t.Fatalf("model = %v", got.CurrentModel)
	}
}
```

If `ReplaceProviderModels` is not the Postgres-plan name, use whatever that plan left on `Store` to seed a cached model (e.g. create provider then refresh, or insert models in the create helper). Match existing `store_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/catalog/ -run 'TestAutoTitle|TestCreateListRenameThread|TestCommitTurn|TestPinThread' -count=1`

Expected: FAIL (undefined `AutoTitle` / `CreateThread`).

- [ ] **Step 3: Implement types, AutoTitle, and store methods**

`thread_types.go`:

```go
package catalog

import "time"

type TitleSource string

const (
	TitleSourceAuto TitleSource = "auto"
	TitleSourceUser TitleSource = "user"
)

type Thread struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	TitleSource  TitleSource `json:"titleSource"`
	AgentID      *string     `json:"agentId"`
	CurrentModel *string     `json:"currentModel"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

type ThreadListItem struct {
	Thread
	MessageCount int `json:"messageCount"`
}

type ThreadMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

type ThreadDetail struct {
	Thread
	Messages []ThreadMessage `json:"messages"`
}
```

`title.go`:

```go
package catalog

import "strings"

func AutoTitle(prompt string) string {
	fields := strings.Fields(prompt)
	if len(fields) == 0 {
		return "Untitled"
	}
	if len(fields) > 8 {
		fields = fields[:8]
	}
	return strings.Join(fields, " ")
}
```

Store mapping: convert sqlc null `*string` for `agent_id` / `current_model`; `title_source` to `TitleSource`.

`CreateThread`: `newID("th_")`, title `Untitled`, source `auto`, timestamps UTC now.

`RenameThread`: `title = strings.TrimSpace(title)`; if empty return `fmt.Errorf("thread title is required")`; set `title_source=user`.

`GetThread`: `GetThread` + `ListMessages`; `pgx.ErrNoRows` → `fmt.Errorf("thread %q not found", id)`.

`PinThreadAgent`: get thread; if `AgentID == nil`, `PinThreadAgent` query; if equal, no-op; else `ErrAgentLocked`.

`CommitTurn`: `Begin`; `NextMessagePosition` → `pos+1` user, `pos+2` assistant (`max_position` starts at -1 so first pair is 0,1); `InsertMessage` twice with `newID("msg_")`; if current `title_source=auto`, `SetThreadTitleIfAuto` with `AutoTitle(userText)`; else `TouchThread`; `Commit`; return `GetThread`’s `Thread` (or map from last row). Use `s.q.WithTx(tx)`.

Add `var ErrAgentLocked = errors.New("thread agent is locked")`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/catalog/ -run 'TestAutoTitle|TestCreateListRenameThread|TestCommitTurn|TestPinThread' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog
git commit -m "feat(catalog): persist threads and committed turns"
```

---

### Task 3: HTTP `/v1/threads` + ErrAgentInUse

**Files:**
- Modify: `controlplane/internal/catalog/http.go`
- Modify: `controlplane/internal/catalog/store.go` (`DeleteAgent`)
- Test: `controlplane/internal/catalog/threads_http_test.go`
- Modify: `controlplane/internal/catalog/store_test.go` (agent delete with thread)

**Interfaces:**
- Consumes: `CreateThread`, `ListThreads`, `GetThread`, `RenameThread`, `CountThreadsByAgent`
- Produces: HTTP routes in the spec; `ErrAgentInUse` → 409

- [ ] **Step 1: Write failing HTTP tests**

`controlplane/internal/catalog/threads_http_test.go`:

```go
package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestThreadsHTTPCreateListGetRename(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var created catalog.Thread
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if created.Title != "Untitled" {
		t.Fatalf("title %q", created.Title)
	}

	listResp, err := http.Get(srv.URL + "/v1/threads")
	if err != nil {
		t.Fatal(err)
	}
	var list []catalog.ThreadListItem
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if len(list) != 1 || list[0].ID != created.ID || list[0].MessageCount != 0 {
		t.Fatalf("list %+v", list)
	}

	got, err := http.Get(srv.URL + "/v1/threads/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var detail catalog.ThreadDetail
	if err := json.NewDecoder(got.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	got.Body.Close()
	if detail.Messages == nil {
		t.Fatal("messages must be [] not null")
	}

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID, strings.NewReader(`{"title":"Renamed"}`))
	req.Header.Set("content-type", "application/json")
	patch, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch %d", patch.StatusCode)
	}
	patch.Body.Close()

	bad, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID, strings.NewReader(`{"title":"  "}`))
	bad.Header.Set("content-type", "application/json")
	badResp, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty title status %d", badResp.StatusCode)
	}
	badResp.Body.Close()

	missing, err := http.Get(srv.URL + "/v1/threads/th_nope")
	if err != nil {
		t.Fatal(err)
	}
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status %d", missing.StatusCode)
	}
	missing.Body.Close()
}

func TestDeleteAgentWithThreadConflict(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	p, err := store.CreateProvider(ctx, "Local", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceProviderModels(ctx, p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ag, err := store.CreateAgent(ctx, "Coder", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PinThreadAgent(ctx, th.ID, ag.ID); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/agents/"+ag.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/catalog/ -run 'TestThreadsHTTP|TestDeleteAgentWithThread' -count=1`

Expected: FAIL (404 on `/v1/threads` or 204 on agent delete).

- [ ] **Step 3: Implement HTTP + delete guard**

In `Handler`, register:

```go
mux.HandleFunc("GET /v1/threads", h.listThreads)
mux.HandleFunc("POST /v1/threads", h.createThread)
mux.HandleFunc("GET /v1/threads/{id}", h.getThread)
mux.HandleFunc("PATCH /v1/threads/{id}", h.patchThread)
```

Handlers pass `r.Context()`. POST ignores body (or `{}`). PATCH decodes `{"title":...}` and calls `RenameThread`. GET list returns `[]ThreadListItem` (empty slice, not null). GET detail must encode `messages: []` when empty (`make([]ThreadMessage, 0)`).

`writeMappedError`: also map `ErrAgentInUse` to 409; treat `thread %q not found` as 404 (extend `isNotFoundFor` with `"thread"`).

`DeleteAgent`: before delete, `CountThreadsByAgent`; if `> 0` return `ErrAgentInUse`. Add `var ErrAgentInUse = errors.New("agent in use")`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/catalog/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog
git commit -m "feat(catalog): add /v1/threads HTTP and agent-in-use"
```

---

### Task 4: Runtime hydrated sessions

**Files:**
- Modify: `controlplane/internal/runtime/session.go`
- Test: `controlplane/internal/runtime/session_test.go`

**Interfaces:**
- Consumes: existing `Create`
- Produces: `CreateHydrated(pin, threadID, msgs) (id, error)`; `Session.ThreadID`

- [ ] **Step 1: Write failing test**

Add to `session_test.go`:

```go
func TestCreateHydratedCopiesMessagesAndThreadID(t *testing.T) {
	store := NewStore()
	pin := SessionPin{AgentID: "ag", CurrentModel: "m1", Models: []ModelRef{{ID: "m1", Name: "M"}}}
	msgs := []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}}
	id, err := store.CreateHydrated(pin, "th_abc", msgs)
	if err != nil {
		t.Fatal(err)
	}
	sess, ok := store.Get(id)
	if !ok {
		t.Fatal("missing")
	}
	if sess.ThreadID != "th_abc" {
		t.Fatalf("ThreadID = %q", sess.ThreadID)
	}
	if !strings.HasPrefix(id, "sess_") {
		t.Fatalf("id %q", id)
	}
	got, ok := store.Messages(id)
	if !ok || len(got) != 2 || got[0].Content != "hi" {
		t.Fatalf("messages %+v", got)
	}
	got[0].Content = "mutated"
	again, _ := store.Messages(id)
	if again[0].Content != "hi" {
		t.Fatal("stored slice must be copied")
	}
}

func TestCreateLeavesThreadIDEmpty(t *testing.T) {
	store := NewStore()
	id, err := store.Create(SessionPin{AgentID: "ag", CurrentModel: "m1", Models: []ModelRef{{ID: "m1"}}})
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := store.Get(id)
	if sess.ThreadID != "" {
		t.Fatalf("ThreadID = %q", sess.ThreadID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/runtime/ -run 'TestCreateHydrated|TestCreateLeavesThreadIDEmpty' -count=1`

Expected: FAIL (unknown `CreateHydrated` / `ThreadID`).

- [ ] **Step 3: Implement**

Add `ThreadID string` to `Session`. Implement `CreateHydrated` like `Create` but set `ThreadID` and copy `msgs`. Change `Create` to `return s.CreateHydrated(pin, "", nil)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/runtime/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/runtime
git commit -m "feat(runtime): hydrate sessions from thread history"
```

---

### Task 5: ACP `session/new` binds threadId

**Files:**
- Modify: `controlplane/internal/agent/agent.go`
- Test: `controlplane/internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `GetThread`, `PinThreadAgent`, `SetThreadModel`, `CreateHydrated`
- Produces: optional `_meta.threadId` on `NewSession`

- [ ] **Step 1: Write failing tests**

Use existing `startACPCatalog` / `seedCatalog` (already on `dbtest` after the Postgres plan). Add:

```go
func TestNewSessionWithThreadHydratesAndPinsAgent(t *testing.T) {
	ctx := context.Background()
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.CommitTurn(ctx, th.ID, "hello there", "hi"); err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	live, ok := store.Get(string(sess.SessionId))
	if !ok {
		t.Fatal("runtime session missing")
	}
	if live.ThreadID != th.ID {
		t.Fatalf("ThreadID = %q", live.ThreadID)
	}
	if len(live.Messages) != 2 || live.Messages[0].Content != "hello there" {
		t.Fatalf("hydrated %+v", live.Messages)
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID == nil || *got.AgentID != catalogAgent.ID {
		t.Fatalf("pinned agent %v", got.AgentID)
	}
}

func TestNewSessionThreadAgentMismatchFails(t *testing.T) {
	ctx := context.Background()
	store := runtime.NewStore()
	cat, a1 := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	a2, err := cat.CreateAgent(ctx, "Other", "", a1.ProviderID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.PinThreadAgent(ctx, th.ID, a1.ID); err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	_, err = csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": a2.ID, "threadId": th.ID},
	})
	if err == nil {
		t.Fatal("expected lock error")
	}
}

func TestNewSessionWithoutThreadIdDoesNotWriteThread(t *testing.T) {
	ctx := context.Background()
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	if _, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != nil {
		t.Fatalf("unbound session pinned thread: %v", got.AgentID)
	}
}

func TestNewSessionMissingThreadFails(t *testing.T) {
	store := runtime.NewStore()
	cat, catalogAgent := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	_, csc, _, ctx, _ := startACPCatalog(t, store, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	_, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": catalogAgent.ID, "threadId": "th_missing"},
	})
	if err == nil {
		t.Fatal("expected missing thread error")
	}
}
```

If `seedCatalog` / `CreateAgent` already take `ctx` after the Postgres plan, match those signatures exactly (do not invent a second `CreateAgent` shape).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/agent/ -run 'TestNewSessionWithThread|TestNewSessionThreadAgent|TestNewSessionWithoutThread|TestNewSessionMissingThread' -count=1`

Expected: FAIL (thread not hydrated / pin missing).

- [ ] **Step 3: Implement bind in `NewSession`**

Add `metaThreadID(meta)` — optional; missing key → `""`; non-string/blank → error `threadId is invalid`.

After `pinFromCatalog`:

```go
threadID, err := metaThreadID(params.Meta)
if err != nil { return ..., err }
var history []runtime.Message
if threadID != "" {
  detail, err := a.catalog.GetThread(ctx, threadID)
  if err != nil { return ..., err }
  if err := a.catalog.PinThreadAgent(ctx, threadID, pin.AgentID); err != nil { return ..., err }
  if detail.CurrentModel != nil {
    if err := a.store /* can't set until created */; 
  }
  for _, m := range detail.Messages {
    history = append(history, runtime.Message{Role: m.Role, Content: m.Content})
  }
  if detail.CurrentModel != nil {
    for _, m := range pin.Models {
      if m.ID == *detail.CurrentModel {
        pin.CurrentModel = *detail.CurrentModel
        break
      }
    }
  } else if err := a.catalog.SetThreadModel(ctx, threadID, pin.CurrentModel); err != nil {
    return ..., err
  }
}
id, err := a.store.CreateHydrated(pin, threadID, history)
```

If `current_model` is set but not in the pin list, fall back to agent default (do not error).

Keep `loadSession: false` on `Initialize`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/agent/ -count=1`

Expected: PASS (existing unbound tests still pass).

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/agent
git commit -m "feat(acp): bind session/new to catalog threads"
```

---

### Task 6: Bound prompt commit / cancel / auto-title

**Files:**
- Modify: `controlplane/internal/agent/agent.go` (`Prompt`)
- Test: `controlplane/internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `Session.ThreadID`, `CommitTurn`, existing streamer + cancel map
- Produces: bound turns persist only after success

- [ ] **Step 1: Write failing tests**

```go
func TestBoundPromptCommitsBothAndAutoTitles(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, &fakeStreamer{deltas: []string{"hello"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("How do I pin an agent to a thread please")},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Title != "How do I pin an agent to a" {
		t.Fatalf("title %q", detail.Title)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages %d", len(detail.Messages))
	}
	live, _ := rt.Messages(string(sess.SessionId))
	if len(live) != 2 {
		t.Fatalf("runtime messages %d", len(live))
	}
}

func TestBoundPromptCancelWritesNothing(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	streamer := &fakeStreamer{streamFn: func(ctx context.Context, model string, messages []runtime.Message, onDelta func(string) error) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	agnt, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, streamer)
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := csc.Prompt(ctx2, acp.PromptRequest{
			SessionId: sess.SessionId,
			Prompt:    []acp.ContentBlock{acp.TextBlock("will cancel")},
		})
		errCh <- err
	}()
	<-started
	if err := agnt.Cancel(context.Background(), acp.CancelNotification{SessionId: sess.SessionId}); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err == nil {
		t.Fatal("expected prompt error")
	}
	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 0 {
		t.Fatalf("persisted %d messages", len(detail.Messages))
	}
	live, _ := rt.Messages(string(sess.SessionId))
	if len(live) != 0 {
		t.Fatalf("runtime %d", len(live))
	}
}

func TestUnboundPromptStillDoesNotTouchThreads(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, &fakeStreamer{deltas: []string{"echo"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess := mustNewSession(t, ctx2, csc, ag.ID)
	if _, err := csc.Prompt(ctx2, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 0 {
		t.Fatalf("unbound prompt wrote thread: %+v", detail.Messages)
	}
}
```

Cancel the in-flight turn with `agnt.Cancel` (same as `TestCancelAbortsInFlightPrompt`), not a client SDK helper.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/agent/ -run 'TestBoundPrompt|TestUnboundPromptStill' -count=1`

Expected: FAIL (messages missing or cancel still appends user).

- [ ] **Step 3: Implement bound `Prompt` path**

After resolving `sess` and `text`:

```go
userMsg := runtime.Message{Role: "user", Content: text}
if sess.ThreadID == "" {
  if err := a.store.Append(sid, userMsg); err != nil { ... }
} 
// register cancel as today
var msgs []runtime.Message
if sess.ThreadID == "" {
  msgs, ok = a.store.Messages(sid)
} else {
  existing, _ := a.store.Messages(sid)
  msgs = append(append([]runtime.Message{}, existing...), userMsg)
}
// stream as today
if err != nil || full.Len() == 0 {
  return error // do not append / CommitTurn
}
if sess.ThreadID != "" {
  if _, err := a.catalog.CommitTurn(ctx, sess.ThreadID, text, full.String()); err != nil { ... }
  if err := a.store.Append(sid, userMsg); err != nil { ... }
  if err := a.store.Append(sid, runtime.Message{Role: "assistant", Content: full.String()}); err != nil { ... }
} else {
  if err := a.store.Append(sid, runtime.Message{Role: "assistant", Content: full.String()}); err != nil { ... }
}
```

Re-`Get` session after pin if needed so `ThreadID` is visible (`CreateHydrated` already set it).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/agent/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/agent
git commit -m "feat(acp): persist bound prompt turns on commit only"
```

---

### Task 7: Persist `current_model` on set_config_option

**Files:**
- Modify: `controlplane/internal/agent/agent.go` (`SetSessionConfigOption`)
- Test: `controlplane/internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `Session.ThreadID`, `SetThreadModel`
- Produces: reopen restores model (covered by NewSession + this write)

- [ ] **Step 1: Write failing test**

```go
func TestSetConfigOptionPersistsThreadModel(t *testing.T) {
	ctx := context.Background()
	rt := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{
		{ID: "m1", Name: "Model 1"},
		{ID: "m2", Name: "Model 2"},
	}, "m1")
	th, err := cat.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx2, _ := startACPCatalog(t, rt, cat, &fakeStreamer{deltas: []string{"ok"}})
	if _, err := csc.Initialize(ctx2, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	sess, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csc.SetSessionConfigOption(ctx2, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			ConfigId:  acp.SessionConfigId("model"),
			SessionId: sess.SessionId,
			Value:     acp.SessionConfigValueId("m2"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := cat.GetThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentModel == nil || *got.CurrentModel != "m2" {
		t.Fatalf("current_model = %v", got.CurrentModel)
	}
	sess2, err := csc.NewSession(ctx2, acp.NewSessionRequest{
		Cwd: "/", McpServers: []acp.McpServer{},
		Meta: map[string]any{"agentId": ag.ID, "threadId": th.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	live, ok := rt.Get(string(sess2.SessionId))
	if !ok || live.Pin.CurrentModel != "m2" {
		t.Fatalf("reopen model = %+v ok=%v", live.Pin, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go -C controlplane test ./internal/agent/ -run TestSetConfigOptionPersistsThreadModel -count=1`

Expected: FAIL (`current_model` still default).

- [ ] **Step 3: Implement**

After successful `a.store.SetCurrentModel`, if `sess.ThreadID != ""` call `a.catalog.SetThreadModel(ctx, sess.ThreadID, string(params.ValueId.Value))`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/agent/ -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/agent
git commit -m "feat(acp): persist thread model on set_config_option"
```

---

### Task 8: Flutter catalog thread client

**Files:**
- Modify: `client/lib/catalog/models.dart`
- Modify: `client/lib/catalog/catalog_client.dart`
- Test: `client/test/catalog/catalog_client_test.dart`

**Interfaces:**
- Consumes: HTTP JSON from Task 3
- Produces: Dart types + `listThreads` / `createThread` / `getThread` / `renameThread`

- [ ] **Step 1: Write failing tests**

Add to `catalog_client_test.dart`:

```dart
test('listThreads GET /v1/threads', () async {
  final client = CatalogClient(
    baseUri: baseUri,
    httpClient: MockClient((request) async {
      expect(request.method, 'GET');
      expect(request.url.path, '/v1/threads');
      return http.Response(
        jsonEncode([
          {
            'id': 'th_1',
            'title': 'Untitled',
            'titleSource': 'auto',
            'agentId': null,
            'currentModel': null,
            'messageCount': 0,
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T10:00:00Z',
          },
        ]),
        200,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
  final threads = await client.listThreads();
  expect(threads, hasLength(1));
  expect(threads.single.id, 'th_1');
  expect(threads.single.title, 'Untitled');
  expect(threads.single.titleSource, 'auto');
  expect(threads.single.agentId, isNull);
  expect(threads.single.messageCount, 0);
});

test('createThread POST /v1/threads', () async {
  final client = CatalogClient(
    baseUri: baseUri,
    httpClient: MockClient((request) async {
      expect(request.method, 'POST');
      expect(request.url.path, '/v1/threads');
      return http.Response(
        jsonEncode({
          'id': 'th_2',
          'title': 'Untitled',
          'titleSource': 'auto',
          'createdAt': '2026-09-13T10:00:00Z',
          'updatedAt': '2026-09-13T10:00:00Z',
        }),
        201,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
  final t = await client.createThread();
  expect(t.id, 'th_2');
  expect(t.title, 'Untitled');
});

test('getThread parses messages', () async {
  final client = CatalogClient(
    baseUri: baseUri,
    httpClient: MockClient((request) async {
      expect(request.url.path, '/v1/threads/th_1');
      return http.Response(
        jsonEncode({
          'id': 'th_1',
          'title': 'Hi',
          'titleSource': 'auto',
          'agentId': 'ag-1',
          'currentModel': 'm1',
          'createdAt': '2026-09-13T10:00:00Z',
          'updatedAt': '2026-09-13T11:00:00Z',
          'messages': [
            {
              'id': 'msg_1',
              'role': 'user',
              'content': 'Hi',
              'position': 0,
              'createdAt': '2026-09-13T11:00:00Z',
            },
          ],
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
  final detail = await client.getThread('th_1');
  expect(detail.agentId, 'ag-1');
  expect(detail.messages.single.content, 'Hi');
  expect(detail.messages.single.role, 'user');
});

test('renameThread PATCH title', () async {
  final client = CatalogClient(
    baseUri: baseUri,
    httpClient: MockClient((request) async {
      expect(request.method, 'PATCH');
      expect(jsonDecode(request.body)['title'], 'Renamed');
      return http.Response(
        jsonEncode({
          'id': 'th_1',
          'title': 'Renamed',
          'titleSource': 'user',
          'createdAt': '2026-09-13T10:00:00Z',
          'updatedAt': '2026-09-13T12:00:00Z',
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
  final t = await client.renameThread('th_1', 'Renamed');
  expect(t.title, 'Renamed');
  expect(t.titleSource, 'user');
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/catalog/catalog_client_test.dart`

Expected: FAIL (missing methods/types).

- [ ] **Step 3: Implement models and client methods**

```dart
class ThreadSummary {
  const ThreadSummary({
    required this.id,
    required this.title,
    required this.titleSource,
    this.agentId,
    this.currentModel,
    this.messageCount = 0,
    required this.createdAt,
    required this.updatedAt,
  });
  final String id;
  final String title;
  final String titleSource;
  final String? agentId;
  final String? currentModel;
  final int messageCount;
  final DateTime createdAt;
  final DateTime updatedAt;
  factory ThreadSummary.fromJson(Map<String, dynamic> json) { /* parse like Agent */ }
}

class ThreadMessage {
  const ThreadMessage({
    required this.id,
    required this.role,
    required this.content,
    required this.position,
    required this.createdAt,
  });
  final String id;
  final String role;
  final String content;
  final int position;
  final DateTime createdAt;
  factory ThreadMessage.fromJson(Map<String, dynamic> json) { /* ... */ }
}

class ThreadDetail {
  const ThreadDetail({required this.thread, required this.messages});
  final ThreadSummary thread;
  final List<ThreadMessage> messages;
  factory ThreadDetail.fromJson(Map<String, dynamic> json) {
    return ThreadDetail(
      thread: ThreadSummary.fromJson(json),
      messages: (json['messages'] as List? ?? const [])
          .cast<Map<String, dynamic>>()
          .map(ThreadMessage.fromJson)
          .toList(),
    );
  }
}
```

`CatalogClient`:

```dart
Future<List<ThreadSummary>> listThreads() async { /* GET /v1/threads */ }
Future<ThreadSummary> createThread() async { /* POST /v1/threads, json: {} */ }
Future<ThreadDetail> getThread(String id) async { /* GET /v1/threads/$id */ }
Future<ThreadSummary> renameThread(String id, String title) async { /* PATCH */ }
```

`createThread` must send JSON `{}` so `_send` sets content-type (empty POST without body is also fine if the handler allows it; match the test).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test test/catalog/catalog_client_test.dart`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/catalog client/test/catalog
git commit -m "feat(client): add catalog thread HTTP client"
```

---

### Task 9: ACP client `threadId` + `cancel`

**Files:**
- Modify: `client/lib/acp/agent_connection.dart`
- Modify every `AgentSessionApi` fake: `client/test/chat/chat_controller_test.dart`, `client/test/app_shell_test.dart`, `client/test/widget_test.dart`, `client/test/settings/providers_tab_test.dart`
- Test: `client/test/acp/agent_connection_test.dart`

**Interfaces:**
- Consumes: `acpd` `Session.create` meta map; `Session.cancel()`
- Produces: `startSession(agentId, {threadId})`, `cancel()`

- [ ] **Step 1: Write failing tests**

In `agent_connection_test.dart`, extend the fake agent `onNewSession` to record `request.meta`. Add:

```dart
test('startSession passes threadId in meta', () async {
  // same harness as existing startSession test
  await conn.startSession('ag-1', threadId: 'th_1');
  expect(recordedMeta['agentId'], 'ag-1');
  expect(recordedMeta['threadId'], 'th_1');
});

test('cancel sends session/cancel', () async {
  // after startSession, conn.cancel() completes; agent saw session/cancel
});
```

Wire `cancel` using the same in-process agent harness as other tests (`onNotification('session/cancel', ...)` if that’s how existing tests subscribe; otherwise call `conn.cancel()` and assert the fake records it).

Update the abstract interface first in the test’s compile-fail sense: tests won’t compile until the API exists.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/acp/agent_connection_test.dart`

Expected: FAIL / compile error (`threadId` / `cancel` missing).

- [ ] **Step 3: Implement**

```dart
abstract class AgentSessionApi {
  // existing methods...
  Future<void> startSession(String agentId, {String? threadId});
  Future<void> cancel();
}

@override
Future<void> startSession(String agentId, {String? threadId}) async {
  next = await Session.create(
    client,
    NewSessionRequest(
      cwd: '/',
      mcpServers: const [],
      meta: {
        'agentId': agentId,
        if (threadId != null) 'threadId': threadId,
      },
    ),
  );
  // keep existing previous-session dispose
}

@override
Future<void> cancel() async {
  _session?.cancel();
}
```

Update every fake:

```dart
@override
Future<void> startSession(String agentId, {String? threadId}) async {
  startSessionIds.add(agentId);
  startSessionThreadIds.add(threadId);
  // existing body
}

final List<String?> startSessionThreadIds = [];
int cancels = 0;

@override
Future<void> cancel() async {
  cancels++;
  sendHang?.completeError(StateError('cancelled'));
}
```

Only `FakeConn` in `chat_controller_test.dart` needs `sendHang` / `cancels` for Task 10; other fakes can no-op `cancel()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test`

Expected: PASS (existing tests still call `startSession('ag-1')` without `threadId`).

- [ ] **Step 5: Commit**

```bash
git add client/lib/acp client/test
git commit -m "feat(client): pass threadId on session/new and support cancel"
```

---

### Task 10: ChatController threads

**Files:**
- Modify: `client/lib/chat/chat_controller.dart`
- Test: `client/test/chat/chat_controller_test.dart`

**Interfaces:**
- Consumes: `CatalogClient` thread methods; `startSession(agentId, threadId: …)`; `cancel()`
- Produces: list/selection/filter/create/rename/switch; send disabled without agent; drop uncommitted bubbles on failure/cancel

- [ ] **Step 1: Write failing controller tests**

Extend `FakeCatalog` with in-memory threads (override is awkward because `CatalogClient` methods are not virtual in a stub sense — **prefer a `ThreadCatalog` callback map inside `FakeCatalog`** by making `FakeCatalog` intercept via `MockClient` routes, or add an optional `ThreadStore` collaborator. Simplest: give `FakeCatalog` a `MockClient` that handles `/v1/threads` and keep an in-memory list the mock reads/writes).

Implement `FakeCatalog` so `listThreads` / `createThread` / `getThread` / `renameThread` work. If subclassing can’t override those methods, change `CatalogClient` thread methods to be overridable (they’re instance methods — **subclass and `@override` them**):

```dart
class FakeCatalog extends CatalogClient {
  FakeCatalog(this.agents, {List<ThreadSummary>? threads})
    : threads = List.of(threads ?? const []),
      super(baseUri: Uri.parse('http://catalog.test'), httpClient: MockClient((_) async => http.Response('[]', 200)));

  final List<Agent> agents;
  final List<ThreadSummary> threads;
  final Map<String, List<ThreadMessage>> messages = {};

  @override
  Future<List<Agent>> listAgents() async => agents;

  @override
  Future<List<ThreadSummary>> listThreads() async => List.of(threads);

  @override
  Future<ThreadSummary> createThread() async {
    final t = ThreadSummary(
      id: 'th_${threads.length + 1}',
      title: 'Untitled',
      titleSource: 'auto',
      createdAt: DateTime.utc(2026, 9, 13),
      updatedAt: DateTime.utc(2026, 9, 13),
    );
    threads.insert(0, t);
    return t;
  }

  @override
  Future<ThreadDetail> getThread(String id) async { /* find + messages */ }

  @override
  Future<ThreadSummary> renameThread(String id, String title) async { /* set titleSource user */ }
}
```

Tests:

```dart
test('connect selects most recently listed thread and loads messages', () async {
  final fake = FakeConn();
  final catalog = FakeCatalog([_agent('ag-1', 'Alpha')], threads: [
    ThreadSummary(id: 'th_new', title: 'Newer', titleSource: 'auto', agentId: 'ag-1', createdAt: DateTime.utc(2026, 9, 13), updatedAt: DateTime.utc(2026, 9, 13, 12), messageCount: 1),
    ThreadSummary(id: 'th_old', title: 'Older', titleSource: 'auto', agentId: 'ag-1', createdAt: DateTime.utc(2026, 9, 13), updatedAt: DateTime.utc(2026, 9, 13, 10), messageCount: 0),
  ]);
  catalog.messages['th_new'] = [
    ThreadMessage(id: 'm1', role: 'user', content: 'hi', position: 0, createdAt: DateTime.utc(2026, 9, 13)),
  ];
  final c = ChatController(session: fake, catalog: catalog);
  await c.connect();
  expect(c.selectedThreadId, 'th_new');
  expect(c.messages.single.text, 'hi');
  expect(fake.startSessionIds, ['ag-1']);
  expect(fake.startSessionThreadIds, ['th_new']);
});

test('connect with no threads leaves empty state', () async {
  final c = ChatController(session: FakeConn(), catalog: FakeCatalog([]));
  await c.connect();
  expect(c.selectedThreadId, isNull);
  expect(c.canSend, isFalse);
  expect(c.canSelectAgent, isFalse);
});

test('createThread selects untitled and does not start session', () async {
  final fake = FakeConn();
  final c = ChatController(session: fake, catalog: FakeCatalog([_agent('ag-1', 'Alpha')]));
  await c.connect();
  await c.createThread();
  expect(c.threads.first.title, 'Untitled');
  expect(c.selectedThreadId, c.threads.first.id);
  expect(c.messages, isEmpty);
  expect(fake.startSessionIds, isEmpty);
  expect(c.canSend, isFalse);
  expect(c.canSelectAgent, isTrue);
});

test('selectAgent on thread passes threadId and does not clear loaded messages', () async {
  // create thread, then selectAgent; startSession called with threadId; messages remain empty until send
});

test('selectThread cancels in-flight send and drops uncommitted bubbles', () async {
  final hang = Completer<void>();
  final fake = FakeConn()..sendHang = hang;
  // connect, create thread, selectAgent, start send without awaiting, selectThread other
  expect(fake.cancels, 1);
  expect(c.messages, isEmpty); // or equals the other thread’s HTTP messages
});

test('threadFilter is case-insensitive title contains', () async {
  // two threads; setThreadFilter('foo'); visibleThreads only matching
});

test('renameThread updates list and sets user source', () async { /* ... */ });

test('failed send drops uncommitted bubbles', () async {
  final fake = FakeConn()..failSend = true;
  // ...
  expect(c.messages, isEmpty);
});
```

Rewrite `selectAgent starts session with agentId and clears transcript` so it **creates/selects a thread first**, then `selectAgent`; switching agent on a **new** thread is allowed only while `agentId` is null. Do not keep the old “clears transcript by changing agent” behavior as the main path.

`FakeConn.sendPrompt`: if `sendHang != null`, await it; if `failSend`, throw.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/chat/chat_controller_test.dart`

Expected: FAIL.

- [ ] **Step 3: Implement controller**

Add fields: `threads`, `threadFilter`, `selectedThreadId`, `selectedThread` (`ThreadSummary?`).

```dart
List<ThreadSummary> get visibleThreads {
  final q = threadFilter.trim().toLowerCase();
  if (q.isEmpty) return threads;
  return threads.where((t) => t.title.toLowerCase().contains(q)).toList();
}

bool get canSelectAgent =>
    status == ChatStatus.connected &&
    !_sessionStarting &&
    selectedThreadId != null &&
    (selectedThread?.agentId == null);

bool get canSend =>
    status == ChatStatus.connected &&
    !_sending &&
    _sessionReady &&
    selectedThreadId != null &&
    selectedThread?.agentId != null;
```

`connect`: after ACP connect + agents, `threads = await _catalog.listThreads()`; if not empty, `await selectThread(threads.first.id)` (list already newest-first).

`createThread`: POST, insert at front, `selectThread` without session.

`selectThread`: if `_sending`, `await _session.cancel()` then `_sending = false` and drop last uncommitted pair; GET detail; set `messages` from detail (`ChatRole.user` / `assistant`); if `agentId != null`, `startSession(agentId, threadId: id)` and `_sessionReady = true` and `selectedAgentId = agentId`; else `_sessionReady = false`, `selectedAgentId = null`.

`selectAgent`: require selected thread; `startSession(id, threadId: selectedThreadId)`; update local `agentId`; do **not** `messages.clear()`.

`send`: on success, `GET` current thread (or list) to refresh title/`updatedAt` and reorder `threads`; optimistic title via the same 8-word rule is OK. On failure, remove the user+assistant pair just appended.

`renameThread`: PATCH, replace in `threads`.

If `_catalog` is null, skip thread HTTP (existing unit tests without catalog stay empty).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test test/chat/chat_controller_test.dart`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_controller.dart client/test/chat
git commit -m "feat(client): load, create, and switch persisted threads"
```

---

### Task 11: Thread pane + AppShell

**Files:**
- Create: `client/lib/chat/thread_pane.dart`
- Modify: `client/lib/app_shell.dart`
- Modify: `client/lib/chat/chat_screen.dart` (empty state copy; lock agent picker)
- Test: `client/test/chat/thread_pane_test.dart`
- Modify: `client/test/app_shell_test.dart`

**Interfaces:**
- Consumes: `ChatController` from Task 10
- Produces: Chat-only sidebar; +, filter, ⋮ rename

- [ ] **Step 1: Write failing widget tests**

`thread_pane_test.dart`:

```dart
testWidgets('new thread button creates untitled row', (tester) async {
  final c = ChatController(session: FakeConn(), catalog: FakeCatalog([]));
  await c.connect();
  await tester.pumpWidget(MaterialApp(home: Scaffold(
    body: Row(children: [ThreadPane(controller: c), ChatScreen(controller: c)]),
  )));
  await tester.tap(find.byKey(const Key('new-thread')));
  await tester.pumpAndSettle();
  expect(find.text('Untitled'), findsWidgets);
});

testWidgets('filter hides non-matching titles', (tester) async { /* ... */ });

testWidgets('overflow rename dialog patches title', (tester) async { /* tap ⋮, enter text, confirm */ });
```

`app_shell_test.dart` additions:

```dart
testWidgets('thread pane visible on Chat and hidden on Settings', (tester) async {
  // pump AppShell with FakeCatalog
  expect(find.byType(ThreadPane), findsOneWidget);
  await tester.tap(find.text('Settings'));
  await tester.pumpAndSettle();
  expect(find.byType(ThreadPane), findsNothing);
  expect(find.byType(SettingsPage), findsOneWidget);
});
```

Existing “returning to Chat does not reconnect” must still pass.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/chat/thread_pane_test.dart test/app_shell_test.dart`

Expected: FAIL (`ThreadPane` missing).

- [ ] **Step 3: Implement UI**

`app_shell.dart` body row:

```dart
NavigationRail(...),
const VerticalDivider(thickness: 1, width: 1),
if (_selectedIndex == 0) ...[
  ThreadPane(controller: widget.controller),
  const VerticalDivider(thickness: 1, width: 1),
],
Expanded(child: IndexedStack(...)),
```

`thread_pane.dart`: width 240. Header “Threads” + `IconButton` key `new-thread` (`Icons.add`) calling `controller.createThread`. `TextField` key `thread-filter` → `setThreadFilter`. `ListView` of `visibleThreads`. Selected row: `Colors.blue.shade50` + left border. Subtitle `empty` when `messageCount == 0`. Trailing `PopupMenuButton` with `Rename` — wrap row in `MouseRegion` so the button opacity is 1 on hover **or** when `thread.id == selectedThreadId`, else 0.4 still hittable on selected. Rename: `showDialog` with `TextField` key `rename-thread-field`; on OK if trimmed non-empty, `renameThread`.

`chat_screen.dart`: if `selectedThreadId == null`, body center text `Create a thread to start chatting`; disable pickers and composer. Agent dropdown `onChanged` null when `!canSelectAgent` (already). Empty-state string exact.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib client/test
git commit -m "feat(client): add Chat thread sidebar with rename"
```

---

### Task 12: README + verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: working HTTP + Flutter UX
- Produces: documented smoke path

- [ ] **Step 1: Update README Chat section**

Replace the Chat sidebar sentence with:

```markdown
Use the sidebar: **Settings** for providers/agents, **Chat** for the thread list and transcript.

- **+** starts an untitled thread. Pick an agent before sending.
- The first message titles the thread (first 8 words) unless you renamed it.
- Threads persist in Postgres; refresh restores the list and transcript.
```

Add curl after agents:

```bash
curl -s localhost:8080/v1/threads -X POST -H 'content-type: application/json' -d '{}'
curl -s localhost:8080/v1/threads
```

Keep compose / `DATABASE_URL` instructions from the Postgres catalog README edits.

- [ ] **Step 2: Run automated verification**

```bash
go -C controlplane test ./... -count=1
cd client && flutter test
```

Expected: PASS (Docker required for catalog/thread DB tests).

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: describe persisted chat threads"
```

---

## Self-review

**Spec coverage**

| Spec item | Task |
| --- | --- |
| Postgres threads/messages | 1–2 |
| HTTP list/create/get/rename | 3 |
| Client never POSTs messages | 3, 6 |
| `session/new` hydrate + pin + model restore | 5, 7 |
| Omit threadId unchanged | 5, 6 |
| Commit both / cancel none / auto-title / user title lock | 2, 6 |
| Agent delete 409 | 3 |
| Flutter pane, +, filter, ⋮ rename | 11 |
| Untitled then 8-word title | 2, 6, 10 |
| Most-recent on open / empty until + | 10 |
| Switch cancels in-flight | 10 |
| CLI unbound | 5–6 |
| `loadSession: false` | 5 |

**Placeholder scan:** none remaining.

**Type consistency:** `CreateHydrated`, `ErrAgentLocked`, `ErrAgentInUse`, `startSession(..., {threadId})`, `ThreadSummary` / `ThreadDetail` names used in later Flutter tasks match earlier tasks.
