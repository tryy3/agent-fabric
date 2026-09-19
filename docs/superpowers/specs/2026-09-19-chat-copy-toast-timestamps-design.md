# Chat copy toast + message timestamps

**Date:** 2026-09-19  
**Status:** approved for planning  
**Parent:** GitHub issue #25 / PR #29  
**Builds on:** [2026-09-19-chat-copy-actions-design.md](./2026-09-19-chat-copy-actions-design.md)

## Problem

1. Copy feedback uses a default full-width `SnackBar`, which feels like a status bar rather than a notification.
2. Hermes shows a posted datetime next to message actions; our user and assistant footers only have Copy.

## Goals

- Make copy feedback a **floating** snackbar (inset, rounded, not edge-to-edge).
- Show a **locale-aware** timestamp next to Copy on user prompts and assistant message footers.
- Plumb real message times from `ThreadMessage.createdAt` (and stamp live sends) so a future user setting can change formatting without rewiring storage.

## Non-goals

- Timestamp on thinking / tool rows.
- User-facing format preference UI (formatter is swappable later).
- Custom toast overlay system beyond Material floating `SnackBar`.
- Changing Copy placement or tool Full/Output behavior.

## Decision

| Topic | Choice |
|-------|--------|
| Copy feedback | `SnackBarBehavior.floating` with modest margin |
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
| `CopyAction` | Use floating `SnackBar` (behavior + margin); keep early-return empty no-op |
| `formatMessageTimestamp(BuildContext, DateTime)` (or similar under `client/lib/chat/`) | Locale-aware display string; single seam for a future setting |
| `ChatBubble.createdAt` | Optional field; set for user + message kinds |
| `bubblesFromThreadMessage` / live send in `chat_controller` | Populate `createdAt` |
| User footer / `_MessageProse` caption row | Muted timestamp text beside Copy |

## Testing

- Widget/unit: floating snackbar shown after copy (behavior floating).
- Unit: timestamp helper returns a non-empty locale string for a fixed `DateTime`.
- Widget: user and assistant surfaces show timestamp text when `createdAt` is set; omit when null.
- Existing copy / selection / tool tab tests stay green.

## Acceptance

- Copy shows a floating snackbar, not a full-width bar.
- User and assistant footers show locale-aware datetime next to Copy when `createdAt` is present.
- Hydrated threads and live sends both populate times.
