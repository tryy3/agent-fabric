# Chat Composer Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-line chat input with an expandable `ChatComposer` that hosts agent/model controls and placeholder attach/mic actions, while sliming the AppBar to status + view-mode only.

**Architecture:** Extract `ChatComposer` as a stateful widget owned by `ChatScreen`. It owns the text controller, focus node, and height mode (roomy / compact / expanded). Agent and model pickers move from the AppBar into the composer toolbar with the same `ChatController` APIs and keys. No backend changes.

**Tech Stack:** Flutter (Nix shell). Work from `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix with `nix develop /home/tryy3/src/agent-fabric -c` and run from `client/`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-18-chat-composer-redesign-design.md`](../specs/2026-09-18-chat-composer-redesign-design.md).
- Desktop toolbar layout only (attach, mic, agent, model → spacer → send). No mobile pill breakpoint.
- Attach + mic: disabled, tooltip exactly `Coming soon`. No file/voice implementation.
- Roomy when `controller.messages.isEmpty`; compact when ≥1 message unless focused or text needs >1 line.
- Enter sends; Shift+Enter inserts newline.
- Keep keys `agent-picker` and `model-picker`. Add `composer-input` and `composer-send`.
- Do not change `ChatController` send/selection semantics.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/chat_composer.dart` | Expandable composer UI + toolbar + pickers + keyboard submit |
| `client/lib/chat/chat_screen.dart` | Slim AppBar (status only); embed `ChatComposer`; drop local input/picker helpers |
| `client/test/chat/chat_composer_test.dart` | Composer height modes, placeholders, Enter/Shift+Enter |
| `client/test/chat/chat_screen_test.dart` | Point finds at composer keys; keep behavior coverage |
| `client/test/widget_test.dart` | App shell still finds pickers/send via new locations |

**Interfaces this plan locks:**

```dart
// chat_composer.dart
class ChatComposer extends StatefulWidget {
  const ChatComposer({
    super.key,
    required this.controller,
  });

  final ChatController controller;
}

// Test keys
const Key('composer-input')
const Key('composer-send')
const Key('composer-attach')  // disabled placeholder
const Key('composer-mic')     // disabled placeholder
const Key('agent-picker')     // unchanged
const Key('model-picker')     // unchanged

// Height helpers (private or @visibleForTesting)
int composerMinLines({required bool hasMessages, required bool focused})
// empty → 4; has messages + !focused → 1; focused → 1 (grows via maxLines)
```

---

### Task 1: Extract `ChatComposer` shell and wire into `ChatScreen`

**Files:**
- Create: `client/lib/chat/chat_composer.dart`
- Modify: `client/lib/chat/chat_screen.dart` (replace bottom `Row` input; keep AppBar pickers for now)
- Modify: `client/test/chat/chat_screen_test.dart` (find `composer-input` / `composer-send` instead of bare `TextField` / `Icons.send` where the composer is intended)
- Test: `client/test/chat/chat_composer_test.dart` (new; basic render + send)

**Interfaces:**
- Consumes: `ChatController.canSend`, `ChatController.send`
- Produces: `ChatComposer({required ChatController controller})` with keys `composer-input`, `composer-send`

- [ ] **Step 1: Write the failing composer smoke test**

Create `client/test/chat/chat_composer_test.dart`:

```dart
import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_composer.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

// Reuse the same FakeConn / FakeCatalog / _agent helpers as chat_screen_test.dart
// (copy the minimal stubs needed into this file for independence).

void main() {
  testWidgets('composer sends on send button when canSend', (tester) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(body: ChatComposer(controller: c)),
      ),
    );
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('composer-input')), 'hello');
    await tester.tap(find.byKey(const Key('composer-send')));
    await tester.pumpAndSettle();

    expect(fake.prompts, ['hello']);
    expect(find.text('hello'), findsNothing); // cleared
  });
}
```

Copy minimal `FakeConn`, `FakeCatalog`, and `_agent` from `chat_screen_test.dart` into this file (same shapes).

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_composer_test.dart'`

Expected: FAIL — `chat_composer.dart` / `ChatComposer` not found.

- [ ] **Step 3: Implement minimal `ChatComposer` + wire `ChatScreen`**

Create `client/lib/chat/chat_composer.dart`:

```dart
import 'package:material_ui/material_ui.dart';

import 'chat_controller.dart';

class ChatComposer extends StatefulWidget {
  const ChatComposer({super.key, required this.controller});

  final ChatController controller;

  @override
  State<ChatComposer> createState() => _ChatComposerState();
}

class _ChatComposerState extends State<ChatComposer> {
  final _input = TextEditingController();
  final _focus = FocusNode();

  @override
  void dispose() {
    _input.dispose();
    _focus.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final text = _input.text;
    _input.clear();
    await widget.controller.send(text);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final c = widget.controller;
        final scheme = Theme.of(context).colorScheme;
        return Material(
          color: scheme.surfaceContainerHighest,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
            side: BorderSide(color: scheme.outlineVariant),
          ),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(12, 8, 8, 8),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  key: const Key('composer-input'),
                  controller: _input,
                  focusNode: _focus,
                  enabled: c.canSend,
                  minLines: 1,
                  maxLines: 8,
                  keyboardType: TextInputType.multiline,
                  textInputAction: TextInputAction.newline,
                  decoration: const InputDecoration(
                    hintText: 'Message',
                    border: InputBorder.none,
                    isDense: true,
                  ),
                ),
                Row(
                  children: [
                    const Spacer(),
                    IconButton(
                      key: const Key('composer-send'),
                      onPressed: c.canSend ? _submit : null,
                      icon: const Icon(Icons.send),
                    ),
                  ],
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}
```

In `chat_screen.dart`:
- Remove `_input` field, dispose, and `_submit`.
- Replace the bottom `Row` (`TextField` + send) with:

```dart
SafeArea(
  child: Align(
    alignment: Alignment.center,
    child: ConstrainedBox(
      constraints: BoxConstraints(maxWidth: width),
      child: Padding(
        padding: const EdgeInsets.all(8),
        child: ChatComposer(controller: c),
      ),
    ),
  ),
),
```

- Add `import 'chat_composer.dart';`
- Leave AppBar agent/model pickers as-is for this task.

Update `chat_screen_test.dart` finds that target the chat input:
- `find.byType(TextField)` → `find.byKey(const Key('composer-input'))` for composer interactions (keep any non-composer TextField finds unchanged — there are none in this file).
- `find.byIcon(Icons.send)` / `find.widgetWithIcon(IconButton, Icons.send)` → `find.byKey(const Key('composer-send'))`.

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_composer_test.dart test/chat/chat_screen_test.dart'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_composer.dart client/lib/chat/chat_screen.dart \
  client/test/chat/chat_composer_test.dart client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
feat(client): extract expandable ChatComposer shell

EOF
)"
```

---

### Task 2: Move agent/model into composer toolbar; slim AppBar

**Files:**
- Modify: `client/lib/chat/chat_composer.dart`
- Modify: `client/lib/chat/chat_screen.dart` (remove `_agentPicker` / `_modelPicker` and AppBar picker row; keep status)
- Modify: `client/test/widget_test.dart` if send icon key changes (should still find pickers)
- Test: existing `chat_screen_test.dart` agent-picker tests + `widget_test.dart`

**Interfaces:**
- Consumes: `canSelectAgent`, `canSelectModel`, `agents`, `selectedAgentId`, `selectAgent`, `modelOptions`, `currentModel`, `selectModel`, `selectedAgentMissing`
- Produces: same keys on toolbar `DropdownButton`s inside composer

- [ ] **Step 1: Write failing assertion that AppBar no longer hosts side-by-side pickers**

In `chat_screen_test.dart`, add:

```dart
testWidgets('agent and model pickers live in the composer not the app bar', (
  tester,
) async {
  final c = ChatController(
    session: FakeConn(),
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  addTearDown(c.dispose);
  await c.connect();
  await c.createThread();

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: ChatScreen(controller: c, displaySettings: displaySettings),
    ),
  );
  await tester.pumpAndSettle();

  final agent = find.byKey(const Key('agent-picker'));
  final model = find.byKey(const Key('model-picker'));
  expect(agent, findsOneWidget);
  expect(model, findsOneWidget);

  expect(
    find.descendant(of: find.byType(AppBar), matching: agent),
    findsNothing,
  );
  expect(
    find.descendant(of: find.byType(ChatComposer), matching: agent),
    findsOneWidget,
  );
  expect(
    find.descendant(of: find.byType(ChatComposer), matching: model),
    findsOneWidget,
  );
});
```

Import `chat_composer.dart` in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart --name "agent and model pickers live"'`

Expected: FAIL — pickers still under `AppBar`.

- [ ] **Step 3: Move pickers into composer; slim AppBar**

Move `_agentPicker` / `_modelPicker` logic into `chat_composer.dart` as private methods on `_ChatComposerState` (same item lists, keys, enablement).

Toolbar row (left → right):

```dart
Row(
  children: [
    // placeholders added in Task 4 — leave Spacer start for now or empty SizedBox
    Flexible(
      child: DropdownButtonHideUnderline(
        child: _agentPicker(c),
      ),
    ),
    const SizedBox(width: 8),
    Flexible(
      child: DropdownButtonHideUnderline(
        child: _modelPicker(c),
      ),
    ),
    const Spacer(),
    IconButton(
      key: const Key('composer-send'),
      onPressed: c.canSend ? _submit : null,
      icon: const Icon(Icons.arrow_upward),
      style: IconButton.styleFrom(
        backgroundColor: c.canSend
            ? Theme.of(context).colorScheme.primary
            : null,
        foregroundColor: c.canSend
            ? Theme.of(context).colorScheme.onPrimary
            : null,
      ),
    ),
  ],
)
```

Keep `DropdownButton` (with `isDense: true`, `isExpanded: true` inside `Flexible`) so `widget_test.dart` casts continue to work.

In `chat_screen.dart` AppBar `bottom`:

```dart
bottom: PreferredSize(
  preferredSize: const Size.fromHeight(28),
  child: Padding(
    padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
    child: Align(
      alignment: Alignment.centerLeft,
      child: Text(
        _statusLabel(c),
        style: Theme.of(context).textTheme.bodySmall,
      ),
    ),
  ),
),
```

Delete `_agentPicker` and `_modelPicker` from `chat_screen.dart`.

Update send-icon assertions in tests to use `composer-send` (already done in Task 1). Update `widget_test.dart` `find.byIcon(Icons.send)` → `find.byKey(const Key('composer-send'))`.

- [ ] **Step 4: Run tests**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart test/chat/chat_composer_test.dart test/widget_test.dart'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_composer.dart client/lib/chat/chat_screen.dart \
  client/test/chat/chat_screen_test.dart client/test/widget_test.dart
git commit -m "$(cat <<'EOF'
feat(client): move agent and model pickers into chat composer

EOF
)"
```

---

### Task 3: Roomy / compact / expanded height modes

**Files:**
- Modify: `client/lib/chat/chat_composer.dart`
- Modify: `client/test/chat/chat_composer_test.dart`

**Interfaces:**
- Consumes: `controller.messages.isEmpty`, focus, text line breaks
- Produces: `@visibleForTesting int composerMinLines({required bool hasMessages, required bool focused})`

- [ ] **Step 1: Write failing height-mode tests**

Add to `chat_composer_test.dart`:

```dart
import 'package:agent_fabric_client/chat/chat_composer.dart' show composerMinLines;
// If private top-level isn't exportable, put composerMinLines in chat_composer.dart
// as a top-level function (not private) used by the State.

test('composerMinLines is roomy for empty threads', () {
  expect(
    composerMinLines(hasMessages: false, focused: false),
    4,
  );
  expect(
    composerMinLines(hasMessages: false, focused: true),
    4,
  );
});

test('composerMinLines is compact when messages exist and unfocused', () {
  expect(
    composerMinLines(hasMessages: true, focused: false),
    1,
  );
});

test('composerMinLines stays at least 1 when focused with messages', () {
  expect(
    composerMinLines(hasMessages: true, focused: true),
    1,
  );
});

testWidgets('empty thread uses roomy minLines on the input', (tester) async {
  final c = ChatController(
    session: FakeConn(),
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  addTearDown(c.dispose);
  await c.connect();
  await c.createThread();

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(body: ChatComposer(controller: c)),
    ),
  );
  await tester.pumpAndSettle();

  final field = tester.widget<TextField>(find.byKey(const Key('composer-input')));
  expect(field.minLines, 4);
  expect(field.maxLines, 8);
});

testWidgets('after a message, unfocused composer is compact', (tester) async {
  final c = ChatController(
    session: FakeConn(),
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  addTearDown(c.dispose);
  await c.connect();
  await c.createThread();
  await c.selectAgent('ag-1');
  await c.send('hi');

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(body: ChatComposer(controller: c)),
    ),
  );
  await tester.pumpAndSettle();

  final field = tester.widget<TextField>(find.byKey(const Key('composer-input')));
  expect(field.minLines, 1);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_composer_test.dart'`

Expected: FAIL — `composerMinLines` missing / minLines always 1.

- [ ] **Step 3: Implement height modes**

In `chat_composer.dart`:

```dart
int composerMinLines({required bool hasMessages, required bool focused}) {
  if (!hasMessages) return 4;
  return 1;
}

// In State:
bool _focused = false;

@override
void initState() {
  super.initState();
  _focus.addListener(() {
    setState(() => _focused = _focus.hasFocus);
  });
}

// In build, wrap the field column in AnimatedSize:
final minLines = composerMinLines(
  hasMessages: c.messages.isNotEmpty,
  focused: _focused,
);
// When focused with messages, keep minLines 1 but allow growth via maxLines: 8.
// Spec: expand on focus — bump minLines to 3 when focused && hasMessages && text is short:
final effectiveMin = (!hasMessages)
    ? 4
    : (_focused ? 3 : 1);
```

Align with the unit helper: update `composerMinLines` to match the widget:

```dart
int composerMinLines({required bool hasMessages, required bool focused}) {
  if (!hasMessages) return 4;
  if (focused) return 3;
  return 1;
}
```

Update the unit tests in Step 1 to expect `3` when `hasMessages && focused`.

Wrap content:

```dart
AnimatedSize(
  duration: const Duration(milliseconds: 150),
  curve: Curves.easeInOut,
  alignment: Alignment.topCenter,
  child: TextField(
    minLines: composerMinLines(
      hasMessages: c.messages.isNotEmpty,
      focused: _focused,
    ),
    maxLines: 8,
    ...
  ),
)
```

- [ ] **Step 4: Run tests**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_composer_test.dart test/chat/chat_screen_test.dart'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_composer.dart client/test/chat/chat_composer_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add roomy and compact chat composer heights

EOF
)"
```

---

### Task 4: Placeholders + Enter-to-send / Shift+Enter newline

**Files:**
- Modify: `client/lib/chat/chat_composer.dart`
- Modify: `client/test/chat/chat_composer_test.dart`

**Interfaces:**
- Produces: keys `composer-attach`, `composer-mic`; keyboard submit behavior

- [ ] **Step 1: Write failing tests**

```dart
testWidgets('attach and mic are disabled with Coming soon tooltip', (tester) async {
  final c = ChatController(
    session: FakeConn(),
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  addTearDown(c.dispose);
  await c.connect();
  await c.createThread();

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(body: ChatComposer(controller: c)),
    ),
  );
  await tester.pumpAndSettle();

  final attach = tester.widget<IconButton>(find.byKey(const Key('composer-attach')));
  final mic = tester.widget<IconButton>(find.byKey(const Key('composer-mic')));
  expect(attach.onPressed, isNull);
  expect(mic.onPressed, isNull);

  expect(
    find.byTooltip('Coming soon'),
    findsNWidgets(2),
  );
});

testWidgets('Enter sends and Shift+Enter inserts newline', (tester) async {
  final fake = FakeConn();
  final c = ChatController(
    session: fake,
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  addTearDown(c.dispose);
  await c.connect();
  await c.createThread();
  await c.selectAgent('ag-1');

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(body: ChatComposer(controller: c)),
    ),
  );
  await tester.pumpAndSettle();

  await tester.enterText(find.byKey(const Key('composer-input')), 'line1');
  await tester.sendKeyEvent(LogicalKeyboardKey.enter);
  await tester.pumpAndSettle();
  expect(fake.prompts, ['line1']);

  await tester.enterText(find.byKey(const Key('composer-input')), 'a');
  await tester.sendKeyDownEvent(LogicalKeyboardKey.shift);
  await tester.sendKeyEvent(LogicalKeyboardKey.enter);
  await tester.sendKeyUpEvent(LogicalKeyboardKey.shift);
  await tester.pumpAndSettle();
  expect(fake.prompts, ['line1']); // no second send
  final field = tester.widget<TextField>(find.byKey(const Key('composer-input')));
  expect(field.controller!.text.contains('\n'), isTrue);
});
```

Import `package:flutter/services.dart` for `LogicalKeyboardKey`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_composer_test.dart --name "attach and mic|Enter sends"'`

Expected: FAIL — missing keys / Enter does not send.

- [ ] **Step 3: Implement placeholders and keyboard handling**

Toolbar leading icons:

```dart
IconButton(
  key: const Key('composer-attach'),
  tooltip: 'Coming soon',
  onPressed: null,
  icon: const Icon(Icons.attach_file),
),
IconButton(
  key: const Key('composer-mic'),
  tooltip: 'Coming soon',
  onPressed: null,
  icon: const Icon(Icons.mic_none),
),
```

Keyboard: wrap the `TextField` with `Focus` (already have `_focus` on the field) and set:

```dart
TextField(
  focusNode: _focus,
  // ...
)

// In initState, after creating _focus:
_focus.onKeyEvent = (node, event) {
  if (event is! KeyDownEvent) {
    return KeyEventResult.ignored;
  }
  if (event.logicalKey != LogicalKeyboardKey.enter &&
      event.logicalKey != LogicalKeyboardKey.numpadEnter) {
    return KeyEventResult.ignored;
  }
  final shift = HardwareKeyboard.instance.isShiftPressed;
  if (shift) {
    return KeyEventResult.ignored; // insert newline
  }
  if (widget.controller.canSend && _input.text.trim().isNotEmpty) {
    _submit();
    return KeyEventResult.handled;
  }
  return KeyEventResult.ignored;
};
```

If `FocusNode.onKeyEvent` is awkward to assign, use:

```dart
Focus(
  onKeyEvent: (node, event) { ... },
  child: TextField(focusNode: _focus, ...),
)
```

Note: some Flutter versions still insert newline before the handler; if the Enter-sends test flakes, switch to `CallbacksShortcuts` / `Shortcuts`+`Actions` with an `Intent`, or insert newline manually on Shift+Enter and always handle Enter. Prefer making the test pass with handled Enter and ignored Shift+Enter.

- [ ] **Step 4: Run full client chat tests**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/ test/widget_test.dart'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_composer.dart client/test/chat/chat_composer_test.dart
git commit -m "$(cat <<'EOF'
feat(client): add composer placeholders and enter-to-send

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Extract `ChatComposer` | 1 |
| Desktop toolbar layout | 2 + 4 |
| Move agent/model out of AppBar; keep status + view-mode | 2 |
| Roomy empty / compact with messages / expand on focus | 3 |
| Grow with content up to maxLines then scroll | 1 + 3 (`maxLines: 8`) |
| Attach + mic disabled, tooltip Coming soon | 4 |
| Enter send / Shift+Enter newline | 4 |
| Keep picker keys; update tests | 1–2 |
| No backend / no mobile pills / no third placeholder | — (non-goals) |

## Self-review notes

- No TBD placeholders left in steps.
- `composerMinLines` signature is consistent across Task 3 tests and implementation (focused+messages → 3).
- Send control key is `composer-send` everywhere after Task 1; icon may be `Icons.arrow_upward` from Task 2.
- `DropdownButton` retained so existing `widget_test.dart` casts keep working.
