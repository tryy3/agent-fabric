# Chat Copy Toast + Message Timestamps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Floating copy snackbar feedback, plus locale-aware timestamps next to Copy on user and assistant message footers.

**Architecture:** Soften `CopyAction` snackbars with `SnackBarBehavior.floating`. Add optional `ChatBubble.createdAt`, hydrate from `ThreadMessage.createdAt` / stamp live sends, and render a muted locale string via a small `formatMessageTimestamp` helper beside Copy.

**Tech Stack:** Flutter (Nix). Work from `/home/tryy3/src/agent-fabric`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/copy_action_test.dart'`. Prefer `MaterialLocalizations` (no new `intl` dependency).

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-chat-copy-toast-timestamps-design.md`](../specs/2026-09-19-chat-copy-toast-timestamps-design.md).
- Floating snackbar only — no custom overlay framework.
- Timestamps on **user** and **assistant message** footers only (not thinking/tool).
- Locale-aware formatter; no user setting UI this slice.
- Omit timestamp when `createdAt` is null.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/copy_action.dart` | Floating snackbar |
| `client/test/chat/copy_action_test.dart` | Assert floating behavior |
| `client/lib/chat/message_timestamp.dart` | `formatMessageTimestamp` |
| `client/test/chat/message_timestamp_test.dart` | Locale string unit/widget smoke |
| `client/lib/chat/chat_bubble.dart` | `createdAt` field + hydrate |
| `client/lib/chat/chat_controller.dart` | Stamp live user/message bubbles |
| `client/lib/chat/agent_bubble.dart` | Assistant caption row timestamp |
| `client/lib/chat/chat_screen.dart` | User footer timestamp |
| `client/test/chat/chat_bubble_test.dart` | Hydration includes `createdAt` |
| `client/test/chat/agent_bubble_test.dart` / `chat_screen_test.dart` | Visible timestamp + omit when null |

**Interfaces:**

```dart
String formatMessageTimestamp(BuildContext context, DateTime when);

// ChatBubble gains:
final DateTime? createdAt;
```

---

### Task 1: Floating copy snackbar

**Files:**
- Modify: `client/lib/chat/copy_action.dart`
- Modify: `client/test/chat/copy_action_test.dart`

**Interfaces:**
- Consumes: existing `CopyAction`
- Produces: floating `SnackBar` after successful copy

- [ ] **Step 1: Extend the copy snackbar test**

In `copy_action_test.dart`, after tapping and finding `'Copied'`, add:

```dart
final bar = tester.widget<SnackBar>(find.byType(SnackBar));
expect(bar.behavior, SnackBarBehavior.floating);
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/copy_action_test.dart'`

Expected: FAIL (`behavior` is null / not floating)

- [ ] **Step 3: Implement floating snackbar**

In `copy_action.dart`, replace the `showSnackBar` call with:

```dart
ScaffoldMessenger.of(context).showSnackBar(
  SnackBar(
    content: Text(snackbarMessage),
    behavior: SnackBarBehavior.floating,
    margin: const EdgeInsets.fromLTRB(16, 0, 16, 16),
    duration: const Duration(seconds: 2),
  ),
);
```

Keep empty early-return and non-null `onPressed`.

- [ ] **Step 4: Run tests — expect PASS**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/copy_action_test.dart'`

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/copy_action.dart client/test/chat/copy_action_test.dart
git commit -m "$(cat <<'EOF'
fix(chat): use floating snackbar for copy feedback

EOF
)"
```

---

### Task 2: `createdAt` + timestamp formatter

**Files:**
- Create: `client/lib/chat/message_timestamp.dart`
- Create: `client/test/chat/message_timestamp_test.dart`
- Modify: `client/lib/chat/chat_bubble.dart`
- Modify: `client/test/chat/chat_bubble_test.dart`
- Modify: `client/lib/chat/chat_controller.dart`

**Interfaces:**
- Produces: `formatMessageTimestamp`, `ChatBubble.createdAt`, hydration + live stamps

- [ ] **Step 1: Write failing tests**

Create `client/test/chat/message_timestamp_test.dart`:

```dart
import 'package:agent_fabric_client/chat/message_timestamp.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('formatMessageTimestamp joins locale date and time', (tester) async {
    late String formatted;
    await tester.pumpWidget(
      MaterialApp(
        locale: const Locale('en', 'US'),
        home: Builder(
          builder: (context) {
            formatted = formatMessageTimestamp(
              context,
              DateTime(2026, 9, 9, 10, 40),
            );
            return const SizedBox.shrink();
          },
        ),
      ),
    );
    expect(formatted, isNotEmpty);
    expect(formatted.toLowerCase(), contains('sep'));
    expect(formatted, contains('10:40'));
  });
}
```

In `chat_bubble_test.dart`, extend an existing user/assistant hydrate case to assert:

```dart
expect(bubbles.single.createdAt, created); // user
// and for assistant message bubble:
expect(
  bubbles.firstWhere((b) => b.kind == ChatBubbleKind.message).createdAt,
  created,
);
```

(Use the `created` / `createdAt` already present on the `ThreadMessage` fixture in that file.)

- [ ] **Step 2: Run tests — expect FAIL**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/message_timestamp_test.dart test/chat/chat_bubble_test.dart'`

Expected: FAIL (missing helper / `createdAt`)

- [ ] **Step 3: Implement**

`message_timestamp.dart`:

```dart
import 'package:material_ui/material_ui.dart';

String formatMessageTimestamp(BuildContext context, DateTime when) {
  final loc = MaterialLocalizations.of(context);
  final local = when.toLocal();
  final date = loc.formatMediumDate(local);
  final time = loc.formatTimeOfDay(TimeOfDay.fromDateTime(local));
  return '$date, $time';
}
```

`ChatBubble`: add `this.createdAt`, field, and `copyWith` / constructor plumbing (preserve existing `createdAt` when `copyWith` omits it).

`bubblesFromThreadMessage`:

```dart
// user:
ChatBubble(kind: ChatBubbleKind.user, text: message.content, createdAt: message.createdAt)
// message kind:
ChatBubble(..., createdAt: message.createdAt)
```

`chat_controller.dart`:

- User send: `ChatBubble(kind: user, text: trimmed, createdAt: DateTime.now())`
- `_growOrAppend` when **adding** a new `ChatBubbleKind.message` (and only then): `createdAt: DateTime.now()`
- When updating an existing bubble via `copyWith`, do not clear `createdAt`

- [ ] **Step 4: Run tests — expect PASS**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/message_timestamp_test.dart test/chat/chat_bubble_test.dart test/chat/chat_controller_test.dart'`

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/message_timestamp.dart client/test/chat/message_timestamp_test.dart \
  client/lib/chat/chat_bubble.dart client/test/chat/chat_bubble_test.dart \
  client/lib/chat/chat_controller.dart
git commit -m "$(cat <<'EOF'
feat(chat): plumb message createdAt and locale timestamp helper

EOF
)"
```

---

### Task 3: Show timestamps beside Copy

**Files:**
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`
- Modify: `client/test/chat/chat_screen_test.dart`

**Interfaces:**
- Consumes: `formatMessageTimestamp`, `bubble.createdAt`
- Produces: muted timestamp text left of Copy on user + assistant footers

- [ ] **Step 1: Write failing widget tests**

Assistant (`agent_bubble_test.dart`):

```dart
testWidgets('message footer shows locale timestamp next to copy', (tester) async {
  final when = DateTime(2026, 9, 9, 10, 40);
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      locale: const Locale('en', 'US'),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('detailed'),
          bubble: ChatBubble(
            kind: ChatBubbleKind.message,
            text: 'hello answer',
            model: 'm1',
            createdAt: when,
          ),
        ),
      ),
    ),
  );
  expect(find.byKey(const Key('copy-message')), findsOneWidget);
  expect(find.textContaining('Sep'), findsOneWidget);
  expect(find.textContaining('10:40'), findsOneWidget);
});

testWidgets('message footer omits timestamp when createdAt is null', (tester) async {
  // same pump without createdAt — Sep/10:40 absent; copy still present
});
```

User (`chat_screen_test.dart`): seed a hydrated user message with known `createdAt` (or set on the bubble the test already uses) and assert timestamp text + `copy-user`. Prefer the existing thread-hydrate path if messages come from `ThreadMessage`; otherwise set `createdAt` on the controller’s user bubble after seed.

- [ ] **Step 2: Run tests — expect FAIL**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart test/chat/chat_screen_test.dart --name timestamp'`

- [ ] **Step 3: Wire UI**

Shared footer fragment (inline is fine):

```dart
Row(
  mainAxisSize: MainAxisSize.min,
  children: [
    if (bubble.createdAt case final when?)
      Padding(
        padding: const EdgeInsets.only(right: 4),
        child: Text(
          formatMessageTimestamp(context, when),
          style: Theme.of(context).textTheme.bodySmall?.copyWith(color: muted),
        ),
      ),
    CopyAction(...),
  ],
)
```

- **Assistant:** put this `Row` as the trailing part of the existing caption row (keep caption `Expanded` / `Spacer` on the left).
- **User:** replace lone `CopyAction` with the same pattern in the trailing footer `Column`.

- [ ] **Step 4: Run related suite**

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test \
  test/chat/copy_action_test.dart \
  test/chat/message_timestamp_test.dart \
  test/chat/chat_bubble_test.dart \
  test/chat/agent_bubble_test.dart \
  test/chat/chat_screen_test.dart'
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/agent_bubble.dart client/lib/chat/chat_screen.dart \
  client/test/chat/agent_bubble_test.dart client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): show locale timestamps next to message copy actions

EOF
)"
```

---

## Spec coverage

| Spec item | Task |
| --- | --- |
| Floating snackbar | 1 |
| `createdAt` plumb + live stamp | 2 |
| Locale formatter helper | 2 |
| User + assistant footer display | 3 |
| Omit when null | 3 |

## Self-review

- No TBD placeholders; MaterialLocalizations avoids new deps.
- `createdAt` preserved across `copyWith` / streaming appends.
- Thinking/tool intentionally untouched.
