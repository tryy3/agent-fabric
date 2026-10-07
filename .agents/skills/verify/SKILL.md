---
name: verify
description: Launch and drive agent-fabric (Go control plane + catalog HTTP /v1 + ACP /acp, Flutter client) in an isolated instance to prove a change works. Use after any controlplane/catalog/ACP change, or when asked to run, smoke-test or confirm behavior in the real app.
---

# Verify agent-fabric

Primary surface: **catalog HTTP (`/v1`) + ACP WebSocket (`/acp`)** on the control plane, driven with `curl` and `acp-cli`. A deterministic fake OpenAI-compatible server replaces the LLM (never call a real provider). Secondary surface: the Flutter client (see `features/`).

All helpers live in this directory; run them from anywhere. Nothing touches the dev stack: separate DB `agentfabric_verify` on the shared Postgres, plane on **:8099**, fake LLM on **:8098**. State/logs in `/tmp/af-verify/run`, evidence in `/tmp/af-verify/evidence` (override with `VERIFY_RUN`, `VERIFY_OUT`, `VERIFY_CP_PORT`, `VERIFY_LLM_PORT`).

## Launch
Prereq: Postgres on :5432 (`docker compose up -d`; check `psql 'postgres://agent:agent@localhost:5432/postgres?sslmode=disable' -c 'select 1'`). Use the repo toolchain (`nix develop` / direnv gives go, psql, flutter).

```bash
.claude/skills/verify/up.sh      # creates DB, builds + starts fake-llm and plane; prints "ready"
.claude/skills/verify/seed.sh    # connection -> models refresh -> assistant; writes $RUN/assistant.id
```
`up.sh` rebuilds the plane binary only when none is answering on the port; after editing Go code run `down.sh` then `up.sh`. Migrations run on plane startup. The plane needs `config.json` in its cwd; `up.sh` writes one into the run dir.

## Doctor
`.claude/skills/verify/doctor.sh` — read-only; checks Postgres, DB, pidfile-owned plane alive, plane and fake LLM answering. Run first whenever anything looks off. Failing plane: `tail /tmp/af-verify/run/controlplane.log`.

## Drive
- Catalog: `curl -s localhost:8099/v1/{assistants,projects,threads,settings,resources,inference/connections,tool/integrations,tools}` (GET/POST/PATCH/DELETE per `internal/catalog/http.go` route table). JSON, camelCase.
- Chat turn over ACP: `.claude/skills/verify/drive-prompt.sh "hello"` → the fake LLM answers `FAKE-REPLY: <your prompt>`, streamed.
- Inference request capture (hop capture): `GET /v1/threads/{id}/captures` for a thread bound by a client that sets `_meta.threadId` (the Flutter app does; `acp-cli` does not, so it leaves no thread row).
- Flutter client (unproven recipe, see features/README.md): `cd client && flutter run -d chrome` or `-d web-server --web-port 8090`; set catalog/ACP to `http://localhost:8099` / `ws://localhost:8099/acp` (runtime URL env in `deploy/README.md`).
- Tool calls / sandbox need Docker or Podman; on this machine both may be broken ("cannot re-exec process to join the existing user namespace"). Then only `kind: local` environments are drivable; say so rather than claiming the sandbox was verified.

## Evidence
Write to `/tmp/af-verify/evidence` (not removed by `down.sh`): the exact curl responses (`curl ... | tee $OUT/x.json`), `drive-prompt.sh` transcripts (saved automatically as `acp-<ts>.txt`), and plane log excerpts (`grep` the line proving the behavior from `$RUN/controlplane.log` and copy it before teardown). A proof shows the action *and* the resulting state: e.g. PATCH response **and** a subsequent GET; a prompt transcript **and** the fake-llm log line `chat stream=true reply=...` in `$RUN/fake-llm.log`. Exercise real routes/ACP, not internal setters; verify persistence (re-GET, DB via `psql $DATABASE_URL`) alongside responses. The only mocked boundary is the LLM.

## Cleanup
`.claude/skills/verify/down.sh` — kills only the pids it recorded, drops `agentfabric_verify`, deletes the run dir. Also run it after any failed attempt. Never `pkill` by name; never touch :8080 or database `agentfabric` (the user's dev stack).

## Helpers
`up.sh`, `doctor.sh`, `seed.sh`, `drive-prompt.sh "<text>"`, `down.sh`, `env.sh` (sourced), `fake-llm/main.go` (stdlib Go; `/v1/models`, `/v1/chat/completions` stream and non-stream).

## Feature map
Per-feature recipes: [features/README.md](features/README.md). A proof that drives one entry point is incomplete when the map lists others.
