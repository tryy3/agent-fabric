# Resources and Environments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace sandbox-overlay containers with catalog container resources that a global environment and a project environment select, so projects share a machine by linking the same resource and keep that resource when the project is deleted.

**Architecture:** `resources` rows hold the container spec (image, name, idle timeout, volumes). Environment JSON on `plane_settings` and `projects.settings` holds the resource id, workspace root, grant overrides, and extra paths. Attach resolves project over global, then starts that container. A Go copy between goose `00010` and `00011` turns today's overlays and `environments` rows into resources. Settings gains Resources and Environment tabs and drops the Sandbox tab.

**Tech Stack:** Go (pgx, sqlc, goose), Flutter, Postgres via `dbtest`. Commands run through `nix develop /home/tryy3/src/agent-fabric -c`.

## Global Constraints

- Kind `container` is the only accepted kind. SSH and VM are not implemented.
- Resource ids are `res_` + 16 hex. Volume ids are `vol_` + 16 hex. Extra path ids are `path_` + 16 hex. Do not reuse `vol_workspace`.
- A project links one resource. Missing project `resourceId` uses the global default. Threads and agents do not override the resource, workspace root, or grants.
- Container names are unique across other resources. Volume names are unique across other resources, including disabled volumes. Mount targets are unique within one container.
- On write, a container or volume name that does not already start with the engine identity prefix is stored with that prefix prepended. Attach uses the stored name as-is.
- `image` is required. Omitted `idleTTLSeconds` is stored as 3600. A present idle timeout must be a positive integer. A container may have zero volumes.
- Deleting a project removes the link and does not delete the resource or any Docker object. Deleting a linked resource, or the global default, is `409` `{ "error": "resource in use" }`.
- Unknown resource id on GET or DELETE is `404` `{ "error": "resource \"<id>\" not found" }`.
- Attach errors: `project "<id>" has no resource`, `resource "<id>" not found`, `no enabled volume targets workspace root "<path>"`, `container "<name>" is running with a different image or mount list`.
- Do not edit `controlplane/internal/db/migrations/00006_projects.up.sql`.
- `db.Migrate` is not the place that runs the copy. `main` and `dbtest.Open` call `appmigrate.RunMigrations` (Task 7), which applies goose through `00010`, seeds plane settings, copies, then applies the rest.
- File grants stay deny-by-default. A grant override whose volume id is not on the resolved resource is ignored at resolve time.

## File map

| File | Responsibility |
| --- | --- |
| `controlplane/internal/db/migrations/00010_resources.up.sql` | Create `resources`, add `plane_settings.environment` |
| `controlplane/internal/db/migrations/00011_drop_environments.up.sql` | Drop isolation, `environment_id`, `environments` |
| `controlplane/internal/db/queries/resources.sql` | Resource CRUD |
| `controlplane/internal/catalog/resources.go` | Validate, prefix, uniqueness, in-use delete |
| `controlplane/internal/catalog/environment.go` | Environment JSON merge and `ResolveEnvironment` |
| `controlplane/internal/catalog/backfill.go` | Copy overlays and environment rows into resources |
| `controlplane/internal/appmigrate/migrate.go` | `RunMigrations` order |
| `controlplane/internal/agent/agent.go` | Attach from the resolved environment |
| `controlplane/internal/workspace/open.go` | Same attach path for the Files pane |
| `client/lib/settings/resources_tab.dart` | Resource pool UI |
| `client/lib/settings/environment_tab.dart` | Global environment UI |
| `client/lib/settings/projects_tab.dart` | Project environment override |

---

### Task 1: Add the resources table and environment column

**Files:**
- Create: `controlplane/internal/db/migrations/00010_resources.up.sql`
- Create: `controlplane/internal/db/migrations/00010_resources.down.sql`
- Create: `controlplane/internal/db/queries/resources.sql`
- Modify: `controlplane/internal/db/queries/settings.sql`
- Modify: `controlplane/internal/db/models.go` (sqlc generate)
- Modify: `controlplane/internal/catalog/overlay.go`
- Modify: `controlplane/internal/catalog/settings.go`
- Test: `controlplane/internal/db/migrate_test.go`

**Interfaces:**
- Consumes: goose through `00009`; `planeSettingsFromDB`; `GetPlaneSettings` / `InsertPlaneSettings` / `UpdatePlaneSettings`.
- Produces: sqlc `InsertResource`, `GetResource`, `ListResources`, `UpdateResource`, `DeleteResource`. `db.PlaneSetting.Environment []byte`. `catalog.PlaneSettings.Environment json.RawMessage`. Resource queries are unused until Task 2.

- [ ] **Step 1: Write the failing migration test**

Add to `controlplane/internal/db/migrate_test.go`:

```go
func TestMigrateAddsResources(t *testing.T) {
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.MigrateTo(ctx, url, 9); err != nil {
		t.Fatalf("migrate to 9: %v", err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var n int
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM information_schema.tables
WHERE table_schema = 'public' AND table_name = 'resources'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("resources existed before 00010: %d", n)
	}

	if err := db.Migrate(ctx, url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM information_schema.columns
WHERE table_name = 'plane_settings' AND column_name = 'environment'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("environment columns = %d", n)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/db/ -run TestMigrateAddsResources -count=1'`

Expected: FAIL because `environment columns = 0`.

- [ ] **Step 3: Add the migration and sqlc queries**

`00010_resources.up.sql`:

```sql
-- +goose Up
CREATE TABLE resources (
    id         text PRIMARY KEY,
    name       text NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('container')),
    spec       jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

ALTER TABLE plane_settings
    ADD COLUMN environment jsonb NOT NULL DEFAULT '{}'::jsonb;
```

`00010_resources.down.sql`:

```sql
-- +goose Down
ALTER TABLE plane_settings DROP COLUMN environment;
DROP TABLE resources;
```

`queries/resources.sql`:

```sql
-- name: ListResources :many
SELECT id, name, kind, spec, created_at, updated_at
FROM resources
ORDER BY created_at ASC, id ASC;

-- name: GetResource :one
SELECT id, name, kind, spec, created_at, updated_at
FROM resources
WHERE id = $1;

-- name: InsertResource :one
INSERT INTO resources (id, name, kind, spec, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, kind, spec, created_at, updated_at;

-- name: UpdateResource :one
UPDATE resources
SET name = $2, spec = $3, updated_at = $4
WHERE id = $1
RETURNING id, name, kind, spec, created_at, updated_at;

-- name: DeleteResource :exec
DELETE FROM resources WHERE id = $1;
```

Replace `queries/settings.sql` statements so every `SELECT`/`RETURNING` includes `environment`, and insert/update write it:

```sql
-- name: GetPlaneSettings :one
SELECT id, sandbox, environment, created_at, updated_at
FROM plane_settings
WHERE id = 'default';

-- name: InsertPlaneSettings :one
INSERT INTO plane_settings (id, sandbox, environment, created_at, updated_at)
VALUES ('default', $1, '{}'::jsonb, $2, $3)
RETURNING id, sandbox, environment, created_at, updated_at;

-- name: UpdatePlaneSettings :one
UPDATE plane_settings
SET sandbox = $1, environment = $2, updated_at = $3
WHERE id = 'default'
RETURNING id, sandbox, environment, created_at, updated_at;
```

`InsertPlaneSettings` gains no new Go parameter for environment because the SQL literal writes `{}`. `UpdatePlaneSettings` gains `Environment []byte` as `$2` and `UpdatedAt` moves to `$3`. After `sqlc generate`, fix every `UpdatePlaneSettings` caller to pass the existing `row.Environment` through unchanged.

In `overlay.go`:

```go
type PlaneSettings struct {
	Sandbox     json.RawMessage `json:"sandbox"`
	Environment json.RawMessage `json:"environment"`
}
```

In `planeSettingsFromDB`:

```go
return PlaneSettings{
	Sandbox:     rawOrDefault(row.Sandbox, "{}"),
	Environment: rawOrDefault(row.Environment, "{}"),
}
```

`PatchPlaneSettings` keeps taking only a sandbox patch. When it calls `UpdatePlaneSettings`, pass `current`'s environment bytes so the column is not wiped.

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane/internal/db && sqlc generate'`

- [ ] **Step 4: Run the test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/db/ ./internal/catalog/ -count=1'`

Expected: PASS. Catalog tests still see `isolation` and `environmentId`.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/db/migrations/00010_resources.up.sql controlplane/internal/db/migrations/00010_resources.down.sql controlplane/internal/db/queries/resources.sql controlplane/internal/db/queries/settings.sql controlplane/internal/db/*.go controlplane/internal/catalog/overlay.go controlplane/internal/catalog/settings.go controlplane/internal/db/migrate_test.go
git commit -m "$(cat <<'EOF'
Add the resources table and plane environment column.

EOF
)"
```

---

### Task 2: Store container resources

**Files:**
- Create: `controlplane/internal/catalog/resources.go`
- Create: `controlplane/internal/catalog/resources_store_test.go`
- Modify: `controlplane/internal/catalog/store.go`
- Modify: `controlplane/internal/server/server.go`

**Interfaces:**
- Consumes: sqlc resource queries; `catalog.ApplyIdentityPrefix`; `sandbox.ValidateContainerName`; `sandbox.ValidateVolumeName`; `newID`.
- Produces:
  - `ErrResourceNotFound = errors.New("resource not found")` (HTTP maps this to `resource "<id>" not found`)
  - `ErrResourceInUse = errors.New("resource in use")`
  - `KindContainer = "container"`
  - `Store.IdentityPrefix string`
  - `CreateResource(ctx, name, kind string, spec json.RawMessage) (Resource, error)`
  - `GetResource(ctx, id string) (Resource, error)`
  - `ListResources(ctx) ([]Resource, error)`
  - `UpdateResource(ctx, id string, name *string, spec json.RawMessage) (Resource, error)`
  - `DeleteResource(ctx, id string) error`
  - `Resource` JSON: `id`, `name`, `kind`, `spec`, `createdAt`, `updatedAt`

- [ ] **Step 1: Write the failing store tests**

Create `controlplane/internal/catalog/resources_store_test.go` with `TestCreateResourcePrefixesAndRejectsDuplicateNames`. Use `catalog.Open(dbtest.Open(t))`, set `store.IdentityPrefix = "dev-"`, and create:

```go
spec := json.RawMessage(`{
  "image": "alpine:3.20",
  "containerName": "work",
  "volumes": [{
    "id": "vol_0123456789abcdef",
    "enabled": true,
    "name": "disk",
    "target": "/workspace",
    "whitelisted": true,
    "read": true,
    "write": true,
    "exec": true
  }]
}`)
got, err := store.CreateResource(ctx, " Work ", catalog.KindContainer, spec)
```

Assert `got.ID` has prefix `res_`, stored spec `containerName` is `dev-work`, volume `name` is `dev-disk`, and `idleTTLSeconds` is `3600`. Second create with container name `dev-work` or volume name `dev-disk` returns an error containing `duplicate`. Create with `kind: "ssh"` returns an error containing `container`. Create with `"image": ""` returns an error containing `image`. `GetResource` of a missing id is `ErrResourceNotFound`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run TestCreateResourcePrefixesAndRejectsDuplicateNames -count=1'`

Expected: FAIL to compile, `CreateResource` undefined.

- [ ] **Step 3: Implement resource validation and CRUD**

Add `IdentityPrefix` on `Store`. In `server.NewMux`, before building handlers, set `catalogStore.IdentityPrefix = engine.Docker.IdentityPrefix`.

`resources.go` decodes spec into:

```go
type containerSpec struct {
	Image          string       `json:"image"`
	Dockerfile     string       `json:"dockerfile,omitempty"`
	BuildContext   string       `json:"buildContext,omitempty"`
	ContainerName  string       `json:"containerName"`
	IdleTTLSeconds int64        `json:"idleTTLSeconds"`
	Volumes        []volumeSpec `json:"volumes"`
}
```

`normalizeContainerSpec` trims strings, rejects kind other than `container`, requires non-empty image and container name, defaults idle timeout to `3600`, rejects idle timeout `<= 0` when the JSON key is present (decode into a `map[string]json.RawMessage` to detect presence, then into the typed struct). Prefix names with `store.IdentityPrefix` via `ApplyIdentityPrefix`. Validate with `sandbox.ValidateContainerName` and `sandbox.ValidateVolumeName`. Reject a volume id that does not match `^vol_[0-9a-f]{16}$`, an empty volume name or target, a repeated target, and a repeated volume name inside the spec. Load every other resource and reject a container name or volume name that matches one of them. `UpdateResource` with `spec == nil` keeps the stored spec. When `spec` is present, merge it onto the stored spec: scalars replace, `null` deletes a key, volumes merge by `id` with the same null-deletes-row rule as overlay volumes. `name == nil` keeps the stored name. Empty trimmed display name is an error. `DeleteResource` returns `ErrResourceNotFound` when `GetResource` misses. In-use checks are Task 4, after environment JSON exists. Until then `DeleteResource` deletes the row.

Encode spec with `json.Marshal` so empty dockerfile and build context are omitted (`omitempty`).

- [ ] **Step 4: Run the test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run TestCreateResourcePrefixesAndRejectsDuplicateNames -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/resources.go controlplane/internal/catalog/resources_store_test.go controlplane/internal/catalog/store.go controlplane/internal/server/server.go
git commit -m "$(cat <<'EOF'
Store container resources with prefixed unique names.

EOF
)"
```

---

### Task 3: Resource HTTP

**Files:**
- Modify: `controlplane/internal/catalog/http.go`
- Create: `controlplane/internal/catalog/resources_http_test.go`

**Interfaces:**
- Consumes: Task 2 store methods and sentinels.
- Produces: `GET/POST /v1/resources`, `GET/PATCH/DELETE /v1/resources/{id}`. `writeMappedError` maps `ErrResourceInUse` to 409 and `ErrResourceNotFound` to 404 with `resource "<id>" not found`.

- [ ] **Step 1: Write the failing HTTP test**

`TestResourcesHTTP` starts `httptest.NewServer(catalog.Handler(store))`. `POST /v1/resources` with the Task 2 spec and `"name":"Work"` returns `201` and `kind == "container"`. `GET /v1/resources` includes it. `PATCH /v1/resources/{id}` with `{"spec":{"image":"alpine:3.21"}}` keeps `containerName` and changes `image`. `GET` missing id returns `404` and body `{"error":"resource \"res_missing\" not found"}`. `DELETE` returns `204` and a following `GET` returns `404`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run TestResourcesHTTP -count=1'`

Expected: FAIL with `404` on `POST` (route missing).

- [ ] **Step 3: Add the routes**

Register the five routes next to providers. `POST` decodes `name`, `kind`, `spec`. `PATCH` decodes optional `name` and `spec` (`spec` omitted means `nil`). Map errors through `writeMappedError`. Add `ErrResourceNotFound` beside `ErrProjectNotFound` in the 404 branch, and format not-found as `fmt.Errorf("resource %q not found", id)` before writing so the body matches the constraint. Keep `errors.Is` against the sentinel to choose 404. Add `ErrResourceInUse` next to `ErrProjectInUse` for 409.

- [ ] **Step 4: Run the test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run 'TestResourcesHTTP|TestCreateResource' -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/http.go controlplane/internal/catalog/resources_http_test.go
git commit -m "$(cat <<'EOF'
Expose container resources on the catalog HTTP API.

EOF
)"
```

---

### Task 4: Environment settings on the plane and the project

**Files:**
- Create: `controlplane/internal/catalog/environment.go`
- Create: `controlplane/internal/catalog/environment_test.go`
- Modify: `controlplane/internal/catalog/settings.go`
- Modify: `controlplane/internal/catalog/http.go`
- Modify: `controlplane/internal/catalog/resources.go`

**Interfaces:**
- Consumes: `PlaneSettings.Environment`; `MergeSettings`; `PatchOverlayJSON` keyed-row helper used for `volumes` and `extraPaths`.
- Produces:
  - `PatchPlaneSettings(ctx, sandboxPatch, environmentPatch json.RawMessage) (PlaneSettings, error)`
  - `EnvironmentFromSettings(raw json.RawMessage) (json.RawMessage, error)` returns the `environment` object or `{}`
  - `ValidateEnvironmentPatch(ctx, resourceSpec json.RawMessage, patch json.RawMessage) error`
  - `DeleteResource` returns `ErrResourceInUse` when `plane_settings.environment.resourceId` or any project's `settings.environment.resourceId` equals the id

- [ ] **Step 1: Write the failing tests**

`TestEnvironmentPatchMergesGrantsByVolumeID`: seed plane environment `{"resourceId":"res_aaaaaaaaaaaaaaaa","grants":[{"volumeId":"vol_0123456789abcdef","read":true}]}`. Patch `{"grants":[{"volumeId":"vol_0123456789abcdef","write":false}]}`. Stored grants for that volume id are one row with `read: true` and `write: false`. A second volume id in the patch is appended. `{"grants":[{"volumeId":"vol_0123456789abcdef"}]}` with a JSON `null` for that element deletes the row. Use the same keyed-null shape `PatchOverlayJSON` already uses for volume rows.

`TestDeleteResourceInUse`: create a resource, `PatchPlaneSettings` with `environment: {"resourceId":"<id>"}`, `DeleteResource` returns `ErrResourceInUse`. Clear the global id with a `null` resourceId patch, set the same id on a project via `UpdateProject` settings `{"environment":{"resourceId":"<id>"}}`, and delete is in use again. After clearing the project key, delete succeeds.

`TestRejectsUnknownResourceAndVolume`: environment patch `resourceId` `res_missing` errors with `resource "res_missing" not found`. A grant whose volume id is not on that resource errors with `unknown volume "<id>"`. A grant patch when no resource resolves errors with `no resource selected`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run 'TestEnvironmentPatch|TestDeleteResourceInUse|TestRejectsUnknownResource' -count=1'`

Expected: FAIL to compile.

- [ ] **Step 3: Merge environment JSON and enforce in-use**

Change `PatchPlaneSettings` to accept both patches. Reject the call only when both are empty. Sandbox still goes through `PatchOverlayJSON`. Environment goes through a new `PatchEnvironmentJSON` that merge-patches objects and keyed-merges `grants` by `volumeId` and `extraPaths` by `id`, using the same null-deletes-row rule as `volumes` in `PatchOverlayJSON`.

Before saving a plane environment patch, resolve the resource id as: the patch's `resourceId` when the key is present and not null, otherwise the stored global id. Load that resource and reject unknown volume ids in the patch's `grants`. Project settings already merge through `MergeSettings`. Add `environment` to that path by calling `PatchEnvironmentJSON` for the `environment` key, with the same validation. The project resource id for the check is the patch's `resourceId` when set, else the stored project `resourceId`, else the global default.

`DeleteResource` lists projects and reads plane settings. Any matching `resourceId` returns `ErrResourceInUse`.

Update the HTTP settings handler so the body is:

```go
type settingsPatch struct {
	Sandbox     json.RawMessage `json:"sandbox"`
	Environment json.RawMessage `json:"environment"`
}
```

Stop requiring a sandbox patch. Call `PatchPlaneSettings(ctx, body.Sandbox, body.Environment)`.

Update every existing `PatchPlaneSettings(ctx, sandbox)` call to pass `nil` as the environment patch.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/environment.go controlplane/internal/catalog/environment_test.go controlplane/internal/catalog/settings.go controlplane/internal/catalog/http.go controlplane/internal/catalog/resources.go
git commit -m "$(cat <<'EOF'
Merge environment settings and keep linked resources.

EOF
)"
```

---

### Task 5: Resolve the environment used to attach

**Files:**
- Modify: `controlplane/internal/catalog/environment.go`
- Modify: `controlplane/internal/catalog/environment_test.go`
- Modify: `controlplane/internal/catalog/http.go`

**Interfaces:**
- Consumes: Task 4 environment JSON and `GetResource`.
- Produces:
  - `ResolveEnvironment(ctx, projectID string) (ResolvedEnvironment, error)`
  - `ResolvedEnvironment` JSON fields: `resourceId`, `resource`, `workspaceRoot`, `volumes`, `extraPaths`
  - `GET /v1/projects/{id}/environment/resolved`
  - Volumes in the resolved view are enabled only, after overrides, with `id`, `name`, `target`, `whitelisted`, `read`, `write`, `exec`

- [ ] **Step 1: Write the failing resolver test**

`TestResolveEnvironmentProjectWins`: create two resources. Global environment selects resource A, workspace root `/global`, and a grant that sets A's volume `write` to false. Project environment selects resource B and sets workspace root `/proj`. Resolved `resourceId` is B, workspace root is `/proj`, and A's grant is ignored because that volume id is not on B. A second project with no `resourceId` resolves to A, workspace root `/global`, and `write: false` on A's volume. A third project and a global environment with no resource id resolve `resource` nil, `resourceId` nil, `volumes` empty, and workspace root `/workspace`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run TestResolveEnvironmentProjectWins -count=1'`

Expected: FAIL to compile.

- [ ] **Step 3: Implement resolution and the preview route**

Resolution order:

1. Project `resourceId` when the key is set and non-empty, else global `resourceId`.
2. Project `workspaceRoot` when set, else global, else `DefaultWorkspaceRoot` (`/workspace`).
3. Enabled volumes from the resource. Missing `enabled` means enabled. Missing grant flags mean `true`.
4. Apply global grant overrides for volume ids that exist, then project overrides. Each override replaces only the flags it sets.
5. Extra paths: start from global, overlay project rows by id. Drop rows with `enabled: false`.

`GET /v1/projects/{id}/environment/resolved` returns that struct with `200`. Do not remove the old sandbox resolved routes in this task.

- [ ] **Step 4: Run the test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run TestResolveEnvironmentProjectWins -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/environment.go controlplane/internal/catalog/environment_test.go controlplane/internal/catalog/http.go
git commit -m "$(cat <<'EOF'
Resolve a project's environment over the global default.

EOF
)"
```

---

### Task 6: Copy existing sandboxes into resources

**Files:**
- Create: `controlplane/internal/catalog/backfill.go`
- Create: `controlplane/internal/catalog/backfill_test.go`

**Interfaces:**
- Consumes: `CreateResource` rules (prefix, uniqueness, defaults) by calling the same `normalizeContainerSpec`; `DecodeOverlay`; `DefaultOverlay`; `ExpandName`; `ApplyIdentityPrefix`; `ProjectVolumeName`; `EnvironmentVolumeName`.
- Produces: `BackfillResources(ctx, pool *pgxpool.Pool, identityPrefix string) error`. Idempotent when `projects.environment_id` is gone, and when a project already has `settings.environment.resourceId`.

- [ ] **Step 1: Write the failing backfill test**

`TestBackfillIsolatedProjectKeepsPhase1Disk` uses `db.MigrateTo(ctx, url, 10)` so `00011` is not applied. Open a pool and `catalog.Open`. Insert no extra settings. The seeded Default project has empty settings. Call `EnsurePlaneSettings` with an empty `DeprecatedSandbox` so the default overlay exists, then `BackfillResources(ctx, pool, "")`.

Assert one resource. Its container name is `agent-fabric-container-` + the project id. Its workspace volume name is the expanded default volume (`agent-fabric-vol-` + project id) because `DefaultOverlay(false)` includes that volume. The project's `settings.environment.resourceId` equals the resource id. Global environment has no `resourceId`. `projects.environment_id` still exists.

`TestBackfillSharedEnvironmentIsOneResource`: create an environment through `CreateEnvironment` and two projects with `isolation=shared` and that `environment_id`, with different `{projectID}` container templates. After backfill, both `resourceId`s match, the workspace volume name is `agent-fabric.env.` + environment id when `volume_name` is null, and the container name is the older project's expanded name.

`TestBackfillRejectsLocal`: set the plane sandbox `kind` to `local` and expect error `local sandbox cannot be migrated for project "<id>"`. No resource row is inserted (transaction rollback).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run 'TestBackfill' -count=1'`

Expected: FAIL to compile.

- [ ] **Step 3: Implement the copy**

Open a `catalog.Store` on `pool` and set `IdentityPrefix` to the argument before creating rows, so names go through `normalizeContainerSpec`. Follow the spec's Migration section. Use one transaction. Detect `projects.environment_id` via `information_schema.columns`; if missing, return nil. Skip projects whose settings already contain a non-empty `environment.resourceId`.

Resolve each project's overlay as plane sandbox merged with project sandbox, using the same merge as `ResolvedProjectSandbox` does today. Do not read agent settings. If the merged overlay has no container name, use `DefaultContainerNameTemplate`. Expand `{projectID}` only. If `{threadID}` or `{random}` remains, return `fmt.Errorf("project %q has an unstable sandbox name", id)`. If resolved kind is `local`, return `fmt.Errorf("local sandbox cannot be migrated for project %q", id)`.

Group shared projects by `environment_id` first. Stored container name is the oldest member (`created_at`, then `id`) after expand and prefix. Workspace volume name is `volume_name` or `agent-fabric.env.{environmentID}` at the workspace root, replacing any overlay volume with that target. Remaining projects group by expanded container name. If that name is already used by a shared group, return `projects "<id>" and "<id>" disagree on container spec`.

Within a group, image, dockerfile, build context, idle timeout, and the attach mount list (name + target) must match or the same disagree error is returned. Grant flags may differ. Oldest project's flags are the resource defaults. Other projects store grant overrides for flags that differ.

An isolated project with no volume targeting the workspace root gets `ProjectVolumeName(projectID)` at that root.

Write `settings.environment.resourceId` on every project. Copy global `workspaceRoot` and `extraPaths` onto `plane_settings.environment` without setting global `resourceId`. Copy a project's own `workspaceRoot` and `extraPaths` onto that project's environment. Remove from plane, project, and agent sandbox objects: `kind`, `workspaceRoot`, `image`, `idleTTLSeconds`, `dockerfile`, `buildContext`, `containerName`, `volumes`, `extraPaths`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run 'TestBackfill' -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/backfill.go controlplane/internal/catalog/backfill_test.go
git commit -m "$(cat <<'EOF'
Copy sandbox overlays into container resources.

EOF
)"
```

---

### Task 7: Drop the old link and attach through the resolver

**Files:**
- Create: `controlplane/internal/db/migrations/00011_drop_environments.up.sql`
- Create: `controlplane/internal/db/migrations/00011_drop_environments.down.sql`
- Create: `controlplane/internal/appmigrate/migrate.go`
- Modify: `controlplane/cmd/controlplane/main.go`
- Modify: `controlplane/internal/db/dbtest/postgres.go`
- Modify: `controlplane/internal/db/queries/projects.sql`
- Modify: `controlplane/internal/db/queries/environments.sql` (delete queries)
- Modify: `controlplane/internal/catalog/project_types.go`
- Modify: `controlplane/internal/catalog/projects.go`
- Modify: `controlplane/internal/catalog/http.go`
- Modify: `controlplane/internal/agent/agent.go`
- Modify: `controlplane/internal/workspace/open.go`
- Modify: tests that read `Isolation`, `EnvironmentID`, `CreateEnvironment`, or `/sandbox/resolved`

**Interfaces:**
- Consumes: `ResolveEnvironment`, `BackfillResources`, `EnsurePlaneSettings`.
- Produces: `appmigrate.RunMigrations(ctx, databaseURL, identityPrefix string, deprecated DeprecatedSandbox) error`. Project JSON has no `isolation` or `environmentId`. `openPromptSandbox` and workspace open build `sandbox.OpenOptions` from `ResolvedEnvironment` only. Routes `GET /v1/projects/{id}/sandbox/resolved` and `GET /v1/agents/{id}/sandbox/resolved` are gone.

- [ ] **Step 1: Write the failing attach test**

In `controlplane/internal/agent/sandbox_options_test.go`, add `TestPromptSandboxUsesLinkedResource`. Create a container resource with image `alpine:3.20`, container name `box`, volume name `disk`, target `/workspace`. Set the project's `settings.environment.resourceId` to it. Set the agent's settings sandbox image to `debian:12`. Build the prompt sandbox options the same way the existing tests do.

Assert Docker image is `alpine:3.20`, container name is `box`, the mount source is `disk`, and the agent image was ignored. A project with no resource id and no global resource id returns an error containing `has no resource`. A linked resource whose only volume target is `/data` and whose resolved workspace root is `/workspace` returns an error containing `no enabled volume targets workspace root "/workspace"`. `DeleteProject` on the linked project leaves the resource row in `ListResources`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/agent/ -run TestPromptSandboxUsesLinkedResource -count=1'`

Expected: FAIL because the image still comes from the overlay (`debian:12` or the default alpine from the agent/global sandbox), or the test does not compile until the helper is pointed at `ResolveEnvironment`.

- [ ] **Step 3: Drop the old columns and switch attach**

`00011_drop_environments.up.sql`:

```sql
-- +goose Up
ALTER TABLE projects DROP COLUMN environment_id;
ALTER TABLE projects DROP COLUMN isolation;
DROP TABLE environments;
```

`00011_drop_environments.down.sql` recreates the `environments` table and the two project columns from `00006_projects.up.sql`. It does not insert rows.

`RunMigrations`:

```go
func RunMigrations(ctx context.Context, databaseURL, identityPrefix string, deprecated DeprecatedSandbox) error {
	if err := db.MigrateTo(ctx, databaseURL, 10); err != nil {
		return err
	}
	pool, err := db.OpenPool(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := Open(pool)
	if _, err := store.EnsurePlaneSettings(ctx, deprecated); err != nil {
		return err
	}
	if err := BackfillResources(ctx, pool, identityPrefix); err != nil {
		return err
	}
	return db.Migrate(ctx, databaseURL)
}
```

Put `RunMigrations` in `controlplane/internal/appmigrate/migrate.go`. That package imports `db` and `catalog`. `catalog` tests import `dbtest`, so `dbtest` must not import `catalog`. `dbtest.Open` calls `appmigrate.RunMigrations(ctx, url, "", catalog.DeprecatedSandbox{})`. `appmigrate` does not import `dbtest`. `main` replaces `db.Migrate` and the later `EnsurePlaneSettings` call with `appmigrate.RunMigrations(ctx, databaseURL, engine.Docker.IdentityPrefix, deprecated)`.

Remove `isolation` and `environment_id` from project sqlc queries and from `catalog.Project`. Remove `CreateEnvironment`, `IsolationIsolated`, `IsolationShared`, and the shared-isolation branches in `projects.go`, `agent.go`, and `workspace/open.go`.

`openPromptSandbox` calls `store.ResolveEnvironment`. On nil resource, return `fmt.Errorf("project %q has no resource", project.ID)`. On a missing row, return `fmt.Errorf("resource %q not found", id)`. Kind is `docker`. Workspace root, image, idle TTL, dockerfile, build context, container name, and mounts come from the resolved resource. Mounts are the resolved volumes (`Type: volume`, `ReadOnly` when `write` is false). Path policy grants are those volume targets plus enabled extra paths. Do not read `agents.settings.sandbox`.

Delete the two `/sandbox/resolved` handlers and their routes. Update catalog, agent, and workspace tests that called them or set isolation. `TestMigrateCreatesCatalogTables` expects `resources` and does not expect `environments`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ ./internal/agent/ ./internal/workspace/ ./internal/db/ -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/db/migrations/00011_drop_environments.up.sql controlplane/internal/db/migrations/00011_drop_environments.down.sql controlplane/internal/appmigrate controlplane/cmd/controlplane/main.go controlplane/internal/db controlplane/internal/catalog controlplane/internal/agent controlplane/internal/workspace
git commit -m "$(cat <<'EOF'
Attach projects to linked container resources.

EOF
)"
```

---

### Task 8: Settings UI for resources and environments

**Files:**
- Modify: `client/lib/catalog/models.dart`
- Modify: `client/lib/catalog/catalog_client.dart`
- Modify: `client/test/catalog/catalog_client_test.dart`
- Create: `client/lib/settings/resources_tab.dart`
- Create: `client/lib/settings/environment_tab.dart`
- Modify: `client/lib/settings/settings_page.dart`
- Modify: `client/lib/settings/projects_tab.dart`
- Modify: `client/lib/settings/agents_tab.dart`
- Modify: `client/test/settings/projects_tab_test.dart`
- Modify: `client/test/settings/agents_tab_test.dart`
- Modify: `client/test/settings/sandbox_tab_test.dart` (replace with resources and environment tests, or delete if the Sandbox tab file is removed)
- Delete: `client/lib/settings/sandbox_tab.dart` once nothing imports it

**Interfaces:**
- Consumes: Task 3 and Task 5 HTTP. Project JSON without `isolation` and `environmentId`. `PlaneSettings.environment`.
- Produces: `CatalogClient.listResources`, `createResource`, `updateResource`, `deleteResource`, `resolvedEnvironment`. Settings tabs: Providers, Agents, Projects, Resources, Environment, Display.

- [ ] **Step 1: Write the failing widget tests**

Replace the sandbox tab expectations. `resources_tab_test.dart` pumps `ResourcesTab` with a fake catalog that returns one container. The list shows the resource name. Tapping add, entering name `Work`, image `alpine:3.20`, container name `work`, and saving calls `createResource`. Adding a volume row sends a `volumes` entry with a `vol_` + 16 hex id.

`environment_tab_test.dart` shows a resource dropdown including an empty choice. Saving a selected resource id calls `patchSettings` with `environment.resourceId`.

In `projects_tab_test.dart`, the isolation control is absent. A dropdown's first item text is `Use global default`. Saving that choice sends a project settings patch that removes `environment.resourceId` (`null`). The resolved preview section shows `no resource` when `resolvedEnvironment` returns `resource: null`.

In `agents_tab_test.dart`, no widget titled `Sandbox` and no `SandboxOverlayForm` is present.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/settings test/catalog/catalog_client_test.dart'`

Expected: FAIL. The Sandbox tab tests and isolation expectations fail first. New tab tests fail to compile until the files exist.

- [ ] **Step 3: Implement the client and the tabs**

`PlaneSettings` gains `environment` (`Map<String, dynamic>`, default `{}`). `Project` drops `isolation` and `environmentId`. `CatalogClient.patchSettings` sends `{ if (sandbox != null) 'sandbox': sandbox, if (environment != null) 'environment': environment }`. Add resource CRUD against `/v1/resources`. Replace `resolvedProjectSandbox` and `resolvedAgentSandbox` with `resolvedEnvironment(projectId)` calling `GET /v1/projects/$projectId/environment/resolved`.

`ResourcesTab` lists name and kind, creates and edits a container (image, container name, idle timeout, volume name, target, enabled, whitelist, read, write, exec), and confirms delete. On a catalog error whose body contains `resource in use`, show that text and leave the row.

`EnvironmentTab` edits the global environment: resource dropdown (empty allowed), workspace root, extra paths, and grant overrides for the selected resource's volumes. It cannot add a mount.

`ProjectsTab` removes the isolated/shared control and the sandbox overlay form. The environment block's first dropdown value is `Use global default`, then each resource. Workspace root, extra paths, and grant overrides save through the project settings patch. The preview reads `resolvedEnvironment`.

`SettingsPage` tab length is 6, in the order above. `AgentsTab` removes the sandbox editor and the coming-soon Sandbox tile. Leave Tools, MCP, and Memory as they are.

Delete `sandbox_tab.dart` and `sandbox_overlay_form.dart` only after no remaining import references them. If `newSandboxVolumeID` is still useful, move the `vol_` / `path_` id helpers next to the resources tab.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/settings test/catalog/catalog_client_test.dart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/catalog client/lib/settings client/test/catalog/catalog_client_test.dart client/test/settings
git commit -m "$(cat <<'EOF'
Edit container resources and project environments in Settings.

EOF
)"
```
