# Chat scroll hit-target (full-pane scroll, constrained column)

**Date:** 2026-09-19  
**Status:** approved  
**Issue:** [#25](https://github.com/tryy3/agent-fabric/issues/25) (scrolling checklist item only)  
**Builds on:** [2026-09-15-chat-document-layout-design.md](./2026-09-15-chat-document-layout-design.md)

## Problem

The chat transcript uses a centered content column (`ChatDisplaySettings.contentWidth`, 560–1200px). Today the `ListView` itself is wrapped in that `ConstrainedBox`, so the scroll hit-target is only as wide as the column. On a wide window, wheel/trackpad/drag scroll does nothing when the pointer is in the left/right gutters of the chat body—awkward and easy to miss.

## Goals

- Keep the compact, user-configurable content column for message layout.
- Make the scroll surface the **full chat body** (`Expanded` region above the composer): gutters included.
- Leave the composer centered and width-constrained as today.
- Preserve auto-scroll-to-end while sending.

## Non-goals

- Other #25 items: text selection, per-container copy, Markdown rendering, failed-vs-completed tool styling.
- Changing `contentWidth` defaults or Settings UI.
- Scrolling when the pointer is over the composer, AppBar, or side panes.
- Pointer-signal forwarding hacks that only handle wheel/trackpad.

## Approach

**Chosen:** Full-width `ListView`; constrain and center each row’s content (Approach 1).

1. Let the message `ListView` fill the `Expanded` chat body (full pane width).
2. Wrap each list item’s content in a centered `ConstrainedBox(maxWidth: contentWidth)` so the reading measure stays the same.
3. Leave the composer row as `Align` + `ConstrainedBox` outside the list (unchanged).

**Rejected:**

- Outer `Listener` / `PointerScrollEvent` forwarding onto a narrow `ListView` — gutters get wheel only, not drag; more brittle.
- Outer full-width scroll view with a non-lazy inner column — fights `ListView.builder` and adds complexity for the same result.

## Layout

```text
┌──────────────────── chat pane ────────────────────┐
│ ← full-width ListView (scroll hit-target) →       │
│          ┌──── content column (≤ W px) ────┐      │
│          │  [ user bubble ───────────── ]  │      │
│          │  agent / activity rows …        │      │
│          └─────────────────────────────────┘      │
├───────────────────────────────────────────────────┤
│          ┌──── composer (≤ W px) ──────────┐      │
│          └─────────────────────────────────┘      │
└───────────────────────────────────────────────────┘
```

**Amends document-layout:** the *scrollable* is no longer the constrained box; the *content* inside the list is. Composer constraint is unchanged.

## Implementation notes

- Primary change: `client/lib/chat/chat_screen.dart` message-list branch.
- Prefer a small helper (inline or private method/widget) that centers + constrains a child to `contentWidth`, used by every non-shrink list row, so user / agent rows stay aligned.
- Stats rows that already return `SizedBox.shrink()` stay as-is (no width wrapper needed).
- Empty / offline / no-thread placeholders stay full-pane centered text; no scroll change.
- When pane width ≤ `contentWidth`, behavior is identical to today.
- Scrollbar may appear at the pane’s right edge rather than the column edge; that is intended.

## Testing

- Update existing `chat_screen_test` content-width check: assert message content (not the `ListView`) sits under a `ConstrainedBox` with `maxWidth == contentWidth`; composer assertion unchanged.
- Add a widget test that, on a wide surface, the message `ListView` (or its scrollable render object) is wider than `contentWidth`, confirming gutters are part of the scroll surface.
- Manual: wide window, narrow column setting, scroll with pointer in left/right gutter; confirm auto-scroll while sending.

## Acceptance

- Wheel, trackpad, and drag scroll work with the pointer in the chat-body gutters.
- Message column and composer still respect `contentWidth`.
- Auto-scroll while sending still works.
- Other #25 polish items remain open.
