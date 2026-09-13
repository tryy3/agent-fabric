# Postgres Catalog Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist the providers/agents catalog in Postgres (pgx + sqlc + goose), with docker compose for local DB and `DATABASE_URL` for any Postgres instance.

**Architecture:** Controlplane opens a `pgxpool` from `DATABASE_URL`, runs embedded goose migrations on startup, and uses sqlc-generated queries from a rewritten `catalog.Store`. JSON/`-data-dir` catalog storage is removed. Tests use an ephemeral local Postgres started via the Nix `postgresql` package (`initdb`/`postgres`) — no Docker/testcontainers required for `go test`.

**Tech Stack:** Go 1.22+, `jackc/pgx/v5`, `sqlc`, `pressly/goose/v3`, Docker Compose Postgres 16 (manual/dev), Nix `sqlc` + `goose` + `postgresql` (test fixture + tooling).

## Global Constraints

- Hard cutover: no JSON fallback, dual-write, or import from `providers.json` / `agents.json`.
- Required config: `DATABASE_URL` (standard Postgres URL).
- Goose migrations run on controlplane startup; fail fast if migrate/connect fails.
- Keep catalog HTTP JSON shapes and status-code behavior (201 create, 409 provider in use, etc.).
- Chat/history persistence is out of scope.
- API keys remain plaintext in DB for this slice.
- Module path: `github.com/tryy3/agent-fabric` (module root `controlplane/`).
- Preserve existing ID prefixes (`prov_`, `agent_`) and domain validation rules.
- Follow TDD: failing test → implement → pass → commit per task.
- Prefer small packages; put DB infrastructure in `internal/db`, domain store in `internal/catalog`.

## File Structure

| Path | Responsibility |
| --- | --- |
| `docker-compose.yml` | Local Postgres 16 + volume + healthcheck |
| `flake.nix` | Add `sqlc`, `goose` (optional `postgresql` client) |
| `.gitignore` | Ignore local `data/` leftover JSON dir |
| `controlplane/internal/db/migrations/00001_catalog.up.sql` | Create `providers` / `agents` |
| `controlplane/internal/db/migrations/00001_catalog.down.sql` | Drop tables |
| `controlplane/internal/db/queries/providers.sql` | sqlc provider CRUD |
| `controlplane/internal/db/queries/agents.sql` | sqlc agent CRUD |
| `controlplane/internal/db/sqlc.yaml` | sqlc config (pgx/v5) |
| `controlplane/internal/db/*.go` | Generated sqlc code (committed) |
| `controlplane/internal/db/migrate.go` | Embed migrations; `Migrate(ctx, databaseURL)` |
| `controlplane/internal/db/pool.go` | `OpenPool(ctx, databaseURL) (*pgxpool.Pool, error)` |
| `controlplane/internal/db/migrate_test.go` | Migrate smoke test via local Postgres fixture |
| `controlplane/internal/db/dbtest/postgres.go` | Shared ephemeral local-Postgres helper for other packages |
| `controlplane/internal/catalog/store.go` | Postgres-backed store (rewrite) |
| `controlplane/internal/catalog/store_test.go` | CRUD tests against local Postgres fixture |
| `controlplane/internal/catalog/http.go` | Pass `r.Context()`; handle list/get errors |
| `controlplane/internal/catalog/refresh.go` | Context-aware Get/Replace |
| `controlplane/internal/agent/agent.go` | Context-aware catalog lookups in `pinFromCatalog` |
| `controlplane/cmd/controlplane/main.go` | `DATABASE_URL`; migrate; drop `-data-dir` |
| `README.md` | Compose + `DATABASE_URL` run instructions |

**Store method signatures after this plan** (all catalog callers must match):

```go
func Open(pool *pgxpool.Pool) *Store

func (s *Store) ListProviders(ctx context.Context) ([]Provider, error)
func (s *Store) GetProvider(ctx context.Context, id string) (Provider, error) // not found: `provider %q not found`
func (s *Store) CreateProvider(ctx context.Context, name, typ, baseURL, apiKey string) (Provider, error)
func (s *Store) UpdateProvider(ctx context.Context, id string, name, baseURL, apiKey *string) (Provider, error)
func (s *Store) DeleteProvider(ctx context.Context, id string) error
func (s *Store) ReplaceProviderModels(ctx context.Context, id string, models []ModelInfo, updatedAt time.Time) (Provider, error)

func (s *Store) ListAgents(ctx context.Context) ([]Agent, error)
func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) // not found: `agent %q not found`
func (s *Store) CreateAgent(ctx context.Context, name, description, providerID, defaultModel string) (Agent, error)
func (s *Store) UpdateAgent(ctx context.Context, id string, name, description, providerID, defaultModel *string) (Agent, error)
func (s *Store) DeleteAgent(ctx context.Context, id string) error

func (s *Store) RefreshModels(ctx context.Context, id string, client *http.Client) (Provider, error) // existing; update internals
```

---

### Task 1: Compose, Nix tooling, gitignore

**Files:**
- Create: `docker-compose.yml`
- Modify: `flake.nix`
- Modify: `.gitignore`
- Test: shell verification (no Go test yet)

**Interfaces:**
- Consumes: none
- Produces: local Postgres via compose; Nix packages `sqlc` and `goose` available in `devShell`

- [ ] **Step 1: Add docker-compose Postgres**

Create `docker-compose.yml` at repo root:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: agent
      POSTGRES_PASSWORD: agent
      POSTGRES_DB: agentfabric
    ports:
      - "5432:5432"
    volumes:
      - agentfabric_pg:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U agent -d agentfabric"]
      interval: 2s
      timeout: 5s
      retries: 10

volumes:
  agentfabric_pg:
```

- [ ] **Step 2: Add sqlc and goose to the Nix flake**

In `flake.nix`, extend `packages` to:

```nix
packages = with pkgs; [
  go
  gopls
  flutter
  chromium
  sqlc
  goose
  postgresql
];
```

- [ ] **Step 3: Ignore local JSON data dir leftovers**

Append to `.gitignore`:

```gitignore
# Local catalog data (legacy JSON dir; unused after Postgres)
/data/
```

- [ ] **Step 4: Verify compose and tools**

```bash
docker compose up -d
docker compose ps
# enter nix/direnv shell if needed
sqlc version
goose -version
```

Expected: Postgres healthy; `sqlc` and `goose` print versions.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml flake.nix flake.lock .gitignore
git commit -m "$(cat <<'EOF'
Add Postgres compose and sqlc/goose Nix tooling.

EOF
)"
```

Note: if `flake.lock` changes after a flake edit, include it.

---

### Task 2: DB package — pool, migrate, local Postgres test helper

**Files:**
- Create: `controlplane/internal/db/migrations/00001_catalog.up.sql`
- Create: `controlplane/internal/db/migrations/00001_catalog.down.sql`
- Create: `controlplane/internal/db/migrate.go`
- Create: `controlplane/internal/db/pool.go`
- Create: `controlplane/internal/db/dbtest/postgres.go`
- Create: `controlplane/internal/db/migrate_test.go`
- Modify: `controlplane/go.mod` / `go.sum` (via `go get`)

**Interfaces:**
- Consumes: none
- Produces:
  - `db.Migrate(ctx context.Context, databaseURL string) error`
  - `db.OpenPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error)`
  - `dbtest.Open(t testing.TB) *pgxpool.Pool` — starts ephemeral local Postgres (`initdb`/`postgres` from PATH / Nix), migrates, returns pool; registers `t.Cleanup`

- [ ] **Step 1: Add dependencies**

```bash
cd /home/tryy3/src/agent-fabric/controlplane
go get github.com/jackc/pgx/v5@v5.7.4
go get github.com/jackc/pgx/v5/stdlib@v5.7.4
go get github.com/pressly/goose/v3@v3.24.1
go mod tidy
```

Do **not** add testcontainers. DB tests use the Nix `postgresql` binaries already in the flake.

- [ ] **Step 2: Write migration SQL**

`controlplane/internal/db/migrations/00001_catalog.up.sql`:

```sql
CREATE TABLE providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    models JSONB NOT NULL DEFAULT '[]'::jsonb,
    models_updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version INT NOT NULL,
    provider_id TEXT NOT NULL REFERENCES providers (id) ON DELETE RESTRICT,
    default_model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX agents_provider_id_idx ON agents (provider_id);
```

`controlplane/internal/db/migrations/00001_catalog.down.sql`:

```sql
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS providers;
```

- [ ] **Step 3: Implement migrate + pool**

`controlplane/internal/db/migrate.go`:

```go
package db

import (
	"context"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func Migrate(ctx context.Context, databaseURL string) error {
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	gdb, err := goose.OpenDBWithDriver("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db for migrate: %w", err)
	}
	defer gdb.Close()

	if err := gdb.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, gdb, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
```

`controlplane/internal/db/pool.go`:

```go
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pool: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 4: Write local Postgres test helper**

`controlplane/internal/db/dbtest/postgres.go`:

Implement `Open(t testing.TB) *pgxpool.Pool` that:
1. Requires `initdb` and `postgres` on `PATH` (provided by Nix flake `postgresql`).
2. Creates a temp data directory via `t.TempDir()`.
3. Runs `initdb` with trust auth for local connections (user `agent`).
4. Picks a free TCP port on `127.0.0.1` and starts `postgres` bound only to localhost (and a unix socket dir under the temp dir).
5. Waits until accepting connections, creates DB `agentfabric` if needed.
6. Calls `db.Migrate` then `db.OpenPool` with `postgres://agent@127.0.0.1:PORT/agentfabric?sslmode=disable`.
7. Registers `t.Cleanup` to stop the postgres process and close the pool.

Do not use testcontainers or Docker. Prefer stdlib `os/exec` + `net` only.

- [ ] **Step 5: Write failing/smoke migrate test**

`controlplane/internal/db/migrate_test.go`:

```go
package db_test

import (
	"context"
	"testing"

	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestMigrateCreatesCatalogTables(t *testing.T) {
	pool := dbtest.Open(t)
	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN ('providers', 'agents')
	`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 tables, got %d", n)
	}
}
```

- [ ] **Step 6: Run test**

```bash
# from nix develop (postgresql on PATH)
go -C controlplane test ./internal/db/...
```

Expected: PASS (Nix `postgresql` required; Docker not required).

- [ ] **Step 7: Commit**

```bash
git add controlplane/go.mod controlplane/go.sum \
  controlplane/internal/db/
git commit -m "$(cat <<'EOF'
Add Postgres migrate/pool helpers and local Postgres test fixture.

EOF
)"
```

---

### Task 3: sqlc queries and generate

**Files:**
- Create: `controlplane/internal/db/sqlc.yaml`
- Create: `controlplane/internal/db/queries/providers.sql`
- Create: `controlplane/internal/db/queries/agents.sql`
- Create: generated Go under `controlplane/internal/db/` (via `sqlc generate`)

**Interfaces:**
- Consumes: migration DDL (as sqlc schema source: `migrations/*.up.sql`)
- Produces: sqlc `Queries` methods used by catalog store (exact names below)

Required query names:

| Query | Purpose |
| --- | --- |
| `ListProviders` | all providers ordered by `created_at` |
| `GetProvider` | by id |
| `InsertProvider` | insert full row |
| `UpdateProvider` | update name/base_url/api_key/updated_at |
| `DeleteProvider` | by id |
| `UpdateProviderModels` | set models jsonb + models_updated_at + updated_at |
| `ListAgents` | all agents ordered by `created_at` |
| `GetAgent` | by id |
| `InsertAgent` | insert full row |
| `UpdateAgent` | update mutable fields + version + updated_at |
| `DeleteAgent` | by id |
| `CountAgentsByProvider` | for in-use check before delete |

- [ ] **Step 1: Add sqlc.yaml**

`controlplane/internal/db/sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries"
    schema: "migrations"
    gen:
      go:
        package: "db"
        out: "."
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
```

If sqlc chokes on `.down.sql` files, set schema to only up files, e.g. `schema: "migrations/00001_catalog.up.sql"`.

- [ ] **Step 2: Write provider queries**

`controlplane/internal/db/queries/providers.sql`:

```sql
-- name: ListProviders :many
SELECT id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
FROM providers
ORDER BY created_at ASC;

-- name: GetProvider :one
SELECT id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
FROM providers
WHERE id = $1;

-- name: InsertProvider :one
INSERT INTO providers (
  id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: UpdateProvider :one
UPDATE providers
SET
  name = $2,
  base_url = $3,
  api_key = $4,
  updated_at = $5
WHERE id = $1
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: UpdateProviderModels :one
UPDATE providers
SET
  models = $2,
  models_updated_at = $3,
  updated_at = $4
WHERE id = $1
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: DeleteProvider :exec
DELETE FROM providers WHERE id = $1;
```

- [ ] **Step 3: Write agent queries**

`controlplane/internal/db/queries/agents.sql`:

```sql
-- name: ListAgents :many
SELECT id, name, description, version, provider_id, default_model, created_at, updated_at
FROM agents
ORDER BY created_at ASC;

-- name: GetAgent :one
SELECT id, name, description, version, provider_id, default_model, created_at, updated_at
FROM agents
WHERE id = $1;

-- name: InsertAgent :one
INSERT INTO agents (
  id, name, description, version, provider_id, default_model, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, name, description, version, provider_id, default_model, created_at, updated_at;

-- name: UpdateAgent :one
UPDATE agents
SET
  name = $2,
  description = $3,
  version = $4,
  provider_id = $5,
  default_model = $6,
  updated_at = $7
WHERE id = $1
RETURNING id, name, description, version, provider_id, default_model, created_at, updated_at;

-- name: DeleteAgent :exec
DELETE FROM agents WHERE id = $1;

-- name: CountAgentsByProvider :one
SELECT COUNT(*)::bigint FROM agents WHERE provider_id = $1;
```

- [ ] **Step 4: Generate**

```bash
cd /home/tryy3/src/agent-fabric/controlplane/internal/db
sqlc generate
```

Expected: Go files created (`models.go`, `db.go`, `providers.sql.go`, `agents.sql.go` or similar). No sqlc errors.

- [ ] **Step 5: Verify package builds**

```bash
go -C controlplane build ./internal/db/...
```

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add controlplane/internal/db/
git commit -m "$(cat <<'EOF'
Add sqlc queries and generated Postgres catalog accessors.

EOF
)"
```

---

### Task 4: Rewrite `catalog.Store` on Postgres (TDD)

**Files:**
- Rewrite: `controlplane/internal/catalog/store.go`
- Modify: `controlplane/internal/catalog/store_test.go`
- Delete JSON helpers (`atomicWrite`, file structs) as part of the rewrite

**Interfaces:**
- Consumes: `*pgxpool.Pool`, `db.New(pool)` / `*db.Queries`, query methods from Task 3
- Produces: store API signatures listed in File Structure

- [ ] **Step 1: Update store tests to use dbtest (expect failures)**

Replace every `catalog.Open(t.TempDir())` / `catalog.Open(dir)` with:

```go
pool := dbtest.Open(t)
store := catalog.Open(pool)
```

Update method calls to pass `context.Background()` (or `t.Context()` on Go 1.24+ — this repo is 1.22, so use `context.Background()`).

Change re-open persistence check in `TestProviderCRUDRoundTrip` from re-`Open(dir)` to:

```go
store2 := catalog.Open(pool) // same DB; proves rows survive a new Store handle
```

Remove assertions about `providers.json` paths.

Change `GetProvider` / `GetAgent` call sites from `(val, ok)` to error checks:

```go
got, err := store.GetProvider(ctx, p.ID)
if err != nil {
  t.Fatalf("GetProvider: %v", err)
}
```

For not-found cases, assert `err != nil` and message/`errors.Is` as appropriate.

- [ ] **Step 2: Run tests — expect compile/fail**

```bash
go -C controlplane test ./internal/catalog/ -run 'TestProvider|TestCreate|TestUpdate|TestDelete|TestReplace' -count=1
```

Expected: fail to compile or fail because JSON `Open(string)` is gone / signatures mismatch.

- [ ] **Step 3: Implement Postgres `store.go`**

Core shape:

```go
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tryy3/agent-fabric/internal/db"
)

type Store struct {
	q *db.Queries
}

func Open(pool *pgxpool.Pool) *Store {
	return &Store{q: db.New(pool)}
}
```

Mapping rules:

- `models` column: marshal/unmarshal `[]ModelInfo` as JSONB (`json.Marshal` / `json.Unmarshal` into `[]byte` or use sqlc’s `[]byte` / `pgtype`).
- `GetProvider` / `GetAgent`: on `errors.Is(err, pgx.ErrNoRows)` return `fmt.Errorf("provider %q not found", id)` / `agent %q not found`.
- Keep validation identical to current JSON store (empty name/baseURL/apiKey, known type, model must be in cache, version increments on agent update, trim trailing `/` on base URL, ID via `newID("prov_")` / `newID("agent_")`).
- `DeleteProvider`: if `CountAgentsByProvider > 0`, return `ErrProviderInUse`; else delete. Also map Postgres FK violation (`23503`) to `ErrProviderInUse` if a race hits the FK.
- `ReplaceProviderModels`: load agents for provider (via `ListAgents` filter in Go, or add a sqlc query `ListAgentsByProvider` if cleaner — prefer adding `ListAgentsByProvider :many` in this task if filtering all agents is awkward), run the same orphan-default rejection, then `UpdateProviderModels`.
- No package-level mutex; rely on SQL.

Optional sqlc addition in this task if needed:

```sql
-- name: ListAgentsByProvider :many
SELECT id, name, description, version, provider_id, default_model, created_at, updated_at
FROM agents
WHERE provider_id = $1;
```

Re-run `sqlc generate` if added.

Helper to map DB row → `Provider` / `Agent` (handle null `models_updated_at`).

- [ ] **Step 4: Run catalog store tests**

```bash
go -C controlplane test ./internal/catalog/ -run 'TestProvider|TestCreate|TestUpdate|TestDelete|TestReplace' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/store.go controlplane/internal/catalog/store_test.go \
  controlplane/internal/db/
git commit -m "$(cat <<'EOF'
Persist catalog providers and agents in Postgres via sqlc.

EOF
)"
```

---

### Task 5: HTTP + refresh call sites

**Files:**
- Modify: `controlplane/internal/catalog/http.go`
- Modify: `controlplane/internal/catalog/http_test.go`
- Modify: `controlplane/internal/catalog/refresh.go`
- Modify: `controlplane/internal/catalog/refresh_test.go`

**Interfaces:**
- Consumes: context-aware store methods from Task 4
- Produces: same HTTP responses as before

- [ ] **Step 1: Update HTTP handlers**

Pass `r.Context()` into all store calls. Examples:

```go
func (h *httpAPI) listProviders(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListProviders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) getProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetProvider(r.Context(), id)
	if err != nil {
		if isNotFoundFor(err, "provider", id) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}
```

Apply the same pattern to agents and to create/update/delete (already error-based). Keep `writeMappedError` behavior.

- [ ] **Step 2: Update `refresh.go`**

```go
p, err := s.GetProvider(ctx, id)
if err != nil {
  return Provider{}, err
}
// ... fetch models ...
return s.ReplaceProviderModels(ctx, id, models, time.Now().UTC())
```

- [ ] **Step 3: Update http/refresh tests to `dbtest.Open`**

Shared pattern at top of each test:

```go
store := catalog.Open(dbtest.Open(t))
```

Update all store method calls with `context.Background()`.

- [ ] **Step 4: Run catalog package tests**

```bash
go -C controlplane test ./internal/catalog/ -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/
git commit -m "$(cat <<'EOF'
Wire catalog HTTP and model refresh to Postgres store contexts.

EOF
)"
```

---

### Task 6: Agent, server tests, and controlplane main

**Files:**
- Modify: `controlplane/internal/agent/agent.go`
- Modify: `controlplane/internal/agent/agent_test.go` (and any other agent tests using `catalog.Open`)
- Modify: `controlplane/internal/server/server_test.go`
- Modify: `controlplane/cmd/controlplane/main.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `db.Migrate`, `db.OpenPool`, `catalog.Open(pool)`
- Produces: controlplane that boots only with `DATABASE_URL`

- [ ] **Step 1: Update `pinFromCatalog`**

`pinFromCatalog` needs a context. Prefer threading the request/session context from the ACP method that calls it (e.g. `NewSession` already has `ctx context.Context`). Change signature to:

```go
func (a *Agent) pinFromCatalog(ctx context.Context, meta map[string]any) (runtime.SessionPin, error) {
	// ...
	ag, err := a.catalog.GetAgent(ctx, agentID)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	p, err := a.catalog.GetProvider(ctx, ag.ProviderID)
	if err != nil {
		return runtime.SessionPin{}, err
	}
	// ... rest unchanged
}
```

Update the caller(s) inside `agent.go` to pass `ctx`.

- [ ] **Step 2: Update agent/server tests**

In `seedCatalog` and friends:

```go
cat := catalog.Open(dbtest.Open(t))
```

Pass contexts into CreateProvider / ReplaceProviderModels / CreateAgent / GetProvider.

- [ ] **Step 3: Rewrite `cmd/controlplane/main.go`**

```go
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/runtime"
	"github.com/tryy3/agent-fabric/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	databaseURL := os.Getenv("DATABASE_URL")
	ctx := context.Background()
	if err := db.Migrate(ctx, databaseURL); err != nil {
		log.Fatal(err)
	}
	pool, err := db.OpenPool(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	cat := catalog.Open(pool)
	store := runtime.NewStore()
	srv := server.New(*addr, store, cat)
	slog.Info("controlplane listening", "addr", *addr, "acp", "/acp", "catalog", "/v1")
	log.Fatal(srv.ListenAndServe())
}
```

- [ ] **Step 4: Update README run instructions**

Replace `-data-dir` / JSON catalog notes with:

```bash
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane
```

Keep the curl provider/agent examples. Note that pointing `DATABASE_URL` at any Postgres works.

- [ ] **Step 5: Run full controlplane tests**

```bash
go -C controlplane test ./... -count=1
```

Expected: all PASS (Nix `postgresql` / local fixture required for DB tests; Docker not required).

- [ ] **Step 6: Manual smoke (optional but recommended)**

```bash
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane
# in another shell: curl create provider / refresh / create agent as in README
```

Expected: catalog CRUD works; process logs listen without `data_dir`.

- [ ] **Step 7: Commit**

```bash
git add controlplane/internal/agent/ controlplane/internal/server/server_test.go \
  controlplane/cmd/controlplane/main.go README.md
git commit -m "$(cat <<'EOF'
Boot controlplane from DATABASE_URL and drop JSON catalog storage.

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| docker compose Postgres | 1 |
| `DATABASE_URL` config | 6 |
| pgx + sqlc | 2, 3, 4 |
| goose on startup | 2, 6 |
| Hard cutover / no JSON import | 4, 6 |
| providers + agents schema | 2 |
| HTTP behavior preserved | 5 |
| local Postgres fixture tests | 2, 4, 5, 6 |
| Nix sqlc/goose | 1 |
| README | 6 |
| No history / encryption / auth | (explicitly omitted) |

## Plan self-review notes

- No TBD placeholders; store signatures are fixed in the File Structure section.
- ID prefixes intentionally remain `prov_` / `agent_` (current code), not the informal `pr-` / `ag-` examples in the design doc.
- If sqlc cannot take the whole `migrations/` dir because of `.down.sql`, point schema at the `.up.sql` file only (called out in Task 3).
