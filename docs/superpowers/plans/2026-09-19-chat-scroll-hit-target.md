# Chat Scroll Hit-Target Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the chat message list scrollable across the full chat body (including left/right gutters) while keeping message content and the composer capped at `contentWidth`.

**Architecture:** Move the `ConstrainedBox(maxWidth: contentWidth)` from wrapping the `ListView` to wrapping each list row’s content (centered). The `ListView` fills the `Expanded` pane so wheel/trackpad/drag hit-testing covers the gutters. Composer layout stays unchanged.

**Tech Stack:** Flutter (Nix shell). Work from `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix with `nix develop /home/tryy3/src/agent-fabric -c` and run from `client/`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-chat-scroll-hit-target-design.md`](../specs/2026-09-19-chat-scroll-hit-target-design.md).
- Scope is **scrolling only** from issue #25 — no selection, copy, markdown, or failed-tool styling.
- Keep `ChatDisplaySettings.contentWidth` (560–1200) as the reading measure for messages and composer.
- Composer remains outside the list, centered + constrained.
- Preserve `_scroll` / `_scrollToEnd` auto-scroll while sending.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/chat_screen.dart` | Full-width message `ListView`; per-row centered content column helper |
| `client/test/chat/chat_screen_test.dart` | Assert content width on rows; assert list is wider than `contentWidth` on a wide surface |

**Interfaces this plan locks:**

```dart
// Private helper on _ChatScreenState (or equivalent local function/widget)
Widget _contentColumn({required double width, required Widget child}) {
  return Align(
    alignment: Alignment.topCenter,
    child: ConstrainedBox(
      constraints: BoxConstraints(maxWidth: width),
      child: child,
    ),
  );
}

// Optional test key on the message ListView
const Key('message-list')
```

---

### Task 1: Full-pane message list scroll surface

**Files:**
- Modify: `client/test/chat/chat_screen_test.dart` (extend content-width test; add wide-pane list width test)
- Modify: `client/lib/chat/chat_screen.dart` (full-width `ListView` + per-row `_contentColumn`)
- Test: `client/test/chat/chat_screen_test.dart`

**Interfaces:**
- Consumes: `ChatDisplaySettings.contentWidth`, existing `ListView` / composer layout
- Produces: `Key('message-list')` on the transcript `ListView`; `_contentColumn` wrapping user and agent rows

- [ ] **Step 1: Write the failing wide-pane scroll-surface test**

In `client/test/chat/chat_screen_test.dart`, add this test after `message list and composer respect content width`:

```dart
  testWidgets('message list fills chat pane wider than content width', (
    tester,
  ) async {
    await displaySettings.setContentWidth(560);
    final conn = FakeConn();
    final c = ChatController(
      session: conn,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');

    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: ChatScreen(controller: c, displaySettings: displaySettings),
      ),
    );
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('composer-input')), 'Hello');
    await tester.tap(find.byKey(const Key('composer-send')));
    await tester.pumpAndSettle();

    final listSize = tester.getSize(find.byKey(const Key('message-list')));
    expect(listSize.width, greaterThan(560));

    final messageBox = tester.widget<ConstrainedBox>(
      find
          .ancestor(
            of: find.text('Hello'),
            matching: find.byType(ConstrainedBox),
          )
          .first,
    );
    expect(messageBox.constraints.maxWidth, 560);
  });
```

Also update the existing test `message list and composer respect content width` so the message assertion still targets the content column (rename the local from `listBox` to `messageBox` for clarity — same finder is fine after the layout change).

- [ ] **Step 2: Run the new test to verify it fails**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart --name "message list fills chat pane"'
```

Expected: FAIL — `find.byKey(const Key('message-list'))` finds nothing (key not present yet), or if you temporarily use `find.byType(ListView)` first, FAIL because list width equals 560.

- [ ] **Step 3: Implement full-width list + per-row content column**

In `client/lib/chat/chat_screen.dart`, on `_ChatScreenState`, add:

```dart
  Widget _contentColumn({required double width, required Widget child}) {
    return Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: width),
        child: child,
      ),
    );
  }
```

Replace the message-list branch inside the `Expanded` (the `Align` → `ConstrainedBox` → `ListView.builder` tree) with a full-width list. Keep empty/offline placeholders unchanged. Structure:

```dart
                    : ListView.builder(
                        key: const Key('message-list'),
                        controller: _scroll,
                        padding: const EdgeInsets.all(16),
                        itemCount: c.messages.length,
                        itemBuilder: (context, index) {
                          final m = c.messages[index];
                          if (m.kind == ChatBubbleKind.stats) {
                            return const SizedBox.shrink();
                          }
                          if (m.kind == ChatBubbleKind.user) {
                            return _contentColumn(
                              width: width,
                              child: Align(
                                alignment: Alignment.centerRight,
                                child: Container(
                                  margin: const EdgeInsets.symmetric(
                                    vertical: 8,
                                  ),
                                  padding: const EdgeInsets.all(12),
                                  decoration: BoxDecoration(
                                    color: Theme.of(context)
                                        .extension<ChatColors>()!
                                        .user
                                        .fill,
                                    borderRadius: BorderRadius.circular(8),
                                  ),
                                  child: MessageText(
                                    text: m.text,
                                    markdown: mode.markdownRender,
                                  ),
                                ),
                              ),
                            );
                          }
                          ChatBubble? stats;
                          if (m.kind == ChatBubbleKind.message &&
                              index + 1 < c.messages.length &&
                              c.messages[index + 1].kind ==
                                  ChatBubbleKind.stats) {
                            stats = c.messages[index + 1];
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

Leave the composer `SafeArea` → `Align` → `ConstrainedBox` block exactly as it is today.

Do **not** change `_scrollToEnd` or the `ScrollController`.

- [ ] **Step 4: Run chat screen tests to verify they pass**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart'
```

Expected: all tests PASS, including both content-width tests.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_screen.dart client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
fix(client): scroll chat body outside content column

EOF
)"
```

---

## Spec coverage (self-review)

| Spec requirement | Task |
| --- | --- |
| Full chat-body scroll hit-target (gutters) | Task 1 |
| Keep message column at `contentWidth` | Task 1 (`_contentColumn`) |
| Composer stays constrained | Task 1 (unchanged composer tree) |
| Auto-scroll while sending preserved | Task 1 (no `_scroll` changes) |
| Stats `SizedBox.shrink` unwrapped | Task 1 |
| Empty/offline placeholders unchanged | Task 1 |
| Widget tests for width + wide list | Task 1 |
| Other #25 items out of scope | — (not in plan) |
