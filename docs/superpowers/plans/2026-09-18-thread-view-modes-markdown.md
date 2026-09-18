# Thread View Modes & Markdown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a per-thread view mode on the control plane and render chat (Pretty = markdown, Detailed = plain) from a client mode registry that also owns thinking/tool visibility knobs.

**Architecture:** Add nullable `threads.view_mode_id` and partial `PATCH /v1/threads/{id}` for `viewModeId`. Flutter keeps a hardcoded `ViewMode` registry (`pretty` / `detailed`), resolves `thread.viewModeId ?? appDefault`, drives `AgentBubble` / user bubbles / thinking / tools from that config, and switches modes via a thread-header dropdown.

**Tech Stack:** Go (pgx/sqlc/goose), Flutter (Nix shell), `flutter_markdown_plus`, `shared_preferences` (content width only after Thinking removal). Work from `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix with `nix develop /home/tryy3/src/agent-fabric -c` and run from `client/`. Go tests: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./...'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-18-thread-view-modes-markdown-design.md`](../specs/2026-09-18-thread-view-modes-markdown-design.md).
- Built-in mode ids only: `pretty`, `detailed`. App default: `pretty`.
- v1 presets differ by `markdownRender` only; thinking/tools both `collapsed`, `toolIO` = `both`, `rawRequests` = false.
- Markdown for assistant answers + user messages only; thinking/tool I/O stay plain.
- Remove Thinking visibility from Display settings; stop reading/writing `chat.thinkingVisibility`.
- No custom mode editor, no Settings default-mode UI, no raw request inspector, no `url_launcher`.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/db/migrations/00005_thread_view_mode.{up,down}.sql` | `view_mode_id` column |
| `controlplane/internal/db/queries/threads.sql` | SELECT/RETURNING + `SetThreadViewMode` |
| `controlplane/internal/db/*.go` | sqlc generated |
| `controlplane/internal/catalog/thread_types.go` | `Thread.ViewModeID` |
| `controlplane/internal/catalog/store.go` | map column; `SetThreadViewMode` |
| `controlplane/internal/catalog/http.go` | partial patch + validation |
| `controlplane/internal/catalog/optional_string.go` | omit vs JSON null for patch |
| `controlplane/internal/catalog/view_modes.go` | allowlist `pretty`/`detailed` |
| `client/lib/chat/view_modes.dart` | `ViewMode`, `ToolIOMode`, registry, `resolveViewMode` |
| `client/lib/chat/message_text.dart` | plain vs `MarkdownBody` helper |
| `client/lib/chat/display_settings.dart` | drop thinking in Task 6; keep width + `VisibilityMode` |
| `client/lib/settings/display_tab.dart` | remove Thinking UI in Task 6 |
| `client/lib/catalog/models.dart` | `ThreadSummary.viewModeId` |
| `client/lib/catalog/catalog_client.dart` | `patchThreadViewMode` |
| `client/lib/chat/chat_controller.dart` | `setThreadViewMode` optimistic + PATCH |
| `client/lib/chat/agent_bubble.dart` | mode knobs → thinking/tools/markdown |
| `client/lib/chat/chat_screen.dart` | header dropdown; pass mode; user markdown |
| `client/pubspec.yaml` | `flutter_markdown_plus` |

**Interfaces this plan locks:**

```go
// catalog
type Thread struct {
  // ...existing fields...
  ViewModeID *string `json:"viewModeId"` // nil => JSON null
}

func ValidViewModeID(id string) bool // "pretty" | "detailed"

func (s *Store) SetThreadViewMode(ctx context.Context, id string, viewModeID *string) (Thread, error)

// PATCH body
type threadPatch struct {
  Title      *string        `json:"title"`
  ViewModeID optionalString `json:"viewModeId"`
}
```

```dart
// view_modes.dart
enum ToolIOMode { input, output, both }

class ViewMode {
  const ViewMode({
    required this.id,
    required this.label,
    required this.markdownRender,
    required this.thinkingVisibility,
    required this.toolVisibility,
    required this.toolIO,
    this.rawRequests = false,
  });
  final String id;
  final String label;
  final bool markdownRender;
  final VisibilityMode thinkingVisibility;
  final VisibilityMode toolVisibility;
  final ToolIOMode toolIO;
  final bool rawRequests;
}

const kDefaultViewModeId = 'pretty';
const kBuiltInViewModes = <ViewMode>[/* pretty, detailed */];
ViewMode resolveViewMode(String? threadViewModeId, {String appDefaultId = kDefaultViewModeId});

// catalog
class ThreadSummary {
  final String? viewModeId;
  ThreadSummary copyWith({Object? viewModeId = _sentinel, ...});
}

// CatalogClient
Future<ThreadSummary> patchThreadViewMode(String id, String? viewModeId);

// ChatController
Future<void> setThreadViewMode(String modeId);

// AgentBubble
class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    required this.viewMode,
    this.stats,
  });
  final ViewMode viewMode;
  // no thinkingMode param
}
```

---

### Task 1: Persist `view_mode_id` (migration + sqlc + Thread JSON)

**Files:**
- Create: `controlplane/internal/db/migrations/00005_thread_view_mode.up.sql`
- Create: `controlplane/internal/db/migrations/00005_thread_view_mode.down.sql`
- Modify: `controlplane/internal/db/queries/threads.sql` (every thread SELECT/RETURNING list + new query)
- Modify: generated `controlplane/internal/db/*.go` via `sqlc generate`
- Modify: `controlplane/internal/catalog/thread_types.go`
- Modify: `controlplane/internal/catalog/store.go` (`threadFromRow`, `threadFromListRow`)
- Test: `controlplane/internal/db/threads_schema_test.go` (extend) or new store test in Task 2

**Interfaces:**
- Consumes: existing `db.Thread` / list rows
- Produces: `Thread.ViewModeID *string`; column present in all thread queries

- [ ] **Step 1: Write the failing schema assertion**

In `controlplane/internal/db/threads_schema_test.go`, after existing inserts work, add:

```go
func TestThreadsViewModeIDColumn(t *testing.T) {
	pool := Open(t) // use same helper as file already uses
	ctx := context.Background()
	var col string
	err := pool.QueryRow(ctx, `
SELECT column_name FROM information_schema.columns
WHERE table_name = 'threads' AND column_name = 'view_mode_id'`).Scan(&col)
	if err != nil {
		t.Fatalf("view_mode_id missing: %v", err)
	}
}
```

(Adapt `Open` / package name to match the existing test file — it uses `dbtest` or local helpers.)

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/db/ -run TestThreadsViewModeIDColumn -v'`

Expected: FAIL (column missing or query error).

- [ ] **Step 3: Add migration + update SQL + regenerate**

`00005_thread_view_mode.up.sql`:

```sql
-- +goose Up
ALTER TABLE threads
  ADD COLUMN view_mode_id text NULL;
```

`00005_thread_view_mode.down.sql`:

```sql
-- +goose Down
ALTER TABLE threads
  DROP COLUMN view_mode_id;
```

Update **every** thread column list in `queries/threads.sql` to include `view_mode_id` after `current_model` (InsertThread RETURNING, ListThreads SELECT, GetThread, GetThreadForUpdate, RenameThread RETURNING, PinThreadAgent RETURNING, SetThreadModel RETURNING).

Add:

```sql
-- name: SetThreadViewMode :one
UPDATE threads
SET view_mode_id = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, agent_id, current_model, view_mode_id, created_at, updated_at;
```

`InsertThread` does **not** need to write `view_mode_id` (defaults NULL).

From `controlplane/internal/db`:

```bash
nix develop /home/tryy3/src/agent-fabric -c sqlc generate
```

Add to `Thread` in `thread_types.go`:

```go
ViewModeID *string `json:"viewModeId"`
```

Update `threadFromRow` / `threadFromListRow`:

```go
ViewModeID: row.ViewModeID,
```

(Field name from sqlc may be `ViewModeID` with `emit_pointers_for_null_types`.)

Fix any compile breaks in store from new Scan fields.

- [ ] **Step 4: Run test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/db/ ./internal/catalog/ -count=1'`

Expected: PASS (or only pre-existing failures unrelated — fix compile errors first).

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/db/migrations/00005_thread_view_mode.up.sql \
  controlplane/internal/db/migrations/00005_thread_view_mode.down.sql \
  controlplane/internal/db/queries/threads.sql \
  controlplane/internal/db/*.go \
  controlplane/internal/catalog/thread_types.go \
  controlplane/internal/catalog/store.go \
  controlplane/internal/db/threads_schema_test.go
git commit -m "$(cat <<'EOF'
feat(controlplane): add threads.view_mode_id column

EOF
)"
```

---

### Task 2: PATCH view mode (store + HTTP validation)

**Files:**
- Create: `controlplane/internal/catalog/optional_string.go`
- Create: `controlplane/internal/catalog/view_modes.go`
- Modify: `controlplane/internal/catalog/store.go` — `SetThreadViewMode`
- Modify: `controlplane/internal/catalog/http.go` — `threadPatch` + `patchThread`
- Modify: `controlplane/internal/catalog/threads_http_test.go`
- Modify: `controlplane/internal/catalog/threads_store_test.go`

**Interfaces:**
- Consumes: `db.SetThreadViewMode`
- Produces: `SetThreadViewMode`, `ValidViewModeID`, partial PATCH

- [ ] **Step 1: Write the failing tests**

Add to `threads_store_test.go`:

```go
func TestSetThreadViewMode(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	ctx := context.Background()
	th, err := store.CreateThread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if th.ViewModeID != nil {
		t.Fatalf("new thread viewModeId=%v, want nil", th.ViewModeID)
	}
	pretty := "pretty"
	updated, err := store.SetThreadViewMode(ctx, th.ID, &pretty)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ViewModeID == nil || *updated.ViewModeID != "pretty" {
		t.Fatalf("got %v", updated.ViewModeID)
	}
	cleared, err := store.SetThreadViewMode(ctx, th.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ViewModeID != nil {
		t.Fatalf("cleared = %v", cleared.ViewModeID)
	}
}
```

Add to `threads_http_test.go` (new test function):

```go
func TestThreadsHTTPPatchViewMode(t *testing.T) {
	store := catalog.Open(dbtest.Open(t))
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/threads", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var created catalog.Thread
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID,
		strings.NewReader(`{"viewModeId":"detailed"}`))
	req.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", patchResp.StatusCode)
	}
	var patched catalog.Thread
	_ = json.NewDecoder(patchResp.Body).Decode(&patched)
	patchResp.Body.Close()
	if patched.ViewModeID == nil || *patched.ViewModeID != "detailed" {
		t.Fatalf("patched %+v", patched)
	}

	// rename-only still works
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID,
		strings.NewReader(`{"title":"Renamed"}`))
	req.Header.Set("Content-Type", "application/json")
	patchResp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(patchResp.Body).Decode(&patched)
	patchResp.Body.Close()
	if patched.Title != "Renamed" {
		t.Fatalf("title %q", patched.Title)
	}
	if patched.ViewModeID == nil || *patched.ViewModeID != "detailed" {
		t.Fatalf("view mode lost: %+v", patched)
	}

	// clear
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID,
		strings.NewReader(`{"viewModeId":null}`))
	req.Header.Set("Content-Type", "application/json")
	patchResp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(patchResp.Body).Decode(&patched)
	patchResp.Body.Close()
	if patched.ViewModeID != nil {
		t.Fatalf("want null, got %v", patched.ViewModeID)
	}

	// invalid
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/v1/threads/"+created.ID,
		strings.NewReader(`{"viewModeId":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	bad, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", bad.StatusCode)
	}
	bad.Body.Close()
}
```

Also update existing rename PATCH test if `threadPatch.Title` becomes `*string` — body `{"title":"Renamed"}` still works.

- [ ] **Step 2: Run tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -run "TestSetThreadViewMode|TestThreadsHTTPPatchViewMode" -v'`

Expected: FAIL (missing symbols / wrong PATCH behavior).

- [ ] **Step 3: Implement**

`view_modes.go`:

```go
package catalog

func ValidViewModeID(id string) bool {
	switch id {
	case "pretty", "detailed":
		return true
	default:
		return false
	}
}
```

`optional_string.go`:

```go
package catalog

import "encoding/json"

// optionalString distinguishes omitted JSON keys from explicit null.
type optionalString struct {
	Present bool
	Value   *string
}

func (o *optionalString) UnmarshalJSON(b []byte) error {
	o.Present = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}
```

`SetThreadViewMode` in `store.go`:

```go
func (s *Store) SetThreadViewMode(ctx context.Context, id string, viewModeID *string) (Thread, error) {
	row, err := s.q.SetThreadViewMode(ctx, db.SetThreadViewModeParams{
		ID:         id,
		ViewModeID: viewModeID,
		UpdatedAt:  timestamptzFromTime(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Thread{}, newThreadNotFound(id)
		}
		return Thread{}, fmt.Errorf("set thread view mode: %w", err)
	}
	return threadFromRow(row), nil
}
```

Replace `threadPatch` + `patchThread` in `http.go`:

```go
type threadPatch struct {
	Title      *string        `json:"title"`
	ViewModeID optionalString `json:"viewModeId"`
}

func (h *httpAPI) patchThread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body threadPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Title == nil && !body.ViewModeID.Present {
		writeError(w, http.StatusBadRequest, "empty patch")
		return
	}
	var th Thread
	var err error
	if body.Title != nil {
		th, err = h.store.RenameThread(r.Context(), id, *body.Title)
		if err != nil {
			writeMappedError(w, err, id)
			return
		}
	}
	if body.ViewModeID.Present {
		if body.ViewModeID.Value != nil && !ValidViewModeID(*body.ViewModeID.Value) {
			writeError(w, http.StatusBadRequest, "invalid viewModeId")
			return
		}
		th, err = h.store.SetThreadViewMode(r.Context(), id, body.ViewModeID.Value)
		if err != nil {
			writeMappedError(w, err, id)
			return
		}
	}
	if body.Title == nil && body.ViewModeID.Present {
		// th already set
	} else if body.ViewModeID.Present == false {
		// th from rename only
	}
	writeJSON(w, http.StatusOK, th)
}
```

Simplify the end to always `writeJSON` with the last successful `th` (rename then optional view-mode overwrite is fine).

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./internal/catalog/ -count=1'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/optional_string.go \
  controlplane/internal/catalog/view_modes.go \
  controlplane/internal/catalog/store.go \
  controlplane/internal/catalog/http.go \
  controlplane/internal/catalog/threads_http_test.go \
  controlplane/internal/catalog/threads_store_test.go
git commit -m "$(cat <<'EOF'
feat(controlplane): patch thread viewModeId

EOF
)"
```

---

### Task 3: Client view mode registry

**Files:**
- Create: `client/lib/chat/view_modes.dart`
- Create: `client/test/chat/view_modes_test.dart`

**Interfaces:**
- Consumes: `VisibilityMode` from `display_settings.dart`
- Produces: `ViewMode`, `ToolIOMode`, `kBuiltInViewModes`, `resolveViewMode`, `kDefaultViewModeId`

- [ ] **Step 1: Write the failing test**

`client/test/chat/view_modes_test.dart`:

```dart
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/view_modes.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('resolve null uses pretty default', () {
    final m = resolveViewMode(null);
    expect(m.id, 'pretty');
    expect(m.markdownRender, isTrue);
    expect(m.thinkingVisibility, VisibilityMode.collapsed);
    expect(m.toolVisibility, VisibilityMode.collapsed);
    expect(m.toolIO, ToolIOMode.both);
    expect(m.rawRequests, isFalse);
  });

  test('resolve detailed', () {
    final m = resolveViewMode('detailed');
    expect(m.id, 'detailed');
    expect(m.markdownRender, isFalse);
  });

  test('unknown id falls back to default', () {
    final m = resolveViewMode('nope');
    expect(m.id, 'pretty');
  });

  test('registry has pretty and detailed labels', () {
    expect(kBuiltInViewModes.map((m) => m.label).toList(), ['Pretty', 'Detailed']);
  });
}
```

(Adjust import package name if tests use relative imports — match existing `test/chat/` style.)

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/view_modes_test.dart'`

Expected: FAIL (library missing).

- [ ] **Step 3: Implement `view_modes.dart`**

```dart
import 'display_settings.dart';

enum ToolIOMode { input, output, both }

class ViewMode {
  const ViewMode({
    required this.id,
    required this.label,
    required this.markdownRender,
    required this.thinkingVisibility,
    required this.toolVisibility,
    required this.toolIO,
    this.rawRequests = false,
  });

  final String id;
  final String label;
  final bool markdownRender;
  final VisibilityMode thinkingVisibility;
  final VisibilityMode toolVisibility;
  final ToolIOMode toolIO;
  final bool rawRequests;
}

const kDefaultViewModeId = 'pretty';

const kBuiltInViewModes = <ViewMode>[
  ViewMode(
    id: 'pretty',
    label: 'Pretty',
    markdownRender: true,
    thinkingVisibility: VisibilityMode.collapsed,
    toolVisibility: VisibilityMode.collapsed,
    toolIO: ToolIOMode.both,
  ),
  ViewMode(
    id: 'detailed',
    label: 'Detailed',
    markdownRender: false,
    thinkingVisibility: VisibilityMode.collapsed,
    toolVisibility: VisibilityMode.collapsed,
    toolIO: ToolIOMode.both,
  ),
];

ViewMode resolveViewMode(
  String? threadViewModeId, {
  String appDefaultId = kDefaultViewModeId,
}) {
  final id = threadViewModeId ?? appDefaultId;
  for (final mode in kBuiltInViewModes) {
    if (mode.id == id) {
      return mode;
    }
  }
  for (final mode in kBuiltInViewModes) {
    if (mode.id == appDefaultId) {
      return mode;
    }
  }
  return kBuiltInViewModes.first;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/view_modes_test.dart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/view_modes.dart client/test/chat/view_modes_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add view mode registry and resolver

EOF
)"
```

---

### Task 4: Catalog models + `patchThreadViewMode`

**Files:**
- Modify: `client/lib/catalog/models.dart` — `ThreadSummary`
- Modify: `client/lib/catalog/catalog_client.dart`
- Modify: `client/test/catalog/catalog_client_test.dart`
- Modify: `client/test/chat/chat_controller_test.dart` — fake catalog stubs

**Interfaces:**
- Consumes: Task 2 API
- Produces: `viewModeId` on summary; `patchThreadViewMode`

- [ ] **Step 1: Write the failing catalog test**

In `catalog_client_test.dart`, mirror `renameThread` test:

```dart
test('patchThreadViewMode PATCH viewModeId', () async {
  // serve PATCH /v1/threads/th_1 expecting {"viewModeId":"detailed"}
  // respond with thread JSON including viewModeId
  final t = await client.patchThreadViewMode('th_1', 'detailed');
  expect(t.viewModeId, 'detailed');
});

test('patchThreadViewMode can clear with null', () async {
  final t = await client.patchThreadViewMode('th_1', null);
  expect(t.viewModeId, isNull);
});
```

Add `viewModeId` parse expectation to an existing create/list fixture if JSON is asserted.

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/catalog/catalog_client_test.dart --name patchThreadViewMode'`

Expected: FAIL.

- [ ] **Step 3: Implement**

`ThreadSummary`: add `this.viewModeId`, parse `json['viewModeId'] as String?`, and:

```dart
ThreadSummary copyWith({
  String? id,
  String? title,
  String? titleSource,
  String? agentId,
  String? currentModel,
  int? messageCount,
  DateTime? createdAt,
  DateTime? updatedAt,
  Object? viewModeId = _unset,
}) {
  return ThreadSummary(
    id: id ?? this.id,
    title: title ?? this.title,
    titleSource: titleSource ?? this.titleSource,
    agentId: agentId ?? this.agentId,
    currentModel: currentModel ?? this.currentModel,
    messageCount: messageCount ?? this.messageCount,
    createdAt: createdAt ?? this.createdAt,
    updatedAt: updatedAt ?? this.updatedAt,
    viewModeId: identical(viewModeId, _unset)
        ? this.viewModeId
        : viewModeId as String?,
  );
}

const _unset = Object();
```

`CatalogClient`:

```dart
Future<ThreadSummary> patchThreadViewMode(String id, String? viewModeId) async {
  final body = await _send(
    'PATCH',
    '/v1/threads/$id',
    json: {'viewModeId': viewModeId},
  );
  return ThreadSummary.fromJson(jsonDecode(body) as Map<String, dynamic>);
}
```

Update every `CatalogClient` fake/mock in tests with `patchThreadViewMode` throwing `UnimplementedError` or a sensible stub.

- [ ] **Step 4: Run tests**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/catalog/catalog_client_test.dart'`

Expected: PASS. Fix compile breaks in other test fakes.

- [ ] **Step 5: Commit**

```bash
git add client/lib/catalog/models.dart client/lib/catalog/catalog_client.dart \
  client/test/catalog/catalog_client_test.dart client/test/chat/chat_controller_test.dart \
  client/test/
git commit -m "$(cat <<'EOF'
feat(client): thread viewModeId in catalog models and API

EOF
)"
```

---

### Task 5: Markdown rendering + mode-driven AgentBubble

**Files:**
- Modify: `client/pubspec.yaml` (+ lockfile via pub get)
- Create: `client/lib/chat/message_text.dart`
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`
- Create: `client/test/chat/message_text_test.dart`

**Interfaces:**
- Consumes: `ViewMode`, `flutter_markdown_plus`
- Produces: `MessageText` widget; `AgentBubble(viewMode:)`

- [ ] **Step 1: Add dependency**

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter pub add flutter_markdown_plus'
```

- [ ] **Step 2: Write failing widget tests**

`message_text_test.dart`:

```dart
testWidgets('plain mode uses Text', (tester) async {
  await tester.pumpWidget(MaterialApp(home: MessageText(text: '**x**', markdown: false)));
  expect(find.byType(Text), findsWidgets);
  expect(find.byType(MarkdownBody), findsNothing);
});

testWidgets('markdown mode uses MarkdownBody', (tester) async {
  await tester.pumpWidget(MaterialApp(home: MessageText(text: '**x**', markdown: true)));
  expect(find.byType(MarkdownBody), findsOneWidget);
});
```

Update `agent_bubble_test.dart`: replace `thinkingMode:` with `viewMode: resolveViewMode('detailed')` / `'pretty'`; assert tool row respects `toolVisibility: hidden` via a test-only `ViewMode` instance if needed:

```dart
const hiddenTools = ViewMode(
  id: 't',
  label: 't',
  markdownRender: false,
  thinkingVisibility: VisibilityMode.collapsed,
  toolVisibility: VisibilityMode.hidden,
  toolIO: ToolIOMode.both,
);
```

- [ ] **Step 3: Run tests — expect fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/message_text_test.dart test/chat/agent_bubble_test.dart'`

- [ ] **Step 4: Implement**

`message_text.dart`:

```dart
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:material_ui/material_ui.dart';

class MessageText extends StatelessWidget {
  const MessageText({super.key, required this.text, required this.markdown});

  final String text;
  final bool markdown;

  @override
  Widget build(BuildContext context) {
    if (!markdown) {
      return Text(text);
    }
    return MarkdownBody(
      data: text,
      selectable: true,
      styleSheet: MarkdownStyleSheet.fromTheme(Theme.of(context)),
    );
  }
}
```

`AgentBubble`: replace `thinkingMode` with `required ViewMode viewMode`.

- Thought: use `viewMode.thinkingVisibility` (same expand/hide rules as before).
- Tool: if `viewMode.toolVisibility == hidden` → `SizedBox.shrink()`; else pass visibility + `toolIO` into `_ToolCallActivity`.
- Message: `_MessageProse` uses `MessageText(text: bubble.text, markdown: viewMode.markdownRender)`.

`_ToolCallActivity`: take `VisibilityMode toolVisibility` and `ToolIOMode toolIO`. Initial expand: `streamingTool || toolVisibility == expanded`. When `toolIO == input`, show only Input; `output` only Output; `both` keep tabs.

- [ ] **Step 5: Run tests — expect pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/message_text_test.dart test/chat/agent_bubble_test.dart'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add client/pubspec.yaml client/pubspec.lock client/lib/chat/message_text.dart \
  client/lib/chat/agent_bubble.dart client/test/chat/
git commit -m "$(cat <<'EOF'
feat(client): render message text from view mode markdown flag

EOF
)"
```

---

### Task 6: Controller + header dropdown + ChatScreen wiring + retire Thinking setting

**Files:**
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/lib/chat/display_settings.dart` — remove thinking field/prefs
- Modify: `client/lib/settings/display_tab.dart` — remove Thinking dropdown
- Modify: `client/test/chat/chat_controller_test.dart`
- Modify: `client/test/chat/chat_screen_test.dart` (create/extend)
- Modify: `client/test/settings/chat_tab_test.dart` (and display preview tests)

**Interfaces:**
- Consumes: `patchThreadViewMode`, `resolveViewMode`, Task 5 `AgentBubble(viewMode:)`
- Produces: `setThreadViewMode`; dropdown in AppBar; user bubble via `MessageText`; Display settings without thinking

- [ ] **Step 1: Write failing controller test**

```dart
test('setThreadViewMode patches and updates selected thread', () async {
  // fake catalog records patch; create thread with viewModeId null
  await c.selectThread(id);
  await c.setThreadViewMode('detailed');
  expect(c.selectedThread?.viewModeId, 'detailed');
  expect(fake.lastPatchViewModeId, 'detailed');
});

test('setThreadViewMode reverts on error', () async {
  fake.failPatch = true;
  await c.selectThread(id);
  await c.setThreadViewMode('detailed');
  expect(c.selectedThread?.viewModeId, isNull);
});
```

- [ ] **Step 2: Run — expect fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_controller_test.dart --name setThreadViewMode'`

- [ ] **Step 3: Implement controller method**

```dart
Future<void> setThreadViewMode(String modeId) async {
  final id = selectedThreadId;
  final catalog = _catalog;
  if (id == null || catalog == null) {
    return;
  }
  final previous = selectedThread?.viewModeId;
  _replaceThread(id, (t) => t.copyWith(viewModeId: modeId));
  notifyListeners();
  try {
    final updated = await catalog.patchThreadViewMode(id, modeId);
    _replaceThread(id, (_) => updated);
    notifyListeners();
  } catch (e) {
    _replaceThread(id, (t) => t.copyWith(viewModeId: previous));
    statusMessage = formatChatError(e); // or snackbar via screen
    notifyListeners();
    rethrow; // screen may catch for SnackBar
  }
}

void _replaceThread(String id, ThreadSummary Function(ThreadSummary) map) {
  threads = [
    for (final t in threads)
      if (t.id == id) map(t) else t,
  ];
}
```

Ensure `selectThread` / list load preserves `viewModeId` from API.

- [ ] **Step 4: Wire ChatScreen**

Resolve:

```dart
final mode = resolveViewMode(c.selectedThread?.viewModeId);
```

AppBar actions / bottom row: when `c.selectedThreadId != null`, show:

```dart
DropdownButton<String>(
  key: const Key('view-mode-dropdown'),
  value: mode.id,
  items: [
    for (final m in kBuiltInViewModes)
      DropdownMenuItem(value: m.id, child: Text(m.label)),
  ],
  onChanged: c.sending
      ? null
      : (id) async {
          if (id == null) return;
          try {
            await c.setThreadViewMode(id);
          } catch (_) {
            if (context.mounted) {
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('Could not update view mode')),
              );
            }
          }
        },
)
```

Pass `viewMode: mode` into `AgentBubble`. User bubble:

```dart
child: MessageText(text: m.text, markdown: mode.markdownRender),
```

Remove `thinkingMode: widget.displaySettings.thinking`.

- [ ] **Step 5: Retire Thinking from Display settings**

Remove `_thinkingKey`, `thinking`, `setThinking` from `ChatDisplaySettings` (keep `VisibilityMode` enum). Remove Thinking dropdown + preview wiring from `display_tab.dart`. Update settings tests:

```dart
test('defaults: contentWidth 720', () async {
  final s = await ChatDisplaySettings.load();
  expect(s.contentWidth, 720);
});
```

Remove `setThinking` tests; assert no Thinking visibility control remains.

- [ ] **Step 6: Widget test for dropdown + markdown**

In `chat_screen_test.dart` (or new): pump screen with fake controller + one user/assistant message containing `**bold**`; with `viewModeId: null` expect `MarkdownBody`; after selecting Detailed expect plain `Text` and patch called.

- [ ] **Step 7: Run client tests**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test'`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add client/lib/chat/chat_controller.dart client/lib/chat/chat_screen.dart \
  client/lib/chat/display_settings.dart client/lib/settings/display_tab.dart \
  client/test/chat/ client/test/settings/
git commit -m "$(cat <<'EOF'
feat(client): per-thread view mode dropdown and transcript wiring

EOF
)"
```

---

### Task 7: Final verification

**Files:** none new

- [ ] **Step 1: Run control plane tests**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd controlplane && go test ./... -count=1'`

Expected: PASS.

- [ ] **Step 2: Run Flutter analyze + tests**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && dart analyze && flutter test'`

Expected: no issues / PASS.

- [ ] **Step 3: Mark design status (optional same commit)**

In `docs/superpowers/specs/2026-09-18-thread-view-modes-markdown-design.md`, set `Status: implemented` only after the feature is merged/done — skip during plan execution until work is complete.

- [ ] **Step 4: Commit only if status/doc touch**

No empty commit.

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| `threads.view_mode_id` + JSON `viewModeId` | 1 |
| PATCH set / clear / reject invalid; omit vs null | 2 |
| Partial PATCH keeps rename working | 2 |
| Hardcoded Pretty/Detailed registry + resolve | 3 |
| Client models + catalog PATCH | 4 |
| Remove Thinking from Display settings | 6 |
| `flutter_markdown_plus` for user + assistant | 5 |
| Thinking/tool visibility from mode; toolIO tabs | 5 |
| Per-thread dropdown; optimistic PATCH; revert | 6 |
| App default `pretty` when null | 3 + 6 |
| No raw requests / custom modes / settings default UI | (non-goals; not tasked) |
| Link tap / url_launcher | (non-goal; styled only) |
