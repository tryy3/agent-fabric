# Delete a project from Settings

**Date:** 2026-09-22
**Status:** approved for planning
**Builds on:** projects Phases 0–5 (PR #31)

## Problem

`DELETE /v1/projects/{id}` exists, and `CatalogClient.deleteProject` calls it, but Settings → Projects has no delete action. The handler also refuses while any thread remains (`409`, `project in use`). The seeded project is named `Personal` and is identified only by that name.

## Goals

- Delete a project from Settings → Projects, behind a confirmation dialog.
- One `DELETE /v1/projects/{id}` removes the project, its settings and remotes, its threads, and those threads' messages.
- Leave linked environments, Docker volumes, and the local project workspace on disk.
- The seeded project is renamed from `Personal` to `Default`. Any project named `Default` cannot be deleted or renamed.
- If Chat had that project selected, returning to Chat selects `Default`.

## Non-goals

- Deleting or garbage-collecting Docker volumes, containers, or `{dataDir}/projects/{id}`.
- Deleting environment rows, including an environment that only this project used.
- A separate "is default" column. The name `Default` is the marker, same as `Personal` is today.
- Import, remotes sync, or stricter isolation cleanup.

## Approach

**Chosen:** one catalog `DELETE` whose store method does the cleanup in a single transaction.

The client already has `deleteProject`. The store deletes that project's threads, then the project row. Messages cascade from threads. Checkpoints cascade from the project. Settings and remotes are columns on the project row. The environment foreign key points from the project at `environments`, so removing the project drops the link and leaves the environment row.

**Rejected:**

- `ON DELETE CASCADE` from `threads.project_id`. The thread delete would be invisible in the store, and it needs a foreign-key migration.
- The client deleting threads one by one, then the project. A thread created between calls makes the project delete fail.

`00006_projects` is not edited. Databases that already applied it keep a valid goose checksum. A new migration renames the seeded row.

## API

`DELETE /v1/projects/{id}`

| Case | Status | Body |
| --- | --- | --- |
| Project removed | 204 | empty |
| Unknown id | 404 | `{ "error": "project \"<id>\" not found" }` |
| Name is `Default` | 409 | `{ "error": "default project cannot be deleted" }` |

`PATCH /v1/projects/{id}` when the stored name is `Default` and the patch sets a different `name`:

| Case | Status | Body |
| --- | --- | --- |
| Rename away from `Default` | 400 | `{ "error": "default project cannot be renamed" }` |
| Patch omits `name`, or sets `name` to `Default` | existing success path | project JSON |

Description, isolation, settings, and remotes on `Default` can still change. Every project whose name is exactly `Default` is protected, not only the oldest. Thread creation with no `projectId` still uses the oldest `Default` row (`created_at ASC`), and creates one named `Default` when none exists.

`project in use` is no longer returned when threads remain. `ErrProjectInUse` stays mapped to 409 only as a foreign-key safety net.

## Data

Inside the existing `inTx` helper:

1. Load the project. Missing → not found.
2. If `name == Default`, return the delete sentinel. No rows change.
3. `DELETE FROM threads WHERE project_id = $1`. `messages.thread_id` is `ON DELETE CASCADE`. `project_checkpoints.thread_id` is `ON DELETE SET NULL`.
4. `DELETE FROM projects WHERE id = $1`. `project_checkpoints.project_id` is `ON DELETE CASCADE`, so checkpoints for that project are removed. `settings` and `remotes` go with the row.

`DeleteProject` does not call the sandbox manager and does not remove host directories.

Rename guard: `UpdateProject`, before writing, if the current name is `Default` and the patch name is non-nil and not `Default`, return the rename sentinel.

Migration `00009_default_project`:

```sql
UPDATE projects SET name = 'Default' WHERE name = 'Personal';
```

Down migration:

```sql
UPDATE projects SET name = 'Personal' WHERE name = 'Default';
```

That also renames a project a user later created with the name `Default`. Accepted for this slice.

Lookups that match `Personal` match `Default` instead:

- Go constant `DefaultProjectName = "Default"` (replaces `PersonalProjectName`).
- sqlc `GetDefaultProject` (`WHERE name = 'Default' ORDER BY created_at ASC LIMIT 1`).
- sqlc `CountNonDefaultProjects` (`WHERE name <> 'Default'`).
- Flutter `_pickDefaultProjectId` compares `name == 'Default'`.

Fresh installs still run `00006`, which inserts `Personal`, then `00009`, which renames it.

## UI

Settings → Projects list, same pattern as Agents:

- `IconButton` tooltip `Delete project`, key `delete-project-<id>`, on each row except when `name == 'Default'`.
- Confirm dialog title `Delete project?`. Body: `Delete <name>? This deletes the project, its settings, and its threads. Workspace files and the linked environment are left in place.`
- Actions: `Cancel`, `Delete`.
- On confirm, call `deleteProject`, then reload the list. On failure, show the error on the tab the way agent delete does.

Chat: `reloadAgents` already runs when the Chat rail destination is selected. It also reloads projects. If `selectedProjectId` is missing from that list, apply the same reset as `selectProject` toward the default project: clear the selected thread, messages, agent, and session, load that project's threads, and select the first thread when one exists.

## Tests

- Store: deleting a project with a thread removes the project, the thread, and its messages; an environment row referenced by the project remains; `Default` delete returns the sentinel and the row remains; renaming `Default` returns the sentinel; patching `Default`'s description succeeds.
- HTTP: delete with threads returns 204 and a following GET is 404; delete `Default` returns 409 with `default project cannot be deleted`; patch rename of `Default` returns 400 with `default project cannot be renamed`.
- Migration: upgrade through `00006` still backfills the name `Personal`; migrating the rest leaves that row named `Default`.
- Flutter Projects tab: `Default` has no delete icon; another project opens the confirm dialog; confirming calls delete and the row disappears; cancel leaves the row.
- Flutter chat: `_pickDefaultProjectId` and the connect test use `Default`. A test that the selected project was removed reloads onto `Default` and clears the old thread.

## Files

- `controlplane/internal/db/migrations/00009_default_project.up.sql` and `.down.sql`
- `controlplane/internal/db/queries/projects.sql`, `settings.sql`, generated sqlc
- `controlplane/internal/catalog/projects.go`, `project_types.go`, `http.go`, `settings.go`
- `controlplane/internal/catalog/projects_store_test.go`, `projects_http_test.go`
- `controlplane/internal/db/migrate_test.go` and other tests that assert the seeded name `Personal`
- `client/lib/settings/projects_tab.dart`, `client/lib/chat/chat_controller.dart`
- `client/test/settings/projects_tab_test.dart`, `client/test/chat/chat_controller_test.dart`, and other client tests whose fixture name is the seeded project
