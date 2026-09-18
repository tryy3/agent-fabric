# Advanced Model Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the composer’s flat model `DropdownButton` with an anchored popover that searches session models and groups them by catalog provider.

**Architecture:** Pure helpers group/filter `ModelOption`s using catalog `Provider` models. `ChatController` caches providers loaded once on connect. A new `ModelPicker` widget (`MenuAnchor` + search + collapsible groups) replaces `_modelPicker` in `ChatComposer`; selection still calls `selectModel`.

**Tech Stack:** Flutter (Nix shell). Work from `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix with `nix develop /home/tryy3/src/agent-fabric -c` and run from `client/`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/model_picker_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-18-advanced-model-picker-design.md`](../specs/2026-09-18-advanced-model-picker-design.md).
- Selectable set = session `modelOptions` only; group via catalog id match.
- Unmatched → group name exactly `Other`.
- No Custom Model ID, Configured section, or favorites in V1.
- Collapsible groups; default expanded; collapse state only for the open popover.
- Keep key `model-picker` on the trigger.
- Do not change `selectModel` / ACP set-model semantics.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/model_picker_grouping.dart` | Pure group + filter helpers |
| `client/lib/chat/model_picker.dart` | Chip trigger + `MenuAnchor` popover UI |
| `client/lib/chat/chat_controller.dart` | Cache `providers`; load on connect |
| `client/lib/chat/chat_composer.dart` | Swap `_modelPicker` for `ModelPicker` |
| `client/test/chat/model_picker_grouping_test.dart` | Unit tests for helpers |
| `client/test/chat/model_picker_test.dart` | Widget tests for popover |
| `client/test/widget_test.dart` | Stop casting model-picker as `DropdownButton` |
| `client/test/chat/chat_controller_test.dart` | Provider cache on connect (if needed) |

**Interfaces this plan locks:**

```dart
// model_picker_grouping.dart
class ModelProviderGroup {
  const ModelProviderGroup({
    this.providerId,
    required this.providerName,
    required this.models,
  });
  final String? providerId; // null for Other
  final String providerName;
  final List<ModelOption> models;
}

const kOtherProviderGroupName = 'Other';

List<ModelProviderGroup> groupModelsByProvider({
  required List<ModelOption> models,
  required List<Provider> providers,
});

List<ModelProviderGroup> filterModelGroups({
  required List<ModelProviderGroup> groups,
  required String query,
});

// chat_controller.dart
List<Provider> providers = []; // cached; empty if no catalog / load failed
// connect() also awaits listProviders() when _catalog != null (best-effort)

// model_picker.dart
class ModelPicker extends StatelessWidget {
  const ModelPicker({super.key, required this.controller});
  final ChatController controller;
}
```

---

### Task 1: Grouping and filter helpers

**Files:**
- Create: `client/lib/chat/model_picker_grouping.dart`
- Test: `client/test/chat/model_picker_grouping_test.dart`

**Interfaces:**
- Consumes: `ModelOption` (`client/lib/acp/agent_connection.dart`), `Provider` / `ModelInfo` (`client/lib/catalog/models.dart`)
- Produces: `ModelProviderGroup`, `groupModelsByProvider`, `filterModelGroups`, `kOtherProviderGroupName`

- [ ] **Step 1: Write the failing unit tests**

Create `client/test/chat/model_picker_grouping_test.dart`:

```dart
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/model_picker_grouping.dart';
import 'package:flutter_test/flutter_test.dart';

Provider _provider({
  required String id,
  required String name,
  required List<ModelInfo> models,
}) {
  final now = DateTime.utc(2026, 9, 18);
  return Provider(
    id: id,
    name: name,
    type: 'openai_compatible',
    baseUrl: 'http://example',
    apiKey: 'k',
    models: models,
    createdAt: now,
    updatedAt: now,
  );
}

void main() {
  test('groups matched models under provider name with count order preserved', () {
    final models = const [
      ModelOption(id: 'm1', name: 'One'),
      ModelOption(id: 'm2', name: 'Two'),
      ModelOption(id: 'orphan', name: 'Orphan'),
    ];
    final providers = [
      _provider(
        id: 'p1',
        name: 'Local',
        models: const [
          ModelInfo(id: 'm1', name: 'One'),
          ModelInfo(id: 'm2', name: 'Two'),
        ],
      ),
    ];

    final groups = groupModelsByProvider(models: models, providers: providers);

    expect(groups, hasLength(2));
    expect(groups[0].providerId, 'p1');
    expect(groups[0].providerName, 'Local');
    expect(groups[0].models.map((m) => m.id), ['m1', 'm2']);
    expect(groups[1].providerId, isNull);
    expect(groups[1].providerName, kOtherProviderGroupName);
    expect(groups[1].models.single.id, 'orphan');
  });

  test('empty providers puts everything in Other', () {
    final groups = groupModelsByProvider(
      models: const [ModelOption(id: 'm1', name: 'One')],
      providers: const [],
    );
    expect(groups, hasLength(1));
    expect(groups.single.providerName, kOtherProviderGroupName);
  });

  test('first provider wins when id appears twice', () {
    final models = const [ModelOption(id: 'm1', name: 'One')];
    final providers = [
      _provider(
        id: 'p1',
        name: 'First',
        models: const [ModelInfo(id: 'm1', name: 'One')],
      ),
      _provider(
        id: 'p2',
        name: 'Second',
        models: const [ModelInfo(id: 'm1', name: 'One')],
      ),
    ];
    final groups = groupModelsByProvider(models: models, providers: providers);
    expect(groups.single.providerName, 'First');
  });

  test('filter hides empty groups and matches name or id case-insensitively', () {
    final groups = [
      const ModelProviderGroup(
        providerId: 'p1',
        providerName: 'Local',
        models: [
          ModelOption(id: 'alpha-id', name: 'Alpha'),
          ModelOption(id: 'beta-id', name: 'Beta'),
        ],
      ),
      const ModelProviderGroup(
        providerName: kOtherProviderGroupName,
        models: [ModelOption(id: 'zz', name: 'Zed')],
      ),
    ];

    final byName = filterModelGroups(groups: groups, query: 'alp');
    expect(byName, hasLength(1));
    expect(byName.single.models.single.name, 'Alpha');

    final byId = filterModelGroups(groups: groups, query: 'BETA-ID');
    expect(byId.single.models.single.id, 'beta-id');

    final none = filterModelGroups(groups: groups, query: 'nope');
    expect(none, isEmpty);
  });

  test('blank query returns groups unchanged', () {
    final groups = [
      const ModelProviderGroup(
        providerName: 'Local',
        models: [ModelOption(id: 'm1', name: 'One')],
      ),
    ];
    expect(filterModelGroups(groups: groups, query: '  '), groups);
  });
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/model_picker_grouping_test.dart'`

Expected: FAIL — library / symbols not found.

- [ ] **Step 3: Implement helpers**

Create `client/lib/chat/model_picker_grouping.dart`:

```dart
import '../acp/agent_connection.dart';
import '../catalog/models.dart';

const kOtherProviderGroupName = 'Other';

class ModelProviderGroup {
  const ModelProviderGroup({
    this.providerId,
    required this.providerName,
    required this.models,
  });

  final String? providerId;
  final String providerName;
  final List<ModelOption> models;
}

List<ModelProviderGroup> groupModelsByProvider({
  required List<ModelOption> models,
  required List<Provider> providers,
}) {
  final idToProvider = <String, Provider>{};
  for (final p in providers) {
    for (final m in p.models) {
      idToProvider.putIfAbsent(m.id, () => p);
    }
  }

  final byProvider = <String, ModelProviderGroup>{};
  final other = <ModelOption>[];

  for (final model in models) {
    final p = idToProvider[model.id];
    if (p == null) {
      other.add(model);
      continue;
    }
    final existing = byProvider[p.id];
    if (existing == null) {
      byProvider[p.id] = ModelProviderGroup(
        providerId: p.id,
        providerName: p.name,
        models: [model],
      );
    } else {
      byProvider[p.id] = ModelProviderGroup(
        providerId: existing.providerId,
        providerName: existing.providerName,
        models: [...existing.models, model],
      );
    }
  }

  final groups = byProvider.values.toList();
  // Preserve provider list order for matched groups.
  groups.sort((a, b) {
    final ai = providers.indexWhere((p) => p.id == a.providerId);
    final bi = providers.indexWhere((p) => p.id == b.providerId);
    return ai.compareTo(bi);
  });
  if (other.isNotEmpty) {
    groups.add(
      ModelProviderGroup(
        providerName: kOtherProviderGroupName,
        models: other,
      ),
    );
  }
  return groups;
}

List<ModelProviderGroup> filterModelGroups({
  required List<ModelProviderGroup> groups,
  required String query,
}) {
  final q = query.trim().toLowerCase();
  if (q.isEmpty) return groups;
  final out = <ModelProviderGroup>[];
  for (final g in groups) {
    final models = [
      for (final m in g.models)
        if (m.name.toLowerCase().contains(q) || m.id.toLowerCase().contains(q))
          m,
    ];
    if (models.isNotEmpty) {
      out.add(
        ModelProviderGroup(
          providerId: g.providerId,
          providerName: g.providerName,
          models: models,
        ),
      );
    }
  }
  return out;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/model_picker_grouping_test.dart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/model_picker_grouping.dart \
  client/test/chat/model_picker_grouping_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add model picker grouping and filter helpers

EOF
)"
```

---

### Task 2: Cache providers on `ChatController`

**Files:**
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/test/chat/chat_controller_test.dart` (or add focused test)
- Ensure FakeCatalog stubs used by screen/composer tests override `listProviders` if needed

**Interfaces:**
- Consumes: `CatalogClient.listProviders`
- Produces: `List<Provider> providers` on controller; populated during `connect()`

- [ ] **Step 1: Write failing test for provider cache**

In `client/test/chat/chat_controller_test.dart`, add a FakeCatalog that returns providers (extend existing fake if present) and:

```dart
test('connect loads and caches providers', () async {
  final catalog = FakeCatalog(agents: [_agent('ag-1', 'Alpha')])
    ..providers = [
      Provider(
        id: 'p1',
        name: 'Local',
        type: 'openai_compatible',
        baseUrl: 'http://x',
        apiKey: 'k',
        models: const [ModelInfo(id: 'm1', name: 'M1')],
        createdAt: DateTime.utc(2026, 9, 18),
        updatedAt: DateTime.utc(2026, 9, 18),
      ),
    ];
  final c = ChatController(session: FakeConn(), catalog: catalog);
  addTearDown(c.dispose);
  expect(c.providers, isEmpty);
  await c.connect();
  expect(c.providers, hasLength(1));
  expect(c.providers.single.name, 'Local');
});
```

If the existing `FakeCatalog` in that file does not implement `listProviders`, add:

```dart
List<Provider> providers = [];
@override
Future<List<Provider>> listProviders() async => List.of(providers);
```

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_controller_test.dart --name "connect loads and caches providers"'`

Expected: FAIL — no `providers` getter / empty after connect.

- [ ] **Step 3: Implement cache**

In `ChatController`:

```dart
List<Provider> providers = [];
```

In `connect()`, inside the `_catalog != null` block alongside `listAgents` / `listThreads`:

```dart
try {
  providers = await _catalog.listProviders();
} catch (_) {
  providers = [];
}
```

Also reload providers in any existing `reloadAgents`-style refresh path if one already reloads catalog lists — only if such a method already loads agents from catalog; do not invent a new refresh API. If only `connect` loads agents, loading providers only in `connect` is enough for V1.

Call `notifyListeners()` after assignment as part of the existing connect success path (already present).

- [ ] **Step 4: Run tests**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_controller_test.dart --name "connect loads and caches providers"'`

Also run a quick subset if other fakes need `listProviders` default:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_controller_test.dart test/chat/chat_screen_test.dart test/chat/chat_composer_test.dart'
```

Expected: PASS. If a FakeCatalog crashes on missing `listProviders`, add `Future<List<Provider>> listProviders() async => const [];` to each test fake that subclasses `CatalogClient`.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_controller.dart client/test/chat/chat_controller_test.dart \
  client/test/chat/chat_screen_test.dart client/test/chat/chat_composer_test.dart \
  client/test/widget_test.dart
# only stage fakes you actually changed
git commit -m "$(cat <<'EOF'
feat(client): cache catalog providers on chat connect

EOF
)"
```

---

### Task 3: `ModelPicker` popover UI and wire into composer

**Files:**
- Create: `client/lib/chat/model_picker.dart`
- Modify: `client/lib/chat/chat_composer.dart` (replace `_modelPicker`)
- Create: `client/test/chat/model_picker_test.dart`
- Modify: `client/test/widget_test.dart` (model-picker is no longer `DropdownButton`)
- Modify: `client/test/chat/chat_screen_test.dart` if needed (key still works)

**Interfaces:**
- Consumes: `controller.modelOptions`, `controller.providers`, `controller.currentModel`, `controller.canSelectModel`, `controller.selectModel`
- Produces: `ModelPicker` with key `model-picker` on the trigger

- [ ] **Step 1: Write failing widget tests**

Create `client/test/chat/model_picker_test.dart` (reuse FakeConn / FakeCatalog patterns from `chat_composer_test.dart`; ensure FakeCatalog returns providers with models matching session options):

```dart
testWidgets('opens popover, filters by search, selects model', (tester) async {
  final fake = FakeConn();
  final catalog = FakeCatalog([_agent('ag-1', 'Alpha')])
    ..providers = [
      // provider with ModelInfo id m1, m2 — match FakeConn startSession models
    ];
  final c = ChatController(session: fake, catalog: catalog);
  addTearDown(c.dispose);
  await c.connect();
  await c.createThread();
  await c.selectAgent('ag-1');

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(body: ModelPicker(controller: c)),
    ),
  );
  await tester.pumpAndSettle();

  await tester.tap(find.byKey(const Key('model-picker')));
  await tester.pumpAndSettle();

  expect(find.text('Local'), findsOneWidget); // or provider name used
  expect(find.textContaining('(2)'), findsWidgets);

  await tester.enterText(find.byKey(const Key('model-picker-search')), 'Model 2');
  await tester.pumpAndSettle();
  expect(find.text('Model 1'), findsNothing);
  expect(find.text('Model 2'), findsOneWidget);

  await tester.tap(find.text('Model 2'));
  await tester.pumpAndSettle();
  expect(fake.setModels, ['m2']);
  expect(find.byKey(const Key('model-picker-search')), findsNothing); // closed
});

testWidgets('disabled when cannot select model', (tester) async {
  // connected but no session ready — canSelectModel false
  // tap does not open search field
});
```

Fill FakeCatalog providers so ids match `FakeConn.startSession` model options (`m1`/`m2`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/model_picker_test.dart'`

Expected: FAIL — `ModelPicker` missing.

- [ ] **Step 3: Implement `ModelPicker` and wire composer**

Create `client/lib/chat/model_picker.dart` using `MenuAnchor` (Flutter Material):

```dart
class ModelPicker extends StatelessWidget {
  const ModelPicker({super.key, required this.controller});
  final ChatController controller;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final enabled =
            controller.canSelectModel && controller.modelOptions.isNotEmpty;
        final label = _currentLabel(controller);
        return MenuAnchor(
          builder: (context, controllerMenu, child) {
            return InkWell(
              key: const Key('model-picker'),
              onTap: !enabled
                  ? null
                  : () {
                      if (controllerMenu.isOpen) {
                        controllerMenu.close();
                      } else {
                        controllerMenu.open();
                      }
                    },
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      label,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodyMedium,
                    ),
                  ),
                  Icon(
                    controllerMenu.isOpen
                        ? Icons.expand_less
                        : Icons.expand_more,
                    size: 18,
                  ),
                ],
              ),
            );
          },
          menuChildren: [
            // MenuAnchor expects MenuItemButton list; for custom panel use
            // a single ConstrainedBox child via builder pattern OR Overlay.
          ],
        );
      },
    );
  }
}
```

**Preferred concrete approach:** use `MenuAnchor` with `style` / custom menu via:

```dart
MenuAnchor(
  crossAxisUnconstrained: false,
  style: const MenuStyle(
    maximumSize: WidgetStatePropertyAll(Size(360, 420)),
  ),
  menuChildren: [
    SubmenuButton( /* avoid */ ),
  ],
)
```

If `MenuAnchor`’s `menuChildren` is awkward for a full search panel, implement with `OverlayPortal` / `CompositedTransformTarget`+`Follower` instead — keep the same keys and behavior. The panel must include:

1. Empty `SizedBox.shrink()` header slot (comment: future favorites).
2. `TextField(key: Key('model-picker-search'), decoration: InputDecoration(hintText: 'Search models…', suffixIcon: clear))`.
3. `StatefulBuilder` or small `StatefulWidget` `_ModelPickerPanel` holding `query` and `Set<String> collapsedIds` (empty = all expanded).
4. For each filtered group: header `InkWell` toggles collapse; shows `▼/▶ Name (n)`; children are model rows with check icon if `model.id == currentModel`.
5. Tap row → `controller.selectModel(model.id)` then close menu.

In `chat_composer.dart`, replace `_modelPicker(c)` call and delete `_modelPicker` method; insert `Flexible(child: ModelPicker(controller: c))`.

Update `widget_test.dart`:
- Remove `DropdownButton` cast on `model-picker`.
- Assert `find.byKey(Key('model-picker'))` still present.
- For disabled-until-connected: tap should not open search (or InkWell onTap null) — assert `canSelectModel` path via not finding search after tap, or check that the trigger’s `onTap` is null by reading the widget if practical. Simplest: keep agent-picker `DropdownButton` assertions; for model-picker only `findsOneWidget` + after pump connected still no session → tap does not show `model-picker-search`.

- [ ] **Step 4: Run tests**

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/model_picker_test.dart test/chat/model_picker_grouping_test.dart test/chat/chat_composer_test.dart test/chat/chat_screen_test.dart test/widget_test.dart test/chat/chat_controller_test.dart'
```

Expected: PASS for these suites (ignore pre-existing `thread_pane_test` unless this change breaks it).

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/model_picker.dart client/lib/chat/chat_composer.dart \
  client/test/chat/model_picker_test.dart client/test/widget_test.dart \
  client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add searchable provider-grouped model picker

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Group by catalog provider; Other for unmatched | 1 |
| Search name/id; hide empty groups | 1 + 3 |
| Cache providers; no spam on open | 2 |
| Anchored popover + chip; key `model-picker` | 3 |
| Collapsible groups, default expanded, session-local | 3 |
| Extensible empty header slot | 3 |
| `selectModel` unchanged; session list only | 3 |
| No Custom ID / Configured / favorites | — (non-goals) |

## Self-review notes

- Helper signatures consistent across Tasks 1–3.
- `Other` spelling locked as `kOtherProviderGroupName = 'Other'`.
- `widget_test` must stop assuming model-picker is `DropdownButton`.
- Provider load is best-effort on connect; empty providers → all models in Other (helper already covers).
