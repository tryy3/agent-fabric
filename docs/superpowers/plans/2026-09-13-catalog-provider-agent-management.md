# Catalog Provider and Agent Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Settings edit and delete providers and agents, unlink agents when a provider is deleted, and keep incomplete agents visible but unselectable in Chat.

**Architecture:** Catalog `DELETE /v1/providers/{id}` unsets `providerId`/`defaultModel` on referencing agents, writes `agents.json`, then deletes the provider. Settings gains tap-to-edit for providers and a confirm-to-delete icon on both lists. Chat reloads agents when returning from Settings; incomplete items are disabled and sending is blocked.

**Tech Stack:** Go 1.22+ stdlib catalog store/HTTP, existing Flutter Settings + Chat (`package:test` / `flutter_test`). No new dependencies.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-09-13-catalog-provider-agent-management-design.md`
- Module root: `controlplane/` (`github.com/tryy3/agent-fabric`); tests: `go -C controlplane test ./...`
- Flutter tests: `cd client && flutter test`
- JSON `providerId` / `defaultModel` are `null` when unset (`*string` in Go, `String?` in Dart), never `""` as the stored form after unlink
- Create agent still requires a valid provider + cached model
- Unlink happens only inside `DeleteProvider`; the client does not PATCH agents to null
- Persist unlink then delete under the store mutex: `agents.json` first, then `providers.json`
- Drop `ErrProviderInUse` and HTTP `409`
- TDD: failing test → implement → pass → commit per task
- CI offline: fakes / `httptest`; no live LLM

## File Structure

| Path | Responsibility |
| --- | --- |
| `controlplane/internal/catalog/types.go` | `Agent.ProviderID` / `DefaultModel` as `*string`; `IsComplete()` |
| `controlplane/internal/catalog/store.go` | Unlink-on-delete; skip/half-set/validate on update |
| `controlplane/internal/catalog/store_test.go` | Store unlink + validation tests |
| `controlplane/internal/catalog/http.go` | Remove `409` mapping |
| `controlplane/internal/catalog/http_test.go` | HTTP unlink `204`, half-set `400` |
| `controlplane/internal/agent/agent.go` | `pinFromCatalog` rejects incomplete agents |
| `controlplane/internal/agent/agent_test.go` | Compile fix + incomplete `session/new` |
| `client/lib/catalog/models.dart` | `String?` + `isComplete` |
| `client/test/catalog/catalog_client_test.dart` | Parse JSON null |
| `client/lib/settings/providers_tab.dart` | Tap-to-edit, delete + confirm |
| `client/test/settings/providers_tab_test.dart` | Edit/delete widget tests |
| `client/lib/settings/agents_tab.dart` | Delete + “Needs provider” |
| `client/test/settings/agents_tab_test.dart` | Delete / incomplete subtitle |
| `client/lib/chat/chat_controller.dart` | Completeness, `reloadAgents`, block send |
| `client/lib/chat/chat_screen.dart` | Disabled picker items + leftover |
| `client/lib/app_shell.dart` | Reload agents when Chat is selected |
| `client/test/chat/chat_controller_test.dart` | Completeness / reload unit tests |
| `client/test/app_shell_test.dart` | Return-to-Chat reloads agents |
| `client/test/chat/chat_screen_test.dart` | Disabled picker labels |

---

### Task 1: Catalog types, unlink-on-delete, update validation

**Files:**
- Modify: `controlplane/internal/catalog/types.go`
- Modify: `controlplane/internal/catalog/store.go`
- Modify: `controlplane/internal/catalog/store_test.go`
- Modify: `controlplane/internal/agent/agent.go` (compile + incomplete pin)
- Modify: `controlplane/internal/agent/agent_test.go` (dereference `ProviderID`; add incomplete session test)

**Interfaces:**
- Consumes: existing `Store` CRUD
- Produces:
  - `Agent.ProviderID *string`, `Agent.DefaultModel *string` with `json:"providerId"` / `json:"defaultModel"` (no `omitempty`)
  - `func (a Agent) IsComplete() bool`
  - `DeleteProvider` unlinks matching agents (nil pointers, bump `version` + `updatedAt`), saves agents then providers; unknown id still `provider %q not found`
  - `UpdateAgent`: both resulting pointers nil → skip model validation; exactly one nil → `fmt.Errorf("provider and model must be set together")`; both set → `validateProviderAndModelLocked(*providerID, *defaultModel)`
  - `CreateAgent` still takes `string` ids and stores pointers to copies
  - `ErrProviderInUse` removed
  - `pinFromCatalog` errors if `!ag.IsComplete()` before `GetProvider`

- [ ] **Step 1: Write the failing tests**

Replace `TestDeleteProviderConflictWhenReferenced` in `store_test.go` with:

```go
func TestDeleteProviderUnlinksReferencingAgents(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAgent("A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if len(store.ListProviders()) != 0 {
		t.Fatal("expected provider gone")
	}
	got, ok := store.GetAgent(a.ID)
	if !ok {
		t.Fatal("agent missing")
	}
	if got.ProviderID != nil || got.DefaultModel != nil {
		t.Fatalf("expected unset ids, got %+v", got)
	}
	if got.Version != a.Version+1 {
		t.Fatalf("version = %d, want %d", got.Version, a.Version+1)
	}
	if !got.UpdatedAt.After(a.UpdatedAt) && !got.UpdatedAt.Equal(a.UpdatedAt) {
		t.Fatalf("updatedAt not bumped: %v vs %v", got.UpdatedAt, a.UpdatedAt)
	}

	store2, err := catalog.Open(store /* cannot: need dir */)
	_ = store2
	_ = err
}
```

Do **not** copy the `store /* cannot */` stub. Use the temp dir:

```go
func TestDeleteProviderUnlinksReferencingAgents(t *testing.T) {
	dir := t.TempDir()
	store, _ := catalog.Open(dir)
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, err := store.CreateAgent("A", "", p.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if len(store.ListProviders()) != 0 {
		t.Fatal("expected provider gone")
	}
	got, ok := store.GetAgent(a.ID)
	if !ok {
		t.Fatal("agent missing")
	}
	if got.ProviderID != nil || got.DefaultModel != nil {
		t.Fatalf("expected unset ids, got %+v", got)
	}
	if got.Version != a.Version+1 {
		t.Fatalf("version = %d, want %d", got.Version, a.Version+1)
	}

	store2, err := catalog.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got2, ok := store2.GetAgent(a.ID)
	if !ok || got2.ProviderID != nil || got2.DefaultModel != nil {
		t.Fatalf("persisted agent = %+v ok=%v", got2, ok)
	}
	if len(store2.ListProviders()) != 0 {
		t.Fatal("persisted providers not empty")
	}
}

func TestDeleteProviderWithNoAgents(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	q, _ := store.CreateProvider("Q", catalog.TypeOpenAICompatible, "http://y/v1", "k")
	_, _ = store.ReplaceProviderModels(q.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAgent("A", "", q.ID, "m1")

	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatal(err)
	}
	got, ok := store.GetAgent(a.ID)
	if !ok || got.ProviderID == nil || *got.ProviderID != q.ID {
		t.Fatalf("unrelated agent mutated: %+v", got)
	}
}

func TestUpdateAgentNameOnlyOnIncomplete(t *testing.T) {
	dir := t.TempDir()
	store, _ := catalog.Open(dir)
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAgent("A", "", p.ID, "m1")
	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	got, err := store.UpdateAgent(a.ID, &name, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if got.Name != "Renamed" || got.ProviderID != nil || got.DefaultModel != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateAgentRejectsHalfSetPair(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAgent("A", "", p.ID, "m1")
	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatal(err)
	}
	pid := p.ID
	_, err := store.UpdateAgent(a.ID, nil, nil, &pid, nil)
	if err == nil || !strings.Contains(err.Error(), "provider and model must be set together") {
		t.Fatalf("err = %v", err)
	}
}
```

Keep the `strings` import already in `store_test.go`. Remove the `errors` import if nothing else uses it after deleting the conflict test.

Add at the end of `agent_test.go`:

```go
func TestNewSessionRejectsIncompleteAgent(t *testing.T) {
	store := runtime.NewStore()
	cat, ag := seedCatalog(t, []catalog.ModelInfo{{ID: "m1", Name: "Model 1"}}, "m1")
	if ag.ProviderID == nil {
		t.Fatal("expected seeded provider")
	}
	if err := cat.DeleteProvider(*ag.ProviderID); err != nil {
		t.Fatal(err)
	}
	_, csc, _, ctx, cancel := startACPCatalog(t, store, cat, &fakeStreamer{})
	defer cancel()
	if _, err := csc.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber}); err != nil {
		t.Fatal(err)
	}
	_, err := csc.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
		Meta:       map[string]any{"agentId": ag.ID},
	})
	if err == nil {
		t.Fatal("expected error for incomplete agent")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/catalog -run 'TestDeleteProviderUnlinks|TestDeleteProviderWithNoAgents|TestUpdateAgentNameOnlyOnIncomplete|TestUpdateAgentRejectsHalfSetPair' -count=1`

Expected: FAIL — `DeleteProvider` still returns `ErrProviderInUse` / unlink not implemented.

- [ ] **Step 3: Write minimal implementation**

`types.go` — change Agent fields and add:

```go
func (a Agent) IsComplete() bool {
	return a.ProviderID != nil && *a.ProviderID != "" &&
		a.DefaultModel != nil && *a.DefaultModel != ""
}
```

`store.go`:

- Delete `var ErrProviderInUse`.
- `CreateAgent`: `pid, model := providerID, defaultModel` then `ProviderID: &pid, DefaultModel: &model`.
- `DeleteProvider`: if any agent has `ProviderID != nil && *ProviderID == id`, set both pointers nil, `Version++`, `UpdatedAt = now`; then `saveAgentsLocked` then remove provider and `saveProvidersLocked`.
- `UpdateAgent`: assign pointer fields when patch args non-nil; then:

```go
switch {
case a.ProviderID == nil && a.DefaultModel == nil:
	// incomplete: skip validateProviderAndModelLocked
case a.ProviderID == nil || a.DefaultModel == nil:
	return Agent{}, fmt.Errorf("provider and model must be set together")
default:
	if err := s.validateProviderAndModelLocked(*a.ProviderID, *a.DefaultModel); err != nil {
		return Agent{}, err
	}
}
```

- `rejectOrphanedAgentDefaultsLocked`: skip if `a.ProviderID == nil || *a.ProviderID != providerID`; skip if `a.DefaultModel == nil`; otherwise use `*a.DefaultModel` in the set lookup and error text.

Fix existing `store_test.go` comparisons: `a.DefaultModel != "m1"` → `a.DefaultModel == nil || *a.DefaultModel != "m1"`.

`agent.go` `pinFromCatalog` after `GetAgent`:

```go
if !ag.IsComplete() {
	return runtime.SessionPin{}, fmt.Errorf("agent %q has no provider", ag.ID)
}
p, ok := a.catalog.GetProvider(*ag.ProviderID)
// ...
if m.ID == *ag.DefaultModel {
// ...
CurrentModel: *ag.DefaultModel,
```

`agent_test.go` `TestNewSessionKeepsModelsWhenReplaceWouldOrphanDefault`: `cat.GetProvider(*catalogAgent.ProviderID)` (guard nil).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./internal/catalog ./internal/agent -count=1`

Expected: PASS. (`./internal/catalog` HTTP tests still expect `409` until Task 2 — run packages separately here. If you run `./...`, HTTP conflict test fails; that is Task 2.)

If `go -C controlplane test ./internal/catalog` runs HTTP tests in the same package (`catalog_test`), **Task 1 catalog tests and HTTP tests share `package catalog_test`**. `go test ./internal/catalog` will run `TestProvidersHTTPErrors` and fail on `409`.

Handle this by **changing the HTTP conflict assertion in the same commit as Task 1 only if you also do Task 2 in this commit**. Prefer: implement Task 1 store + types + agent, and **immediately continue Task 2 in the same sitting before `go test ./internal/catalog` can be green**, OR split HTTP tests... They cannot be split. **Combine Task 1+2 verification:** after Task 1 implementation, `TestDeleteProviderUnlinks*` may still not be runnable in isolation from HTTP if you use `go test ./internal/catalog` without `-run`. Using `-run` in step 2/4 for store tests is OK; full package wait until Task 2.

Step 4 store-only: `go -C controlplane test ./internal/catalog -run 'TestDeleteProvider|TestUpdateAgent|TestCreateAgent|TestReplaceProvider|TestProviderCRUD|TestCreateProvider|TestUpdateProvider' -count=1`

And: `go -C controlplane test ./internal/agent -count=1`

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/types.go controlplane/internal/catalog/store.go controlplane/internal/catalog/store_test.go controlplane/internal/agent/agent.go controlplane/internal/agent/agent_test.go
git commit -m "feat(catalog): unlink agents when deleting a provider"
```

---

### Task 2: Catalog HTTP — no 409, null fields, half-set 400

**Files:**
- Modify: `controlplane/internal/catalog/http.go`
- Modify: `controlplane/internal/catalog/http_test.go`

**Interfaces:**
- Consumes: Task 1 `DeleteProvider` unlink + `UpdateAgent` half-set error
- Produces: `DELETE /v1/providers/{id}` → `204` even when agents existed; `PATCH /v1/agents/{id}` with a half-set pair → `400` and `{"error":"provider and model must be set together"}`; no `errors.Is(err, ErrProviderInUse)` branch

- [ ] **Step 1: Write the failing tests**

In `TestProvidersHTTPErrors`, the block that DELETEs an in-use provider currently expects `StatusConflict`. **Move the failing-refresh assertion before that delete** (delete will remove the provider). Then change delete to expect `204`/`200` and assert the agent is unlinked:

```go
	refresh, err := http.Post(srv.URL+"/v1/providers/"+p.ID+"/models/refresh", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer refresh.Body.Close()
	if refresh.StatusCode != http.StatusBadGateway {
		t.Fatalf("refresh fail status %d", refresh.StatusCode)
	}
	_ = decodeError(t, refresh)

	del, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/providers/"+p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	delResp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent && delResp.StatusCode != http.StatusOK {
		t.Fatalf("delete status %d", delResp.StatusCode)
	}

	listed, err := http.Get(srv.URL + "/v1/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer listed.Body.Close()
	var agents []catalog.Agent
	if err := json.NewDecoder(listed.Body).Decode(&agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ProviderID != nil || agents[0].DefaultModel != nil {
		t.Fatalf("unlinked agents = %+v", agents)
	}
```

Remove the old `conflict` / `StatusConflict` block entirely.

Add:

```go
func TestAgentsHTTPPatchHalfSetOnIncomplete(t *testing.T) {
	store, _ := catalog.Open(t.TempDir())
	p, _ := store.CreateProvider("P", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	_, _ = store.ReplaceProviderModels(p.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())
	a, _ := store.CreateAgent("Helper", "", p.ID, "m1")
	if err := store.DeleteProvider(p.ID); err != nil {
		t.Fatal(err)
	}

	p2, _ := store.CreateProvider("P2", catalog.TypeOpenAICompatible, "http://127.0.0.1:9/v1", "sk")
	_, _ = store.ReplaceProviderModels(p2.ID, []catalog.ModelInfo{{ID: "m1", Name: "M1"}}, time.Now().UTC())

	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()

	body := fmt.Sprintf(`{"providerId":%q}`, p2.ID)
	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/v1/agents/"+a.ID, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
	msg := decodeError(t, resp)
	if !strings.Contains(msg, "provider and model must be set together") {
		t.Fatalf("error = %q", msg)
	}
}
```

Add `"strings"` to `http_test.go` imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go -C controlplane test ./internal/catalog -run 'TestProvidersHTTPErrors|TestAgentsHTTPPatchHalfSetOnIncomplete' -count=1`

Expected: FAIL if HTTP still maps leftover `409` (only if Task 1 left `ErrProviderInUse` mapping and delete still 409). After Task 1 unlink, `TestProvidersHTTPErrors` already fails on `StatusConflict` vs actual `204`. That is the failing test for this task if Task 1 landed first.

- [ ] **Step 3: Write minimal implementation**

In `http.go` `writeMappedError`, delete the `ErrProviderInUse` / `StatusConflict` branch. Drop the `errors` import if unused.

No handler signature changes; `deleteProvider` already calls `store.DeleteProvider`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go -C controlplane test ./... -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add controlplane/internal/catalog/http.go controlplane/internal/catalog/http_test.go
git commit -m "fix(catalog): delete in-use providers with 204 and unlink"
```

---

### Task 3: Dart Agent model — nullable ids + isComplete

**Files:**
- Modify: `client/lib/catalog/models.dart`
- Modify: `client/test/catalog/catalog_client_test.dart`
- Modify: `client/lib/settings/agents_tab.dart` (compile: `agent.defaultModel ?? ''` in the list subtitle until Task 5)

**Interfaces:**
- Consumes: JSON `null` for `providerId` / `defaultModel`
- Produces:
  - `final String? providerId;`
  - `final String? defaultModel;`
  - `bool get isComplete => (providerId != null && providerId!.isNotEmpty) && (defaultModel != null && defaultModel!.isNotEmpty);`
  - `fromJson` uses `json['providerId'] as String?` and `json['defaultModel'] as String?`

- [ ] **Step 1: Write the failing test**

Add to `catalog_client_test.dart`:

```dart
  test('listAgents parses null providerId and defaultModel', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        return http.Response(
          jsonEncode([
            {
              'id': 'ag-1',
              'name': 'Work',
              'version': 2,
              'providerId': null,
              'defaultModel': null,
              'createdAt': '2026-09-12T09:00:00Z',
              'updatedAt': '2026-09-12T10:00:00Z',
            },
          ]),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final agents = await client.listAgents();
    expect(agents.single.providerId, isNull);
    expect(agents.single.defaultModel, isNull);
    expect(agents.single.isComplete, isFalse);
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && flutter test test/catalog/catalog_client_test.dart`

Expected: FAIL — `as String` throws / `isComplete` missing.

- [ ] **Step 3: Write minimal implementation**

In `models.dart` `Agent`:

```dart
  final String? providerId;
  final String? defaultModel;

  bool get isComplete =>
      providerId != null &&
      providerId!.isNotEmpty &&
      defaultModel != null &&
      defaultModel!.isNotEmpty;

  factory Agent.fromJson(Map<String, dynamic> json) {
    return Agent(
      id: json['id'] as String,
      name: json['name'] as String,
      description: json['description'] as String? ?? '',
      version: json['version'] as int? ?? 0,
      providerId: json['providerId'] as String?,
      defaultModel: json['defaultModel'] as String?,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }
```

In `agents_tab.dart` list subtitle, use `agent.defaultModel ?? ''` so analyze/tests compile.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/catalog/models.dart client/test/catalog/catalog_client_test.dart client/lib/settings/agents_tab.dart
git commit -m "feat(client): parse optional agent provider and model"
```

---

### Task 4: Providers tab — edit and delete

**Files:**
- Modify: `client/lib/settings/providers_tab.dart`
- Modify: `client/test/settings/providers_tab_test.dart`

**Interfaces:**
- Consumes: `CatalogClient.updateProvider`, `deleteProvider`, `listAgents`
- Produces: tap row → editor dialog (prefilled); trailing delete `Key('delete-provider-\$id')`; confirm `Delete provider?`; if `listAgents()` has `providerId == id`, body lists those names and says deleting will unset their provider and model; confirm `Delete` calls `deleteProvider`

- [ ] **Step 1: Write the failing tests**

Extend `FakeCatalogClient` in `providers_tab_test.dart`:

```dart
class FakeCatalogClient extends CatalogClient {
  FakeCatalogClient({
    List<Provider>? providers,
    List<Agent>? agents,
    this.refreshError,
  })  : providers = List.of(providers ?? const []),
        agents = List.of(agents ?? const []),
        super(
          baseUri: Uri.parse('http://catalog.test'),
          httpClient: MockClient(
            (_) async => http.Response('unused', 500),
          ),
        );

  final List<Provider> providers;
  final List<Agent> agents;
  Map<String, String>? lastCreate;
  Map<String, String?>? lastUpdate;
  String? lastDeleteId;
  String? lastRefreshId;
  final Object? refreshError;

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);

  @override
  Future<Provider> updateProvider(
    String id, {
    String? name,
    String? baseUrl,
    String? apiKey,
  }) async {
    lastUpdate = {'id': id, 'name': name, 'baseUrl': baseUrl, 'apiKey': apiKey};
    final index = providers.indexWhere((p) => p.id == id);
    final current = providers[index];
    final updated = _provider(
      id: id,
      name: name ?? current.name,
    );
    providers[index] = updated;
    return updated;
  }

  @override
  Future<void> deleteProvider(String id) async {
    lastDeleteId = id;
    providers.removeWhere((p) => p.id == id);
  }

  @override
  Future<List<Provider>> listProviders() async => List.of(providers);

  @override
  Future<Provider> createProvider({
    required String name,
    required String type,
    required String baseUrl,
    required String apiKey,
  }) async {
    lastCreate = {
      'name': name,
      'type': type,
      'baseUrl': baseUrl,
      'apiKey': apiKey,
    };
    final created = _provider(id: 'prov-new', name: name);
    providers.add(created);
    return created;
  }

  @override
  Future<Provider> refreshModels(String id) async {
    lastRefreshId = id;
    if (refreshError != null) {
      throw refreshError!;
    }
    final index = providers.indexWhere((p) => p.id == id);
    final updated = _provider(
      id: id,
      name: providers[index].name,
      models: const [ModelInfo(id: 'm2', name: 'Model 2')],
      modelsUpdatedAt: DateTime.utc(2026, 9, 12, 12),
    );
    providers[index] = updated;
    return updated;
  }
}
```

Add this helper next to `_provider` in the same file:

```dart
Agent _agent({
  required String id,
  required String name,
  String? providerId,
  String? defaultModel,
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    version: 1,
    providerId: providerId,
    defaultModel: defaultModel,
    createdAt: now,
    updatedAt: now,
  );
}
```

Tests:

```dart
  testWidgets('tap provider row opens editor and save patches', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [_provider(id: 'prov-1', name: 'Local')],
    );
    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Local'));
    await tester.pumpAndSettle();
    expect(find.text('Edit provider'), findsOneWidget);

    await tester.enterText(find.widgetWithText(TextField, 'Name'), 'Renamed');
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(catalog.lastUpdate?['id'], 'prov-1');
    expect(catalog.lastUpdate?['name'], 'Renamed');
    expect(find.text('Renamed'), findsOneWidget);
  });

  testWidgets('delete provider confirms then deletes', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [_provider(id: 'prov-1', name: 'Local')],
    );
    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-provider-prov-1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete provider?'), findsOneWidget);
    expect(find.textContaining('Local'), findsWidgets);

    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'prov-1');
    expect(find.text('Local'), findsNothing);
  });

  testWidgets('delete in-use provider lists agent names', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [_provider(id: 'prov-1', name: 'Local')],
      agents: [
        Agent(
          id: 'ag-1',
          name: 'Work',
          version: 1,
          providerId: 'prov-1',
          defaultModel: 'm1',
          createdAt: DateTime.utc(2026, 9, 12, 9),
          updatedAt: DateTime.utc(2026, 9, 12, 9),
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-provider-prov-1')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Work'), findsOneWidget);
    expect(find.textContaining('unset'), findsOneWidget);

    await tester.tap(find.widgetWithText(TextButton, 'Cancel'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, isNull);
    expect(find.text('Local'), findsOneWidget);
  });
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/settings/providers_tab_test.dart`

Expected: FAIL — no edit dialog / delete key.

- [ ] **Step 3: Write minimal implementation**

In `providers_tab.dart`:

- `ListTile.onTap` → `_openEditor(provider: provider)` (rename `_createProvider` / `_CreateProviderDialog` to accept optional `Provider? provider`; title `Add provider` vs `Edit provider`; button `Create` vs `Save`; prefill controllers; save calls `updateProvider` with name, baseUrl, apiKey).
- Trailing: `Row(mainAxisSize: min)` with existing Refresh `TextButton` and `IconButton(key: Key('delete-provider-${provider.id}'), tooltip: 'Delete provider', icon: Icons.delete, onPressed: () => _confirmDelete(provider))`.
- `_confirmDelete`: `final agents = await widget.catalog.listAgents();` filter `a.providerId == provider.id`; `showDialog` as above; on Delete, `await widget.catalog.deleteProvider(provider.id)` then `_reload()`. Errors set `_error` like refresh.

Keep FAB add as `_openEditor()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test test/settings/providers_tab_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/settings/providers_tab.dart client/test/settings/providers_tab_test.dart
git commit -m "feat(settings): edit and delete providers"
```

---

### Task 5: Agents tab — delete and Needs provider

**Files:**
- Modify: `client/lib/settings/agents_tab.dart`
- Modify: `client/test/settings/agents_tab_test.dart`

**Interfaces:**
- Consumes: `CatalogClient.deleteAgent`, `Agent.isComplete`
- Produces: list subtitle `Needs provider` when `!agent.isComplete`; trailing `Key('delete-agent-\$id')`; confirm `Delete agent?` with the agent name; confirm calls `deleteAgent`

- [ ] **Step 1: Write the failing tests**

On `FakeCatalogClient` in `agents_tab_test.dart` add `String? lastDeleteId` and:

```dart
  @override
  Future<void> deleteAgent(String id) async {
    lastDeleteId = id;
    agents.removeWhere((a) => a.id == id);
  }
```

Allow `_agent` / `Agent(...)` to omit provider (already `String?` from Task 3). Add:

```dart
  testWidgets('incomplete agent shows Needs provider', (tester) async {
    final catalog = FakeCatalogClient(
      agents: [
        _agent(
          id: 'ag-1',
          name: 'Work',
          providerId: null,
          defaultModel: null,
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AgentsTab(catalog: catalog)));
    await tester.pumpAndSettle();
    expect(find.text('Work'), findsOneWidget);
    expect(find.text('Needs provider'), findsOneWidget);
  });

  testWidgets('delete agent confirms then deletes', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [
        _provider(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
      agents: [
        _agent(id: 'ag-1', name: 'Work', providerId: 'prov-1', defaultModel: 'm1'),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AgentsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-agent-ag-1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete agent?'), findsOneWidget);

    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'ag-1');
    expect(find.text('Work'), findsNothing);
  });
```

Change `_agent` so `providerId` and `defaultModel` are `String?` with defaults `'prov-1'` / `'m1'` for existing tests, and allow passing `null`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/settings/agents_tab_test.dart`

Expected: FAIL — missing subtitle / delete key.

- [ ] **Step 3: Write minimal implementation**

ListTile:

```dart
subtitle: Text(
  !agent.isComplete
      ? 'Needs provider'
      : (agent.description.isEmpty
            ? (agent.defaultModel ?? '')
            : agent.description),
),
trailing: IconButton(
  key: Key('delete-agent-${agent.id}'),
  tooltip: 'Delete agent',
  icon: const Icon(Icons.delete),
  onPressed: () => _confirmDelete(agent),
),
```

`_confirmDelete`: dialog title `Delete agent?`, content `Delete ${agent.name}?`, Cancel / Delete; Delete → `deleteAgent` then `_reload()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test test/settings/agents_tab_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/settings/agents_tab.dart client/test/settings/agents_tab_test.dart
git commit -m "feat(settings): delete agents and show incomplete state"
```

---

### Task 6: Chat — disabled incomplete agents, block send, reload on return

**Files:**
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/lib/app_shell.dart`
- Modify: `client/test/chat/chat_controller_test.dart`
- Modify: `client/test/app_shell_test.dart`
- Create: `client/test/chat/chat_screen_test.dart`

**Interfaces:**
- Consumes: `Agent.isComplete`, `CatalogClient.listAgents`
- Produces:
  - `bool get selectedAgentIsComplete`
  - `bool get selectedAgentMissing` — `selectedAgentId != null` and no agent with that id
  - `canSend` also requires `selectedAgentIsComplete`
  - `Future<void> reloadAgents()` — `listAgents()`, if selected is incomplete or missing set `_sessionReady = false` (do not call `startSession`)
  - `selectAgent` returns immediately without `startSession` if the id is missing or incomplete
  - `AppShell.onDestinationSelected`: when `index == 0`, `widget.controller.reloadAgents()`
  - Picker: incomplete items `enabled: false`, label `'{name} — needs provider'`; if `selectedAgentId` not in list, extra disabled item `'(deleted)'`; `value: c.selectedAgentId`
  - Status line when selected incomplete: `This agent needs a provider`; when missing: `This agent was deleted` (not prefixed with `Error:`)

- [ ] **Step 1: Write the failing tests**

In `chat_controller_test.dart`, add a helper incomplete agent and:

```dart
Agent _incomplete(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    version: 2,
    providerId: null,
    defaultModel: null,
    createdAt: now,
    updatedAt: now,
  );
}

  test('incomplete selected agent cannot send and reloadAgents does not startSession', () async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    await c.connect();
    await c.selectAgent('ag-1');
    expect(c.canSend, isTrue);

    catalog.agents
      ..clear()
      ..add(_incomplete('ag-1', 'Alpha'));
    await c.reloadAgents();
    expect(c.selectedAgentId, 'ag-1');
    expect(c.selectedAgentIsComplete, isFalse);
    expect(c.canSend, isFalse);
    expect(fake.startSessionIds, ['ag-1']);

    await c.selectAgent('ag-1');
    expect(fake.startSessionIds, ['ag-1']);
  });

  test('reloadAgents with deleted selection keeps id and blocks send', () async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final c = ChatController(session: fake, catalog: catalog);
    await c.connect();
    await c.selectAgent('ag-1');
    catalog.agents.clear();
    await c.reloadAgents();
    expect(c.selectedAgentId, 'ag-1');
    expect(c.selectedAgentMissing, isTrue);
    expect(c.canSend, isFalse);
  });
```

`FakeCatalog.agents` is already `final List<Agent> agents` — mutating via `clear`/`add` works.

In `app_shell_test.dart`, add these helpers (file already has `_FakeConn`):

```dart
class FakeCatalog extends CatalogClient {
  FakeCatalog(this.agents)
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient(
          (_) async => http.Response(
            '[]',
            200,
            headers: {'content-type': 'application/json'},
          ),
        ),
      );

  final List<Agent> agents;

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);
}

Agent _agent(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    version: 1,
    providerId: 'prov-1',
    defaultModel: 'm1',
    createdAt: now,
    updatedAt: now,
  );
}
```

Then:

```dart
  testWidgets('returning to Chat reloads agents without reconnect', (tester) async {
    final session = _FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')]);
    final controller = ChatController(session: session, catalog: catalog);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(home: AppShell(controller: controller, catalog: catalog)),
    );
    await tester.pump();
    await tester.pump();
    expect(session.connects, 1);
    expect(controller.agents, hasLength(1));

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();
    catalog.agents.add(_agent('ag-2', 'Beta'));

    await tester.tap(find.text('Chat'));
    await tester.pumpAndSettle();
    expect(session.connects, 1);
    expect(controller.agents.map((a) => a.id), ['ag-1', 'ag-2']);
  });
```

Create `client/test/chat/chat_screen_test.dart`:

```dart
  testWidgets('incomplete agent is labeled and not selectable', (tester) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([
        _agent('ag-1', 'Alpha'),
        _incomplete('ag-2', 'Work'),
      ]),
    );
    addTearDown(c.dispose);
    await c.connect();

    await tester.pumpWidget(MaterialApp(home: ChatScreen(controller: c)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('agent-picker')));
    await tester.pumpAndSettle();
    expect(find.text('Work — needs provider'), findsOneWidget);

    await tester.tap(find.text('Work — needs provider'));
    await tester.pumpAndSettle();
    expect(fake.startSessionIds, isEmpty);
    expect(c.selectedAgentId, isNull);
  });
```

Put `FakeConn`, `FakeCatalog`, `_agent`, and `_incomplete` at the top of `chat_screen_test.dart` using the same definitions as in `chat_controller_test.dart` (this task). Import `chat_screen.dart`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/chat/chat_controller_test.dart test/app_shell_test.dart test/chat/chat_screen_test.dart`

Expected: FAIL — `reloadAgents` missing, picker still enables all items.

- [ ] **Step 3: Write minimal implementation**

`chat_controller.dart`:

```dart
  bool get selectedAgentMissing {
    final id = selectedAgentId;
    if (id == null) {
      return false;
    }
    return !agents.any((a) => a.id == id);
  }

  bool get selectedAgentIsComplete {
    final id = selectedAgentId;
    if (id == null) {
      return false;
    }
    for (final a in agents) {
      if (a.id == id) {
        return a.isComplete;
      }
    }
    return false;
  }

  bool get canSend =>
      status == ChatStatus.connected &&
      !_sending &&
      _sessionReady &&
      selectedAgentIsComplete;

  Future<void> reloadAgents() async {
    if (_catalog == null) {
      return;
    }
    agents = await _catalog.listAgents();
    if (!selectedAgentIsComplete) {
      _sessionReady = false;
    }
    notifyListeners();
  }

  Future<void> selectAgent(String agentId) async {
    final match = agents.where((a) => a.id == agentId);
    if (match.isEmpty || !match.first.isComplete) {
      return;
    }
    final previousReady = _sessionReady;
    _sessionStarting = true;
    _sessionReady = false;
    notifyListeners();
    try {
      await _session.startSession(agentId);
      messages.clear();
      selectedAgentId = agentId;
      _sessionReady = true;
      status = ChatStatus.connected;
      statusMessage = null;
    } catch (e) {
      _sessionReady = previousReady;
      statusMessage = formatChatError(e);
    } finally {
      _sessionStarting = false;
      notifyListeners();
    }
  }
```

`app_shell.dart`:

```dart
onDestinationSelected: (index) {
  setState(() => _selectedIndex = index);
  if (index == 0) {
    widget.controller.reloadAgents();
  }
},
```

`chat_screen.dart` `_agentPicker`:

```dart
  Widget _agentPicker(ChatController c) {
    final items = <DropdownMenuItem<String>>[
      for (final agent in c.agents)
        DropdownMenuItem(
          value: agent.id,
          enabled: agent.isComplete,
          child: Text(
            agent.isComplete ? agent.name : '${agent.name} — needs provider',
          ),
        ),
    ];
    if (c.selectedAgentMissing && c.selectedAgentId != null) {
      items.add(
        DropdownMenuItem(
          value: c.selectedAgentId,
          enabled: false,
          child: const Text('(deleted)'),
        ),
      );
    }
    return DropdownButton<String>(
      key: const Key('agent-picker'),
      isExpanded: true,
      hint: const Text('Agent'),
      value: c.selectedAgentId,
      items: items,
      onChanged: c.canSelectAgent
          ? (id) {
              if (id != null) {
                c.selectAgent(id);
              }
            }
          : null,
    );
  }
```

`_statusLabel`: before the switch, if `c.selectedAgentMissing` return `This agent was deleted`; if `c.selectedAgentId != null && !c.selectedAgentIsComplete` return `This agent needs a provider`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd client && flutter test`

Expected: PASS

Also: `go -C controlplane test ./... -count=1` still PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_controller.dart client/lib/chat/chat_screen.dart client/lib/app_shell.dart client/test/chat/chat_controller_test.dart client/test/app_shell_test.dart client/test/chat/chat_screen_test.dart
git commit -m "feat(chat): block incomplete agents and reload catalog on show"
```

---

## Self-review (plan vs spec)

| Spec requirement | Task |
| --- | --- |
| Optional `providerId` / `defaultModel` JSON null | 1, 3 |
| Create still requires provider+model | 1 (existing create tests) |
| DeleteProvider unlinks, agents.json then providers.json, bump version | 1 |
| Drop 409 / ErrProviderInUse | 1, 2 |
| Update half-set 400; name-only on incomplete OK | 1, 2 |
| session/new incomplete → error | 1 |
| Providers tap-to-edit + delete confirm + in-use names | 4 |
| Agents delete confirm + Needs provider | 5 |
| Chat disabled labeled items | 6 |
| Selected incomplete: keep selection, block send | 6 |
| Selected missing leftover `(deleted)` | 6 |
| Reload agents when Chat visible | 6 |
| No client PATCH-to-null | 4 (delete provider only) |
