# Dock Tab Chrome & Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Style dock tabs as Hybrid+Soft IDE chrome (muted inactive text, high-contrast active, thin top accent) and add type icons plus dirty/chat status indicators using `TabbedViewTheme` and `DockingItem.leading` / buttons — no custom tab-strip rewrite.

**Architecture:** Pure helpers build theme and leading widgets from `ColorScheme` / item id. `DockLayoutController` attaches leadings when creating cores/docs and exposes small mutators that call `layout.rebuild()` after updating `DockingItem` fields. A thin chat binder in `AppShell` listens to `ChatController` + `focusedItemId`. Dirty docs get `closable: false` plus a circle `TabButton` that closes (package cannot swap icon glyph on hover; hover uses color only — matches spec fallback spirit without a second close control).

**Tech Stack:** Flutter, `docking` ^1.16.2, `tabbed_view` (transitive), existing `ChatController` / `WorkspaceController` / `material_ui`.

**Spec:** [`docs/superpowers/specs/2026-09-23-dock-tab-chrome-design.md`](../specs/2026-09-23-dock-tab-chrome-design.md)

## Global Constraints

- No fork of `docking` / `tabbed_view`; no custom tab-strip widget.
- Visual: Hybrid + Soft — strip behind inactive tabs, subtle vertical dividers, active fill matches content, thin top accent from `colorScheme.primary`, inactive text/icon ~40–45% `onSurface` opacity, active high-contrast `onSurface`.
- Chat focused → always plain chat icon; ignore app/browser blur.
- Chat unfocused + `sending` → loading leading; unfocused + turn done + unread → blue leading dot.
- Selecting chat tab clears unread immediately.
- Dirty: circle replaces native `×` (`closable: false` + close `TabButton`); click closes (existing dirty confirm interceptor stays). No package glyph-swap on hover — use `hoverColor` only.
- No unread persistence; no per-language file icons; no transport/WebSocket changes.
- After mutating `DockingItem.leading` / `buttons` / `closable`, call `layout.rebuild()`.

## File map

| File | Responsibility |
| --- | --- |
| `client/lib/dock/dock_tab_theme.dart` | `TabbedViewThemeData` from `ColorScheme` |
| `client/lib/dock/dock_tab_icons.dart` | Type / status leading builders + dirty close `TabButton` factory |
| `client/lib/dock/dock_chat_tab_status.dart` | Pure resolve + unread flag helper for chat leading kind |
| `client/lib/dock/dock_layout_controller.dart` | Attach leadings on create; mutators for chat leading / dirty close UI |
| `client/lib/app_shell.dart` | Apply theme; wire chat binder + dirty listeners |
| `client/test/dock/dock_tab_theme_test.dart` | Theme smoke / contrast expectations |
| `client/test/dock/dock_tab_icons_test.dart` | Leading / dirty button helpers |
| `client/test/dock/dock_chat_tab_status_test.dart` | Resolve matrix + unread transitions |
| `client/test/dock/dock_layout_controller_test.dart` | Leading attached; rebuild mutators |
| `client/test/app_shell_test.dart` | Theme present; chat status / dirty chrome if practical |

---

### Task 1: Chat tab status pure logic

**Files:**
- Create: `client/lib/dock/dock_chat_tab_status.dart`
- Test: `client/test/dock/dock_chat_tab_status_test.dart`

**Interfaces:**
- Consumes: none
- Produces:
  - `enum DockChatTabLead { plain, loading, unread }`
  - `DockChatTabLead resolveDockChatTabLead({required bool chatFocused, required bool sending, required bool unread})`
  - `class DockChatTabUnread` with `bool value`, `void clear()`, `void markIfUnfocused({required bool chatFocused})` (sets unread only when `!chatFocused`)

- [ ] **Step 1: Write the failing test**

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:agent_fabric_client/dock/dock_chat_tab_status.dart';

void main() {
  group('resolveDockChatTabLead', () {
    test('focused always plain even when sending or unread', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: true,
          sending: true,
          unread: true,
        ),
        DockChatTabLead.plain,
      );
    });

    test('unfocused sending is loading', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: false,
          sending: true,
          unread: false,
        ),
        DockChatTabLead.loading,
      );
    });

    test('unfocused not sending with unread is unread', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: false,
          sending: false,
          unread: true,
        ),
        DockChatTabLead.unread,
      );
    });

    test('unfocused idle is plain', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: false,
          sending: false,
          unread: false,
        ),
        DockChatTabLead.plain,
      );
    });
  });

  group('DockChatTabUnread', () {
    test('markIfUnfocused sets only when unfocused', () {
      final u = DockChatTabUnread();
      u.markIfUnfocused(chatFocused: true);
      expect(u.value, isFalse);
      u.markIfUnfocused(chatFocused: false);
      expect(u.value, isTrue);
    });

    test('clear resets', () {
      final u = DockChatTabUnread()..markIfUnfocused(chatFocused: false);
      u.clear();
      expect(u.value, isFalse);
    });
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && flutter test test/dock/dock_chat_tab_status_test.dart`

Expected: FAIL — library not found

- [ ] **Step 3: Write minimal implementation**

```dart
enum DockChatTabLead { plain, loading, unread }

DockChatTabLead resolveDockChatTabLead({
  required bool chatFocused,
  required bool sending,
  required bool unread,
}) {
  if (chatFocused) return DockChatTabLead.plain;
  if (sending) return DockChatTabLead.loading;
  if (unread) return DockChatTabLead.unread;
  return DockChatTabLead.plain;
}

class DockChatTabUnread {
  bool value = false;

  void clear() => value = false;

  void markIfUnfocused({required bool chatFocused}) {
    if (!chatFocused) value = true;
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd client && flutter test test/dock/dock_chat_tab_status_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_chat_tab_status.dart client/test/dock/dock_chat_tab_status_test.dart
git commit -m "Add pure chat dock-tab status resolve helpers."
```

---

### Task 2: Type icons and dirty-close button helpers

**Files:**
- Create: `client/lib/dock/dock_tab_icons.dart`
- Test: `client/test/dock/dock_tab_icons_test.dart`

**Interfaces:**
- Consumes: `DockIds`, `WorkspaceAppId`, `DockChatTabLead`, `TabLeadingBuilder` / `TabStatus` from `tabbed_view`, `TabButton` / `IconProvider`
- Produces:
  - `TabLeadingBuilder dockTabLeadingForId(dynamic id, {DockChatTabLead chatLead = DockChatTabLead.plain})`
  - `TabLeadingBuilder dockTabLeadingForApp(WorkspaceAppId app)`
  - `TabButton dirtyCloseTabButton({required VoidCallback onClose})` — circle `IconProvider.path`, `onPressed: onClose`
  - Keys: `Key('dock-tab-leading-chat')`, `Key('dock-tab-leading-chat-loading')`, `Key('dock-tab-leading-chat-unread')`, `Key('dock-tab-dirty-close')`

- [ ] **Step 1: Write the failing test**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tabbed_view/tabbed_view.dart';
import 'package:agent_fabric_client/dock/dock_ids.dart';
import 'package:agent_fabric_client/dock/dock_chat_tab_status.dart';
import 'package:agent_fabric_client/dock/dock_tab_icons.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';

void main() {
  testWidgets('core ids get distinct leading widgets', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (context) {
            final threads = dockTabLeadingForId(DockIds.threads)!(
              context,
              TabStatus.normal,
            );
            final files = dockTabLeadingForId(DockIds.files)!(
              context,
              TabStatus.normal,
            );
            final chat = dockTabLeadingForId(DockIds.chat)!(
              context,
              TabStatus.selected,
            );
            return Row(children: [threads!, files!, chat!]);
          },
        ),
      ),
    );
    expect(find.byKey(const Key('dock-tab-leading-threads')), findsOneWidget);
    expect(find.byKey(const Key('dock-tab-leading-files')), findsOneWidget);
    expect(find.byKey(const Key('dock-tab-leading-chat')), findsOneWidget);
  });

  testWidgets('chat loading and unread leadings', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (context) {
            final loading = dockTabLeadingForId(
              DockIds.chat,
              chatLead: DockChatTabLead.loading,
            )!(context, TabStatus.normal);
            final unread = dockTabLeadingForId(
              DockIds.chat,
              chatLead: DockChatTabLead.unread,
            )!(context, TabStatus.normal);
            return Row(children: [loading!, unread!]);
          },
        ),
      ),
    );
    expect(
      find.byKey(const Key('dock-tab-leading-chat-loading')),
      findsOneWidget,
    );
    expect(
      find.byKey(const Key('dock-tab-leading-chat-unread')),
      findsOneWidget,
    );
  });

  test('dirty close button uses key and invokes onClose', () {
    var closed = false;
    final button = dirtyCloseTabButton(onClose: () => closed = true);
    expect(button.toolTip, isNotNull);
    button.onPressed!();
    expect(closed, isTrue);
  });

  testWidgets('text editor app leading exists', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (context) {
            final w = dockTabLeadingForApp(WorkspaceAppId.textEditor)!(
              context,
              TabStatus.normal,
            );
            return w!;
          },
        ),
      ),
    );
    expect(find.byIcon(Icons.description_outlined), findsOneWidget);
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && flutter test test/dock/dock_tab_icons_test.dart`

Expected: FAIL — missing library

- [ ] **Step 3: Write minimal implementation**

Implement in `dock_tab_icons.dart`:

- Icon size ~14; color = `DefaultTextStyle` / `IconTheme` or `Theme.of(context).colorScheme.onSurface` with opacity `status == TabStatus.selected ? 1.0 : 0.45`.
- Cores: `Icons.forum_outlined` (threads), `Icons.folder_outlined` (files), `Icons.chat_bubble_outline` (chat plain).
- Chat loading: small `SizedBox` + `CircularProgressIndicator(strokeWidth: 2)` with key `dock-tab-leading-chat-loading`.
- Chat unread: `Container` 8×8 circle `colorScheme.primary` with key `dock-tab-leading-chat-unread`.
- Apps: textEditor → `Icons.description_outlined`; webPreview → `Icons.language`; imagePreview → `Icons.image_outlined`; audioPreview → `Icons.audiotrack`; download → `Icons.download_outlined`.
- Docs: if `DockIds.isDoc(id)`, parse app name after last `:` and map to `WorkspaceAppId.values.byName` (try/catch → description icon).
- `dirtyCloseTabButton`: `TabButton(icon: IconProvider.path(_filledCircle), onPressed: onClose, toolTip: 'Close', /* store key via toolTip or document that widget tests find IconProvider path */)`.

For the dirty key: wrap is not possible on `TabButton`. Prefer asserting `onPressed` in unit test; in widget/shell tests find by tooltip `'Close (unsaved)'` or add `toolTip: 'Close unsaved'`. Spec key `dock-tab-dirty-close` can be applied later in a tiny wrapper if docking ever allows widgets — for now use `toolTip: 'Close unsaved'` as the find handle and mention in test comments.

Circle path:

```dart
Path _filledCircle(Size size) {
  return Path()
    ..addOval(Rect.fromCircle(
      center: Offset(size.width / 2, size.height / 2),
      radius: size.shortestSide * 0.28,
    ));
}
```

- [ ] **Step 4: Run tests**

Run: `cd client && flutter test test/dock/dock_tab_icons_test.dart`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_tab_icons.dart client/test/dock/dock_tab_icons_test.dart
git commit -m "Add dock tab type icons and dirty-close button helper."
```

---

### Task 3: Hybrid+Soft `TabbedViewTheme`

**Files:**
- Create: `client/lib/dock/dock_tab_theme.dart`
- Test: `client/test/dock/dock_tab_theme_test.dart`
- Modify: `client/lib/app_shell.dart` (theme wire-up in Task 3 step so chrome is visible)

**Interfaces:**
- Consumes: `ColorScheme`, `TabbedViewThemeData` / related theme types from `tabbed_view`
- Produces: `TabbedViewThemeData buildDockTabTheme(ColorScheme scheme)`

- [ ] **Step 1: Write the failing test**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:agent_fabric_client/dock/dock_tab_theme.dart';

void main() {
  test('muted default text; selected uses onSurface and primary top border', () {
    const scheme = ColorScheme.dark();
    final theme = buildDockTabTheme(scheme);
    // Normal tabs use tab.textStyle (no normalStatus in tabbed_view 1.18).
    expect(theme.tab.textStyle?.color?.a, lessThan(0.5));
    expect(theme.tab.selectedStatus.fontColor, scheme.onSurface);
    expect(theme.tabsArea.color, isNotNull);
    final dec = theme.tab.selectedStatus.decoration as BoxDecoration?;
    expect(dec?.border?.top.color, scheme.primary);
    expect(dec?.border?.top.width, greaterThanOrEqualTo(1.5));
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && flutter test test/dock/dock_tab_theme_test.dart`

Expected: FAIL

- [ ] **Step 3: Implement `buildDockTabTheme`**

Match Hybrid+Soft from the spec:

- `tabsArea.color` = slightly elevated surface (e.g. `scheme.surfaceContainerHighest` or `surface.withValues(alpha: …)` blended).
- `tabsArea.middleGap` = 0 or 1; light vertical separation via tab decorations / gap bottom border `scheme.outlineVariant` at low width.
- Normal tab: muted `textStyle` color `onSurface.withValues(alpha: 0.45)`, transparent/minimal decoration.
- Selected: fill = `scheme.surface` (same as content), `fontColor` = `onSurface`, `Border(top: BorderSide(color: scheme.primary, width: 2))`, no heavy full outline.
- Highlighted (hover): slightly brighter than normal, still muted vs selected.
- Content area: `scheme.surface`, no heavy chrome clash with `MultiSplitView` dividers.
- Close icon colors: muted for normal, stronger on selected.

Read `TabThemeData` / `TabStatusThemeData` in the pub-cache while implementing so field names match exactly.

- [ ] **Step 4: Wire into `AppShell`**

Replace the sparse `TabbedViewThemeData(menu: …)` with:

```dart
child: TabbedViewTheme(
  data: buildDockTabTheme(Theme.of(context).colorScheme).copyWith(
    // preserve menu divider if still needed
  ),
  child: Docking(...),
),
```

Merge existing `menu.dividerThickness` / `dividerColor` into `buildDockTabTheme` or copy after build so split menu chrome stays.

- [ ] **Step 5: Run tests**

Run:

```bash
cd client && flutter test test/dock/dock_tab_theme_test.dart test/app_shell_test.dart
```

Expected: PASS (app_shell still green)

- [ ] **Step 6: Commit**

```bash
git add client/lib/dock/dock_tab_theme.dart client/test/dock/dock_tab_theme_test.dart client/lib/app_shell.dart
git commit -m "Theme dock tabs with Hybrid+Soft active contrast."
```

---

### Task 4: Attach leadings in `DockLayoutController` + chat/dirty mutators

**Files:**
- Modify: `client/lib/dock/dock_layout_controller.dart`
- Modify: `client/test/dock/dock_layout_controller_test.dart`

**Interfaces:**
- Consumes: `dockTabLeadingForId`, `dockTabLeadingForApp`, `dirtyCloseTabButton`, `DockChatTabLead`
- Produces:
  - Cores/docs created with `leading:` set
  - `void setChatTabLead(DockChatTabLead lead)` — finds chat item, sets `leading`, `layout.rebuild()`
  - `void setDocumentDirtyClose(String dockId, {required bool dirty, required VoidCallback onClose})` — sets `closable` / `buttons`, `layout.rebuild()`

- [ ] **Step 1: Extend unit tests**

Add to `dock_layout_controller_test.dart`:

```dart
test('default cores have leading builders', () {
  final c = DockLayoutController();
  c.resetToDefault(widgets: _stubWidgets());
  for (final id in [DockIds.threads, DockIds.files, DockIds.chat]) {
    final item = c.layout.findDockingItem(id)!;
    expect(item.leading, isNotNull, reason: id);
  }
});

test('openDocument attaches leading', () {
  final c = DockLayoutController();
  c.resetToDefault(widgets: _stubWidgets());
  c.openDocument(
    view: const OpenView(
      viewId: 'v1',
      path: 'a.txt',
      appId: WorkspaceAppId.textEditor,
    ),
    child: const SizedBox(),
  );
  final id = DockIds.doc('a.txt', WorkspaceAppId.textEditor);
  expect(c.layout.findDockingItem(id)!.leading, isNotNull);
});

test('setChatTabLead updates leading and stays findable', () {
  final c = DockLayoutController();
  c.resetToDefault(widgets: _stubWidgets());
  c.setChatTabLead(DockChatTabLead.unread);
  // leading is a new closure — just ensure no throw and item exists
  expect(c.layout.findDockingItem(DockIds.chat)!.leading, isNotNull);
});

test('setDocumentDirtyClose toggles closable', () {
  final c = DockLayoutController();
  c.resetToDefault(widgets: _stubWidgets());
  c.openDocument(
    view: const OpenView(
      viewId: 'v1',
      path: 'a.txt',
      appId: WorkspaceAppId.textEditor,
    ),
    child: const SizedBox(),
  );
  final id = DockIds.doc('a.txt', WorkspaceAppId.textEditor);
  c.setDocumentDirtyClose(id, dirty: true, onClose: () {});
  final item = c.layout.findDockingItem(id)!;
  expect(item.closable, isFalse);
  expect(item.buttons, isNotEmpty);
  c.setDocumentDirtyClose(id, dirty: false, onClose: () {});
  expect(c.layout.findDockingItem(id)!.closable, isTrue);
});
```

Use existing `_stubWidgets` helpers already in the test file.

- [ ] **Step 2: Run tests — expect FAIL**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

Expected: FAIL on new assertions / missing methods

- [ ] **Step 3: Implement**

In `_core(...)`:

```dart
leading: dockTabLeadingForId(id),
name: _coreTitle(id), // optional: 'Threads' / 'Files' / 'Chat' — keep lowercase ids if tests assert names; prefer nicer titles only if no test breaks
```

In `openDocument` when constructing `DockingItem`:

```dart
leading: dockTabLeadingForApp(view.appId),
```

In `_ShellLayoutCodec.buildDockingItem`, set `leading: dockTabLeadingForId(id)` the same way.

```dart
void setChatTabLead(DockChatTabLead lead) {
  final item = layout.findDockingItem(DockIds.chat);
  if (item == null) return;
  item.leading = dockTabLeadingForId(DockIds.chat, chatLead: lead);
  layout.rebuild();
}

void setDocumentDirtyClose(
  String dockId, {
  required bool dirty,
  required VoidCallback onClose,
}) {
  final item = layout.findDockingItem(dockId);
  if (item == null) return;
  if (dirty) {
    item.closable = false;
    item.buttons = [dirtyCloseTabButton(onClose: onClose)];
  } else {
    item.closable = true;
    item.buttons = const [];
  }
  layout.rebuild();
}
```

**Note:** `DockingItem.buttons` is assigned an unmodifiable list in the constructor; check if the field is mutable (`List<TabButton>? buttons` is a public field in docking 1.16.2 — reassignment works). If the setter is missing, assign `item.buttons = [...]` only if the field is not final — it is not final.

**Close path for dirty button:** `onClose` from AppShell should invoke the same path as the package close (interceptor → confirm). Prefer:

```dart
onClose: () {
  // Prefer layout API used by tab close — e.g. remove via interceptor
}
```

In AppShell (Task 5), pass a callback that calls `_interceptItemClose` / `_confirmDirtyClose` / `layout.removeItem` consistently with the package’s close button. For unit tests, empty `onClose` is enough.

- [ ] **Step 4: Run tests — expect PASS**

Run: `cd client && flutter test test/dock/dock_layout_controller_test.dart`

- [ ] **Step 5: Commit**

```bash
git add client/lib/dock/dock_layout_controller.dart client/test/dock/dock_layout_controller_test.dart
git commit -m "Attach dock tab leadings and chat/dirty mutators."
```

---

### Task 5: Wire chat binder and dirty listeners in `AppShell`

**Files:**
- Modify: `client/lib/app_shell.dart`
- Modify: `client/test/app_shell_test.dart` (add focused tests where practical)

**Interfaces:**
- Consumes: `DockChatTabUnread`, `resolveDockChatTabLead`, `DockLayoutController.setChatTabLead` / `setDocumentDirtyClose`, `ChatController.sending`, `focusedItemId`
- Produces: live tab chrome in the running shell

- [ ] **Step 1: Write / extend widget tests**

Add tests that drive controller state without needing full ACP if possible:

```dart
testWidgets('chat tab leading goes loading when sending and unfocused', (
  tester,
) async {
  // Pump AppShell with a Controllable ChatController fake or real controller
  // where sending can be set; focus files via dock selection / ensureCore.
  // Prefer finding Key('dock-tab-leading-chat-loading').
});
```

If a full AppShell test is too heavy, add a focused test harness widget in `test/dock/dock_chat_tab_binder_test.dart` that:

1. Builds `DockLayoutController` + fake `ChangeNotifier` with `bool sending`
2. Runs the same binder function extracted as `void syncChatDockTab({...})` in `dock_chat_tab_status.dart` or `app_shell.dart` top-level for testability

**Preferred:** extract:

```dart
void syncChatDockTabStatus({
  required DockLayoutController dock,
  required DockChatTabUnread unread,
  required bool sending,
  required bool wasSending,
}) {
  final chatFocused = dock.focusedItemId == DockIds.chat;
  if (chatFocused) unread.clear();
  if (wasSending && !sending) {
    unread.markIfUnfocused(chatFocused: chatFocused);
  }
  dock.setChatTabLead(
    resolveDockChatTabLead(
      chatFocused: chatFocused,
      sending: sending,
      unread: unread.value,
    ),
  );
}
```

Unit-test `syncChatDockTabStatus` in `dock_chat_tab_status_test.dart` (extend Task 1 file or add cases here before wiring).

Dirty listener sketch in AppShell `initState`:

```dart
_workspace.addListener(_syncDirtyDockTabs);
// and/or per-document listeners when docs open
```

`_syncDirtyDockTabs`:

```dart
void _syncDirtyDockTabs() {
  for (final view in _workspace.openViews) {
    final id = DockIds.doc(view.path, view.appId);
    final doc = _workspace.documentFor(view.path);
    final dirty = doc?.isDirty ?? false;
    _dock.setDocumentDirtyClose(
      id,
      dirty: dirty,
      onClose: () {
        final item = _dock.layout.findDockingItem(id);
        if (item == null) return;
        if (!_interceptItemClose(item)) return;
        _dock.layout.removeItem(item: item);
        // onItemClose path may already run — mirror package close behavior
      },
    );
  }
}
```

Study how `Docking` invokes `onItemClose` after a successful close so the dirty button does not double-remove. Prefer calling the same internal path the close icon uses (trigger `item` close via layout API if documented). If unsure, implement `onClose` to only call `_interceptItemClose` + `confirmDirtyViewClose` + `workspace.closeView` like the existing close handler.

Also update `_onItemSelection` to call `syncChatDockTabStatus` after updating `focusedItemId`.

Listen to `widget.controller` (`ChatController`) for `sending` transitions: keep `bool _wasSending` on state.

- [ ] **Step 2: Run failing tests / implement binder**

- [ ] **Step 3: Run full client dock + shell tests**

```bash
cd client && flutter test test/dock test/app_shell_test.dart
```

Expected: PASS

- [ ] **Step 4: Manual check (document in commit body if useful)**

Run the app, open Threads|Files|Chat|editor: confirm muted vs white active, chat spinner when switching away mid-turn, unread dot when turn completes, dirty circle when editing.

- [ ] **Step 5: Commit**

```bash
git add client/lib/app_shell.dart client/lib/dock/dock_chat_tab_status.dart client/test/app_shell_test.dart client/test/dock/dock_chat_tab_status_test.dart
git commit -m "Wire chat and dirty status into dock tab chrome."
```

---

### Task 6: Plan/spec polish gate

**Files:** none required (verification only)

- [ ] **Step 1: Spec coverage sweep**

Confirm each Decisions row in the spec is implemented or explicitly deferred with the package hover limitation noted in the dirty helper dartdoc.

- [ ] **Step 2: Run broader regression**

```bash
cd client && flutter test test/dock test/app_shell_test.dart test/workspace/workspace_pane_test.dart
```

Expected: PASS

- [ ] **Step 3: Commit only if Step 1 produced doc comment / test fixes**

```bash
git add -u client/
git commit -m "Tighten dock tab chrome coverage after verification."
```

Skip empty commit if nothing changed.

---

## Self-review (author)

1. **Spec coverage:** Theme Hybrid+Soft → Task 3; type icons → Tasks 2+4; dirty circle-close → Tasks 2+4+5; chat loading/unread/clear-on-focus → Tasks 1+5; no transport work → honored; package hover glyph limit → documented in Architecture + Task 2.
2. **Placeholders:** None intentional; imports use `package:agent_fabric_client/...`.
3. **Types:** `DockChatTabLead`, `resolveDockChatTabLead`, `DockChatTabUnread`, `setChatTabLead`, `setDocumentDirtyClose`, `syncChatDockTabStatus` used consistently across tasks.
