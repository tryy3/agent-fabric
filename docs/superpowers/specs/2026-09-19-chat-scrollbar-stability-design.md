# Chat scrollbar stability (filter zero-height list slots)

**Date:** 2026-09-19  
**Status:** approved  
**Issue:** [#25](https://github.com/tryy3/agent-fabric/issues/25) (extension — scrollbar polish)  
**Branch:** `fix/chat-scroll-hit-target`  
**Builds on:** [2026-09-19-chat-scroll-hit-target-design.md](./2026-09-19-chat-scroll-hit-target-design.md)

## Problem

While scrolling a mostly static chat thread, the Material scrollbar thumb size jumps a lot. Scroll still works; the chrome is wrong.

Root cause: the transcript uses `ListView.builder`, which estimates `maxScrollExtent` from the average height of laid-out children. `ChatBubbleKind.stats` entries remain in `itemCount` but render as `SizedBox.shrink()`. Those zero-height children poison the average, so the estimated content length (and thus thumb size) swings as different rows are measured. Variable-height real rows (thinking, tools, long answers) contribute too; zero-height slots are the self-inflicted worst case.

Live streaming / expand-collapse still change real content height; some thumb motion there is expected and accepted.

## Goals

- Remove zero-height `stats` slots from the message `ListView` so scroll metrics are estimated only from visible rows.
- Keep Stats footer data wiring: a `message` row still receives its following `stats` bubble from the full `messages` list.
- Keep full-pane scroll hit-target, `_contentColumn`, composer, and auto-scroll-while-sending unchanged.
- Land on the same PR/branch as the scroll hit-target fix.

## Non-goals

- Exact, non-estimated scroll extents (non-lazy `Column` / full layout of history).
- Per-kind `itemExtent` / custom extent managers.
- Hiding or restyling the scrollbar.
- Eliminating thumb motion while content is actively growing or collapsing.

## Approach

**Chosen:** Filter visible list children (Approach 1).

1. Build `visible = messages.where((m) => m.kind != ChatBubbleKind.stats).toList()` (or equivalent index mapping) for `ListView.builder`.
2. Drop the `SizedBox.shrink()` branch for stats in the item builder — stats never appear as items.
3. When rendering a `message` kind, resolve `stats` by finding that bubble in the full `messages` list and reading the next element if it is `stats` (same semantics as today’s `index + 1` look-ahead).

**Rejected:** Non-lazy scrollables (cost on long threads); estimated fixed heights per kind (brittle as bubbles evolve).

## Testing

- Existing “stats bubble is not shown as a card; Stats action is on message” still passes.
- Assert the message `ListView`’s child count / built children exclude `stats` kinds when a turn includes usage stats (no zero-height list slots).
- Manual: scroll a finished multi-turn thread with mixed bubble heights; thumb should be noticeably more stable than before. Streaming may still move the thumb.

## Acceptance

- Finished-thread scrollbar thumb no longer jumps wildly from zero-height list slots.
- Stats action still available when usage data exists.
- Scroll gutters + content width behavior from the parent design remain intact.
