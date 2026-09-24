# Workspace pane left of chat

## Goal

On desktop Chat, place the Workspace pane (file tree + editor) between the Threads pane and the main chat area, instead of to the right of chat.

## Layout

When Chat is selected and the workspace pane is open:

`NavigationRail → ThreadPane → WorkspacePane (360px) → Chat (flex)`

When the workspace pane is closed, chat expands into the remaining space as today. Settings keeps its current layout (no threads or workspace columns).

## Behavior unchanged

- Fixed workspace width: **360px**
- Files toggle in the chat header still opens/closes the pane
- Narrow viewports (`width < 720`) still push `WorkspacePage` full-screen
- `WorkspacePane` internals (file explorer, editor/preview, git actions) are unchanged
- No backend or catalog API changes

## Implementation

Reorder children in `client/lib/app_shell.dart` so the workspace `ListenableBuilder` sits after `ThreadPane` and before the `Expanded` IndexedStack. Keep dividers between columns: threads | workspace | chat.

## Testing

- Existing `app_shell_test.dart` “Files toggle opens workspace pane” remains the primary regression check
- Optionally assert that when open, `WorkspacePane` appears to the left of `ChatScreen` in the shell row

## Out of scope

- Resizable panes or different default width
- Moving workspace into `ChatScreen`
- Configurable column order
- Changes to Settings, mobile `WorkspacePage`, or workspace controller logic
