# Chat Scrollbar Stability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop zero-height `stats` bubbles from poisoning `ListView.builder` scroll-extent estimates so the scrollbar thumb is more stable on finished threads.

**Architecture:** Build the message list from visible bubbles only (`kind != stats`). Resolve the Stats footer by looking up the message in the full `messages` list and reading the following `stats` bubble. Keep full-pane scroll, `_contentColumn`, composer, and auto-scroll unchanged.

**Tech Stack:** Flutter (Nix shell). Work from `/home/tryy3/src/agent-fabric` on branch `fix/chat-scroll-hit-target`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-chat-scrollbar-stability-design.md`](../specs/2026-09-19-chat-scrollbar-stability-design.md).
- Same branch / PR as scroll hit-target (`fix/chat-scroll-hit-target`).
- Do not remove `stats` from the controller model — only from list children.
- Accept remaining thumb motion during streaming / expand-collapse.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/chat_screen.dart` | Visible-message list; stats look-ahead from full `messages` |
| `client/test/chat/chat_screen_test.dart` | Assert list child count excludes stats; Stats action still works |

**Interfaces this plan locks:**

```dart
// In ChatScreen build (message list branch):
final visible = [
  for (final m in c.messages)
    if (m.kind != ChatBubbleKind.stats) m,
];

// Stats for a message bubble m:
ChatBubble? statsFor(ChatBubble message) {
  final i = c.messages.indexOf(message);
  if (i < 0 || i + 1 >= c.messages.length) return null;
  final next = c.messages[i + 1];
  return next.kind == ChatBubbleKind.stats ? next : null;
}
```

Prefer `indexWhere` by identity/`indexOf` on the same list instance (bubbles are the same objects). If `indexOf` is fragile with duplicates, walk with an index loop over `c.messages` once to pair message→stats, or look ahead while iterating visible only by scanning full list — simplest: when building visible item `m` of kind message, find index in `c.messages`.

---

### Task 1: Filter stats out of the message ListView

**Files:**
- Modify: `client/lib/chat/chat_screen.dart` (message `ListView.builder`)
- Modify: `client/test/chat/chat_screen_test.dart`
- Test: `client/test/chat/chat_screen_test.dart`

**Interfaces:**
- Consumes: `c.messages`, existing `AgentBubble(stats:)`, `Key('message-list')`
- Produces: `itemCount == visible.length` with no `stats` children

- [ ] **Step 1: Write the failing list-composition test**

Add after the existing stats test in `chat_screen_test.dart`:

```dart
  testWidgets('message list omits stats bubbles as children', (tester) async {
    final conn = FakeConn()
      ..chunksToEmit = ['hello']
      ..usageToEmit = const TurnUsage(elapsedMs: 50, deltas: 1);
    final c = ChatController(
      session: conn,
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

    await tester.enterText(find.byKey(const Key('composer-input')), 'hi');
    await tester.tap(find.byKey(const Key('composer-send')));
    await tester.pumpAndSettle();

    // Controller still has a stats bubble in the model.
    expect(
      c.messages.where((m) => m.kind == ChatBubbleKind.stats),
      isNotEmpty,
    );

    final list = tester.widget<ListView>(find.byKey(const Key('message-list')));
    final delegate = list.childrenDelegate as SliverChildBuilderDelegate;
    expect(delegate.estimatedChildCount, c.messages.length - 1);
    expect(
      delegate.estimatedChildCount,
      c.messages.where((m) => m.kind != ChatBubbleKind.stats).length,
    );

    // Stats footer still wired.
    expect(find.byKey(const Key('stats-action')), findsOneWidget);
  });
```

(If `estimatedChildCount` is null on this Flutter version, assert `itemCount` via the builder’s visible length another way: e.g. expose `Key('message-list')` and compare `c.messages.where(...).length` to the number of non-shrink list element finders — prefer `estimatedChildCount` / `childCount` on the delegate.)

- [ ] **Step 2: Run test to verify it fails**

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart --name "message list omits stats"'
```

Expected: FAIL — `estimatedChildCount` (or equivalent) still equals full `messages.length` (includes stats).

- [ ] **Step 3: Implement visible-list filtering**

In `chat_screen.dart`, replace the `ListView.builder` message branch body so that before the builder:

```dart
final visible = [
  for (final m in c.messages)
    if (m.kind != ChatBubbleKind.stats) m,
];
```

Then:

```dart
ListView.builder(
  key: const Key('message-list'),
  controller: _scroll,
  padding: const EdgeInsets.all(16),
  itemCount: visible.length,
  itemBuilder: (context, index) {
    final m = visible[index];
    if (m.kind == ChatBubbleKind.user) {
      return _contentColumn(/* unchanged user bubble */);
    }
    ChatBubble? stats;
    if (m.kind == ChatBubbleKind.message) {
      final fullIndex = c.messages.indexOf(m);
      if (fullIndex >= 0 &&
          fullIndex + 1 < c.messages.length &&
          c.messages[fullIndex + 1].kind == ChatBubbleKind.stats) {
        stats = c.messages[fullIndex + 1];
      }
    }
    return _contentColumn(
      width: width,
      child: AgentBubble(
        bubble: m,
        viewMode: mode,
        stats: stats,
      ),
    );
  },
),
```

Remove the `if (m.kind == ChatBubbleKind.stats) return SizedBox.shrink()` branch entirely.

- [ ] **Step 4: Run chat screen tests**

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart'
```

Expected: all PASS, including the new test and `stats bubble is not shown as a card; Stats action is on message`.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_screen.dart client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
fix(client): omit stats slots from chat list scroll metrics

EOF
)"
```

Then push to update PR #29:

```bash
git push
```

---

## Spec coverage (self-review)

| Spec requirement | Task |
| --- | --- |
| Filter stats from ListView children | Task 1 |
| Stats footer still from full messages | Task 1 (`indexOf` look-ahead) |
| Keep scroll gutters / content column / auto-scroll | Task 1 (untouched) |
| Tests for omit + Stats action | Task 1 |
| Accept streaming thumb motion | — (no change) |
