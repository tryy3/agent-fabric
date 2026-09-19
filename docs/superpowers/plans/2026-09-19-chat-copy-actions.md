# Chat Copy Actions Per Container Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add always-visible Copy controls on user, thinking, tool, and assistant surfaces; retarget tool tabs to Full/Output with a structured clipboard dump.

**Architecture:** Extract pure tool formatting + clipboard helpers; add a small shared `CopyAction` widget (clipboard + snackbar); wire footer actions under user/assistant and header actions on thinking/tool; change tool tabs to Full (labeled stacked sections) / Output with default Full.

**Tech Stack:** Flutter (Nix). Work from `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix with `nix develop /home/tryy3/src/agent-fabric -c` and run from `client/`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/tool_format_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-chat-copy-actions-design.md`](../specs/2026-09-19-chat-copy-actions-design.md).
- Always-visible Copy only (no hover gate; no edit/fork).
- Tool clipboard is always the structured dump, independent of tab.
- Empty tool fields use `—` (same sentinel as empty tool UI body today).
- Whole-container copy uses source text (`bubble.text`), not visual markdown transform.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.
- Existing selection / markdown / Stats / keepalive tests must keep passing.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/tool_format.dart` | `formatToolValue`, `formatToolCopyText`, Full-tab section helpers |
| `client/test/chat/tool_format_test.dart` | Unit tests for formatters |
| `client/lib/chat/copy_action.dart` | Shared `CopyAction` icon button |
| `client/test/chat/copy_action_test.dart` | Widget tests for clipboard + snackbar + empty no-op |
| `client/lib/chat/view_modes.dart` | Rename `ToolIOMode.input` → `ToolIOMode.full` |
| `client/lib/chat/agent_bubble.dart` | Full/Output tabs; header/footer Copy wiring |
| `client/lib/chat/chat_screen.dart` | User bubble footer Copy |
| `client/test/chat/agent_bubble_test.dart` | Update Input→Full tests; add copy / default-tab cases |
| `client/test/chat/chat_screen_test.dart` | User copy affordance |

**Interfaces this plan locks:**

```dart
/// Pretty-print tool input/output for UI and clipboard.
String formatToolValue(Object? value);

/// Structured clipboard dump (any tab). Empty fields use `—`.
String formatToolCopyText({
  required String title,
  Object? input,
  Object? output,
});

class CopyAction extends StatelessWidget {
  const CopyAction({
    super.key,
    required this.text,
    this.tooltip = 'Copy',
    this.snackbarMessage = 'Copied',
  });

  final String text;
  final String tooltip;
  final String snackbarMessage;
}
```

Keys:

| Control | Key |
| --- | --- |
| User copy | `copy-user` |
| Thinking copy | `copy-thinking` |
| Tool copy | `copy-tool-<toolCallId>` |
| Assistant copy | `copy-message` |
| Full tab | `tool-tab-full` |
| Output tab | `tool-tab-output` |

---

### Task 1: Tool format helpers

**Files:**
- Create: `client/lib/chat/tool_format.dart`
- Create: `client/test/chat/tool_format_test.dart`
- Test: `client/test/chat/tool_format_test.dart`

**Interfaces:**
- Consumes: `dart:convert` (`jsonDecode`, `JsonEncoder`)
- Produces: `formatToolValue`, `formatToolCopyText`

- [ ] **Step 1: Write the failing unit tests**

Create `client/test/chat/tool_format_test.dart`:

```dart
import 'package:agent_fabric_client/chat/tool_format.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('formatToolValue', () {
    test('null is empty string', () {
      expect(formatToolValue(null), '');
    });

    test('map pretty-prints as indented JSON', () {
      expect(
        formatToolValue({'path': 'notes.txt'}),
        '{\n  "path": "notes.txt"\n}',
      );
    });

    test('JSON string decodes then pretty-prints', () {
      expect(
        formatToolValue('{"path":"notes.txt"}'),
        '{\n  "path": "notes.txt"\n}',
      );
    });

    test('plain string passes through', () {
      expect(formatToolValue('hello'), 'hello');
    });
  });

  group('formatToolCopyText', () {
    test('structured dump with args and output', () {
      expect(
        formatToolCopyText(
          title: 'skill_view',
          input: {'name': 'bike-maintenance'},
          output: {'success': true},
        ),
        'tool: skill_view\n'
        'args: {\n  "name": "bike-maintenance"\n}\n'
        'output:\n'
        '{\n  "success": true\n}',
      );
    });

    test('empty fields use em dash sentinel', () {
      expect(
        formatToolCopyText(title: 'Read file', input: null, output: null),
        'tool: Read file\n'
        'args: —\n'
        'output:\n'
        '—',
      );
    });

    test('empty formatted string uses sentinel', () {
      expect(
        formatToolCopyText(title: 'x', input: '', output: ''),
        'tool: x\n'
        'args: —\n'
        'output:\n'
        '—',
      );
    });
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/tool_format_test.dart'`

Expected: FAIL (library / functions not found)

- [ ] **Step 3: Write minimal implementation**

Create `client/lib/chat/tool_format.dart` by moving the body of today’s private `_formatToolValue` from `agent_bubble.dart` into a public `formatToolValue`, then add:

```dart
import 'dart:convert';

String formatToolValue(Object? value) {
  if (value == null) return '';
  Object? decoded = value;
  if (value is String) {
    try {
      decoded = jsonDecode(value);
    } on FormatException {
      return value;
    }
  }
  if (decoded is Map || decoded is List) {
    return const JsonEncoder.withIndent('  ').convert(decoded);
  }
  return decoded.toString();
}

String formatToolCopyText({
  required String title,
  Object? input,
  Object? output,
}) {
  String section(Object? value) {
    final formatted = formatToolValue(value);
    return formatted.isEmpty ? '—' : formatted;
  }

  return 'tool: $title\n'
      'args: ${section(input)}\n'
      'output:\n'
      '${section(output)}';
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/tool_format_test.dart'`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/tool_format.dart client/test/chat/tool_format_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): add tool format and copy-text helpers

EOF
)"
```

---

### Task 2: CopyAction widget

**Files:**
- Create: `client/lib/chat/copy_action.dart`
- Create: `client/test/chat/copy_action_test.dart`
- Test: `client/test/chat/copy_action_test.dart`

**Interfaces:**
- Consumes: `Clipboard`, `ScaffoldMessenger` snackbar pattern from `markdown_link_dialog.dart`
- Produces: `CopyAction`

- [ ] **Step 1: Write the failing widget tests**

Create `client/test/chat/copy_action_test.dart`:

```dart
import 'package:agent_fabric_client/chat/copy_action.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('copies text and shows snackbar', (tester) async {
    final copied = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>?;
          copied.add(args?['text'] as String? ?? '');
        }
        return null;
      },
    );
    addTearDown(() {
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      );
    });

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: CopyAction(
            key: const Key('copy-test'),
            text: 'hello world',
            snackbarMessage: 'Copied',
          ),
        ),
      ),
    );

    await tester.tap(find.byKey(const Key('copy-test')));
    await tester.pumpAndSettle();

    expect(copied, ['hello world']);
    expect(find.text('Copied'), findsOneWidget);
  });

  testWidgets('empty text is a no-op', (tester) async {
    final copied = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>?;
          copied.add(args?['text'] as String? ?? '');
        }
        return null;
      },
    );
    addTearDown(() {
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      );
    });

    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: CopyAction(key: Key('copy-empty'), text: ''),
        ),
      ),
    );

    await tester.tap(find.byKey(const Key('copy-empty')));
    await tester.pumpAndSettle();

    expect(copied, isEmpty);
    expect(find.text('Copied'), findsNothing);
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/copy_action_test.dart'`

Expected: FAIL (CopyAction not found)

- [ ] **Step 3: Write minimal implementation**

Create `client/lib/chat/copy_action.dart`:

```dart
import 'package:material_ui/material_ui.dart';
import 'package:flutter/services.dart';

class CopyAction extends StatelessWidget {
  const CopyAction({
    super.key,
    required this.text,
    this.tooltip = 'Copy',
    this.snackbarMessage = 'Copied',
  });

  final String text;
  final String tooltip;
  final String snackbarMessage;

  @override
  Widget build(BuildContext context) {
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    return IconButton(
      tooltip: tooltip,
      icon: Icon(Icons.copy_outlined, size: 18, color: muted),
      visualDensity: VisualDensity.compact,
      constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
      padding: EdgeInsets.zero,
      onPressed: text.isEmpty
          ? null
          : () async {
              await Clipboard.setData(ClipboardData(text: text));
              if (!context.mounted) return;
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text(snackbarMessage)),
              );
            },
    );
  }
}
```

Notes:
- Prefer `Icons.copy_outlined` (or `Icons.copy` if outlined missing in material_ui).
- Keep the control compact so it fits activity headers.
- If `onPressed: null` for empty makes the icon look too disabled, keep `onPressed` non-null but return early — either way must satisfy the empty no-op test.

- [ ] **Step 4: Run test to verify it passes**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/copy_action_test.dart'`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/copy_action.dart client/test/chat/copy_action_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): add shared CopyAction control

EOF
)"
```

---

### Task 3: Full / Output tool tabs

**Files:**
- Modify: `client/lib/chat/view_modes.dart`
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`
- Modify: any other references to `ToolIOMode.input` (grep)

**Interfaces:**
- Consumes: `formatToolValue` from Task 1
- Produces: `ToolIOMode.full` (replaces `input`); tabs `tool-tab-full` / `tool-tab-output`; default tab Full

- [ ] **Step 1: Update failing / outdated tests first**

In `client/test/chat/agent_bubble_test.dart`, rewrite `tool call expands to Input/Output tabs` to:

```dart
testWidgets('tool call expands to Full/Output tabs; defaults to Full', (
  tester,
) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('detailed'),
          bubble: const ChatBubble(
            kind: ChatBubbleKind.toolCall,
            toolCallId: 'call_1',
            toolTitle: 'Read file',
            toolStatus: 'completed',
            toolInput: {'path': 'notes.txt'},
            toolOutput: {'content': 'hello'},
          ),
        ),
      ),
    ),
  );

  expect(find.text('Read file'), findsOneWidget);
  await tester.tap(find.byKey(const Key('activity-tool-call_1')));
  await tester.pumpAndSettle();

  expect(find.byKey(const Key('tool-tab-full')), findsOneWidget);
  expect(find.byKey(const Key('tool-tab-output')), findsOneWidget);
  // Default Full: args and output both visible as labeled sections.
  expect(find.text('Args'), findsOneWidget);
  expect(find.text('Output'), findsWidgets); // section label and/or tab
  expect(find.textContaining('"path": "notes.txt"'), findsOneWidget);
  expect(find.textContaining('"content": "hello"'), findsOneWidget);

  await tester.tap(find.byKey(const Key('tool-tab-output')));
  await tester.pumpAndSettle();
  expect(find.textContaining('"content": "hello"'), findsOneWidget);
  expect(find.textContaining('"path": "notes.txt"'), findsNothing);
});
```

Also change any `ToolIOMode.input` references in tests to `ToolIOMode.full`.

- [ ] **Step 2: Run test to verify it fails**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart'`

Expected: FAIL on Full tab / default / Args labels (still Input/Output behavior)

- [ ] **Step 3: Implement Full/Output**

1. In `view_modes.dart`:

```dart
enum ToolIOMode { full, output, both }
```

2. In `agent_bubble.dart`:
   - Import `tool_format.dart`; delete private `_formatToolValue`; use `formatToolValue`.
   - Default tab index: always `0` (Full) when `toolIO == both` (remove “prefer Output when output exists”).
   - Map modes:

```dart
final body = switch (widget.toolIO) {
  ToolIOMode.full => _FullToolBody(...),
  ToolIOMode.output => SelectableText(... output ...),
  ToolIOMode.both => _tabIndex == 0
      ? _FullToolBody(...)
      : SelectableText(... output ...),
};
```

3. Tab labels/keys: `Full` / `tool-tab-full`, `Output` / `tool-tab-output`.

4. `_FullToolBody`: Column of labeled sections:

```text
Tool
  <title>
Args
  <monospace selectable formatted input or —>
Output
  <monospace selectable formatted output or —>
```

Use muted `labelLarge` for section titles (`Tool`, `Args`, `Output`). Values via `SelectableText` with monospace (title can be plain `Text` or selectable).

5. Remove auto-switch-to-output in `didUpdateWidget` when output arrives (default stays Full; user can switch manually).

- [ ] **Step 4: Run tests**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart test/chat/view_modes_test.dart'`

Expected: PASS. Fix any compile breaks from `ToolIOMode.input` rename across the repo (`rg 'ToolIOMode\.input'`).

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/view_modes.dart client/lib/chat/agent_bubble.dart client/test/chat/agent_bubble_test.dart
# plus any other files that referenced ToolIOMode.input
git commit -m "$(cat <<'EOF'
feat(chat): switch tool tabs to Full/Output

EOF
)"
```

---

### Task 4: Copy on thinking and tool headers

**Files:**
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`

**Interfaces:**
- Consumes: `CopyAction`, `formatToolCopyText`
- Produces: keys `copy-thinking`, `copy-tool-<id>`

- [ ] **Step 1: Write failing tests**

Add to `agent_bubble_test.dart`:

```dart
testWidgets('thinking copy copies full text without expanding', (tester) async {
  final copied = <String>[];
  tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
    SystemChannels.platform,
    (call) async {
      if (call.method == 'Clipboard.setData') {
        final args = call.arguments as Map<dynamic, dynamic>?;
        copied.add(args?['text'] as String? ?? '');
      }
      return null;
    },
  );
  addTearDown(() {
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      null,
    );
  });

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
          bubble: ChatBubble(
            kind: ChatBubbleKind.thought,
            text: 'hmm\nmore detail',
          ),
        ),
      ),
    ),
  );

  expect(find.text('hmm\nmore detail'), findsNothing);
  await tester.tap(find.byKey(const Key('copy-thinking')));
  await tester.pumpAndSettle();
  expect(copied, ['hmm\nmore detail']);
  expect(find.text('hmm\nmore detail'), findsNothing); // still collapsed
});

testWidgets('tool copy dumps structured text regardless of tab', (tester) async {
  final copied = <String>[];
  tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
    SystemChannels.platform,
    (call) async {
      if (call.method == 'Clipboard.setData') {
        final args = call.arguments as Map<dynamic, dynamic>?;
        copied.add(args?['text'] as String? ?? '');
      }
      return null;
    },
  );
  addTearDown(() {
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      null,
    );
  });

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('detailed'),
          bubble: const ChatBubble(
            kind: ChatBubbleKind.toolCall,
            toolCallId: 'call_1',
            toolTitle: 'skill_view',
            toolInput: {'name': 'bike-maintenance'},
            toolOutput: {'success': true},
          ),
        ),
      ),
    ),
  );

  await tester.tap(find.byKey(const Key('activity-tool-call_1')));
  await tester.pumpAndSettle();
  await tester.tap(find.byKey(const Key('tool-tab-output')));
  await tester.pumpAndSettle();

  await tester.tap(find.byKey(const Key('copy-tool-call_1')));
  await tester.pumpAndSettle();

  expect(
    copied.single,
    formatToolCopyText(
      title: 'skill_view',
      input: {'name': 'bike-maintenance'},
      output: {'success': true},
    ),
  );
});
```

Import `flutter/services.dart` and `tool_format.dart` in the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart --name copy'`

Expected: FAIL (missing keys)

- [ ] **Step 3: Wire CopyAction into headers**

In thinking and tool header `Row`s, insert before the chevron:

```dart
CopyAction(
  key: const Key('copy-thinking'), // or Key('copy-tool-${id}')
  text: /* thought text OR formatToolCopyText(...) */,
),
```

Critical: wrap so the IconButton tap does **not** hit the parent `InkWell` `onTap` (IconButton usually wins hit-test; if expand still toggles, wrap with `GestureDetector(onTap: () {}, behavior: HitTestBehavior.opaque, child: CopyAction(...))` or similar).

Tool title for copy: `widget.bubble.toolTitle ?? 'Tool call'`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart'`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/agent_bubble.dart client/test/chat/agent_bubble_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): add copy actions on thinking and tool rows

EOF
)"
```

---

### Task 5: Copy on assistant caption and user bubble

**Files:**
- Modify: `client/lib/chat/agent_bubble.dart` (`_MessageProse`)
- Modify: `client/lib/chat/chat_screen.dart` (user bubble)
- Modify: `client/test/chat/agent_bubble_test.dart`
- Modify: `client/test/chat/chat_screen_test.dart`

**Interfaces:**
- Consumes: `CopyAction`
- Produces: keys `copy-message`, `copy-user`

- [ ] **Step 1: Write failing tests**

Add to `agent_bubble_test.dart`:

```dart
testWidgets('message copy copies source text next to caption', (tester) async {
  final copied = <String>[];
  tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
    SystemChannels.platform,
    (call) async {
      if (call.method == 'Clipboard.setData') {
        final args = call.arguments as Map<dynamic, dynamic>?;
        copied.add(args?['text'] as String? ?? '');
      }
      return null;
    },
  );
  addTearDown(() {
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      null,
    );
  });

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('detailed'),
          bubble: const ChatBubble(
            kind: ChatBubbleKind.message,
            text: 'hello answer',
            model: 'm1',
          ),
        ),
      ),
    ),
  );

  expect(find.byKey(const Key('copy-message')), findsOneWidget);
  await tester.tap(find.byKey(const Key('copy-message')));
  await tester.pumpAndSettle();
  expect(copied, ['hello answer']);
});
```

Add a focused test in `chat_screen_test.dart` (or extend an existing pump that shows a user message) asserting `find.byKey(const Key('copy-user'))` and clipboard content equals the user prompt. Reuse the screen’s existing harness for creating a thread + seeding a user bubble if one already exists; otherwise pump a minimal `ChatScreen` with a controller that has one user message.

- [ ] **Step 2: Run tests to verify they fail**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/agent_bubble_test.dart test/chat/chat_screen_test.dart --name copy'`

Expected: FAIL (missing keys)

- [ ] **Step 3: Implement footers**

**Assistant (`_MessageProse`):** change caption into a row:

```dart
Row(
  children: [
    if (caption.isNotEmpty)
      Expanded(
        child: Text(caption, style: /* muted bodySmall */),
      )
    else
      const Spacer(),
    CopyAction(
      key: const Key('copy-message'),
      text: bubble.text,
    ),
  ],
)
```

Keep Stats chip below as today. Show the caption row whenever there is a caption **or** copyable text (always show Copy for non-empty message text; if both empty, omit row).

**User (`chat_screen.dart`):** wrap the bubble + footer:

```dart
Align(
  alignment: Alignment.centerRight,
  child: Column(
    crossAxisAlignment: CrossAxisAlignment.end,
    children: [
      Container( /* existing user bubble with MessageText */ ),
      CopyAction(
        key: const Key('copy-user'),
        text: m.text,
      ),
    ],
  ),
)
```

- [ ] **Step 4: Run full related suite**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test \
  test/chat/tool_format_test.dart \
  test/chat/copy_action_test.dart \
  test/chat/agent_bubble_test.dart \
  test/chat/chat_screen_test.dart \
  test/chat/message_text_test.dart \
  test/chat/selection_transformer_test.dart \
  test/chat/view_modes_test.dart'
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/agent_bubble.dart client/lib/chat/chat_screen.dart \
  client/test/chat/agent_bubble_test.dart client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): add copy actions on user and assistant messages

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| User footer Copy | Task 5 |
| Thinking header Copy (full text, collapsed OK) | Task 4 |
| Tool header Copy (structured dump, any tab) | Task 1 + 4 |
| Assistant caption-row Copy | Task 5 |
| Full / Output tabs; default Full; labeled stacked Full body | Task 3 |
| Empty fields `—` | Task 1 |
| Snackbar feedback; empty no-op | Task 2 |
| Tap copy must not expand activity | Task 4 |
| No edit/fork/hover/Stats-copy | (non-goals; not implemented) |

## Self-review notes

- No TBD/placeholder steps; commands and code are concrete.
- `ToolIOMode.full` naming is consistent across Tasks 3–5.
- Copy keys match the File Structure table.
- `formatToolCopyText` is the single source of truth for tool clipboard shape.
