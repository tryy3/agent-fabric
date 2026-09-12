# Postgres catalog persistence

**Date:** 2026-09-12  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-12-dynamic-agents-providers-settings-design.md](./2026-09-12-dynamic-agents-providers-settings-design.md)

## Problem

The catalog (providers and agents) persists as JSON under `-data-dir`. That was fine for a quick local slice, but it does not match where the project is going: a self-hosted control plane that will also hold chat history later, and that should run cleanly on a personal server.

JSON has no real schema migrations, weak concurrency story, and is a dead end for querying history and related data. We need a durable store that is privacy-friendly (self-hosted), configurable, and easy to point at either a local Docker Postgres or a remote instance.

## Goals

- Persist providers and agents in **Postgres**.
- Local/dev: **docker compose** spins up Postgres.
- Remote/server: same app, different **`DATABASE_URL`**.
- Access layer: **pgx** + **sqlc** (typed SQL, no ORM).
- Schema changes: **goose**, applied **on controlplane startup**.
- Hard cutover: remove JSON catalog storage; **no import** from existing `providers.json` / `agents.json`.
- Keep catalog HTTP API behavior the same (`/v1/providers`, `/v1/agents`, model refresh).
- Hermetic catalog tests via an **ephemeral local Postgres** started with Nix `postgresql` (`initdb`/`postgres`); Docker not required for `go test`.

## Non-goals

- Persisted chat / thread / message history (follow-up)
- Encrypting API keys at rest (still plaintext in DB, same as JSON today)
- Auth / multi-user / tenancy
- JSON fallback or dual-write mode
- One-shot migration from existing JSON files
- Object/file blob storage
- Connection poolers, replicas, or managed-cloud-only features
- Non-Docker local Postgres as a first-class path in this slice (compose is the documented local path)

## Approach

**Chosen:** Postgres catalog store + compose (Approach 1).

- Single required config: `DATABASE_URL`.
- Embed goose migrations; migrate before serving.
- sqlc generates typed queries; `catalog.Store` uses them over a `pgxpool`.
- Drop `-data-dir` for catalog (remove the flag if nothing else uses it).

**Rejected:**

- Dual JSON/Postgres mode or import path — still early; hard cutover is simpler.
- ORM (GORM/Ent) — heavier than needed; sqlc keeps SQL explicit.
- History tables in this slice — ACP session vs durable thread semantics are not settled yet.
- Separate migrate-only CLI as the primary path — startup migrate keeps “one command to run” for personal use.

## Architecture

```text
docker compose          controlplane
──────────────          ────────────
  postgres ◄──────────  DATABASE_URL (pgx pool)
                            │
                     goose migrate (on startup)
                            │
                     sqlc queries
                            │
                     catalog.Store (Postgres)
                            │
              HTTP /v1/providers · /v1/agents
              (ACP runtime unchanged; sessions still in-memory)
```

| Layer | Role |
| --- | --- |
| Compose Postgres | Local/dev database with a named volume and healthcheck |
| `DATABASE_URL` | Only connection config; same for compose or remote |
| goose | Versioned SQL schema; embedded and applied at process start |
| sqlc | Typed CRUD for providers and agents |
| catalog.Store | Domain API used by HTTP handlers; no JSON files |
| ACP / runtime | Unchanged; transcripts remain in-memory for now |

## Data model

### `providers`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `text` PK | e.g. `pr-…` |
| `name` | `text` | |
| `type` | `text` | e.g. `openai_compatible` |
| `base_url` | `text` | |
| `api_key` | `text` | plaintext in this slice |
| `models` | `jsonb` | cached `[{id, name}, …]` |
| `models_updated_at` | `timestamptz` nullable | |
| `created_at` | `timestamptz` | |
| `updated_at` | `timestamptz` | |

### `agents`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `text` PK | e.g. `ag-…` |
| `name` | `text` | |
| `description` | `text` | default `''` |
| `version` | `int` | |
| `provider_id` | `text` FK → `providers(id)` | |
| `default_model` | `text` | |
| `created_at` | `timestamptz` | |
| `updated_at` | `timestamptz` | |

**Integrity:** Deleting a provider that still has agents fails with the same domain error as today (`ErrProviderInUse`), enforced via FK/check in the store layer.

**Concurrency:** Prefer DB transactions over the previous in-process mutex + full-file rewrite.

## Components & layout

```text
docker-compose.yml
controlplane/
  cmd/controlplane/          # require DATABASE_URL; migrate; serve
  internal/db/               # migrations (embed), sqlc config + generated code, pool helpers
  internal/catalog/          # Store on sqlc; HTTP unchanged in behavior
flake.nix                    # add sqlc, goose (+ optional psql client)
```

### Compose

- Postgres 16 service, port `5432`, named volume, healthcheck.
- Example credentials: user/pass/db `agent` / `agent` / `agentfabric` (exact values documented in README).
- Documented flow: start compose → export `DATABASE_URL` → `go -C controlplane run ./cmd/controlplane`.

### Startup sequence

1. Parse `DATABASE_URL` — missing or invalid → exit with a clear error.
2. Open `pgxpool` — fail if unreachable.
3. Run goose up — fail if migrate fails.
4. Wire catalog store and serve HTTP + ACP as today.

### Tooling workflow

- Edit goose SQL + sqlc query files → run `sqlc generate` → commit generated Go.
- Normal runs do not require a separate goose CLI; the binary migrates itself.
- Nix `devShell` provides `sqlc` and `goose` for local generation and optional manual migrate/debug.

## Errors and lifecycle

- Unreachable DB or failed migration: process exits; do not serve a half-ready API.
- Catalog validation and HTTP status codes stay aligned with the current API.
- WebSocket/ACP disconnect behavior unchanged (no catalog impact).

## Testing

- Catalog store and HTTP tests use an **ephemeral local Postgres** (Nix `postgresql`) so `go test ./...` stays hermetic without Docker.
- No JSON catalog fallback in production or tests.
- CI needs the Nix `postgresql` package on PATH (via flake `devShell`); Docker Compose remains for manual/dev runs only.

## Verification

**Manual**

```text
docker compose up -d
export DATABASE_URL='postgres://agent:agent@localhost:5432/agentfabric?sslmode=disable'
go -C controlplane run ./cmd/controlplane
# existing curl catalog flows against /v1/providers and /v1/agents
```

**Automated**

- Store CRUD + provider-in-use delete behavior against local Postgres fixture.
- HTTP catalog tests against the same store path.
- `go test ./...` with no external API keys; Nix `postgresql` / local fixture required for DB tests (no Docker).

## Success criteria

1. Compose brings up a usable Postgres instance.
2. Controlplane starts only with a valid `DATABASE_URL`, migrates, and serves.
3. Providers and agents CRUD + model refresh work against Postgres.
4. JSON catalog files and `-data-dir` catalog usage are gone.
5. Pointing `DATABASE_URL` at a non-compose Postgres works without code changes.
6. Catalog tests pass via the local Postgres fixture.

## Follow-ups (explicitly later)

- Durable chat history (threads / messages)
- Encrypt secrets at rest
- Auth / multi-user
- Nix-friendly non-Docker Postgres for day-to-day coding (optional ergonomics)
- File / blob storage choice when attachments appear
