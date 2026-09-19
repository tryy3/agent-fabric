# Chat SuperListView Scrollbar Stability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the chat transcript `ListView.builder` with `SuperListView.builder` so measured row extents are retained and the scrollbar thumb stays stable after a full first pass of a static thread.

**Architecture:** Add `super_sliver_list` and swap the message list widget in `chat_screen.dart`. Keep the existing `ScrollController`, visible-message filtering (no stats slots), `_contentColumn`, composer, and `_scrollToEnd` (`jumpTo(maxScrollExtent)`). No `ListController` in v1.

**Tech Stack:** Flutter (Nix shell). Branch `fix/chat-scroll-hit-target` at `/home/tryy3/src/agent-fabric`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-chat-super-list-scrollbar-design.md`](../specs/2026-09-19-chat-super-list-scrollbar-design.md).
- Same branch / PR #29 as other #25 scroll fixes.
- Keep stats filtering and full-pane / content-column layout.
- Do not add DIY height cache or `ListController` unless required to compile/run.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD where tests must change; dependency add is a prerequisite step.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/pubspec.yaml` / `client/pubspec.lock` | Add `super_sliver_list: ^0.4.1` |
| `client/lib/chat/chat_screen.dart` | `SuperListView.builder` instead of `ListView.builder` |
| `client/test/chat/chat_screen_test.dart` | Finders/casts that assumed `ListView` |

**Interfaces this plan locks:**

```dart
import 'package:super_sliver_list/super_sliver_list.dart';

SuperListView.builder(
  key: const Key('message-list'),
  controller: _scroll,
  padding: const EdgeInsets.all(16),
  itemCount: visible.length,
  itemBuilder: (context, index) { /* unchanged */ },
);
```

`SuperListView` extends `BoxScrollView`, **not** `ListView` — do not cast the keyed widget to `ListView`.

---

### Task 1: Depend on `super_sliver_list` and swap the message list

**Files:**
- Modify: `client/pubspec.yaml` (add dependency)
- Modify: `client/pubspec.lock` (via `flutter pub get`)
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/test/chat/chat_screen_test.dart`
- Test: `client/test/chat/chat_screen_test.dart`

**Interfaces:**
- Consumes: existing visible list + `_scroll` + `_contentColumn`
- Produces: `SuperListView.builder` under `Key('message-list')`

- [ ] **Step 1: Update the failing ListView cast test**

In `message list omits stats bubbles as children`, replace:

```dart
final list = tester.widget<ListView>(find.byKey(const Key('message-list')));
final delegate = list.childrenDelegate as SliverChildBuilderDelegate;
expect(delegate.estimatedChildCount, c.messages.length - 1);
expect(
  delegate.estimatedChildCount,
  c.messages.where((m) => m.kind != ChatBubbleKind.stats).length,
);
```

with a check that does not require `ListView`:

```dart
expect(find.byKey(const Key('message-list')), findsOneWidget);
expect(find.byType(SuperListView), findsOneWidget);
expect(
  c.messages.where((m) => m.kind != ChatBubbleKind.stats).length,
  c.messages.length -
      c.messages.where((m) => m.kind == ChatBubbleKind.stats).length,
);
// Visible rows only: user + message (+ thought/tool if present); stats still in model.
expect(
  c.messages.where((m) => m.kind == ChatBubbleKind.stats),
  isNotEmpty,
);
expect(find.byKey(const Key('stats-action')), findsOneWidget);
```

Add import:

```dart
import 'package:super_sliver_list/super_sliver_list.dart';
```

(Adjust if the public type name differs after `pub get` — use the widget type exported by the package for `SuperListView.builder`.)

Also add a focused assertion that the message list is a `SuperListView`:

```dart
  testWidgets('message list uses SuperListView', (tester) async {
    final c = ChatController(
      session: FakeConn(),
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('message-list')), findsOneWidget);
    expect(
      tester.widget(find.byKey(const Key('message-list'))).runtimeType.toString(),
      contains('SuperListView'),
    );
  });
```

(Prefer `isA<SuperListView<dynamic>>()` or the concrete generic if analyzer-friendly.)

- [ ] **Step 2: Run the SuperListView test — expect RED**

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart --name "message list uses SuperListView"'
```

Expected: FAIL (still `ListView`, or type mismatch) **or** compile failure until the package is added — if the import fails, add the dependency next then re-run to get a clean RED on the type assertion.

- [ ] **Step 3: Add the dependency**

In `client/pubspec.yaml` under `dependencies:`:

```yaml
  super_sliver_list: ^0.4.1
```

Then:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter pub get'
```

- [ ] **Step 4: Swap to SuperListView.builder**

In `chat_screen.dart`:

```dart
import 'package:super_sliver_list/super_sliver_list.dart';
```

Replace `ListView.builder(` with `SuperListView.builder(` — keep `key`, `controller`, `padding`, `itemCount`, `itemBuilder` identical. Do not introduce `ListController` unless the constructor requires it (it should be optional).

- [ ] **Step 5: Run full chat_screen tests**

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart'
```

Expected: all PASS. Fix any remaining `ListView` casts.

- [ ] **Step 6: Commit and push**

```bash
git add client/pubspec.yaml client/pubspec.lock client/lib/chat/chat_screen.dart client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
fix(client): use SuperListView for stable chat scroll metrics

EOF
)"
git push
```

(Only push if updating PR #29; skip push if the controller will handle it.)

---

## Spec coverage (self-review)

| Spec requirement | Task |
| --- | --- |
| Add `super_sliver_list` | Task 1 |
| `SuperListView.builder` swap | Task 1 |
| Keep ScrollController / auto-scroll | Task 1 |
| Keep stats filter + content column | Task 1 (untouched logic) |
| Update tests for non-ListView type | Task 1 |
| No ListController unless needed | Task 1 |
