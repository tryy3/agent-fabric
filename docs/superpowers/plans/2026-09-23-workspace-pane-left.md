# Workspace Pane Left of Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Place the Workspace pane between Threads and Chat on desktop, keeping a fixed 360px width and existing toggle/mobile behavior.

**Architecture:** Reorder children in `AppShell`'s body `Row` so the workspace `ListenableBuilder` sits after `ThreadPane` and before the `Expanded` chat/settings stack. Move the vertical divider to the trailing edge of the workspace column so separators stay `threads | workspace | chat`.

**Tech Stack:** Flutter (`material_ui`), `flutter_test`.

## Global Constraints

- Desktop column order when workspace is open: `NavigationRail → ThreadPane → WorkspacePane (360px) → Chat (flex)`.
- Workspace width stays **360px**.
- Files toggle and narrow (`width < 720`) full-screen `WorkspacePage` behavior stay unchanged.
- Do not change `WorkspacePane` internals, Settings layout, or any backend/catalog code.
- Spec: `docs/superpowers/specs/2026-09-23-workspace-pane-left-design.md`.

## File map

| File | Role |
|------|------|
| `client/lib/app_shell.dart` | Shell column order; only file with layout change |
| `client/test/app_shell_test.dart` | Widget test asserting workspace is left of chat |

---

### Task 1: Move workspace column left of chat

**Files:**
- Modify: `client/lib/app_shell.dart` (Chat branch of the body `Row`, ~lines 117–156)
- Modify: `client/test/app_shell_test.dart` (extend or add after `Files toggle opens workspace pane`)
- Test: `client/test/app_shell_test.dart`

**Interfaces:**
- Consumes: existing `_workspace.paneOpen`, `_workspace.togglePane()`, `WorkspacePane(controller:)`, `ChatScreen(..., filesOpen:, onToggleFiles:)`.
- Produces: unchanged public APIs; desktop visual order only.

- [ ] **Step 1: Write the failing position assertion**

In `client/test/app_shell_test.dart`, after the existing `Files toggle opens workspace pane` test (or as a new test with the same setup), assert horizontal order once the pane is open:

```dart
  testWidgets('workspace pane opens left of chat', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: _emptyCatalog(),
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    await tester.tap(find.byKey(const Key('files-toggle')));
    await tester.pumpAndSettle();

    expect(find.byType(WorkspacePane), findsOneWidget);
    expect(find.byType(ThreadPane), findsOneWidget);
    expect(find.byType(ChatScreen), findsOneWidget);

    final threadX = tester.getTopLeft(find.byType(ThreadPane)).dx;
    final workspaceX = tester.getTopLeft(find.byType(WorkspacePane)).dx;
    final chatX = tester.getTopLeft(find.byType(ChatScreen)).dx;

    expect(threadX, lessThan(workspaceX));
    expect(workspaceX, lessThan(chatX));
  });
```

- [ ] **Step 2: Run the new test and confirm it fails**

Run:

```bash
cd client && flutter test test/app_shell_test.dart --name 'workspace pane opens left of chat'
```

Expected: FAIL — `workspaceX` is currently greater than `chatX` (pane is still on the right).

- [ ] **Step 3: Reorder shell columns**

In `client/lib/app_shell.dart`, replace the Chat-mode children after the nav divider so workspace comes before the `Expanded` stack. Put the divider **after** the 360px pane (not before it):

```dart
          if (_selectedIndex == 0) ...[
            ThreadPane(controller: widget.controller),
            const VerticalDivider(thickness: 1, width: 1),
          ],
          if (_selectedIndex == 0)
            ListenableBuilder(
              listenable: _workspace,
              builder: (context, _) {
                if (!_workspace.paneOpen) {
                  return const SizedBox.shrink();
                }
                return Row(
                  children: [
                    SizedBox(
                      width: 360,
                      child: WorkspacePane(controller: _workspace),
                    ),
                    const VerticalDivider(thickness: 1, width: 1),
                  ],
                );
              },
            ),
          Expanded(
            child: IndexedStack(
              index: _selectedIndex,
              children: [
                ChatScreen(
                  controller: widget.controller,
                  displaySettings: widget.displaySettings,
                  filesOpen: _workspace.paneOpen,
                  onToggleFiles: _toggleFiles,
                ),
                SettingsPage(
                  catalog: _catalog,
                  displaySettings: widget.displaySettings,
                  appearanceSettings: widget.appearanceSettings,
                ),
              ],
            ),
          ),
```

Remove the former trailing workspace block that sat after `Expanded`.

- [ ] **Step 4: Run app_shell tests**

Run:

```bash
cd client && flutter test test/app_shell_test.dart
```

Expected: all tests PASS, including `workspace pane opens left of chat` and `Files toggle opens workspace pane`.

- [ ] **Step 5: Commit**

```bash
git add client/lib/app_shell.dart client/test/app_shell_test.dart
git commit -m "$(cat <<'EOF'
Move workspace pane left of chat.

EOF
)"
```

---

## Spec coverage (self-review)

| Spec requirement | Task |
|------------------|------|
| Order: Threads → Workspace → Chat | Task 1 |
| Width 360px | Task 1 (unchanged `SizedBox`) |
| Toggle / mobile unchanged | Task 1 (no edits to `_toggleFiles`) |
| Test: toggle still opens pane | Existing test kept; Task 1 adds order assert |
| Out of scope: pane internals, Settings, backend | No tasks touch those |

No placeholders; single-task plan matches single-layout change.
