# controlplane

Go module root (`github.com/tryy3/agent-fabric`). Shared product rules live in the repo-root `AGENTS.md`; this file is local deltas only.

## Commands

Run the server with process cwd containing `sandbox.json` (this directory’s example is typical):

```bash
# from repo root
go -C controlplane run ./cmd/controlplane
# or from this directory
go run ./cmd/controlplane
```

`-addr` and `DATABASE_URL` override engine file listen/DB. Migrations run on startup via `appmigrate`.

```bash
go test ./...                                    # needs postgresql on PATH
go test -tags=integration ./internal/sandbox/docker/   # needs Docker or Podman
```

Regenerate sqlc after editing `internal/db/queries/` or `internal/db/sqlc.yaml` (use the `sqlc` from the Nix shell).

## Constraints

- Package layout: `cmd/controlplane`, `cmd/acp-cli`, logic under `internal/` — do not introduce a second Go module here without an explicit module split.
- Never commit real provider API keys into `data/`, fixtures, or sample JSON.
- Sandbox docker integration tests are build-tagged `integration`; default `go test ./...` skips them.
