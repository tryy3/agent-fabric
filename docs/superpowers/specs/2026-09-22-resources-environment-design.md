# Resources and environments

**Date:** 2026-09-22
**Status:** approved for planning
**Builds on:** projects Phases 0–5 (issue #31)

## Problem

A project's machine is a merged sandbox overlay: engine file, then global settings, then project settings, then agent settings. Docker container and volume names are templates filled in when the machine is attached. Shared isolation is the only durable link, and it points at an `environments` row that is mostly a name and a volume.

That makes it hard to create a container once, attach it to a project, detach it when the project is deleted, and attach the same container to another project.

## Goals

- Keep a pool of durable resources, edited only in global Settings → Resources.
- This slice runs one kind: `container`. SSH and VM are later kinds that use the same project link.
- A volume belongs to one container. A Docker volume name cannot be used by two containers.
- A project environment links exactly one resource. The global environment supplies the default. A project may override it. Threads and agents do not.
- Deleting a project removes the link and leaves the resource, the Docker container, and the Docker volumes.
- Two projects that resolve to the same resource share that container and its disks.
- Split start settings from harness settings. The resource is how the container is started. The environment is which resource to use, the workspace root, and path grants.

## Non-goals

- Implementing SSH, a VM, or a host-directory resource.
- Thread-level or agent-level resource, workspace root, or path-grant overrides.
- Agent tool-permission editing.
- Deleting or garbage-collecting Docker containers, Docker volumes, or `{dataDir}/projects/{id}`.
- Recreating a running container when its image or mounts differ.
- Per-session containers and `{projectID}` / `{threadID}` / `{random}` name templates on resources.

## Approach

**Chosen:** catalog resource rows, with environment settings layered on the global plane and on the project.

A container resource stores the `docker run` side. Environment JSON stores the harness side. Attach resolves the project environment over the global environment, loads that one resource, and starts it. The old `environments` table and the isolated/shared switch go away. Sharing is two projects pointing at the same resource.

**Rejected:**

- An environment row that owns the resource. Global-then-project is a merge, so a single environment row still needs overlay fields beside it.
- Keeping template containers as a fallback when no resource is linked. New projects would keep creating disks on their own, which is the behavior this replaces.

`00006_projects` is not edited. A new migration adds the resource table. The control plane copies existing rows in Go, then a following migration drops the old link.

## Concepts

`sandbox.json` stays process config: database URL, listen address, data directory, Docker binary, runtime, and identity prefix. It is not a resource and not an environment.

**Resource.** Edited only under Settings → Resources. Kind `container` stores image, optional Dockerfile and build context, a literal container name, idle timeout, and volumes. One resource is one container, reused by the stored name.

**Volume.** A child of a container, not a top-level resource. It stores a literal Docker volume name, a mount target, an enabled flag, and default path grants (whitelist, read, write, exec).

**Environment.** Not a table. Global `plane_settings.environment` holds the default resource id, workspace root, extra paths, and grant changes. `projects.settings.environment` stores the same shape sparsely. Missing `resourceId` on the project means use the global default. Threads inherit the project. Agents do not take part.

**Attach.** The moment chat or the Files pane needs the machine. If neither layer names a resource, attach fails. The workspace root must match an enabled volume target. Deleting a project does not delete the resource or any Docker object.

## Data

### `resources`

| Column | |
| --- | --- |
| `id` | `res_` + 16 hex |
| `name` | Display name. Required. Not unique. |
| `kind` | `container` only, in this slice |
| `spec` | JSON below |
| `created_at`, `updated_at` | timestamptz |

Container spec:

```json
{
  "image": "alpine:3.20",
  "dockerfile": "Dockerfile",
  "buildContext": ".",
  "containerName": "dev-agent-fabric-work",
  "idleTTLSeconds": 3600,
  "volumes": [
    {
      "id": "vol_0123456789abcdef",
      "enabled": true,
      "name": "dev-agent-fabric-work",
      "target": "/workspace",
      "whitelisted": true,
      "read": true,
      "write": true,
      "exec": true
    }
  ]
}
```

`dockerfile` and `buildContext` are omitted when empty. `image` is required. Omitted `idleTTLSeconds` is stored as 3600. A container may have zero volumes.

Volume ids are `vol_` + 16 hex. New ids are generated on create. The old overlay id `vol_workspace` is not reused.

Container names are unique across other resources. Volume names are unique across other resources, including disabled volumes. Mount targets are unique within one container. Names live in JSON, so the store checks uniqueness on create and update. On write, a container name or volume name that does not already start with the engine identity prefix is stored with that prefix prepended. Attach uses the stored name as-is.

### Environment JSON

Stored on `plane_settings.environment` (new column, default `{}`) and at `projects.settings.environment`.

```json
{
  "resourceId": "res_0123456789abcdef",
  "workspaceRoot": "/workspace",
  "grants": [
    { "volumeId": "vol_0123456789abcdef", "write": false }
  ],
  "extraPaths": [
    {
      "id": "path_0123456789abcdef",
      "enabled": true,
      "path": "/opt/tools",
      "whitelisted": true,
      "read": true,
      "write": false,
      "exec": false
    }
  ]
}
```

A missing key inherits. `null` deletes a key, same as other settings patches. Grant rows merge by `volumeId`. Extra paths merge by `id`. A grant lists only the flags it changes. Omitted flags keep the resource default.

There is no foreign key. The global default lives in JSON too. The store rejects an unknown `resourceId` and refuses to delete a resource that is the global default or linked by any project.

`projects.isolation`, `projects.environment_id`, and the `environments` table are dropped after the copy below. Project JSON no longer includes `isolation` or `environmentId`.

`plane_settings.sandbox`, `projects.settings.sandbox`, and `agents.settings.sandbox` are not read on attach. The copy removes these keys from those objects: `kind`, `workspaceRoot`, `image`, `idleTTLSeconds`, `dockerfile`, `buildContext`, `containerName`, `volumes`, `extraPaths`. Agent tool settings are untouched. The `sandbox` column and `agents.settings` stay.

## Migration

`db.Migrate` applies goose through `00010`, runs the Go copy, then applies the rest.

`00010_resources.up.sql` creates `resources` and adds `plane_settings.environment`. It does not drop the old link. `00006_projects` is not edited.

The Go copy runs in one transaction. If `projects.environment_id` is already gone, it returns. A project that already has `settings.environment.resourceId` is left as-is, so a retry after a partial copy does not create a second resource. On any error below, the transaction rolls back and `00011` is not applied.

For each project, resolve the overlay from plane sandbox merged with that project's sandbox. Do not read the agent overlay. Expand `{projectID}` only, then apply the identity prefix from `sandbox.json`.

- A template that still contains `{threadID}` or `{random}` aborts with `project "<id>" has an unstable sandbox name`.
- Resolved kind `local` aborts with `local sandbox cannot be migrated for project "<id>"`. The host directory is left on disk. This slice does not turn it into a container.
- Projects whose `isolation` is `shared` and which share an `environment_id` become one resource, even when their expanded container names differ. The stored container name is the oldest linked project's expanded name (`created_at`, then `id`). The workspace volume name is `environments.volume_name` when set, otherwise `agent-fabric.env.{environmentID}` at the workspace root. Other containers that used to share only that disk are left in Docker and are not attached.
- Remaining projects with the same expanded container name become one resource. If that name is already the stored name of a shared group, the copy aborts with `projects "<id>" and "<id>" disagree on container spec`.
- Within a group, image, Dockerfile, build context, idle timeout, and the attach mount list must match. The attach mount list is the expanded overlay volumes after the shared workspace volume replaces the volume whose target is the workspace root. If they do not match, the copy aborts with `projects "<id>" and "<id>" disagree on container spec`. Grant-flag differences do not abort. The oldest project's flags (`created_at`, then `id`) become the resource defaults. Each other project stores grant overrides for flags that differ.
- An isolated project with no overlay volume targeting the workspace root gets the implicit volume `agent-fabric.proj.{projectID}` at that root, which is the disk attach already uses.
- Every existing project stores `settings.environment.resourceId`. The global `resourceId` is left unset, so existing projects do not start sharing a disk. Global `workspaceRoot` and `extraPaths` move from the plane sandbox onto `plane_settings.environment`. A project's own `workspaceRoot` and `extraPaths` move onto that project's environment.

`00011_drop_environments.up.sql` drops `projects.environment_id`, `projects.isolation`, and `environments`. The control plane's `db.Migrate` is the supported path. Running `00011` without the Go copy drops the old link before resources exist.

`00010` down drops `resources` and `plane_settings.environment`. `00011` down recreates `environments` and the `projects.isolation` and `projects.environment_id` columns. Neither down restores old rows or Docker names.

## API

JSON stays camelCase. Errors stay `{ "error": "<message>" }`.

### Resources

| Call | Result |
| --- | --- |
| `GET /v1/resources` | `200` list |
| `POST /v1/resources` | `201` resource. Body is `name`, `kind`, `spec`. |
| `GET /v1/resources/{id}` | `200` resource |
| `PATCH /v1/resources/{id}` | `200`. Merge-patch: scalars replace, `null` deletes a key, volumes merge by `id`. |
| `DELETE /v1/resources/{id}` | `204` when nothing points at it |

`POST` and `PATCH` return `400` for: kind other than `container`, empty display name, empty image, idle timeout present and not a positive integer, empty container name, duplicate container name, duplicate volume name, duplicate mount target, a volume id that is not `vol_` + 16 hex, or an enabled or disabled volume missing name or target.

`DELETE` of a resource that is `plane_settings.environment.resourceId` or any project's `settings.environment.resourceId` returns `409` `{ "error": "resource in use" }`. Unknown id on `GET` or `DELETE` returns `404` `{ "error": "resource \"<id>\" not found" }`.

Removing a volume row, renaming a container, or deleting a resource does not remove the Docker container or the Docker volume. The next attach uses the stored name. A running container whose image or mount list differs still fails with `container "<name>" is running with a different image or mount list` and is not recreated.

### Environment

`GET /v1/settings` includes `environment`. `PATCH /v1/settings` accepts `{ "environment": { ... } }` and merges it with the grant and extra-path rules above. A sandbox patch is no longer required. A sandbox patch is still accepted and stored, and attach ignores it.

`PATCH /v1/projects/{id}` writes the project override at `settings.environment`. There is no separate link route. An unknown `resourceId` on either patch returns `400` `{ "error": "resource \"<id>\" not found" }`.

Grant rows are checked against one resource at write time. A global environment patch uses the `resourceId` in that patch when it sets one, otherwise the stored global default. A project patch uses the `resourceId` in that patch when it sets one, otherwise the stored project override, otherwise the global default. A grant whose `volumeId` is not on that resource returns `400` `{ "error": "unknown volume \"<id>\"" }`. A grant patch while that resource is missing returns `400` `{ "error": "no resource selected" }`.

`GET /v1/projects/{id}/environment/resolved` returns `200`:

```json
{
  "resourceId": null,
  "resource": null,
  "workspaceRoot": "/workspace",
  "volumes": [],
  "extraPaths": []
}
```

When a resource resolves, `resourceId` is its id and `resource` is the resource JSON. `volumes` lists enabled volumes after grant overrides, with `id`, `name`, `target`, `whitelisted`, `read`, `write`, and `exec`. Disabled volumes are omitted. A grant override whose `volumeId` is not on the resolved resource is ignored. That happens when the global default resource changes after a project stored overrides for the previous one. When no resource resolves, `resourceId` and `resource` are null, `volumes` is empty, and `workspaceRoot` is still resolved. This preview is not an attach failure.

`GET /v1/projects/{id}/sandbox/resolved` and `GET /v1/agents/{id}/sandbox/resolved` are removed.

### Resolution

1. Resource id is the project's when that key is set, otherwise the global default.
2. Workspace root is the project's when that key is set, otherwise the global value, otherwise `/workspace`.
3. Grants start from the resource's enabled volumes.
4. Global grant overrides apply by volume id, then project overrides. An override whose volume id is not on the resolved resource is ignored.
5. Extra paths merge by id, project over global. A disabled extra path is not a grant.

Chat and the Files pane both use this view. The agent id is not an input.

## Attach errors

Chat and the Files pane keep their current error surface. No new dialog.

| Case | Error |
| --- | --- |
| No resolved resource | `project "<id>" has no resource` |
| Id set, row missing | `resource "<id>" not found` |
| Workspace root is not an enabled volume target | `no enabled volume targets workspace root "<path>"` |
| Running container name has a different image or mount list | `container "<name>" is running with a different image or mount list` |

Docker and runtime failures stay as they are. After the idle timeout stops a container, the next attach starts that same name again. Named volumes remain.

File access stays deny-by-default. The whitelist is the resolved volume grants plus enabled extra paths. `read_file` needs read. `write_file` needs write. A mounted volume with `whitelisted: false` is in the container and hidden from file tools. Relative paths join the workspace root. An agent overlay cannot change these grants.

Deleting a project removes `settings.environment` with the project row and leaves the resource. It does not call Docker.

## UI

Settings tabs are Providers, Agents, Projects, Resources, Environment, and Display. The Sandbox tab is removed.

**Resources.** List shows name and kind. Create asks for a display name and, for a container, image, container name, and idle timeout. Volumes are edited on that container: Docker volume name, mount target, enabled, and default whitelist, read, write, and exec. Delete asks for confirmation. A `409` shows "resource in use" and leaves the resource. Deleting a volume row or the resource does not remove Docker objects.

**Environment.** One resource dropdown, which may be empty. Workspace root, extra paths, and grant overrides for the selected resource's volumes. Overrides cannot add a mount.

**Projects.** The isolated/shared control is removed. The environment block has a resource dropdown whose first choice is "Use global default", plus workspace root, extra paths, and grant overrides. The resolved preview calls `GET /v1/projects/{id}/environment/resolved`. A null resource is shown as "no resource".

**Agents.** The sandbox overlay editor is removed. Tool permissions stay out of this slice.

Chat and the Files pane do not grow a resource picker. They attach the resolved resource. When none is selected, they show the attach error.

## Tests

Catalog tests, without a Docker daemon:

- A created container stores prefixed container and volume names. A second resource cannot reuse either name.
- A grant override for an unknown volume id is rejected.
- Resolution applies resource defaults, then global, then project. The project resource id wins. With neither id set, the resolved resource is null.
- Deleting a resource that is the global default or linked by a project returns "resource in use." Deleting an unlinked resource succeeds and does not call Docker.
- The copy turns a shared environment into one container resource and links every project that pointed at it. An isolated project gets its own resource using the Docker names it already has, including `agent-fabric.proj.{projectID}` when that is the workspace disk. The global resource id stays unset. `{threadID}`, `{random}`, and kind `local` abort the copy. Disagreeing image or mount lists abort the copy.
- Attach options use the resolved view. An agent sandbox overlay does not change the resource, workspace root, or grants. A workspace root with no enabled volume target fails with the existing error. Deleting a project leaves the resource row.

HTTP tests cover list, create, get, patch, and delete for `/v1/resources`, the settings `environment` patch, project `settings.environment`, and `GET /v1/projects/{id}/environment/resolved`. The old sandbox resolved routes are absent.

Flutter tests cover the Resources editor, including adding a volume, the Environment tab, the project control whose first choice is "Use global default," and the absence of the agent sandbox overlay.

The existing Docker test that refuses to recreate a running container with a different image or mount list stays.

```bash
go -C controlplane test ./internal/catalog/ ./internal/agent/ ./internal/workspace/
cd client && flutter test test/settings test/catalog/catalog_client_test.dart
```
