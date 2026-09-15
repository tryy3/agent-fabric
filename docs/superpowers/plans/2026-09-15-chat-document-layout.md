# Chat Document Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace full-width agent accent cards with a Hermes-style document transcript: centered content column (default 720px), user bubble only, plain agent prose, Thinking as a slim activity row, and dense stats behind a footer Stats popover (retire Stats visibility setting).

**Architecture:** Keep `ChatBubbleKind` / controller mapping unchanged (including `stats` bubbles). Change only presentation: `ChatDisplaySettings` gains `contentWidth` and drops `stats`; `AgentBubble` renders thought as an activity row and message as plain prose + optional Stats action; `chat_screen` centers a shared max-width column, skips rendering `stats` rows, and passes the following stats bubble into the message widget for the popover.

**Tech Stack:** Flutter (Nix shell), `shared_preferences`, `flutter_test`. Work from repo root `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix commands with `nix develop /home/tryy3/src/agent-fabric -c` and run them from `client/`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-15-chat-document-layout-design.md`](../specs/2026-09-15-chat-document-layout-design.md).
- Default content width **720**; settings slider **560–1200**, step **20**; prefs key `chat.contentWidth`.
- Retire Stats visibility (`chat.statsVisibility` ignored; remove API + Settings UI). Thinking visibility unchanged.
- Do not fold `stats` off the controller list this slice — skip the row in UI and wire the following stats bubble into the message footer.
- No markdown, tool rows, Retry/Fork placeholders, agent avatar/trace chrome.
- No control-plane / ACP / `parts[]` changes. Do not commit `controlplane/data/`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/display_settings.dart` | `thinking` + `contentWidth`; remove `stats` |
| `client/lib/settings/chat_tab.dart` | Thinking dropdown + content-width slider |
| `client/lib/chat/chat_bubble.dart` | Add `activityDescription` helper (thought one-liner) |
| `client/lib/chat/agent_bubble.dart` | Activity row + plain prose + Stats popover; drop accent cards / statsMode |
| `client/lib/chat/chat_screen.dart` | Content column; skip stats rows; pass stats into message |
| `client/test/settings/chat_tab_test.dart` | Width + no Stats setting |
| `client/test/chat/chat_bubble_test.dart` | `activityDescription` unit tests |
| `client/test/chat/agent_bubble_test.dart` | Activity row, prose, Stats popover |
| `client/test/chat/chat_screen_test.dart` | Column width + no Stats card in list |

**Interfaces this plan locks:**

```dart
// display_settings.dart
class ChatDisplaySettings extends ChangeNotifier {
  VisibilityMode thinking;
  int contentWidth; // default 720
  Future<void> setThinking(VisibilityMode mode);
  Future<void> setContentWidth(int width); // clamp 560..1200
  // no stats / setStats
}

// chat_bubble.dart
String activityDescription(String text); // first non-empty line, trimmed

// agent_bubble.dart
class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    this.thinkingMode = VisibilityMode.collapsed,
    this.stats, // following ChatBubbleKind.stats bubble; message footer only
  });
  final ChatBubble bubble;
  final VisibilityMode thinkingMode;
  final ChatBubble? stats;
}

List<String> statsLines(ChatBubble bubble); // was private _statsLines; export for popover + tests
```

---

### Task 1: Retire Stats setting; add content width

**Files:**
- Modify: `client/lib/chat/display_settings.dart`
- Modify: `client/lib/settings/chat_tab.dart`
- Modify: `client/test/settings/chat_tab_test.dart`
- Modify: any compile break from removed `stats` / `setStats` / `statsMode` — fix call sites minimally so the package analyzes (full UI rewrite is later tasks). Prefer stubbing `AgentBubble.statsMode` removal by deleting the parameter and ignoring it in callers until Task 3–4.

**Interfaces:**
- Consumes: `SharedPreferences`, existing `VisibilityMode`
- Produces: `contentWidth`, `setContentWidth`, no `stats`

- [ ] **Step 1: Write the failing tests**

Replace `client/test/settings/chat_tab_test.dart` defaults/persistence coverage as follows (keep Thinking tests; remove Stats expectations):

```dart
test('defaults: thinking collapsed, contentWidth 720', () async {
  final s = await ChatDisplaySettings.load();
  expect(s.thinking, VisibilityMode.collapsed);
  expect(s.contentWidth, 720);
});

test('setContentWidth persists and clamps', () async {
  final s = await ChatDisplaySettings.load();
  await s.setContentWidth(500); // below min
  expect(s.contentWidth, 560);
  await s.setContentWidth(2000); // above max
  expect(s.contentWidth, 1200);
  await s.setContentWidth(800);
  final s2 = await ChatDisplaySettings.load();
  expect(s2.contentWidth, 800);
});

testWidgets('Chat tab has no Stats dropdown; slider updates width', (tester) async {
  final s = await ChatDisplaySettings.load();
  await tester.pumpWidget(MaterialApp(home: ChatTab(settings: s)));
  expect(find.byKey(const Key('stats-visibility')), findsNothing);
  expect(find.byKey(const Key('content-width')), findsOneWidget);
  await tester.drag(find.byKey(const Key('content-width')), const Offset(40, 0));
  await tester.pumpAndSettle();
  expect(s.contentWidth, isNot(720));
});
```

Update the existing “hidden thinking still shows answer” test: remove `statsMode:` arguments (they will be gone). Keep asserting no Thinking when hidden; **do not** assert `find.text('Stats')` is absent for the wrong reason — after Task 3 Stats becomes a button label; for this task only fix compile by dropping `statsMode`. If the test still expects `find.text('Stats')` findsNothing with only thought+message and no stats bubble passed, that remains valid until Task 3 adds a Stats button only when `stats` is provided.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/settings/chat_tab_test.dart
```

Expected: FAIL — `contentWidth` / `setContentWidth` / `Key('content-width')` missing; or `stats` still present.

- [ ] **Step 3: Implement settings**

`display_settings.dart`:

```dart
class ChatDisplaySettings extends ChangeNotifier {
  ChatDisplaySettings._(this._prefs, this.thinking, this.contentWidth);

  static const _thinkingKey = 'chat.thinkingVisibility';
  static const _widthKey = 'chat.contentWidth';
  static const minContentWidth = 560;
  static const maxContentWidth = 1200;
  static const defaultContentWidth = 720;

  final SharedPreferences _prefs;
  VisibilityMode thinking;
  int contentWidth;

  static Future<ChatDisplaySettings> load() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getInt(_widthKey);
    final width = (raw ?? defaultContentWidth)
        .clamp(minContentWidth, maxContentWidth);
    return ChatDisplaySettings._(
      prefs,
      _parse(prefs.getString(_thinkingKey)),
      width,
    );
  }

  Future<void> setContentWidth(int width) async {
    contentWidth = width.clamp(minContentWidth, maxContentWidth);
    await _prefs.setInt(_widthKey, contentWidth);
    notifyListeners();
  }
  // setThinking unchanged; delete stats / setStats / _statsKey
}
```

`chat_tab.dart`: remove Stats `DropdownButtonFormField`. Add:

```dart
Text('Content width (${settings.contentWidth}px)'),
Slider(
  key: const Key('content-width'),
  value: settings.contentWidth.toDouble(),
  min: ChatDisplaySettings.minContentWidth.toDouble(),
  max: ChatDisplaySettings.maxContentWidth.toDouble(),
  divisions: (ChatDisplaySettings.maxContentWidth -
          ChatDisplaySettings.minContentWidth) ~/
      20,
  label: '${settings.contentWidth}',
  onChanged: (v) => settings.setContentWidth(v.round()),
),
```

Fix compile errors: remove `display.stats` / `statsMode:` from `chat_screen.dart` and tests (pass nothing yet). Remove `statsMode` from `AgentBubble` constructor and delete `_stats` branch usage of `statsMode` temporarily — keep rendering stats card until Task 3 if needed for other tests, **or** make `_stats()` always use collapsed ExpansionTile without a mode parameter. Prefer: delete `statsMode` parameter; stats card still renders when kind is stats (Task 4 will stop mounting it).

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/settings/chat_tab_test.dart
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/display_settings.dart client/lib/settings/chat_tab.dart client/test/settings/chat_tab_test.dart client/lib/chat/agent_bubble.dart client/lib/chat/chat_screen.dart client/test/chat/agent_bubble_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): add content width setting; retire stats visibility

EOF
)"
```

---

### Task 2: `activityDescription` helper

**Files:**
- Modify: `client/lib/chat/chat_bubble.dart`
- Modify: `client/test/chat/chat_bubble_test.dart`

**Interfaces:**
- Consumes: thought `text` string
- Produces: `String activityDescription(String text)`

- [ ] **Step 1: Write the failing tests**

Append to `chat_bubble_test.dart`:

```dart
test('activityDescription uses first non-empty line', () {
  expect(activityDescription('hmm\nmore'), 'hmm');
  expect(activityDescription('\n  plan a story  \nrest'), 'plan a story');
  expect(activityDescription(''), '');
  expect(activityDescription('single'), 'single');
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/chat_bubble_test.dart
```

Expected: FAIL — `activityDescription` not defined.

- [ ] **Step 3: Implement**

In `chat_bubble.dart`:

```dart
String activityDescription(String text) {
  for (final line in text.split('\n')) {
    final trimmed = line.trim();
    if (trimmed.isNotEmpty) {
      return trimmed;
    }
  }
  return '';
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/chat_bubble_test.dart
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_bubble.dart client/test/chat/chat_bubble_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): add activityDescription for thinking rows

EOF
)"
```

---

### Task 3: AgentBubble — activity row, plain prose, Stats popover

**Files:**
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`

**Interfaces:**
- Consumes: `activityDescription`, `VisibilityMode`, optional `stats` bubble
- Produces: new `AgentBubble` API (`stats` instead of `statsMode`); `statsLines` public; no accent-bar cards; `stats` kind returns `SizedBox.shrink()`

- [ ] **Step 1: Rewrite failing widget tests**

Replace `agent_bubble_test.dart` with tests that match the new UI (delete accent-color card test and ExpansionTile-key-on-Stats tests):

```dart
testWidgets('thought collapsed shows title + description; expands to body', (
  tester,
) async {
  await tester.pumpWidget(
    const MaterialApp(
      home: Scaffold(
        body: AgentBubble(
          bubble: ChatBubble(
            kind: ChatBubbleKind.thought,
            text: 'hmm\nmore detail',
          ),
        ),
      ),
    ),
  );
  expect(find.text('Thinking'), findsOneWidget);
  expect(find.text('hmm'), findsOneWidget); // description visible
  expect(find.text('more detail'), findsNothing);
  await tester.tap(find.byKey(const Key('activity-thinking')));
  await tester.pumpAndSettle();
  expect(find.text('more detail'), findsOneWidget);
});

testWidgets('hidden thinking omits the row', (tester) async {
  await tester.pumpWidget(
    const MaterialApp(
      home: Scaffold(
        body: AgentBubble(
          thinkingMode: VisibilityMode.hidden,
          bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 'hmm'),
        ),
      ),
    ),
  );
  expect(find.text('Thinking'), findsNothing);
});

testWidgets('message is plain prose with caption; Stats opens popover', (
  tester,
) async {
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: AgentBubble(
          bubble: const ChatBubble(
            kind: ChatBubbleKind.message,
            text: 'hello',
            model: 'm1',
            providerName: 'Local',
            predictedPerSecond: 35.5,
          ),
          stats: ChatBubble(
            kind: ChatBubbleKind.stats,
            usage: const TurnUsage(elapsedMs: 50, deltas: 1),
            stopReason: 'end_turn',
          ),
        ),
      ),
    ),
  );
  expect(find.text('hello'), findsOneWidget);
  expect(find.textContaining('m1'), findsOneWidget);
  expect(find.byKey(const Key('stats-action')), findsOneWidget);
  // No accent card: nearest decorated Container with left border must not wrap hello
  await tester.tap(find.byKey(const Key('stats-action')));
  await tester.pumpAndSettle();
  expect(find.text('elapsedMs: 50'), findsOneWidget);
  expect(find.text('stopReason: end_turn'), findsOneWidget);
});

testWidgets('Stats action omitted without usage/stopReason', (tester) async {
  await tester.pumpWidget(
    const MaterialApp(
      home: Scaffold(
        body: AgentBubble(
          bubble: ChatBubble(kind: ChatBubbleKind.message, text: 'hello'),
        ),
      ),
    ),
  );
  expect(find.byKey(const Key('stats-action')), findsNothing);
});

testWidgets('stats kind widget is empty', (tester) async {
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: AgentBubble(
          bubble: ChatBubble(
            kind: ChatBubbleKind.stats,
            usage: const TurnUsage(elapsedMs: 1),
          ),
        ),
      ),
    ),
  );
  expect(find.text('Stats'), findsNothing);
  expect(find.byKey(const Key('stats-action')), findsNothing);
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
      .widget(find.byKey(const Key('activity-thinking')))
      .key;
  await pump(text: 'hmm more', streamingThought: true);
  expect(
    tester.widget(find.byKey(const Key('activity-thinking'))).key,
    first,
  );
  await pump(
    text: 'hmm more',
    streamingThought: true,
    mode: VisibilityMode.expanded,
  );
  expect(
    tester.widget(find.byKey(const Key('activity-thinking'))).key,
    isNot(first),
  );
});
```

Note: put `ValueKey('thinking-$streamingThought-$thinkingMode')` on the activity widget so the last test works the same way ExpansionTile did.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/agent_bubble_test.dart
```

Expected: FAIL — missing keys / popover / still cards.

- [ ] **Step 3: Implement `AgentBubble`**

Rewrite `agent_bubble.dart` along these lines:

```dart
class AgentBubble extends StatelessWidget {
  const AgentBubble({
    super.key,
    required this.bubble,
    this.thinkingMode = VisibilityMode.collapsed,
    this.stats,
  });

  final ChatBubble bubble;
  final VisibilityMode thinkingMode;
  final ChatBubble? stats;

  @override
  Widget build(BuildContext context) {
    return switch (bubble.kind) {
      ChatBubbleKind.user => const SizedBox.shrink(),
      ChatBubbleKind.thought => _ThoughtActivity(
          bubble: bubble,
          thinkingMode: thinkingMode,
        ),
      ChatBubbleKind.message => _MessageProse(bubble: bubble, stats: stats),
      ChatBubbleKind.stats => const SizedBox.shrink(),
    };
  }
}

List<String> statsLines(ChatBubble bubble) { /* move existing _statsLines body */ }

String? _statsDescriptionGate(ChatBubble? stats) {
  if (stats == null) return null;
  final stop = stats.stopReason ?? '';
  if (stats.usage == null && stop.isEmpty) return null;
  return 'ok';
}
```

`_ThoughtActivity`: `StatefulWidget` or use `ExpansionTile` **without** the old `_card` chrome. Prefer a custom header:

- `Key`: `ValueKey('thinking-${bubble.streamingThought}-$thinkingMode')` on the root, and `Key('activity-thinking')` on the tappable header.
- Initially expanded when `streamingThought || thinkingMode == expanded`.
- Header row: `Icons.lightbulb_outline`, bold `Thinking`, `Expanded(child: Text(activityDescription(bubble.text), maxLines: 1, overflow: TextOverflow.ellipsis, style: muted))`, chevron.
- Expanded body: padded `Text(bubble.text)` on `Theme.of(context).colorScheme.surfaceContainerHighest` (or `Colors.black12`) — no left accent bar.

`_MessageProse`:

```dart
Column(
  crossAxisAlignment: CrossAxisAlignment.start,
  children: [
    Text(bubble.text.isEmpty ? '…' : bubble.text),
    if (caption.isNotEmpty) Text(caption, style: bodySmall muted),
    if (_hasStats(stats))
      TextButton(
        key: const Key('stats-action'),
        onPressed: () => showDialog(
          context: context,
          builder: (_) => AlertDialog(
            title: const Text('Stats'),
            content: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final line in statsLines(stats!)) Text(line),
              ],
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(context),
                child: const Text('Close'),
              ),
            ],
          ),
        ),
        child: const Text('Stats'),
      ),
  ],
)
```

`_hasStats`: `stats != null && (stats.usage != null || (stats.stopReason?.isNotEmpty ?? false))`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/agent_bubble_test.dart
```

Expected: PASS. Fix the streaming key test if the finder needs `find.byWidgetPredicate` for `ValueKey` vs `Key('activity-thinking')` — keep both keys as specified.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/agent_bubble.dart client/test/chat/agent_bubble_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): document-style agent prose, thinking row, stats popover

EOF
)"
```

---

### Task 4: Chat screen content column + wire stats into messages

**Files:**
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/test/chat/chat_screen_test.dart`
- Modify: `client/test/settings/chat_tab_test.dart` if the “hidden thinking” test still uses old `AgentBubble` API

**Interfaces:**
- Consumes: `displaySettings.contentWidth`, `AgentBubble(..., stats:)`
- Produces: shared max-width column for list + composer; no visible stats rows

- [ ] **Step 1: Write / update failing screen tests**

Add to `chat_screen_test.dart` (reuse existing pump helpers):

```dart
testWidgets('stats bubble is not shown as a card; Stats action is on message', (
  tester,
) async {
  // After connect + send that yields thought/message/usage (existing fake),
  // expect find.text('elapsedMs:') findsNothing until Stats tapped;
  // expect find.byKey(Key('stats-action')) findsOneWidget after turn completes.
});

testWidgets('message list respects content width', (tester) async {
  final display = await ChatDisplaySettings.load();
  await display.setContentWidth(560);
  // pump ChatScreen with that displaySettings
  // find a message Text ancestor ConstrainedBox with maxWidth 560
});
```

Concrete assertions for width (after pumping a thread with at least one user message):

```dart
final box = tester.widget<ConstrainedBox>(
  find
      .ancestor(
        of: find.text('Hello'), // or whatever the fixture user text is
        matching: find.byType(ConstrainedBox),
      )
      .first,
);
expect(box.constraints.maxWidth, 560);
```

If the list’s ConstrainedBox is an ancestor of every item, that works. Prefer putting **one** `ConstrainedBox` around the `ListView` so the finder is reliable.

Keep existing streaming/thinking-above-answer tests; update any `statsMode` / ExpansionTile assumptions.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/chat_screen_test.dart
```

Expected: FAIL — no column constraint / stats still as ExpansionTile card if old path remains.

- [ ] **Step 3: Implement `chat_screen` layout**

In `build`, read `final width = widget.displaySettings.contentWidth.toDouble();`.

Replace body column children with:

```dart
Expanded(
  child: c.selectedThreadId == null
      ? const Center(child: Text('Create a thread to start chatting'))
      : Align(
          alignment: Alignment.topCenter,
          child: ConstrainedBox(
            constraints: BoxConstraints(maxWidth: width),
            child: ListView.builder(
              controller: _scroll,
              padding: const EdgeInsets.all(16),
              itemCount: c.messages.length,
              itemBuilder: (context, index) {
                final m = c.messages[index];
                if (m.kind == ChatBubbleKind.stats) {
                  return const SizedBox.shrink();
                }
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
                ChatBubble? stats;
                if (m.kind == ChatBubbleKind.message &&
                    index + 1 < c.messages.length &&
                    c.messages[index + 1].kind == ChatBubbleKind.stats) {
                  stats = c.messages[index + 1];
                }
                return AgentBubble(
                  bubble: m,
                  thinkingMode: widget.displaySettings.thinking,
                  stats: stats,
                );
              },
            ),
          ),
        ),
),
SafeArea(
  child: Align(
    alignment: Alignment.center,
    child: ConstrainedBox(
      constraints: BoxConstraints(maxWidth: width),
      child: Padding(
        padding: const EdgeInsets.all(8),
        child: Row(
          children: [
            Expanded(
              child: TextField(
                controller: _input,
                enabled: c.canSend,
                onSubmitted: (_) => _submit(),
                decoration: const InputDecoration(
                  hintText: 'Message',
                  border: OutlineInputBorder(),
                ),
              ),
            ),
            const SizedBox(width: 8),
            IconButton(
              onPressed: c.canSend ? _submit : null,
              icon: const Icon(Icons.send),
            ),
          ],
        ),
      ),
    ),
  ),
),
```

- [ ] **Step 4: Run screen + related tests**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test test/chat/chat_screen_test.dart test/chat/agent_bubble_test.dart test/settings/chat_tab_test.dart
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_screen.dart client/test/chat/chat_screen_test.dart client/test/settings/chat_tab_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): constrain transcript column and wire stats footer

EOF
)"
```

---

### Task 5: Full client verify + analyze

**Files:**
- Modify: any remaining references to `statsMode`, `setStats`, `.stats` on `ChatDisplaySettings`, or accent-card tests
- Optionally touch README only if it still describes accent-bar stats bubbles (one sentence)

**Interfaces:** none new

- [ ] **Step 1: Grep for stale API**

```bash
cd /home/tryy3/src/agent-fabric && rg 'statsMode|setStats|statsVisibility|chat\.statsVisibility' client/
```

Expected: no matches in `lib/` or `test/` (legacy prefs key may be mentioned only in this plan/spec).

- [ ] **Step 2: Run full Flutter tests + analyzer**

```bash
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c flutter test
cd /home/tryy3/src/agent-fabric/client && nix develop /home/tryy3/src/agent-fabric -c dart analyze
```

Expected: all tests PASS; analyzer clean (or only pre-existing infos unrelated to this work).

- [ ] **Step 3: Fix any failures**

Apply minimal fixes; re-run the failing file until green.

- [ ] **Step 4: Commit if there were fixups**

```bash
git add -u client/
git commit -m "$(cat <<'EOF'
fix(chat): finish document layout cleanup

EOF
)"
```

Skip empty commit if nothing changed.

- [ ] **Step 5: Mark spec status**

In `docs/superpowers/specs/2026-09-15-chat-document-layout-design.md`, set **Status:** `implemented` (or `approved for planning` → leave until merge — prefer `implemented` only after this task’s tests pass).

```bash
git add docs/superpowers/specs/2026-09-15-chat-document-layout-design.md
git commit -m "$(cat <<'EOF'
docs: mark chat document layout spec implemented

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Content column default 720, slider 560–1200 step 20 | 1, 4 |
| User bubble retained | 4 (unchanged widget) |
| Agent prose plain (no card) | 3 |
| Thinking activity row + description | 2, 3 |
| Stats not a transcript card; popover via action | 3, 4 |
| Retire Stats visibility setting | 1 |
| Thinking visibility kept | 1, 3 |
| Keep stats bubble in controller list | 4 (skip render) |
| No markdown / tools / retry-fork | non-goals — no tasks |

## Self-review notes

- No TBD placeholders; stats wiring path is explicit (look-ahead in `itemBuilder`).
- `AgentBubble.stats` naming matches Tasks 3–4.
- Width clamp constants live on `ChatDisplaySettings` and are reused by the slider.
