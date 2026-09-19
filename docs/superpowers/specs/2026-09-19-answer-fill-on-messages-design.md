# Use ChatColors.answer on assistant message body

**Date:** 2026-09-19  
**Status:** approved for planning  
**Parent:** PR #29 / follow-up to tool status styling  
**Builds on:** document layout `_MessageProse`, existing `ChatColors.answer` Appearance token

## Problem

`ChatColors.answer` is customizable in Appearance and shown in the settings preview, but the live chat transcript never reads it. Assistant messages render as plain on-surface text, so changing Answer colors has no effect in the conversation.

## Goals

- Apply `chat.answer.fill` behind the assistant **message body** only.
- Keep caption, Copy, and Stats outside the fill (current footer layout).
- Respect Appearance Answer overrides automatically via the theme extension.
- Keep light/dark defaults coherent with existing answer tokens.

## Non-goals

- Left accent bar (`answer.bar`) on messages.
- Wrapping caption / Copy / Stats inside the fill.
- Changing user, thinking, tool, or stats chrome.
- New Appearance controls.

## Decision

| Topic | Choice |
|-------|--------|
| Treatment | Soft fill behind message text only (Approach 1) |
| Fill color | `Theme.extension<ChatColors>().answer.fill` |
| Bar | Unused in transcript for now |
| Padding / radius | ~12 padding, 8 radius (align with user bubble) |
| Streaming placeholder | Same fill around `…` |
| Footer | Caption, Copy, Stats remain outside below the fill |

## Surfaces

### `_MessageProse` (`agent_bubble.dart`)

Wrap `MessageText` in:

```dart
Container(
  width: double.infinity,
  padding: const EdgeInsets.all(12),
  decoration: BoxDecoration(
    color: chat.answer.fill,
    borderRadius: BorderRadius.circular(8),
  ),
  child: MessageText(...),
)
```

Leave the caption/Copy row and Stats action as siblings below, unchanged.

### Appearance

No settings UI changes. Preview already uses `answer.fill`; live chat now matches that intent for the body.

## Testing

- Widget test: assistant message body container uses `ChatColors.light().answer.fill` (and/or dark).
- Widget test: Copy / Stats are not descendants of that fill container (or remain findable outside it).
- Manual: Appearance Answer fill override retints assistant bodies after theme rebuild.

## Done when

Changing Appearance → Answer fill visibly changes assistant message bodies; caption/Copy/Stats stay unfilled.
