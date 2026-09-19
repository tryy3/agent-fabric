# Tool Call Status Styling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Distinguish tool completed / pending / failed with quiet status accents, retint thinking to soft cyan, and paint run-stop errors red on the existing chat status line.

**Architecture:** Retint default `ChatColors.thinking` to soft cyan. Decouple tool chrome from thinking by using `ChatColors.stats` for fill/base icon. Map tool status to a small pure helper that returns icon/status colors (amber failed, primary pending, muted completed). Tint `_statusLabel` text with `colorScheme.error` when the label is an error.

**Tech Stack:** Flutter (Nix). Work from `/home/tryy3/src/agent-fabric`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/ui/theme/chat_colors_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-tool-call-status-styling-design.md`](../specs/2026-09-19-tool-call-status-styling-design.md).
- Failed tools = amber **icon + status text only**; fill stays neutral (`stats`).
- Completed tools = gray/neutral — no green success chrome.
- Red = run-stop / transport errors on the **status line only**, never tool rows.
- Thinking defaults become soft cyan (`#ECFEFF` / `#0891B2` light); do not change answer/user/stats defaults.
- No new Appearance pickers; no spinner in v1; no dedicated `ChatColorRole.tool`.
- Keep literal status strings (`completed`, `failed`, …).
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/ui/theme/chat_colors.dart` | Soft-cyan thinking defaults (light + dark) |
| `client/test/ui/theme/chat_colors_test.dart` | Assert new thinking defaults |
| `client/lib/chat/tool_status_style.dart` | Resolve visual + icon/status colors |
| `client/test/chat/tool_status_style_test.dart` | Unit tests for mapping + colors |
| `client/lib/chat/agent_bubble.dart` | Apply tool chrome + status accents; thinking keeps `chat.thinking` |
| `client/test/chat/agent_bubble_test.dart` | Widget asserts for completed/failed/pending + thinking cyan |
| `client/lib/chat/chat_screen.dart` | Error color on status line when label is an error |
| `client/test/chat/chat_screen_test.dart` | Status line error color |

**Interfaces:**

```dart
enum ToolStatusVisual { completed, pending, failed }

ToolStatusVisual resolveToolStatusVisual({
  required String? status,
  required bool streaming,
});

Color toolFailedAmber(Brightness brightness);

Color toolStatusIconColor({
  required ToolStatusVisual visual,
  required ChatColors chat,
  required ColorScheme scheme,
  required Brightness brightness,
});

Color toolStatusLabelColor({
  required ToolStatusVisual visual,
  required ColorScheme scheme,
  required Brightness brightness,
  required Color muted,
});

bool isChatStatusErrorLabel(ChatController c); // or inline predicate in ChatScreen
```

---

### Task 1: Soft-cyan thinking defaults

**Files:**
- Modify: `client/lib/ui/theme/chat_colors.dart`
- Modify: `client/test/ui/theme/chat_colors_test.dart`

**Interfaces:**
- Produces: new `ChatColors.light().thinking` / `ChatColors.dark().thinking` defaults

- [ ] **Step 1: Update failing expectations in `chat_colors_test.dart`**

Replace thinking expectations:

```dart
test('light defaults match current bubble palette', () {
  final c = ChatColors.light();
  expect(c.thinking.fill, const Color(0xFFECFEFF));
  expect(c.thinking.bar, const Color(0xFF0891B2));
  // answer / stats / user unchanged:
  expect(c.answer.fill, const Color(0xFFCCFBF1));
  expect(c.answer.bar, const Color(0xFF0F766E));
  expect(c.stats.fill, const Color(0xFFE4E4E7));
  expect(c.stats.bar, const Color(0xFF71717A));
  expect(c.user.fill, const Color(0xFFBBDEFB));
  expect(c.user.bar, const Color(0xFF2196F3));
});

test('dark defaults use distinct darker fills', () {
  final c = ChatColors.dark();
  expect(c.thinking.fill, const Color(0xFF083344));
  expect(c.thinking.bar, const Color(0xFF22D3EE));
  expect(c.answer.fill, const Color(0xFF134E4A));
  expect(c.answer.bar, const Color(0xFF2DD4BF));
  expect(c.stats.fill, const Color(0xFF3F3F46));
  expect(c.stats.bar, const Color(0xFFA1A1AA));
  expect(c.user.fill, const Color(0xFF1E3A5F));
  expect(c.user.bar, const Color(0xFF60A5FA));
});
```

Also update the `withOverride` test’s unchanged-bar assertion from `0xFFD97706` to `0xFF0891B2`.

- [ ] **Step 2: Run test — expect FAIL**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/ui/theme/chat_colors_test.dart'`

Expected: FAIL (actual still amber)

- [ ] **Step 3: Retint thinking defaults**

In `chat_colors.dart`:

```dart
factory ChatColors.light() {
  return const ChatColors(
    thinking: RoleColors(fill: Color(0xFFECFEFF), bar: Color(0xFF0891B2)),
    answer: RoleColors(fill: Color(0xFFCCFBF1), bar: Color(0xFF0F766E)),
    stats: RoleColors(fill: Color(0xFFE4E4E7), bar: Color(0xFF71717A)),
    user: RoleColors(fill: Color(0xFFBBDEFB), bar: Color(0xFF2196F3)),
  );
}

factory ChatColors.dark() {
  return const ChatColors(
    thinking: RoleColors(fill: Color(0xFF083344), bar: Color(0xFF22D3EE)),
    answer: RoleColors(fill: Color(0xFF134E4A), bar: Color(0xFF2DD4BF)),
    stats: RoleColors(fill: Color(0xFF3F3F46), bar: Color(0xFFA1A1AA)),
    user: RoleColors(fill: Color(0xFF1E3A5F), bar: Color(0xFF60A5FA)),
  );
}
```

Leave `color_presets.dart` amber swatches alone (still valid picker choices).

- [ ] **Step 4: Run tests — expect PASS**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/ui/theme/chat_colors_test.dart test/ui/theme/app_theme_test.dart test/settings/appearance_settings_test.dart'`

- [ ] **Step 5: Commit**

```bash
git add client/lib/ui/theme/chat_colors.dart client/test/ui/theme/chat_colors_test.dart
git commit -m "$(cat <<'EOF'
feat(theme): retint thinking defaults to soft cyan

EOF
)"
```

---

### Task 2: Tool status style helper + AgentBubble wiring

**Files:**
- Create: `client/lib/chat/tool_status_style.dart`
- Create: `client/test/chat/tool_status_style_test.dart`
- Modify: `client/lib/chat/agent_bubble.dart`
- Modify: `client/test/chat/agent_bubble_test.dart`

**Interfaces:**
- Consumes: `ChatBubble.toolStatus`, `ChatBubble.streamingTool`, `ChatColors.stats`
- Produces: `resolveToolStatusVisual`, `toolFailedAmber`, `toolStatusIconColor`, `toolStatusLabelColor`

- [ ] **Step 1: Write failing unit + widget tests**

Create `client/test/chat/tool_status_style_test.dart`:

```dart
import 'package:agent_fabric_client/chat/tool_status_style.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  test('maps failed / completed / pending statuses', () {
    expect(
      resolveToolStatusVisual(status: 'failed', streaming: false),
      ToolStatusVisual.failed,
    );
    expect(
      resolveToolStatusVisual(status: 'completed', streaming: false),
      ToolStatusVisual.completed,
    );
    expect(
      resolveToolStatusVisual(status: 'pending', streaming: false),
      ToolStatusVisual.pending,
    );
    expect(
      resolveToolStatusVisual(status: 'in_progress', streaming: false),
      ToolStatusVisual.pending,
    );
    expect(
      resolveToolStatusVisual(status: null, streaming: true),
      ToolStatusVisual.pending,
    );
    expect(
      resolveToolStatusVisual(status: 'unknown', streaming: false),
      ToolStatusVisual.completed,
    );
  });

  test('icon and label colors follow visual', () {
    final chat = ChatColors.light();
    const scheme = ColorScheme.light();
    expect(
      toolStatusIconColor(
        visual: ToolStatusVisual.completed,
        chat: chat,
        scheme: scheme,
        brightness: Brightness.light,
      ),
      chat.stats.bar,
    );
    expect(
      toolStatusIconColor(
        visual: ToolStatusVisual.pending,
        chat: chat,
        scheme: scheme,
        brightness: Brightness.light,
      ),
      scheme.primary,
    );
    expect(
      toolStatusIconColor(
        visual: ToolStatusVisual.failed,
        chat: chat,
        scheme: scheme,
        brightness: Brightness.light,
      ),
      toolFailedAmber(Brightness.light),
    );
    expect(
      toolStatusLabelColor(
        visual: ToolStatusVisual.failed,
        scheme: scheme,
        brightness: Brightness.light,
        muted: const Color(0xFF888888),
      ),
      toolFailedAmber(Brightness.light),
    );
    expect(
      toolStatusLabelColor(
        visual: ToolStatusVisual.completed,
        scheme: scheme,
        brightness: Brightness.light,
        muted: const Color(0xFF888888),
      ),
      const Color(0xFF888888),
    );
  });
}
```

Append to `agent_bubble_test.dart`:

```dart
testWidgets('tool completed uses stats chrome; failed tints icon+status amber', (
  tester,
) async {
  final light = ChatColors.light();
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('detailed'),
          bubble: const ChatBubble(
            kind: ChatBubbleKind.toolCall,
            toolCallId: 'ok',
            toolTitle: 'read',
            toolStatus: 'completed',
            streamingTool: false,
          ),
        ),
      ),
    ),
  );
  final completedIcon = tester.widget<Icon>(find.byIcon(Icons.build_outlined));
  expect(completedIcon.color, light.stats.bar);

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('detailed'),
          bubble: const ChatBubble(
            kind: ChatBubbleKind.toolCall,
            toolCallId: 'bad',
            toolTitle: 'bash',
            toolStatus: 'failed',
            streamingTool: false,
          ),
        ),
      ),
    ),
  );
  final failedIcon = tester.widget<Icon>(find.byIcon(Icons.build_outlined));
  expect(failedIcon.color, const Color(0xFFD97706));
  final status = tester.widget<Text>(find.text('failed'));
  expect(status.style?.color, const Color(0xFFD97706));

  await tester.tap(find.byKey(const Key('activity-tool-bad')));
  await tester.pumpAndSettle();
  final fill = tester
      .widgetList<Container>()
      .map((c) => c.decoration)
      .whereType<BoxDecoration>()
      .firstWhere((d) => d.color == light.stats.fill, orElse: () => const BoxDecoration());
  expect(fill.color, light.stats.fill);
});

testWidgets('tool pending uses primary icon color', (tester) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: Scaffold(
        body: AgentBubble(
          viewMode: resolveViewMode('detailed'),
          bubble: const ChatBubble(
            kind: ChatBubbleKind.toolCall,
            toolCallId: 'run',
            toolTitle: 'search',
            toolStatus: 'pending',
            streamingTool: true,
          ),
        ),
      ),
    ),
  );
  final icon = tester.widget<Icon>(find.byIcon(Icons.build_outlined));
  expect(icon.color, AppTheme.light().colorScheme.primary);
});

testWidgets('thinking icon uses cyan thinking.bar', (tester) async {
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
          bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 'hmm'),
        ),
      ),
    ),
  );
  final icon = tester.widget<Icon>(find.byIcon(Icons.lightbulb_outline));
  expect(icon.color, ChatColors.light().thinking.bar);
  expect(icon.color, const Color(0xFF0891B2));
});
```

- [ ] **Step 2: Run tests — expect FAIL**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/tool_status_style_test.dart test/chat/agent_bubble_test.dart'`

Expected: FAIL (missing library / wrong colors)

- [ ] **Step 3: Implement helper**

Create `client/lib/chat/tool_status_style.dart`:

```dart
import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';

enum ToolStatusVisual { completed, pending, failed }

ToolStatusVisual resolveToolStatusVisual({
  required String? status,
  required bool streaming,
}) {
  if (status == 'failed') return ToolStatusVisual.failed;
  if (status == 'completed') return ToolStatusVisual.completed;
  if (streaming ||
      status == null ||
      status == 'pending' ||
      status == 'in_progress') {
    return ToolStatusVisual.pending;
  }
  return ToolStatusVisual.completed;
}

Color toolFailedAmber(Brightness brightness) {
  return brightness == Brightness.dark
      ? const Color(0xFFFBBF24)
      : const Color(0xFFD97706);
}

Color toolStatusIconColor({
  required ToolStatusVisual visual,
  required ChatColors chat,
  required ColorScheme scheme,
  required Brightness brightness,
}) {
  return switch (visual) {
    ToolStatusVisual.failed => toolFailedAmber(brightness),
    ToolStatusVisual.pending => scheme.primary,
    ToolStatusVisual.completed => chat.stats.bar,
  };
}

Color toolStatusLabelColor({
  required ToolStatusVisual visual,
  required ColorScheme scheme,
  required Brightness brightness,
  required Color muted,
}) {
  return switch (visual) {
    ToolStatusVisual.failed => toolFailedAmber(brightness),
    ToolStatusVisual.pending => muted,
    ToolStatusVisual.completed => muted,
  };
}
```

- [ ] **Step 4: Wire `agent_bubble.dart` `_ToolCallActivityState.build`**

Import `tool_status_style.dart`. Replace thinking-based tool chrome with:

```dart
final visual = resolveToolStatusVisual(
  status: widget.bubble.toolStatus,
  streaming: widget.bubble.streamingTool,
);
final iconColor = toolStatusIconColor(
  visual: visual,
  chat: chat,
  scheme: theme.colorScheme,
  brightness: theme.brightness,
);
final statusColor = toolStatusLabelColor(
  visual: visual,
  scheme: theme.colorScheme,
  brightness: theme.brightness,
  muted: muted,
);
```

In the header row:

```dart
Icon(Icons.build_outlined, size: 18, color: iconColor),
// ...
if (widget.bubble.toolStatus case final status?)
  Text(
    status.replaceAll('_', ' '),
    style: theme.textTheme.bodySmall?.copyWith(color: statusColor),
  ),
```

Expanded `Container` decoration:

```dart
decoration: BoxDecoration(
  color: chat.stats.fill,
  borderRadius: BorderRadius.circular(8),
),
```

Leave `_ThoughtActivity` on `chat.thinking` unchanged (already picks up cyan defaults from Task 1).

- [ ] **Step 5: Run tests — expect PASS**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/tool_status_style_test.dart test/chat/agent_bubble_test.dart'`

If the pending-icon assertion fails because `AppTheme.light().colorScheme.primary` differs from the pumped theme, assert against `Theme.of(tester.element(find.byIcon(Icons.build_outlined))).colorScheme.primary` instead.

- [ ] **Step 6: Commit**

```bash
git add client/lib/chat/tool_status_style.dart \
  client/test/chat/tool_status_style_test.dart \
  client/lib/chat/agent_bubble.dart \
  client/test/chat/agent_bubble_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): style tool status with amber fail and neutral chrome

EOF
)"
```

---

### Task 3: Red status line for run-stop errors

**Files:**
- Modify: `client/lib/chat/chat_screen.dart`
- Modify: `client/test/chat/chat_screen_test.dart`

**Interfaces:**
- Consumes: `ChatController.status`, `ChatController.statusMessage`, `_statusLabel`
- Produces: error-colored status `Text` when the label is an error

- [ ] **Step 1: Write failing widget test**

In `chat_screen_test.dart`, add:

```dart
testWidgets('error status line uses colorScheme.error', (tester) async {
  final fake = FakeConn()..failConnect = true;
  final c = ChatController(
    session: fake,
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  addTearDown(c.dispose);
  await c.connect(); // sets ChatStatus.error

  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: ChatScreen(controller: c, displaySettings: displaySettings),
    ),
  );
  await tester.pumpAndSettle();

  final errorText = find.textContaining('Error:');
  expect(errorText, findsOneWidget);
  final style = tester.widget<Text>(errorText).style;
  expect(style?.color, AppTheme.light().colorScheme.error);
});
```

If `failConnect` does not leave a visible Error label in this harness, set `c.status` / `c.statusMessage` the same way other controller tests do after a failed connect, or select an agent then force `status = ChatStatus.error` via a known failure path already covered in `chat_controller_test.dart`. Prefer asserting on whatever `_statusLabel` actually renders (`Error: …`).

Also add a smoke check that `Connected` does **not** use error color:

```dart
testWidgets('connected status line is not error-colored', (tester) async {
  final fake = FakeConn();
  final c = ChatController(
    session: fake,
    catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
  );
  addTearDown(c.dispose);
  await c.connect();
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.light(),
      home: ChatScreen(controller: c, displaySettings: displaySettings),
    ),
  );
  await tester.pumpAndSettle();
  final connected = tester.widget<Text>(find.text('Connected'));
  expect(connected.style?.color, isNot(AppTheme.light().colorScheme.error));
});
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart --name "error status line"'`

Expected: FAIL (status text still uses default `bodySmall` color)

- [ ] **Step 3: Tint status line on errors**

In `chat_screen.dart`, replace the status `Text` with:

```dart
child: Text(
  _statusLabel(c),
  style: Theme.of(context).textTheme.bodySmall?.copyWith(
    color: _isErrorStatus(c)
        ? Theme.of(context).colorScheme.error
        : null,
  ),
),
```

Add helpers next to `_statusLabel`:

```dart
bool _isErrorStatus(ChatController c) {
  if (c.status == ChatStatus.error) return true;
  if (c.status == ChatStatus.connected && c.statusMessage != null) {
    return true;
  }
  return false;
}
```

Do **not** treat “This agent was deleted” / “needs a provider” as red unless they already go through the Error: path (they do not — leave them default).

- [ ] **Step 4: Run tests — expect PASS**

Run: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/chat_screen_test.dart test/chat/agent_bubble_test.dart test/chat/tool_status_style_test.dart test/ui/theme/chat_colors_test.dart'`

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_screen.dart client/test/chat/chat_screen_test.dart
git commit -m "$(cat <<'EOF'
feat(chat): paint run-stop status line errors in red

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Thinking soft cyan defaults | Task 1 |
| Tool chrome uses stats (not thinking) | Task 2 |
| Completed muted / failed amber icon+status | Task 2 |
| Pending primary icon | Task 2 |
| Status words retained | Task 2 (unchanged strings) |
| Red status line for run-stop errors | Task 3 |
| No Appearance pickers / no spinner / no tool role | All (omitted) |
| Widget tests for states | Tasks 1–3 |

## Self-review notes

- No TBD/placeholder steps; concrete colors and signatures match the spec.
- Amber values reuse the old thinking bars so failure indication stays familiar after the cyan retint.
- `color_presets.dart` intentionally unchanged — amber remains a manual picker option.
