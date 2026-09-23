# Dockable Panes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the fixed desktop columns (Threads | Workspace | Chat) with a `docking`-based host so cores and each open editor/preview are resizable, tabbable, rearrangable dock items, with shell layout persisted and rail toggles to reopen closed cores.

**Architecture:** Keep `NavigationRail` fixed. Chat mode shows `Docking` inside an `IndexedStack` (Settings is the other child so Chat stays mounted). `DockLayoutController` owns `DockingLayout`, focus, core toggles, document open/close placement, and prefs persist/restore. `WorkspaceController` keeps documents/tree/git and stops owning `groups` / `paneOpen`.

**Tech Stack:** Flutter, `docking` (^1.16.2), `shared_preferences`, existing `material_ui` / workspace / chat widgets.

**Spec:** [`docs/superpowers/specs/2026-09-23-dockable-panes-design.md`](../specs/2026-09-23-dockable-panes-design.md)

## Global Constraints

- Package: `docking` only (not `dock_panel` / Riverpod).
- Core dock IDs: `threads`, `files`, `chat` (string).
- Document dock ID: `doc:{path}:{appId.name}` (e.g. `doc:index.html:textEditor`).
- Default layout: `DockingRow([threads, files, chat])` with weights `0.18`, `0.16`, `0.66`.
- Persist key: `dock_shell_layout_v1` in `SharedPreferences`.
- On restore, strip any item whose id is a `String` starting with `doc:`; never reopen documents from prefs.
- Corrupt / failed restore → default classic layout; do not throw to UI.
- Rail destination order: Chat, Threads, Files, Settings.
- Desktop only for docking; `width < 720` still pushes `WorkspacePage`.
- Remove Chat header Files toggle (`files-toggle` key); Files show/hide is the rail Files destination (`Key('rail-files')`).
- `DockingItem` for `chat` and all `doc:` items: `keepAlive: true`.
- No backend / catalog changes.

## File map

| File | Responsibility |
| --- | --- |
| `client/pubspec.yaml` | Add `docking` |
| `client/lib/dock/dock_ids.dart` | Core ID constants + doc id helpers |
| `client/lib/dock/dock_layout_controller.dart` | Layout mutations, focus, persist |
| `client/lib/dock/dock_view_body.dart` | Public view body for one `OpenView` (moved from private `_ViewBody`) |
| `client/lib/dock/files_dock_panel.dart` | Files core: explorer + checkpoint/history/save chrome |
| `client/lib/app_shell.dart` | Rail + IndexedStack(Docking, Settings) |
| `client/lib/workspace/workspace_controller.dart` | Drop groups/paneOpen; open views list + dock callbacks |
| `client/lib/workspace/workspace_pane.dart` | Mobile `WorkspacePage` only (or thin wrapper) |
| `client/lib/workspace/open_with.dart` | Keep `OpenView`; remove `EditorGroup` when unused |
| `client/lib/chat/thread_pane.dart` | Remove fixed `SizedBox(width: 240)` |
| `client/lib/chat/chat_screen.dart` | Remove `filesOpen` / `onToggleFiles` |
| `client/test/dock/dock_layout_controller_test.dart` | Unit tests |
| `client/test/app_shell_test.dart` | Rail Files toggle + layout order |
| `client/test/workspace/workspace_pane_test.dart` | Adapt to dock / openViews |

---

### Task 1: Add `docking` dependency

**Files:**
- Modify: `client/pubspec.yaml`
- Test: (dependency resolve)

**Interfaces:**
- Consumes: none
- Produces: `docking` importable from `package:docking/docking.dart`

- [ ] **Step 1: Add dependency**

In `client/pubspec.yaml` under `dependencies:`:

```yaml
  docking: ^1.16.2
```

- [ ] **Step 2: Resolve**

Run: `cd client && flutter pub get`

Expected: exit 0; `docking` in `.dart_tool/package_config.json`

- [ ] **Step 3: Commit**

```bash
git add client/pubspec.yaml client/pubspec.lock
git commit -m "Add docking package for desktop pane host."
```

---

### Task 2: Dock IDs + default layout controller

**Files:**
- Create: `client/lib/dock/dock_ids.dart`
- Create: `client/lib/dock/dock_layout_controller.dart`
- Create: `client/test/dock/dock_layout_controller_test.dart`

**Interfaces:**
- Consumes: `package:docking/docking.dart`
- Produces:
  - `DockIds.threads` / `.files` / `.chat` (`String`)
  - `DockIds.doc(String path, WorkspaceAppId app)` → `String`
  - `DockIds.isDoc(dynamic id)` → `bool`
  - `class DockLayoutController extends ChangeNotifier`
  - `DockingLayout get layout`
  - `dynamic focusedItemId`
  - `void resetToDefault({required DockItemWidgets widgets})` — builds default row
  - `bool hasItem(dynamic id)`
  - `DockItemWidgets` typedef/class holding `Widget` builders for cores (and later docs)

```dart
class DockItemWidgets {
  const DockItemWidgets({
    required this.threads,
    required this.files,
    required this.chat,
  });
  final Widget threads;
  final Widget files;
  final Widget chat;
}
```

For unit tests, pass `const SizedBox()` / `Text('…')` placeholders.

- [ ] **Step 1: Write failing test — default layout**

Create `client/test/dock/dock_layout_controller_test.dart`:

```dart
import 'package:agent_fabric_client/dock/dock_ids.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:docking/docking.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

DockItemWidgets _stubs() => const DockItemWidgets(
      threads: SizedBox(),
      files: SizedBox(),
      chat: SizedBox(),
    );

void main() {
  test('default layout is threads | files | chat', () {
    final c = DockLayoutController();
    c.resetToDefault(widgets: _stubs());

    expect(c.hasItem(DockIds.threads), isTrue);
    expect(c.hasItem(DockIds.files), isTrue);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.layout.root, isA<DockingRow>());
    final row = c.layout.root! as DockingRow;
    expect(row.children.map((a) => (a as DockingItem).id), [
      DockIds.threads,
      DockIds.files,
      DockIds.chat,
    ]);
  });
}
```

- [ ] **Step 2: Run test — expect fail**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: FAIL (library/URI not found or class missing)

- [ ] **Step 3: Implement IDs + controller default**

`client/lib/dock/dock_ids.dart`:

```dart
import '../workspace/open_with.dart';

abstract final class DockIds {
  static const threads = 'threads';
  static const files = 'files';
  static const chat = 'chat';

  static String doc(String path, WorkspaceAppId app) => 'doc:$path:${app.name}';

  static bool isDoc(dynamic id) => id is String && id.startsWith('doc:');
}
```

`client/lib/dock/dock_layout_controller.dart` — minimal:

```dart
import 'package:docking/docking.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

import '../workspace/open_with.dart';
import 'dock_ids.dart';

class DockItemWidgets {
  const DockItemWidgets({
    required this.threads,
    required this.files,
    required this.chat,
  });
  final Widget threads;
  final Widget files;
  final Widget chat;
}

class DockLayoutController extends ChangeNotifier {
  DockLayoutController();

  final DockingLayout layout = DockingLayout();
  dynamic focusedItemId;
  DockItemWidgets? _widgets;

  bool hasItem(dynamic id) => layout.findDockingItem(id) != null;

  DockingItem _core(String id, Widget child, {required double weight}) {
    return DockingItem(
      id: id,
      name: id,
      weight: weight,
      closable: true,
      keepAlive: id == DockIds.chat,
      widget: child,
    );
  }

  void resetToDefault({required DockItemWidgets widgets}) {
    _widgets = widgets;
    layout.root = DockingRow([
      _core(DockIds.threads, widgets.threads, weight: 0.18),
      _core(DockIds.files, widgets.files, weight: 0.16),
      _core(DockIds.chat, widgets.chat, weight: 0.66),
    ]);
    focusedItemId = DockIds.chat;
    notifyListeners();
  }
}
```

Adjust constructor APIs if the installed `docking` version names parameters differently — match the package you resolved in Task 1 (`DockingItem` / `DockingRow` docs).

- [ ] **Step 4: Run test — expect pass**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_ids.dart client/lib/dock/dock_layout_controller.dart client/test/dock/dock_layout_controller_test.dart
git commit -m "Add DockLayoutController with classic default layout."
```

---

### Task 3: Toggle cores (show / hide / focus)

**Files:**
- Modify: `client/lib/dock/dock_layout_controller.dart`
- Modify: `client/test/dock/dock_layout_controller_test.dart`

**Interfaces:**
- Consumes: `resetToDefault`, `DockItemWidgets`
- Produces:
  - `void toggleCore(String coreId)` — if present, remove; else re-insert
  - `void ensureCore(String coreId)` — no-op if present; else insert
  - Insert rules: `threads` leftmost on root; `files` after threads (or leftmost if threads absent); `chat` rightmost on root

- [ ] **Step 1: Write failing tests**

Append to `dock_layout_controller_test.dart`:

```dart
  test('toggleCore removes and restores files', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.toggleCore(DockIds.files);
    expect(c.hasItem(DockIds.files), isFalse);
    c.toggleCore(DockIds.files);
    expect(c.hasItem(DockIds.files), isTrue);
  });

  test('ensureCore focuses existing chat', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.threads;
    c.ensureCore(DockIds.chat);
    expect(c.hasItem(DockIds.chat), isTrue);
    expect(c.focusedItemId, DockIds.chat);
  });
```

- [ ] **Step 2: Run tests — expect fail**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: FAIL on missing `toggleCore` / `ensureCore`

- [ ] **Step 3: Implement toggle / ensure**

Use `layout.removeItem(item: …)` when present.

When re-inserting, use `layout.addItemOnRoot` / `layout.addItemOn` with `DropPosition.left` or `.right` per rules above, building the core `DockingItem` from `_widgets!`. If `_widgets` is null, no-op.

Set `focusedItemId` to the core id on ensure/toggle-open.

Call `notifyListeners()` after mutations. Also register `layout.addListener(notifyListeners)` in the controller constructor (and remove in `dispose`) so drag-rearranges from the `Docking` widget notify the controller.

- [ ] **Step 4: Run tests — expect pass**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_layout_controller.dart client/test/dock/dock_layout_controller_test.dart
git commit -m "Support toggling dock core panels."
```

---

### Task 4: Open / close document dock items

**Files:**
- Modify: `client/lib/dock/dock_layout_controller.dart`
- Modify: `client/test/dock/dock_layout_controller_test.dart`

**Interfaces:**
- Consumes: focused item, `DockIds.doc`
- Produces:
  - `void openDocument({required OpenView view, required Widget child, bool toSide = false})`
  - `void closeDocument(String dockId)`
  - `void clearDocuments()`
  - If item with same dock id exists → set `focusedItemId` only
  - Else if `toSide` → `addItemOn` focused target with `DropPosition.right`
  - Else → `addItemOn` focused target with `dropIndex` into same tabs (or `addItemOn` with `dropPosition: null` / tab drop per package API — use the API that adds as a new tab on the focused item’s tab group; if focus is a lone item, create tabs)
  - Document items: `closable: true`, `keepAlive: true`, `name: view.tabLabel`

- [ ] **Step 1: Write failing tests**

```dart
  test('openDocument adds tab beside focused core', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.files;
    final view = OpenView(
      viewId: 'view-1',
      path: 'index.html',
      appId: WorkspaceAppId.textEditor,
    );
    c.openDocument(view: view, child: const Text('ed'));
    expect(c.hasItem(DockIds.doc('index.html', WorkspaceAppId.textEditor)), isTrue);
  });

  test('openDocument toSide splits relative to focus', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.focusedItemId = DockIds.chat;
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    c.openDocument(
      view: OpenView(
        viewId: 'view-2',
        path: 'b.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('b'),
      toSide: true,
    );
    expect(c.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)), isTrue);
    expect(c.hasItem(DockIds.doc('b.txt', WorkspaceAppId.textEditor)), isTrue);
    // hierarchy should not be a single DockingTabs of three cores only —
    // at least one DockingRow or DockingColumn involving the two docs
    expect(c.layout.hierarchy(nameInfo: true), contains('a.txt'));
  });

  test('clearDocuments removes only doc items', () {
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    c.clearDocuments();
    expect(c.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)), isFalse);
    expect(c.hasItem(DockIds.chat), isTrue);
  });
```

Import `open_with.dart` in the test file.

- [ ] **Step 2: Run tests — expect fail**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: FAIL missing methods

- [ ] **Step 3: Implement open / close / clear**

Resolve focus target: `layout.findDockingItem(focusedItemId)` falling back to `chat` then any item.

Use `layout.addItemOn(newItem: …, targetArea: target, dropPosition: toSide ? DropPosition.right : null, dropIndex: toSide ? null : <tab index or 0>)` per docking docs for tab vs split.

`clearDocuments`: collect ids where `DockIds.isDoc(id)`, then `layout.removeItemByIds(ids)`.

- [ ] **Step 4: Run tests — expect pass**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_layout_controller.dart client/test/dock/dock_layout_controller_test.dart
git commit -m "Open and close document dock items from focus."
```

---

### Task 5: Persist and restore shell layout

**Files:**
- Modify: `client/lib/dock/dock_layout_controller.dart`
- Modify: `client/test/dock/dock_layout_controller_test.dart`

**Interfaces:**
- Consumes: `shared_preferences`, docking `stringify` / `load`, `LayoutParserMixin`, `AreaBuilderMixin`
- Produces:
  - `static const prefsKey = 'dock_shell_layout_v1'`
  - `Future<void> persist()`
  - `Future<void> restore({required DockItemWidgets widgets})`
  - On restore success: rebuild cores from saved string; after `load`, `removeItemByIds` for every `doc:` id found
  - On failure: `resetToDefault(widgets: widgets)`

Implement parser/builder as a private class on the controller (or mixin on a helper) that maps `threads` / `files` / `chat` to widgets from `DockItemWidgets`. For `doc:` ids during load, build a temporary empty `DockingItem` then strip after load.

Listen to `layout` and debounce persist (~300ms) OR call `persist()` from `notifyListeners` path after user-driven changes — prefer explicit `schedulePersist()` from layout listener to avoid async in notify.

- [ ] **Step 1: Write failing tests**

```dart
  test('persist round-trip keeps cores and drops docs', () async {
    SharedPreferences.setMockInitialValues({});
    final c = DockLayoutController()..resetToDefault(widgets: _stubs());
    c.openDocument(
      view: OpenView(
        viewId: 'view-1',
        path: 'a.txt',
        appId: WorkspaceAppId.textEditor,
      ),
      child: const Text('a'),
    );
    await c.persist();

    final c2 = DockLayoutController();
    await c2.restore(widgets: _stubs());
    expect(c2.hasItem(DockIds.threads), isTrue);
    expect(c2.hasItem(DockIds.files), isTrue);
    expect(c2.hasItem(DockIds.chat), isTrue);
    expect(c2.hasItem(DockIds.doc('a.txt', WorkspaceAppId.textEditor)), isFalse);
  });

  test('corrupt prefs falls back to default', () async {
    SharedPreferences.setMockInitialValues({
      DockLayoutController.prefsKey: 'not-a-layout',
    });
    final c = DockLayoutController();
    await c.restore(widgets: _stubs());
    expect(c.hasItem(DockIds.chat), isTrue);
  });
```

- [ ] **Step 2: Run tests — expect fail**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: FAIL

- [ ] **Step 3: Implement persist / restore**

Follow docking “Save and load” docs (`stringify` + `load` with parser + builder). After successful load, strip docs. Wrap `load` in try/catch → `resetToDefault`.

- [ ] **Step 4: Run tests — expect pass**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_layout_controller.dart client/test/dock/dock_layout_controller_test.dart
git commit -m "Persist dock shell layout without open documents."
```

---

### Task 6: Extract view body + files panel; unfix ThreadPane

**Files:**
- Create: `client/lib/dock/dock_view_body.dart`
- Create: `client/lib/dock/files_dock_panel.dart`
- Modify: `client/lib/workspace/workspace_pane.dart`
- Modify: `client/lib/chat/thread_pane.dart`
- Test: existing workspace tests still compile after export

**Interfaces:**
- Consumes: `WorkspaceController`, existing `_ViewBody` switch
- Produces:
  - `class DockViewBody extends StatelessWidget` — same switch as today’s `_ViewBody`
  - `class FilesDockPanel extends StatelessWidget` — header (Workspace label + save/checkpoint/history) + `FileExplorer`
  - `ThreadPane` fills parent (no `SizedBox(width: 240)`)

- [ ] **Step 1: Move `_ViewBody` to `DockViewBody`**

Copy the switch from `workspace_pane.dart` into `dock_view_body.dart` as public `DockViewBody`. Make `workspace_pane.dart` `_ViewBody` delegate to `DockViewBody` **or** update mobile page to use `DockViewBody` after Task 8.

- [ ] **Step 2: Add `FilesDockPanel`**

Reuse header actions from current `WorkspacePane` (save / checkpoint / history keys unchanged: `save-file`, `checkpoint-button`, `history-button`) + `FileExplorer(controller: …)` with key `file-explorer`.

- [ ] **Step 3: Remove ThreadPane fixed width**

Replace `return SizedBox(width: 240, child: AnimatedBuilder(...))` with `return AnimatedBuilder(...)` so the pane expands inside the dock.

- [ ] **Step 4: Run analyze / quick tests**

Run: `cd client && dart analyze lib/dock lib/chat/thread_pane.dart lib/workspace/workspace_pane.dart`

Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_view_body.dart client/lib/dock/files_dock_panel.dart client/lib/workspace/workspace_pane.dart client/lib/chat/thread_pane.dart
git commit -m "Extract dock panels and unfix ThreadPane width."
```

---

### Task 7: Slim WorkspaceController — open views without EditorGroup

**Files:**
- Modify: `client/lib/workspace/workspace_controller.dart`
- Modify: `client/lib/workspace/open_with.dart` (remove `EditorGroup` if unused)
- Modify: `client/test/workspace/workspace_pane_test.dart`
- Test: `client/test/workspace/workspace_pane_test.dart`

**Interfaces:**
- Consumes: optional `void Function(OpenView view, {required bool toSide})? onViewOpened` and `void Function(String viewId)? onViewClosed` set by AppShell
- Produces:
  - `final List<OpenView> openViews = []`
  - `OpenView? focusedView` tracked by `focusedViewId`
  - `openWith` / `openDefault` update `openViews` and call `onViewOpened`
  - `closeView` removes from `openViews`, calls `onViewClosed`, `_maybeCloseDocument`
  - Remove: `paneOpen`, `togglePane`, `groups`, `focusedGroupId`, `_placeView`, `focusGroup`, `focusTab`
  - `setProjectId` clears `openViews` and calls a new `VoidCallback? onDocumentsCleared` so dock can `clearDocuments()`
  - Always `refreshTree` when project set (not gated on `paneOpen`)

- [ ] **Step 1: Rewrite failing workspace tests that assert `groups`**

In `workspace_pane_test.dart`, change assertions from `workspace.groups` to `workspace.openViews` / dock integration. For tests that only check save/openWith document state, drop `paneOpen = true` and assert `openViews.length` / paths instead of group tabs.

Example replacement for split test intent:

```dart
  test('opening html web preview notifies toSide when editor open', () async {
    // after wiring: open text editor then web preview → onViewOpened called with toSide true
  });
```

Keep catalog/document tests working without UI groups.

- [ ] **Step 2: Run tests — expect fail**

Run: `cd client && flutter test test/workspace/workspace_pane_test.dart`

Expected: FAIL against old `groups` API

- [ ] **Step 3: Implement slim controller**

Update `openWith` to append/focus `OpenView` in `openViews` and invoke `onViewOpened?.call(view, toSide: split)`.

- [ ] **Step 4: Run tests — expect pass**

Run: `cd client && flutter test test/workspace/workspace_pane_test.dart`

Expected: PASS (update any remaining group references)

- [ ] **Step 5: Commit**

```bash
git add client/lib/workspace/workspace_controller.dart client/lib/workspace/open_with.dart client/test/workspace/workspace_pane_test.dart
git commit -m "Move workspace open views out of EditorGroup model."
```

---

### Task 8: Wire AppShell — rail + Docking host

**Files:**
- Modify: `client/lib/app_shell.dart`
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/test/app_shell_test.dart`

**Interfaces:**
- Consumes: `DockLayoutController`, `WorkspaceController`, `DockItemWidgets`, `FilesDockPanel`, `ThreadPane`, `ChatScreen`, `DockViewBody`
- Produces: desktop shell per spec

**AppShell behavior:**

1. Create `DockLayoutController`; after first frame, `restore` or `resetToDefault` with widgets:
   - threads: `ThreadPane(controller: chat)`
   - files: `FilesDockPanel(controller: workspace)`
   - chat: `ChatScreen(...)` **without** files toggle
2. Wire `workspace.onViewOpened` → `dock.openDocument(view: view, toSide: toSide, child: DockViewBody(...))`
3. Wire `workspace.onViewClosed` → `dock.closeDocument(DockIds.doc(...))` (map viewId → OpenView to build id, or pass dock id from open)
4. Wire `workspace.onDocumentsCleared` → `dock.clearDocuments()`
5. Body:

```dart
Row(
  children: [
    NavigationRail(
      selectedIndex: _railIndex,
      onDestinationSelected: _onRail,
      destinations: const [
        NavigationRailDestination(icon: Icon(Icons.chat), label: Text('Chat')),
        NavigationRailDestination(icon: Icon(Icons.forum_outlined), label: Text('Threads')),
        NavigationRailDestination(icon: Icon(Icons.folder_outlined), label: Text('Files')),
        NavigationRailDestination(icon: Icon(Icons.settings), label: Text('Settings')),
      ],
      // Keys: Key('rail-chat'), Key('rail-threads'), Key('rail-files'), Key('rail-settings')
    ),
    Expanded(
      child: IndexedStack(
        index: _railIndex == 3 ? 1 : 0,
        children: [
          Docking(
            layout: _dock.layout,
            onItemSelection: (item) => _dock.focusedItemId = item.id,
            onItemClose: (item) { /* sync workspace.closeView for docs; cores just removed */ },
            itemCloseInterceptor: (item) { /* dirty doc confirm if needed */ return true; },
          ),
          SettingsPage(...),
        ],
      ),
    ),
  ],
)
```

6. Rail handlers:
   - Chat (0): `_railIndex = 0`; `_dock.ensureCore(DockIds.chat)`
   - Threads (1): `_railIndex = 0` (stay on dock); `_dock.toggleCore(DockIds.threads)`; optionally set selected visual — keep `_railIndex` at 0 and use a separate highlight, **or** set `_railIndex = 1` while stack index stays 0 when `1` or `2`. Prefer: `_stackIndex = (_railIndex == 3) ? 1 : 0` and allow `_railIndex` in `{0,1,2}` all show dock.
   - Files (2): toggle files; if `width < 720`, push `WorkspacePage` instead of toggle
   - Settings (3): `_railIndex = 3`

7. Remove `filesOpen` / `onToggleFiles` from `ChatScreen`.

8. Theme: wrap `Docking` in `MultiSplitViewTheme` / `TabbedViewTheme` lightly so dividers are visible (thickness ≥ 4).

- [ ] **Step 1: Rewrite app_shell tests**

Replace Files header toggle tests:

```dart
  testWidgets('rail Files toggles files dock panel', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    // pump AppShell…
    expect(find.byKey(const Key('file-explorer')), findsOneWidget); // default layout includes files
    await tester.tap(find.byKey(const Key('rail-files')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('file-explorer')), findsNothing);
    await tester.tap(find.byKey(const Key('rail-files')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('file-explorer')), findsOneWidget);
  });

  testWidgets('default dock order threads then files then chat', (tester) async {
    // same size setup
    // expect dx order ThreadPane < FilesDockPanel/file-explorer < ChatScreen
  });
```

Remove expectations for `Key('files-toggle')` and `WorkspacePane` as desktop host.

Settings test: ThreadPane may still be in the offstage `IndexedStack`/`Docking` — assert `SettingsPage` visible and prefer `findsWidgets` / hit-testable checks. If ThreadPane remains in tree offstage, assert `SettingsPage` and that Chat rail returns to dock.

- [ ] **Step 2: Run tests — expect fail**

Run: `cd client && flutter test test/app_shell_test.dart`

Expected: FAIL

- [ ] **Step 3: Implement AppShell + ChatScreen cleanup**

- [ ] **Step 4: Run tests — expect pass**

Run: `cd client && flutter test test/app_shell_test.dart test/dock/dock_layout_controller_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/app_shell.dart client/lib/chat/chat_screen.dart client/test/app_shell_test.dart
git commit -m "Host Chat mode in docking with rail core toggles."
```

---

### Task 9: Mobile WorkspacePage + dirty close + final cleanup

**Files:**
- Modify: `client/lib/workspace/workspace_pane.dart` (keep `WorkspacePage` usable without dock groups — explorer + single-column open views list or reuse internal tabs for mobile only)
- Modify: `client/lib/dock/dock_layout_controller.dart` / AppShell close interceptor if dirty confirm missing
- Modify: any leftover references to `paneOpen` / `EditorGroup` / `WorkspacePane` on desktop

**Interfaces:**
- Consumes: slim `WorkspaceController.openViews`
- Produces: `WorkspacePage` that lists/opens files on narrow screens without `docking`

- [ ] **Step 1: Ensure mobile page works**

`WorkspacePage` body: `FilesDockPanel` plus, if `openViews` non-empty, a simple tab strip + `DockViewBody` for `focusedView` (local mobile-only UI OK; do not use `docking` package here).

- [ ] **Step 2: Dirty close**

If `FileDocument` dirty for path being closed, show dialog Save / Discard / Cancel before `workspace.closeView`. Implement interceptor in AppShell `itemCloseInterceptor`.

- [ ] **Step 3: Full client test pass**

Run: `cd client && flutter test`

Expected: PASS (fix any stragglers)

- [ ] **Step 4: Commit**

```bash
git add client/lib client/test
git commit -m "Finish dockable panes mobile path and dirty close."
```

---

## Self-review (plan vs spec)

| Spec requirement | Task |
| --- | --- |
| `docking` package | 1 |
| Fixed rail; dock host; IndexedStack Settings | 8 |
| Cores threads/files/chat | 2–3, 8 |
| Each editor/preview is dock item | 4, 7–8 |
| Closable cores + rail toggles | 3, 8 |
| Persist shell not docs | 5 |
| Default classic weights | 2 |
| Open into focused / toSide | 4, 7 |
| `<720` WorkspacePage | 8–9 |
| Remove chat Files toggle | 8 |
| keepAlive chat + docs | 2, 4 |
| Tests unit + widget | 2–5, 8 |
| Out of scope left out | — |

No placeholders left that block implementation; docking parameter names must be verified against the resolved package version in Task 1 and adjusted in Tasks 2–5 if the API differs slightly.
