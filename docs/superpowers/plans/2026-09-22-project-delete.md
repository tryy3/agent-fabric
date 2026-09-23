# Project Delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete a project from Settings with one `DELETE`, removing its threads and settings, leaving environments and workspace disks, and renaming the protected seeded project to `Default`.

**Architecture:** A new goose migration renames `Personal` to `Default`. `catalog.Store.DeleteProject` runs in one transaction: refuse `Default`, delete that project's threads, then delete the project row. Messages and checkpoints follow existing foreign keys. The Flutter Projects tab confirms, then calls the existing `CatalogClient.deleteProject`. Returning to Chat reloads projects and selects `Default` when the previous selection is gone.

**Tech Stack:** Go (pgx, sqlc, goose), Flutter, Postgres via `dbtest`.

## Global Constraints

- `DELETE /v1/projects/{id}` is the only delete call. Success is `204` with an empty body.
- Unknown id is `404` with `{ "error": "project \"<id>\" not found" }`.
- A project whose name is exactly `Default` cannot be deleted: `409` `{ "error": "default project cannot be deleted" }`.
- Patching that project to a different name is `400` `{ "error": "default project cannot be renamed" }`. Omitting `name`, or setting it to `Default`, still succeeds. Description, isolation, settings, and remotes can change.
- Do not edit `controlplane/internal/db/migrations/00006_projects.up.sql`.
- Migration `00009` up: `UPDATE projects SET name = 'Default' WHERE name = 'Personal';`
- Migration `00009` down: `UPDATE projects SET name = 'Personal' WHERE name = 'Default';`
- `DeleteProject` does not call the sandbox manager and does not remove host directories or environment rows.
- Confirm dialog title is `Delete project?`. Body is `Delete <name>? This deletes the project, its settings, and its threads. Workspace files and the linked environment are left in place.`
- Delete icon key is `delete-project-<id>`, tooltip `Delete project`, hidden when `name == 'Default'`.
- Thread creation with no `projectId` uses the oldest `Default` row (`created_at ASC`) and creates one named `Default` when none exists.

---

### Task 1: Rename the seeded project to Default

**Files:**
- Create: `controlplane/internal/db/migrations/00009_default_project.up.sql`
- Create: `controlplane/internal/db/migrations/00009_default_project.down.sql`
- Modify: `controlplane/internal/db/queries/projects.sql`
- Modify: `controlplane/internal/db/queries/settings.sql`
- Modify: `controlplane/internal/db/projects.sql.go` (sqlc generate)
- Modify: `controlplane/internal/db/settings.sql.go` (sqlc generate)
- Modify: `controlplane/internal/catalog/project_types.go`
- Modify: `controlplane/internal/catalog/projects.go`
- Modify: `controlplane/internal/catalog/settings.go`
- Modify: `controlplane/internal/db/migrate_test.go`
- Modify: `controlplane/internal/db/threads_schema_test.go`
- Modify: `controlplane/internal/catalog/projects_store_test.go`
- Modify: `controlplane/internal/catalog/projects_http_test.go`
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/test/chat/chat_controller_test.dart`
- Modify: `client/test/chat/thread_pane_test.dart`
- Modify: `client/test/catalog/catalog_client_test.dart`
- Modify: `client/test/settings/agents_tab_test.dart`
- Test: `controlplane/internal/db/migrate_test.go`

**Interfaces:**
- Consumes: goose migrations through `00008`; sqlc query names `GetPersonalProject` and `CountNonPersonalProjects`.
- Produces: `catalog.DefaultProjectName = "Default"`; sqlc `GetDefaultProject(ctx) (db.Project, error)` and `CountNonDefaultProjects(ctx) (int64, error)`. `GetPersonalProject`, `CountNonPersonalProjects`, and `PersonalProjectName` are removed.

- [ ] **Step 1: Write the failing migration assertion**

In `controlplane/internal/db/migrate_test.go`, replace the single `db.Migrate` call and the `"Personal"` checks in `TestMigrateBackfillsPersonalProject` with a stop at version 6, then a full migrate:

```go
	if err := db.MigrateTo(ctx, url, 6); err != nil {
		t.Fatalf("migrate to 6: %v", err)
	}

	var name, projectID string
	if err := pool.QueryRow(ctx, `
SELECT p.name, t.project_id
FROM threads t
JOIN projects p ON p.id = t.project_id
WHERE t.id = 'th_old'`).Scan(&name, &projectID); err != nil {
		t.Fatalf("backfill join: %v", err)
	}
	if name != "Personal" {
		t.Fatalf("project name %q", name)
	}
	if !strings.HasPrefix(projectID, "proj_") {
		t.Fatalf("project id %q", projectID)
	}

	if err := db.Migrate(ctx, url); err != nil {
		t.Fatalf("migrate remaining: %v", err)
	}

	if err := pool.QueryRow(ctx, `
SELECT p.name FROM threads t
JOIN projects p ON p.id = t.project_id
WHERE t.id = 'th_old'`).Scan(&name); err != nil {
		t.Fatalf("renamed join: %v", err)
	}
	if name != "Default" {
		t.Fatalf("project name %q", name)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE name = 'Default'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("default count = %d", n)
	}
```

Replace the function from its existing `var name, projectID string` through the final `personal count` check with the block above. Keep the `MigrateTo(ctx, url, 5)` setup and the pre-projects thread insert.

- [ ] **Step 2: Run the migration test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/db/ -run TestMigrateBackfillsPersonalProject -count=1'`

Expected: FAIL with `project name "Personal"` from the post-`Migrate` assertion.

- [ ] **Step 3: Add the migration and point lookups at Default**

Create `controlplane/internal/db/migrations/00009_default_project.up.sql`:

```sql
-- +goose Up
UPDATE projects SET name = 'Default' WHERE name = 'Personal';
```

Create `controlplane/internal/db/migrations/00009_default_project.down.sql`:

```sql
-- +goose Down
UPDATE projects SET name = 'Personal' WHERE name = 'Default';
```

In `controlplane/internal/db/queries/projects.sql`, replace the `GetPersonalProject` query with:

```sql
-- name: GetDefaultProject :one
SELECT id, name, description, isolation, environment_id, settings, remotes, created_at, updated_at
FROM projects
WHERE name = 'Default'
ORDER BY created_at ASC
LIMIT 1;
```

In `controlplane/internal/db/queries/settings.sql`, replace `CountNonPersonalProjects` with:

```sql
-- name: CountNonDefaultProjects :one
SELECT count(*) FROM projects WHERE name <> 'Default';
```

In `controlplane/internal/catalog/project_types.go`, replace `PersonalProjectName = "Personal"` with:

```go
	DefaultProjectName = "Default"
```

In `controlplane/internal/catalog/projects.go` `resolveProjectID`, replace `GetPersonalProject` and `PersonalProjectName`:

```go
	row, err := s.q.GetDefaultProject(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			created, err := s.CreateProject(ctx, DefaultProjectName, "", IsolationIsolated)
			if err != nil {
				return "", err
			}
			return created.ID, nil
		}
		return "", fmt.Errorf("get default project: %w", err)
	}
	return row.ID, nil
```

In `controlplane/internal/catalog/settings.go` `preservePhase1Volumes`, replace `CountNonPersonalProjects` with `CountNonDefaultProjects`.

In `controlplane/internal/catalog/projects_store_test.go`, use `catalog.DefaultProjectName` in `TestCreateListProject`. Rename `TestCreateThreadDefaultsToPersonal` to `TestCreateThreadDefaultsToDefault`.

In `controlplane/internal/catalog/projects_http_test.go`, change the seeded-name check to `seeded[0].Name != catalog.DefaultProjectName`.

In `controlplane/internal/db/threads_schema_test.go`, change both `WHERE name = 'Personal'` filters to `WHERE name = 'Default'`.

In `client/lib/chat/chat_controller.dart` `_pickDefaultProjectId`, compare `p.name == 'Default'`.

In `client/test/chat/chat_controller_test.dart`, set `_personalProject.name` to `'Default'` and expect `c.selectedProject?.name` to be `'Default'`. Rename the test string `connect lists threads for the Personal project` to `connect lists threads for the Default project`. Leave the thread title `Personal notes` as it is.

In `client/test/chat/thread_pane_test.dart`, change the project fixture `name: 'Personal'` to `name: 'Default'`. Leave the thread title `Personal notes`.

In `client/test/catalog/catalog_client_test.dart` `listProjects`, change the JSON `'name': 'Personal'` and `expect(projects.single.name, 'Personal')` to `'Default'`.

In `client/test/settings/agents_tab_test.dart`, change the project fixture `name: 'Personal'` to `name: 'Default'`.

Run sqlc from the directory that contains `sqlc.yaml`:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane/internal/db && sqlc generate'
```

Expected: `projects.sql.go` defines `GetDefaultProject` and no longer defines `GetPersonalProject`. `settings.sql.go` defines `CountNonDefaultProjects` and no longer defines `CountNonPersonalProjects`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/db/ ./internal/catalog/ -count=1'`

Expected: PASS.

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_controller_test.dart test/chat/thread_pane_test.dart test/catalog/catalog_client_test.dart test/settings/agents_tab_test.dart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/db/migrations/00009_default_project.up.sql \
  controlplane/internal/db/migrations/00009_default_project.down.sql \
  controlplane/internal/db/queries/projects.sql \
  controlplane/internal/db/queries/settings.sql \
  controlplane/internal/db/projects.sql.go \
  controlplane/internal/db/settings.sql.go \
  controlplane/internal/catalog/project_types.go \
  controlplane/internal/catalog/projects.go \
  controlplane/internal/catalog/settings.go \
  controlplane/internal/db/migrate_test.go \
  controlplane/internal/db/threads_schema_test.go \
  controlplane/internal/catalog/projects_store_test.go \
  controlplane/internal/catalog/projects_http_test.go \
  client/lib/chat/chat_controller.dart \
  client/test/chat/chat_controller_test.dart \
  client/test/chat/thread_pane_test.dart \
  client/test/catalog/catalog_client_test.dart \
  client/test/settings/agents_tab_test.dart
git commit -m "$(cat <<'EOF'
feat: rename the seeded project to Default

Lookups follow the new name so thread creation still finds the oldest Default project after upgrade.
EOF
)"
```

---

### Task 2: Delete threads with the project

**Files:**
- Modify: `controlplane/internal/db/queries/projects.sql`
- Modify: `controlplane/internal/db/projects.sql.go` (sqlc generate)
- Modify: `controlplane/internal/catalog/projects.go`
- Modify: `controlplane/internal/catalog/projects_store_test.go`
- Modify: `controlplane/internal/catalog/projects_http_test.go`
- Test: `controlplane/internal/catalog/projects_store_test.go`

**Interfaces:**
- Consumes: `catalog.Store.inTx`, `db.Queries.DeleteProject`, `catalog.CreateThreadForProject`, `catalog.CommitTurn`, `catalog.InsertCheckpoint`, `catalog.CreateEnvironment`, `catalog.CreateSharedProject`, `catalog.ErrThreadNotFound`, `catalog.ErrProjectNotFound`.
- Produces: `db.Queries.DeleteThreadsByProject(ctx, projectID string) error`. `DeleteProject` removes the project even when it has threads. Messages and checkpoints for that project are gone. The environment row remains.

- [ ] **Step 1: Write the failing store test**

Replace `TestDeleteProjectInUse` in `controlplane/internal/catalog/projects_store_test.go` with:

```go
func TestDeleteProjectRemovesThreadsMessagesAndCheckpoints(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	store := catalog.Open(pool)
	p, err := store.CreateProject(ctx, "Busy", "", "")
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.CreateThreadForProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitTurn(ctx, th.ID, "hello", catalog.AssistantTurn{Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	threadID := th.ID
	if _, err := store.InsertCheckpoint(ctx, p.ID, "abc123", "before delete", &threadID, nil); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetProject(ctx, p.ID); !errors.Is(err, catalog.ErrProjectNotFound) {
		t.Fatalf("project: %v", err)
	}
	if _, err := store.GetThread(ctx, th.ID); !errors.Is(err, catalog.ErrThreadNotFound) {
		t.Fatalf("thread: %v", err)
	}
	var messages, checkpoints int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM messages`).Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_checkpoints`).Scan(&checkpoints); err != nil {
		t.Fatal(err)
	}
	if messages != 0 || checkpoints != 0 {
		t.Fatalf("messages=%d checkpoints=%d", messages, checkpoints)
	}
}

func TestDeleteProjectLeavesEnvironment(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	volume := "agent-fabric.env.custom"
	env, err := store.CreateEnvironment(ctx, "shared-tools", "docker", &volume)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.CreateSharedProject(ctx, "Shared work", "", env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetEnvironment(ctx, env.ID)
	if err != nil || got.ID != env.ID {
		t.Fatalf("environment: %v %+v", err, got)
	}
}
```

- [ ] **Step 2: Run the store test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run "TestDeleteProjectRemovesThreadsMessagesAndCheckpoints|TestDeleteProjectLeavesEnvironment" -count=1'`

Expected: FAIL. `TestDeleteProjectRemovesThreadsMessagesAndCheckpoints` fails because delete returns `project in use`. `TestDeleteProjectLeavesEnvironment` may already pass; that is fine. The thread test must fail.

- [ ] **Step 3: Delete threads inside the project delete transaction**

Append to `controlplane/internal/db/queries/projects.sql`:

```sql
-- name: DeleteThreadsByProject :exec
DELETE FROM threads WHERE project_id = $1;
```

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane/internal/db && sqlc generate'
```

Replace `DeleteProject` in `controlplane/internal/catalog/projects.go` with:

```go
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetProject(ctx, id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return newProjectNotFound(id)
			}
			return fmt.Errorf("get project: %w", err)
		}
		if err := q.DeleteThreadsByProject(ctx, id); err != nil {
			return fmt.Errorf("delete project threads: %w", err)
		}
		if err := q.DeleteProject(ctx, id); err != nil {
			if isFKViolation(err) {
				return ErrProjectInUse
			}
			return fmt.Errorf("delete project: %w", err)
		}
		return nil
	})
}
```

Replace `TestProjectsHTTPDeleteConflictWhenThreadsRemain` in `controlplane/internal/catalog/projects_http_test.go` with:

```go
func TestProjectsHTTPDeleteRemovesThreads(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/projects", "application/json", strings.NewReader(`{"name":"Busy"}`))
	if err != nil {
		t.Fatal(err)
	}
	var p catalog.Project
	_ = json.NewDecoder(resp.Body).Decode(&p)
	resp.Body.Close()

	threadBody := `{"projectId":"` + p.ID + `"}`
	thResp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(threadBody))
	if err != nil {
		t.Fatal(err)
	}
	if thResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(thResp.Body)
		thResp.Body.Close()
		t.Fatalf("create thread %d %s", thResp.StatusCode, body)
	}
	var th catalog.Thread
	if err := json.NewDecoder(thResp.Body).Decode(&th); err != nil {
		t.Fatal(err)
	}
	thResp.Body.Close()

	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/projects/"+p.ID, nil)
	delResp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete %d", delResp.StatusCode)
	}

	missing, err := http.Get(srv.URL + "/v1/projects/" + p.ID)
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("project %d", missing.StatusCode)
	}
	missingThread, err := http.Get(srv.URL + "/v1/threads/" + th.ID)
	if err != nil {
		t.Fatal(err)
	}
	missingThread.Body.Close()
	if missingThread.StatusCode != http.StatusNotFound {
		t.Fatalf("thread %d", missingThread.StatusCode)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -count=1'`

Expected: PASS. `TestDeleteProjectRemovesThreadsMessagesAndCheckpoints`, `TestDeleteProjectLeavesEnvironment`, and `TestProjectsHTTPDeleteRemovesThreads` pass.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/db/queries/projects.sql \
  controlplane/internal/db/projects.sql.go \
  controlplane/internal/catalog/projects.go \
  controlplane/internal/catalog/projects_store_test.go \
  controlplane/internal/catalog/projects_http_test.go
git commit -m "$(cat <<'EOF'
feat: delete a project's threads with the project

Messages and checkpoints go with the row. The linked environment row stays.
EOF
)"
```

---

### Task 3: Protect the Default project

**Files:**
- Modify: `controlplane/internal/catalog/store.go`
- Modify: `controlplane/internal/catalog/projects.go`
- Modify: `controlplane/internal/catalog/http.go`
- Modify: `controlplane/internal/catalog/projects_store_test.go`
- Modify: `controlplane/internal/catalog/projects_http_test.go`
- Test: `controlplane/internal/catalog/projects_store_test.go`

**Interfaces:**
- Consumes: `catalog.DefaultProjectName` from Task 1. `DeleteProject` from Task 2.
- Produces: `catalog.ErrDefaultProject` (`default project cannot be deleted`) and `catalog.ErrDefaultProjectRename` (`default project cannot be renamed`). `DELETE` maps the first to 409. `PATCH` maps the second to 400.

- [ ] **Step 1: Write the failing store and HTTP tests**

Append to `controlplane/internal/catalog/projects_store_test.go`:

```go
func TestDefaultProjectCannotBeDeletedOrRenamed(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	list, err := store.ListProjects(ctx)
	if err != nil || len(list) != 1 || list[0].Name != catalog.DefaultProjectName {
		t.Fatalf("seeded: %v %+v", err, list)
	}
	def := list[0]

	if err := store.DeleteProject(ctx, def.ID); !errors.Is(err, catalog.ErrDefaultProject) {
		t.Fatalf("delete default: %v", err)
	}
	if _, err := store.GetProject(ctx, def.ID); err != nil {
		t.Fatal(err)
	}

	other, err := store.CreateProject(ctx, catalog.DefaultProjectName, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteProject(ctx, other.ID); !errors.Is(err, catalog.ErrDefaultProject) {
		t.Fatalf("delete second default: %v", err)
	}

	renamed := "Renamed"
	if _, err := store.UpdateProject(ctx, def.ID, &renamed, nil, nil, nil); !errors.Is(err, catalog.ErrDefaultProjectRename) {
		t.Fatalf("rename: %v", err)
	}
	desc := "kept"
	updated, err := store.UpdateProject(ctx, def.ID, nil, &desc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != catalog.DefaultProjectName || updated.Description != "kept" {
		t.Fatalf("description patch = %+v", updated)
	}
	same := catalog.DefaultProjectName
	updated, err = store.UpdateProject(ctx, def.ID, &same, nil, nil, nil)
	if err != nil || updated.Name != catalog.DefaultProjectName {
		t.Fatalf("same name: %v %+v", err, updated)
	}
}
```

Append to `controlplane/internal/catalog/projects_http_test.go`:

```go
func TestProjectsHTTPDefaultGuards(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	listResp, err := http.Get(srv.URL + "/v1/projects")
	if err != nil {
		t.Fatal(err)
	}
	var seeded []catalog.Project
	if err := json.NewDecoder(listResp.Body).Decode(&seeded); err != nil {
		t.Fatal(err)
	}
	listResp.Body.Close()
	if len(seeded) != 1 || seeded[0].Name != catalog.DefaultProjectName {
		t.Fatalf("seeded %+v", seeded)
	}
	def := seeded[0]

	del, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/projects/"+def.ID, nil)
	delResp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(delResp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusConflict || payload.Error != "default project cannot be deleted" {
		t.Fatalf("delete %d %q", delResp.StatusCode, payload.Error)
	}

	patch, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/projects/"+def.ID, strings.NewReader(`{"name":"Renamed"}`))
	patch.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patch)
	if err != nil {
		t.Fatal(err)
	}
	payload.Error = ""
	if err := json.NewDecoder(patchResp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusBadRequest || payload.Error != "default project cannot be renamed" {
		t.Fatalf("rename %d %q", patchResp.StatusCode, payload.Error)
	}

	ok, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/projects/"+def.ID, strings.NewReader(`{"description":"kept"}`))
	ok.Header.Set("Content-Type", "application/json")
	okResp, err := http.DefaultClient.Do(ok)
	if err != nil {
		t.Fatal(err)
	}
	var updated catalog.Project
	if err := json.NewDecoder(okResp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK || updated.Name != catalog.DefaultProjectName || updated.Description != "kept" {
		t.Fatalf("description %d %+v", okResp.StatusCode, updated)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run "TestDefaultProjectCannotBeDeletedOrRenamed|TestProjectsHTTPDefaultGuards" -count=1'`

Expected: FAIL to compile. `catalog.ErrDefaultProject` and `catalog.ErrDefaultProjectRename` are undefined. Do not add the sentinels in this step.

- [ ] **Step 3: Refuse delete and rename**

In `controlplane/internal/catalog/store.go`, next to `ErrProjectInUse`:

```go
	ErrDefaultProject       = errors.New("default project cannot be deleted")
	ErrDefaultProjectRename = errors.New("default project cannot be renamed")
```

In `DeleteProject`, after the `GetProject` call succeeds and before `DeleteThreadsByProject`, load the name. Replace the `GetProject` block with:

```go
		row, err := q.GetProject(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return newProjectNotFound(id)
			}
			return fmt.Errorf("get project: %w", err)
		}
		if row.Name == DefaultProjectName {
			return ErrDefaultProject
		}
```

In `UpdateProject`, inside the `if name != nil` block, after the empty-name check and before `current.Name = trimmed`:

```go
		if current.Name == DefaultProjectName && trimmed != DefaultProjectName {
			return Project{}, ErrDefaultProjectRename
		}
```

In `writeMappedError` in `controlplane/internal/catalog/http.go`, before the not-found check:

```go
	if errors.Is(err, ErrDefaultProject) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, ErrDefaultProjectRename) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/store.go \
  controlplane/internal/catalog/projects.go \
  controlplane/internal/catalog/http.go \
  controlplane/internal/catalog/projects_store_test.go \
  controlplane/internal/catalog/projects_http_test.go
git commit -m "$(cat <<'EOF'
feat: keep the Default project from being deleted or renamed

Any project named Default returns 409 on delete and 400 on rename.
EOF
)"
```

---

### Task 4: Settings delete confirmation

**Files:**
- Modify: `client/lib/settings/projects_tab.dart`
- Modify: `client/test/settings/projects_tab_test.dart`
- Test: `client/test/settings/projects_tab_test.dart`

**Interfaces:**
- Consumes: `CatalogClient.deleteProject(String id)` (already on the client).
- Produces: a delete icon on each Projects row except `name == 'Default'`, and a confirm dialog that calls `deleteProject` only after Delete.

- [ ] **Step 1: Write the failing widget test**

On `FakeProjectsCatalog` in `client/test/settings/projects_tab_test.dart`, add:

```dart
  String? lastDeleteId;

  @override
  Future<void> deleteProject(String id) async {
    lastDeleteId = id;
    projects.removeWhere((p) => p.id == id);
  }
```

Append this test:

```dart
  testWidgets('delete project confirms, and Default has no delete icon', (
    tester,
  ) async {
    final catalog = FakeProjectsCatalog(
      projects: [
        _project(id: 'proj_default', name: 'Default'),
        _project(id: 'proj_land', name: 'Landing'),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: ProjectsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('delete-project-proj_default')), findsNothing);
    expect(find.byKey(const Key('delete-project-proj_land')), findsOneWidget);

    await tester.tap(find.byKey(const Key('delete-project-proj_land')));
    await tester.pumpAndSettle();
    expect(find.text('Delete project?'), findsOneWidget);
    expect(
      find.text(
        'Delete Landing? This deletes the project, its settings, and its threads. Workspace files and the linked environment are left in place.',
      ),
      findsOneWidget,
    );

    await tester.tap(find.widgetWithText(TextButton, 'Cancel'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, isNull);
    expect(find.text('Landing'), findsOneWidget);

    await tester.tap(find.byKey(const Key('delete-project-proj_land')));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'proj_land');
    expect(find.text('Landing'), findsNothing);
    expect(find.text('Default'), findsOneWidget);
  });
```

- [ ] **Step 2: Run the widget test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/settings/projects_tab_test.dart'`

Expected: FAIL because `delete-project-proj_land` is not found.

- [ ] **Step 3: Add the confirm dialog**

In `_ProjectsTabState` in `client/lib/settings/projects_tab.dart`, add:

```dart
  Future<void> _confirmDelete(Project project) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) {
        return AlertDialog(
          title: const Text('Delete project?'),
          content: Text(
            'Delete ${project.name}? This deletes the project, its settings, and its threads. Workspace files and the linked environment are left in place.',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(false),
              child: const Text('Cancel'),
            ),
            TextButton(
              onPressed: () => Navigator.of(context).pop(true),
              child: const Text('Delete'),
            ),
          ],
        );
      },
    );
    if (confirmed != true || !mounted) {
      return;
    }
    try {
      await widget.catalog.deleteProject(project.id);
      await _reload();
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = e.toString();
      });
    }
  }
```

On the `ListTile` in `_buildBody`, add a trailing button only when the name is not `Default`:

```dart
              return ListTile(
                key: Key('project-${project.id}'),
                title: Text(project.name),
                subtitle: Text(project.isolation),
                trailing: project.name == 'Default'
                    ? null
                    : IconButton(
                        key: Key('delete-project-${project.id}'),
                        tooltip: 'Delete project',
                        icon: const Icon(Icons.delete),
                        onPressed: () => _confirmDelete(project),
                      ),
                onTap: () => _openEditor(project),
              );
```

- [ ] **Step 4: Run the widget test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/settings/projects_tab_test.dart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/settings/projects_tab.dart client/test/settings/projects_tab_test.dart
git commit -m "$(cat <<'EOF'
feat: confirm project delete from Settings

Default has no delete icon. Other projects delete only after the dialog confirms.
EOF
)"
```

---

### Task 5: Chat falls back to Default

**Files:**
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/test/chat/chat_controller_test.dart`
- Test: `client/test/chat/chat_controller_test.dart`

**Interfaces:**
- Consumes: `_pickDefaultProjectId` matching `name == 'Default'` from Task 1. `selectProject(String id)`. `FakeCatalog.projects` and `FakeCatalog.listThreads`.
- Produces: `reloadAgents` reloads projects. If `selectedProjectId` is missing, it calls `selectProject` with the default project id.

- [ ] **Step 1: Write the failing controller test**

Append to `client/test/chat/chat_controller_test.dart`:

```dart
  test('reloadAgents selects Default when the selected project is gone', () async {
    final landing = Project(
      id: 'proj_land',
      name: 'Landing',
      createdAt: DateTime.utc(2026, 9, 20),
      updatedAt: DateTime.utc(2026, 9, 20),
    );
    final catalog = FakeCatalog(
      [],
      projects: [_personalProject, landing],
      threads: [
        _thread(
          id: 'th_p',
          title: 'Default notes',
          projectId: _personalProject.id,
        ),
        _thread(id: 'th_l', title: 'Landing chat', projectId: landing.id),
      ],
    );
    final c = ChatController(session: FakeConn(), catalog: catalog);
    await c.connect();
    await c.selectProject(landing.id);
    expect(c.selectedThreadId, 'th_l');

    catalog.projects.removeWhere((p) => p.id == landing.id);
    await c.reloadAgents();

    expect(c.selectedProjectId, _personalProject.id);
    expect(c.selectedProject?.name, 'Default');
    expect(c.selectedThreadId, 'th_p');
    expect(c.threads.single.id, 'th_p');
  });
```

`_personalProject.name` is already `'Default'` from Task 1.

- [ ] **Step 2: Run the controller test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_controller_test.dart --name "reloadAgents selects Default"'`

Expected: FAIL. `selectedProjectId` stays `proj_land` because `reloadAgents` does not reload projects.

- [ ] **Step 3: Reload projects inside reloadAgents**

Replace `reloadAgents` in `client/lib/chat/chat_controller.dart` with:

```dart
  Future<void> reloadAgents() async {
    final catalog = _catalog;
    if (catalog == null) {
      return;
    }
    try {
      agents = await catalog.listAgents();
    } catch (e) {
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    try {
      projects = await catalog.listProjects();
    } catch (e) {
      statusMessage = formatChatError(e);
      notifyListeners();
      return;
    }
    final selectedMissing =
        selectedProjectId == null ||
        !projects.any((p) => p.id == selectedProjectId);
    if (selectedMissing) {
      final next = _pickDefaultProjectId();
      if (next == null) {
        selectedProjectId = null;
        selectedThreadId = null;
        messages.clear();
        threads = [];
        selectedAgentId = null;
        _sessionReady = false;
        notifyListeners();
        return;
      }
      if (next != selectedProjectId) {
        await selectProject(next);
      }
    }
    if (_sessionStarting) {
      notifyListeners();
      return;
    }
    final id = selectedAgentId;
    if (id != null && selectedAgentIsComplete && !_sessionReady) {
      await _startSession(id, rebind: true);
      return;
    }
    if (!selectedAgentIsComplete) {
      _sessionReady = false;
    }
    notifyListeners();
  }
```

`selectProject` clears the old thread, messages, agent, and session, then loads the default project's threads and selects the first one.

- [ ] **Step 4: Run the controller test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_controller_test.dart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_controller.dart client/test/chat/chat_controller_test.dart
git commit -m "$(cat <<'EOF'
feat: select Default when the open project was deleted

Returning to Chat reloads projects and drops a selection that no longer exists.
EOF
)"
```
