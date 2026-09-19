# Answer Fill on Assistant Messages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Paint assistant message bodies with `ChatColors.answer.fill` so Appearance Answer overrides affect the live transcript.

**Architecture:** In `_MessageProse`, wrap only `MessageText` in a padded rounded `Container` keyed for tests, using `chat.answer.fill`. Caption, Copy, and Stats stay outside as siblings.

**Tech Stack:** Flutter (Nix). Work from `/home/tryy3/src/agent-fabric`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart --name "answer fill"'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-answer-fill-on-messages-design.md`](../specs/2026-09-19-answer-fill-on-messages-design.md).
- Soft fill behind **message text only** — caption, Copy, Stats outside.
- Color: `chat.answer.fill` (no left bar / `answer.bar` in transcript).
- Padding ~12, radius 8 (align with user bubble).
- Streaming `…` uses the same fill.
- No Appearance UI changes; no thinking/tool/user chrome changes.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/agent_bubble.dart` | Wrap `MessageText` in answer fill container |
| `client/test/chat/agent_bubble_test.dart` | Assert fill color; Copy outside fill |

---

### Task 1: Answer fill on `_MessageProse`

**Files:**
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`

**Interfaces:**
- Consumes: `Theme.of(context).extension<ChatColors>()!.answer.fill`
- Produces: keyed fill wrapper around message body

- [ ] **Step 1: Write failing widget tests**

Append to `agent_bubble_test.dart`:

```dart
testWidgets('message body uses answer.fill; copy stays outside', (tester) async {
  final light = ChatColors.light();
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('pretty'),
          bubble: const ChatBubble(
            kind: ChatBubbleKind.message,
            text: 'hello answer',
            model: 'm',
          ),
        ),
      ),
    ),
  );

  final fill = tester.widget<Container>(
    find.byKey(const Key('answer-fill')),
  );
  final decoration = fill.decoration! as BoxDecoration;
  expect(decoration.color, light.answer.fill);
  expect(decoration.borderRadius, BorderRadius.circular(8));
  expect(find.descendant(
    of: find.byKey(const Key('answer-fill')),
    matching: find.text('hello answer'),
  ), findsOneWidget);
  expect(find.descendant(
    of: find.byKey(const Key('answer-fill')),
    matching: find.byKey(const Key('copy-message')),
  ), findsNothing);
  expect(find.byKey(const Key('copy-message')), findsOneWidget);
});

testWidgets('empty streaming message still uses answer.fill', (tester) async {
  final light = ChatColors.light();
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: const Scaffold(
        body: AgentBubble(
          viewMode: ViewMode(
            id: 'pretty',
            label: 'Pretty',
            description: '',
            markdownRender: true,
            thinkingVisibility: VisibilityMode.collapsed,
            toolVisibility: VisibilityMode.collapsed,
            toolIO: ToolIOMode.both,
          ),
          bubble: ChatBubble(kind: ChatBubbleKind.message, text: ''),
        ),
      ),
    ),
  );
  final fill = tester.widget<Container>(
    find.byKey(const Key('answer-fill')),
  );
  expect((fill.decoration! as BoxDecoration).color, light.answer.fill);
  expect(find.descendant(
    of: find.byKey(const Key('answer-fill')),
    matching: find.text('…'),
  ), findsOneWidget);
});
```

Import `ChatColors` if not already imported (file already imports it).

- [ ] **Step 2: Run tests — expect FAIL**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart --name "answer fill|streaming message still uses"'`

Expected: FAIL (`answer-fill` key not found)

- [ ] **Step 3: Implement fill wrapper**

In `_MessageProse.build`, replace the bare `MessageText(...)` with:

```dart
Container(
  key: const Key('answer-fill'),
  width: double.infinity,
  padding: const EdgeInsets.all(12),
  decoration: BoxDecoration(
    color: chat.answer.fill,
    borderRadius: BorderRadius.circular(8),
  ),
  child: MessageText(
    text: bubble.text.isEmpty ? '…' : bubble.text,
    markdown: markdown,
  ),
),
```

Leave caption/Copy row and Stats block unchanged below.

- [ ] **Step 4: Run tests — expect PASS**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart --name "answer fill|streaming message still uses"'`

Also run any existing message/copy tests that still apply:
`nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart --name "copy|Stats|message"'`

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/agent_bubble.dart client/test/chat/agent_bubble_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): paint assistant message bodies with answer fill

EOF
)"
```

---

## Spec coverage

| Requirement | Task |
| --- | --- |
| Soft fill on message text | Task 1 |
| Caption/Copy/Stats outside | Task 1 |
| Streaming `…` filled | Task 1 |
| Appearance token `answer.fill` | Task 1 |
| No bar / no settings UI | Omitted by design |
