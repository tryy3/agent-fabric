# Chat Part Bubbles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render the chat transcript as a flat stream of typed bubbles in arrival order (user, thinking, answer, stats) with accent-bar agent cards, without changing the control plane.

**Architecture:** `ChatController.messages` becomes `List<ChatBubble>`. Live ACP events grow or append the last bubble of that kind. Reload maps each catalog `ThreadMessage` to one or more bubbles (parts order, with a thought/content/usage fallback). `AgentBubble` is the shared left-bar card; the user row stays a right-aligned fill. Catalog HTTP, ACP, and Settings keys stay as they are.

**Tech Stack:** Flutter 3, existing `package:test` / `flutter_test`, Material `ExpansionTile`. Work from `/home/tryy3/src/agent-fabric/.worktrees/chat-transparency` on `feat/chat-transparency`. Flutter is not on default PATH: prefix commands with `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c` and run them from `client/`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-14-chat-part-bubbles-design.md`](../specs/2026-09-14-chat-part-bubbles-design.md).
- No tools, MCP, plans, `sent` parts, or permission UI.
- No extra DB rows; still one user row + one assistant row with `parts[]`.
- LLM hydrate stays `role` + `content` only (plane unchanged; no new Go tests).
- Caption `model · provider · tok/s` is a footer on the **answer** bubble; tok/s omitted when unknown; never hidden when Stats is hidden.
- Thinking/Stats settings remain collapsed / expanded / hidden (`chat.thinkingVisibility` / `chat.statsVisibility`). `hidden` omits that bubble; data still persisted.
- Thinking opens while `streamingThought` is true, then follows the setting. ExpansionTile `ValueKey` includes `streamingThought` and visibility mode, **not** thought text.
- New bubble when incoming kind ≠ last kind; same kind appends. Today’s path is thought → message → stats.
- Cancel / stream error / thinking-without-content still write **no** catalog rows (drop uncommitted bubbles).
- Do not commit `controlplane/data/`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/chat_bubble.dart` | `ChatBubbleKind`, `ChatBubble`, `bubblesFromThreadMessage`, `bubbleCaption` |
| `client/lib/chat/agent_bubble.dart` | Accent-bar card for thought / message / stats |
| `client/lib/chat/chat_controller.dart` | `List<ChatBubble> messages`; live grow/append; GET mapping |
| `client/lib/chat/chat_screen.dart` | ListView of user vs `AgentBubble`; auto-scroll while sending |
| `client/lib/chat/chat_message.dart` | Delete after controller/UI migrate |
| `client/lib/chat/assistant_turn.dart` | Delete after `AgentBubble` exists |
| `client/test/chat/chat_bubble_test.dart` | Mapping + caption unit tests |
| `client/test/chat/agent_bubble_test.dart` | Widget tests (replaces `assistant_turn_test.dart`) |
| `client/test/chat/chat_controller_test.dart` | Assert kinds/order instead of `ChatMessage.thought` |
| `client/test/chat/chat_screen_test.dart` | Thinking above answer while streaming |
| `client/test/settings/chat_tab_test.dart` | Hidden thinking uses `AgentBubble` |
| `README.md` | One-line: parts appear as bubbles in arrival order |

**Interfaces this plan adds** (later tasks consume these names exactly):

```dart
enum ChatBubbleKind { user, thought, message, stats }

class ChatBubble {
  const ChatBubble({
    required this.kind,
    this.text = '',
    this.model,
    this.providerName,
    this.predictedPerSecond,
    this.usage,
    this.stopReason,
    this.streamingThought = false,
  });
  final ChatBubbleKind kind;
  final String text;
  final String? model;
  final String? providerName;
  final double? predictedPerSecond;
  final TurnUsage? usage;
  final String? stopReason;
  final bool streamingThought;
  ChatBubble copyWith({...});
}

List<ChatBubble> bubblesFromThreadMessage(ThreadMessage message);
String bubbleCaption(ChatBubble bubble); // message kind only; empty otherwise

class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    this.thinkingMode = VisibilityMode.collapsed,
    this.statsMode = VisibilityMode.collapsed,
  });
  final ChatBubble bubble;
  final VisibilityMode thinkingMode;
  final VisibilityMode statsMode;
}
```

Accent bar colors (lock): thought `Color(0xFFD97706)`, message `Color(0xFF18181B)`, stats `Color(0xFF71717A)`.

---

### Task 1: ChatBubble model + thread mapping

**Files:**
- Create: `client/lib/chat/chat_bubble.dart`
- Create: `client/test/chat/chat_bubble_test.dart`
- Modify: none of the live controller yet

**Interfaces:**
- Consumes: `ThreadMessage` (`content`, `role`, `thought`, `model`, `providerName`, `stopReason`, `usage`) from `client/lib/catalog/models.dart`; `TurnUsage` from ACP
- Produces: `ChatBubbleKind`, `ChatBubble`, `bubblesFromThreadMessage`, `bubbleCaption`

- [ ] **Step 1: Write failing tests**

```dart
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final created = DateTime.utc(2026, 9, 14);

  test('user row maps to one user bubble', () {
    final bubbles = bubblesFromThreadMessage(
      ThreadMessage(
        id: 'm1',
        role: 'user',
        content: 'hi',
        position: 0,
        createdAt: created,
      ),
    );
    expect(bubbles.map((b) => b.kind).toList(), [ChatBubbleKind.user]);
    expect(bubbles.single.text, 'hi');
  });

  test('assistant thought then content then usage maps in that order', () {
    final bubbles = bubblesFromThreadMessage(
      ThreadMessage(
        id: 'm2',
        role: 'assistant',
        content: 'hello',
        position: 1,
        createdAt: created,
        thought: 'hmm',
        model: 'm1',
        providerName: 'Local',
        stopReason: 'end_turn',
        usage: const TurnUsage(predictedPerSecond: 35.5, deltas: 1),
      ),
    );
    expect(bubbles.map((b) => b.kind).toList(), [
      ChatBubbleKind.thought,
      ChatBubbleKind.message,
      ChatBubbleKind.stats,
    ]);
    expect(bubbles[0].text, 'hmm');
    expect(bubbles[1].text, 'hello');
    expect(bubbles[1].model, 'm1');
    expect(bubbles[1].providerName, 'Local');
    expect(bubbles[1].predictedPerSecond, 35.5);
    expect(bubbles[2].usage?.deltas, 1);
    expect(bubbles[2].stopReason, 'end_turn');
  });

  test('assistant content only is a single message bubble', () {
    final bubbles = bubblesFromThreadMessage(
      ThreadMessage(
        id: 'm3',
        role: 'assistant',
        content: 'hello',
        position: 1,
        createdAt: created,
      ),
    );
    expect(bubbles.map((b) => b.kind).toList(), [ChatBubbleKind.message]);
  });

  test('bubbleCaption joins model provider tok/s', () {
    expect(
      bubbleCaption(
        const ChatBubble(
          kind: ChatBubbleKind.message,
          text: 'hello',
          model: 'm1',
          providerName: 'Local',
          predictedPerSecond: 35.5,
        ),
      ),
      'm1 · Local · 35.5 tok/s',
    );
    expect(
      bubbleCaption(const ChatBubble(kind: ChatBubbleKind.thought, text: 'x')),
      '',
    );
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `client/`):

```bash
nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test test/chat/chat_bubble_test.dart
```

Expected: FAIL (`chat_bubble.dart` not found).

- [ ] **Step 3: Implement**

`client/lib/chat/chat_bubble.dart`:

```dart
import '../acp/agent_connection.dart';
import '../catalog/models.dart';

enum ChatBubbleKind { user, thought, message, stats }

class ChatBubble {
  const ChatBubble({
    required this.kind,
    this.text = '',
    this.model,
    this.providerName,
    this.predictedPerSecond,
    this.usage,
    this.stopReason,
    this.streamingThought = false,
  });

  final ChatBubbleKind kind;
  final String text;
  final String? model;
  final String? providerName;
  final double? predictedPerSecond;
  final TurnUsage? usage;
  final String? stopReason;
  final bool streamingThought;

  ChatBubble copyWith({
    String? text,
    String? model,
    String? providerName,
    double? predictedPerSecond,
    TurnUsage? usage,
    String? stopReason,
    bool? streamingThought,
  }) {
    return ChatBubble(
      kind: kind,
      text: text ?? this.text,
      model: model ?? this.model,
      providerName: providerName ?? this.providerName,
      predictedPerSecond: predictedPerSecond ?? this.predictedPerSecond,
      usage: usage ?? this.usage,
      stopReason: stopReason ?? this.stopReason,
      streamingThought: streamingThought ?? this.streamingThought,
    );
  }
}

List<ChatBubble> bubblesFromThreadMessage(ThreadMessage message) {
  if (message.role == 'user') {
    return [ChatBubble(kind: ChatBubbleKind.user, text: message.content)];
  }
  final out = <ChatBubble>[];
  final thought = message.thought;
  if (thought != null && thought.isNotEmpty) {
    out.add(ChatBubble(kind: ChatBubbleKind.thought, text: thought));
  }
  out.add(
    ChatBubble(
      kind: ChatBubbleKind.message,
      text: message.content,
      model: message.model,
      providerName: message.providerName,
      predictedPerSecond: message.usage?.predictedPerSecond,
    ),
  );
  final stop = message.stopReason;
  if (message.usage != null || (stop != null && stop.isNotEmpty)) {
    out.add(
      ChatBubble(
        kind: ChatBubbleKind.stats,
        usage: message.usage,
        stopReason: stop,
      ),
    );
  }
  return out;
}

String bubbleCaption(ChatBubble bubble) {
  if (bubble.kind != ChatBubbleKind.message) {
    return '';
  }
  final tok = bubble.predictedPerSecond;
  return [
    bubble.model,
    bubble.providerName,
    if (tok != null) '$tok tok/s',
  ].whereType<String>().where((part) => part.isNotEmpty).join(' · ');
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test test/chat/chat_bubble_test.dart`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_bubble.dart client/test/chat/chat_bubble_test.dart
git commit -m "feat(chat): map thread messages to typed bubbles"
```

---

### Task 2: Controller live list is bubbles

**Files:**
- Modify: `client/lib/chat/chat_controller.dart` (import `chat_bubble.dart` instead of `chat_message.dart`)
- Modify: `client/test/chat/chat_controller_test.dart`
- Modify: `client/lib/chat/chat_screen.dart` only enough to compile: `m.kind == ChatBubbleKind.user` vs `Text(m.text)` for other kinds (Task 4 replaces this with `AgentBubble`)

**Interfaces:**
- Consumes: `ChatBubble`, `ChatBubbleKind`, `bubblesFromThreadMessage`, `AgentThoughtDelta` / `AgentMessageDelta` / `AgentUsageEvent`
- Produces: `List<ChatBubble> messages`; `bool get sending`; send does **not** pre-create an empty assistant bubble

- [ ] **Step 1: Write failing controller tests**

Replace the accumulation assertions in `chat_controller_test.dart`. Keep FakeConn/FakeCatalog. Change:

`send appends user message and streams assistant text` to expect kinds `[user, message]` and texts `hi` / `hello`.

Replace `send accumulates thought separately from assistant text` with:

```dart
test('send accumulates thought separately from assistant text', () async {
  final conn = FakeConn()
    ..thoughtsToEmit = ['why']
    ..chunksToEmit = ['hello']
    ..usageToEmit = const TurnUsage(
      predictedPerSecond: 35.5,
      deltas: 1,
      stopReason: 'end_turn',
    );
  final c = ChatController(
    session: conn,
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  await c.connect();
  await c.createThread();
  await c.selectAgent('ag-1');
  await c.send('hi');
  expect(c.messages.map((m) => m.kind).toList(), [
    ChatBubbleKind.user,
    ChatBubbleKind.thought,
    ChatBubbleKind.message,
    ChatBubbleKind.stats,
  ]);
  expect(c.messages[0].text, 'hi');
  expect(c.messages[1].text, 'why');
  expect(c.messages[2].text, 'hello');
  expect(c.messages[2].model, 'm1');
  expect(c.messages[2].providerName, 'Local');
  expect(c.messages[2].predictedPerSecond, 35.5);
  expect(c.messages[3].usage?.predictedPerSecond, 35.5);
  expect(c.messages[3].stopReason, 'end_turn');
  expect(c.messages[1].streamingThought, isFalse);
});

test('two thought deltas stay one thought bubble', () async {
  final conn = FakeConn()
    ..thoughtsToEmit = ['why', ' not']
    ..chunksToEmit = ['hello'];
  final c = ChatController(
    session: conn,
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  await c.connect();
  await c.createThread();
  await c.selectAgent('ag-1');
  await c.send('hi');
  expect(
    c.messages.where((m) => m.kind == ChatBubbleKind.thought).length,
    1,
  );
  expect(
    c.messages.firstWhere((m) => m.kind == ChatBubbleKind.thought).text,
    'why not',
  );
});
```

Update `selectThread maps persisted parts onto ChatMessage` to assert kinds `[user, thought, message, stats]` (or whatever the fixture contains) via `bubblesFromThreadMessage` order.

Update `send refresh replaces live bubbles with persisted GET parts` to assert thought/message **kinds** and persisted texts (not `messages.last.thought`).

Every remaining `ChatRole` / `ChatMessage` / `.thought` / `.role` assertion in this file must compile against `ChatBubble`. User rows: `kind == ChatBubbleKind.user`. Assistant answer text: the `message` kind. `messages.isNotEmpty` stays valid.

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test test/chat/chat_controller_test.dart`

Expected: FAIL (`ChatMessage` / `thought` / `role` no longer the right shape, or compile error once the test file imports `chat_bubble.dart`).

- [ ] **Step 3: Implement controller**

`messages` type: `final List<ChatBubble> messages = [];`

Add:

```dart
bool get sending => _sending;
```

`send`: do **not** add a placeholder assistant bubble. After the user bubble:

```dart
await _session.sendPrompt(trimmed, onEvent: (event) {
  if (epoch != _sendEpoch) {
    return;
  }
  switch (event) {
    case AgentThoughtDelta(:final text):
      _growOrAppend(
        ChatBubbleKind.thought,
        append: text,
        streamingThought: true,
      );
    case AgentMessageDelta(:final text):
      _growOrAppend(
        ChatBubbleKind.message,
        append: text,
        model: currentModel,
        providerName: _selectedProviderName(),
      );
    case AgentUsageEvent(:final usage):
      _growOrAppend(
        ChatBubbleKind.stats,
        usage: usage,
        stopReason: usage.stopReason,
      );
      _stampPredictedPerSecond(usage.predictedPerSecond);
  }
  notifyListeners();
});
```

Helpers (same file):

```dart
void _growOrAppend(
  ChatBubbleKind kind, {
  String append = '',
  String? model,
  String? providerName,
  TurnUsage? usage,
  String? stopReason,
  bool? streamingThought,
}) {
  if (messages.isNotEmpty && messages.last.kind == kind) {
    final last = messages.last;
    messages[messages.length - 1] = last.copyWith(
      text: last.text + append,
      model: model ?? last.model,
      providerName: providerName ?? last.providerName,
      usage: usage ?? last.usage,
      stopReason: stopReason ?? last.stopReason,
      streamingThought: streamingThought ?? last.streamingThought,
    );
    return;
  }
  messages.add(
    ChatBubble(
      kind: kind,
      text: append,
      model: model,
      providerName: providerName,
      usage: usage,
      stopReason: stopReason,
      streamingThought: streamingThought ?? false,
    ),
  );
}

void _stampPredictedPerSecond(double? tok) {
  if (tok == null) {
    return;
  }
  for (var i = messages.length - 1; i >= 0; i--) {
    if (messages[i].kind == ChatBubbleKind.message) {
      messages[i] = messages[i].copyWith(predictedPerSecond: tok);
      return;
    }
  }
}
```

After `sendPrompt` returns (same epoch): set `streamingThought: false` on **every** thought bubble that is still true (usually the last thought), not only `messages.last`.

`_refreshSelectedThread` when `detail.messages.isNotEmpty`:

```dart
messages
  ..clear()
  ..addAll(detail.messages.expand(bubblesFromThreadMessage));
```

`selectThread` success path: same `expand(bubblesFromThreadMessage)` instead of `_chatMessageFromThread`. Delete `_chatMessageFromThread`.

`_dropUncommitted` is unchanged (slices `messages` from `_uncommittedStart`).

In `chat_screen.dart`, switch the ListView off `ChatRole` so the package still compiles: user kind keeps the right-aligned blue bubble; every other kind is `Align(alignment: Alignment.centerLeft, child: Text(m.text.isEmpty ? '…' : m.text))`. Do not build `AgentBubble` yet.

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test test/chat/chat_controller_test.dart test/chat/chat_bubble_test.dart`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_controller.dart client/test/chat/chat_controller_test.dart
git commit -m "feat(chat): accumulate ACP events as a bubble stream"
```

---

### Task 3: AgentBubble accent-bar widgets

**Files:**
- Create: `client/lib/chat/agent_bubble.dart`
- Create: `client/test/chat/agent_bubble_test.dart`
- Delete after Task 4: `client/lib/chat/assistant_turn.dart`, `client/test/chat/assistant_turn_test.dart` (keep them compiling until Task 4, or migrate tests in this task and leave `AssistantTurnTile` unused)

**Interfaces:**
- Consumes: `ChatBubble`, `bubbleCaption`, `VisibilityMode`
- Produces: `AgentBubble` widget

- [ ] **Step 1: Write failing widget tests**

`client/test/chat/agent_bubble_test.dart`:

```dart
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('thought is collapsed; caption sits on the answer', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              AgentBubble(
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.thought,
                  text: 'hmm',
                ),
              ),
              AgentBubble(
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.message,
                  text: 'hello',
                  model: 'm1',
                  providerName: 'Local',
                  predictedPerSecond: 35.5,
                ),
              ),
              AgentBubble(
                bubble: ChatBubble(
                  kind: ChatBubbleKind.stats,
                  usage: const TurnUsage(
                    predictedPerSecond: 35.5,
                    deltas: 1,
                    elapsedMs: 50,
                    stopReason: 'end_turn',
                  ),
                  stopReason: 'end_turn',
                ),
              ),
            ],
          ),
        ),
      ),
    );
    expect(find.text('hello'), findsOneWidget);
    expect(find.textContaining('m1'), findsOneWidget);
    expect(find.textContaining('Local'), findsOneWidget);
    expect(find.textContaining('35.5'), findsOneWidget);
    expect(find.text('hmm'), findsNothing);
    expect(find.text('Answer'), findsNothing);
    await tester.tap(find.text('Thinking'));
    await tester.pumpAndSettle();
    expect(find.text('hmm'), findsOneWidget);
    expect(tester.getTopLeft(find.text('Thinking')).dy,
        lessThan(tester.getTopLeft(find.text('hello')).dy));
  });

  testWidgets('hidden thinking omits the thought bubble; caption remains', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              AgentBubble(
                thinkingMode: VisibilityMode.hidden,
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.thought,
                  text: 'hmm',
                ),
              ),
              AgentBubble(
                statsMode: VisibilityMode.hidden,
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.message,
                  text: 'hello',
                  model: 'm1',
                  providerName: 'Local',
                  predictedPerSecond: 35.5,
                ),
              ),
            ],
          ),
        ),
      ),
    );
    expect(find.text('Thinking'), findsNothing);
    expect(find.text('Stats'), findsNothing);
    expect(find.text('hello'), findsOneWidget);
    expect(find.textContaining('35.5'), findsOneWidget);
  });

  testWidgets('thinking key stable across text while streamingThought stays true', (
    tester,
  ) async {
    Future<void> pump({
      required String text,
      required bool streamingThought,
      VisibilityMode mode = VisibilityMode.collapsed,
    }) {
      return tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: AgentBubble(
              thinkingMode: mode,
              bubble: ChatBubble(
                kind: ChatBubbleKind.thought,
                text: text,
                streamingThought: streamingThought,
              ),
            ),
          ),
        ),
      );
    }

    await pump(text: 'hmm', streamingThought: true);
    final first = tester
        .widget<ExpansionTile>(find.widgetWithText(ExpansionTile, 'Thinking'))
        .key;
    await pump(text: 'hmm more', streamingThought: true);
    expect(
      tester
          .widget<ExpansionTile>(find.widgetWithText(ExpansionTile, 'Thinking'))
          .key,
      first,
    );
    await pump(
      text: 'hmm more',
      streamingThought: true,
      mode: VisibilityMode.expanded,
    );
    expect(
      tester
          .widget<ExpansionTile>(find.widgetWithText(ExpansionTile, 'Thinking'))
          .key,
      isNot(first),
    );
  });

  testWidgets('stats omitted without usage or stopReason', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: AgentBubble(
            bubble: ChatBubble(kind: ChatBubbleKind.stats),
          ),
        ),
      ),
    );
    expect(find.text('Stats'), findsNothing);
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test test/chat/agent_bubble_test.dart`

Expected: FAIL (`AgentBubble` not found).

- [ ] **Step 3: Implement `AgentBubble`**

White card, 12px radius, 4px left bar. User kind is not rendered here (`SizedBox.shrink()` if passed).

- `thought`: if `thinkingMode == hidden` → shrink. Else `ExpansionTile` title `Thinking`, `initiallyExpanded: bubble.streamingThought || thinkingMode == expanded`, key `Key('thinking-${bubble.streamingThought}-$thinkingMode')`, child `Text(bubble.text)`, bar color `0xFFD97706`.
- `message`: column of `Text(bubble.text.isEmpty ? '…' : bubble.text)` and, if `bubbleCaption(bubble)` is not empty, caption `Text` with `bodySmall` / `Color(0xFF71717A)`. No “Answer” label. Bar `0xFF18181B`.
- `stats`: if `statsMode == hidden` or (`usage == null` && empty stopReason) → shrink. Else `ExpansionTile` title `Stats`, `initiallyExpanded: statsMode == expanded`, key `Key('stats-$statsMode')`, children one `Text` per present usage field + `stopReason` (same field list as current `_statsLines` in `assistant_turn.dart`). Bar `0xFF71717A`.

Wrap the card in `Align(alignment: Alignment.centerLeft)` and `Container` margin `vertical: 4`, padding `12`, decoration white + `Border(left: BorderSide(color: bar, width: 4))` + `BorderRadius.circular(12)`.

Copy `_statsLines` into `agent_bubble.dart` (do not import `assistant_turn.dart`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test test/chat/agent_bubble_test.dart test/chat/chat_bubble_test.dart`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/agent_bubble.dart client/test/chat/agent_bubble_test.dart
git commit -m "feat(chat): render agent parts as accent-bar bubbles"
```

---

### Task 4: Wire ChatScreen, retire turn tile, auto-scroll

**Files:**
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/test/chat/chat_screen_test.dart`
- Modify: `client/test/settings/chat_tab_test.dart` (hidden thinking test uses `AgentBubble`)
- Delete: `client/lib/chat/assistant_turn.dart`, `client/lib/chat/chat_message.dart`, `client/test/chat/assistant_turn_test.dart`
- Modify: `README.md` (Thinking/Stats bullet: bubbles in arrival order, caption on the answer)

**Interfaces:**
- Consumes: `ChatBubble`, `AgentBubble`, `ChatController.sending`, `ChatDisplaySettings`
- Produces: transcript ListView of typed bubbles; auto-scroll while `sending`

- [ ] **Step 1: Write failing screen test**

In `chat_screen_test.dart`, keep `thinking stays open while streaming then follows collapsed default`. Add:

```dart
testWidgets('thinking appears above the answer while streaming', (tester) async {
  final conn = FakeConn()
    ..thoughtsToEmit = ['hmm']
    ..chunksToEmit = ['hello']
    ..sendHang = Completer<void>();
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
      home: ChatScreen(controller: c, displaySettings: displaySettings),
    ),
  );
  await tester.pumpAndSettle();
  await tester.enterText(find.byType(TextField), 'hi');
  await tester.tap(find.byIcon(Icons.send));
  await tester.pump();
  expect(find.text('hmm'), findsOneWidget);
  expect(tester.getTopLeft(find.text('Thinking')).dy,
      lessThan(tester.getTopLeft(find.text('hello')).dy));
  conn.sendHang!.complete();
  await tester.pumpAndSettle();
});
```

If the answer is still `…` during hang (chunks already emitted in FakeConn before hang), assert `hello` or `…` **below** Thinking — match actual FakeConn order in `chat_screen_test.dart` (thoughts/chunks emit, **then** `sendHang`). After pump, both `hmm` and `hello` should exist, thinking higher.

In `chat_tab_test.dart`, replace `AssistantTurnTile` + `ChatMessage` with a `Column` of `AgentBubble`s (thought hidden + message) like Task 3’s hidden test.

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test test/chat/chat_screen_test.dart test/settings/chat_tab_test.dart`

Expected: FAIL (answer still above thinking, and/or `AssistantTurnTile` / `ChatMessage` imports break after you switch the test).

- [ ] **Step 3: Implement screen**

`ListView.builder`:

```dart
final m = c.messages[index];
if (m.kind == ChatBubbleKind.user) {
  return Align(
    alignment: Alignment.centerRight,
    child: Container(
      margin: const EdgeInsets.symmetric(vertical: 4),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.blue.shade100,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(m.text),
    ),
  );
}
return AgentBubble(
  bubble: m,
  thinkingMode: display.thinking,
  statsMode: display.stats,
);
```

Attach a `ScrollController`. In `initState`, `widget.controller.addListener(_scrollToEnd)`; dispose both. `_scrollToEnd`: if `!widget.controller.sending` return; post-frame `jumpTo(maxScrollExtent)` when `hasClients`.

Remove imports of `assistant_turn.dart` / `chat_message.dart`. Delete those files and `assistant_turn_test.dart`.

README bullet (~line 70): thinking, answer, and stats are **separate bubbles in arrival order**; caption stays on the answer.

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric/.worktrees/chat-transparency -c flutter test`

Expected: PASS (full client suite).

- [ ] **Step 5: Commit**

```bash
git add client README.md
git commit -m "feat(chat): show thinking, answer, and stats as chat bubbles"
```

---

## Self-review

**Spec coverage**

| Spec item | Task |
| --- | --- |
| Flat stream, user prompt as grouping | 2, 4 |
| New bubble on kind change; append same kind | 2 |
| Caption on answer; Stats separate | 1, 3 |
| Accent bar colors + labels | 3 |
| Settings collapsed/expanded/hidden | 3, 4 |
| Thinking open while streaming | 2, 4 |
| ValueKey without thought text | 3 |
| Reload from thread thought/content/usage | 1, 2 |
| Cancel drops uncommitted | 2 (existing `_dropUncommitted`) |
| Auto-scroll in flight | 4 |
| No Go / no tools UI | — |
| Empty answer `…` | 3 |
| README | 4 |

**Type consistency:** `ChatBubble` / `ChatBubbleKind` / `bubblesFromThreadMessage` / `bubbleCaption` / `AgentBubble` / `sending` used as defined in Task 1–4.

**Placeholders:** none.
