# Chat copy toast + message timestamps

**Date:** 2026-09-19  
**Status:** approved for planning  
**Parent:** GitHub issue #25 / PR #29  
**Builds on:** [2026-09-19-chat-copy-actions-design.md](./2026-09-19-chat-copy-actions-design.md)

## Problem

1. Copy feedback should feel like a normal small notification (top-right toast), not a full-width or bottom snackbar.
2. Hermes shows a posted datetime next to message actions; our user and assistant footers only have Copy.

## Goals

- Make copy feedback a **small top-right overlay toast** (not a SnackBar).
- Show a **locale-aware** timestamp next to Copy on user prompts and assistant message footers.
- Plumb real message times from `ThreadMessage.createdAt` (and stamp live sends) so a future user setting can change formatting without rewiring storage.

## Non-goals

- Timestamp on thinking / tool rows.
- User-facing format preference UI (formatter is swappable later).
- Third-party toast packages.
- Changing Copy placement or tool Full/Output behavior.

## Decision

| Topic | Choice |
|-------|--------|
| Copy feedback | Top-right overlay toast (`showCopyToast`); replaces prior toast on rapid re-copy |
| Timestamp surfaces | User footer + assistant caption row only |
| Timestamp format | Locale-aware via Flutter (`MaterialLocalizations` / `intl` `DateFormat`) |
| Data | `ChatBubble.createdAt` from `ThreadMessage.createdAt`; live bubbles use `DateTime.now()` |
| Missing time | Omit the label (Copy alone) |

## Layout

```text
[ user bubble ───────────── ]
  9 Sep, 10:40 AM   [Copy]     ← locale string; trailing under bubble

Agent prose…
model · provider · tok/s
9 Sep, 10:40 AM   [Copy]
[ Stats ]
```

Exact locale string depends on device locale; English example above is illustrative.

## Components

| Piece | Job |
| --- | --- |
| `CopyAction` / `showCopyToast` | Top-right overlay chip; auto-dismiss ~2s; keep early-return empty no-op |
| `formatMessageTimestamp(BuildContext, DateTime)` (or similar under `client/lib/chat/`) | Locale-aware display string; single seam for a future setting |
| `ChatBubble.createdAt` | Optional field; set for user + message kinds |
| `bubblesFromThreadMessage` / live send in `chat_controller` | Populate `createdAt` |
| User footer / `_MessageProse` caption row | Muted timestamp text beside Copy |

## Testing

- Widget: top-right toast shown after copy (`copy-toast` key); no `SnackBar`.
- Unit: timestamp helper returns a non-empty locale string for a fixed `DateTime`.
- Widget: user and assistant surfaces show timestamp text when `createdAt` is set; omit when null.
- Existing copy / selection / tool tab tests stay green.

## Acceptance

- Copy shows a small top-right toast, not a SnackBar.
- User and assistant footers show locale-aware datetime next to Copy when `createdAt` is present.
- Hydrated threads and live sends both populate times.
