# Chat scrollbar stability via SuperListView

**Date:** 2026-09-19  
**Status:** approved  
**Issue:** [#25](https://github.com/tryy3/agent-fabric/issues/25) (extension — scrollbar polish)  
**Branch:** `fix/chat-scroll-hit-target`  
**Builds on:** [2026-09-19-chat-scrollbar-stability-design.md](./2026-09-19-chat-scrollbar-stability-design.md) (stats filtering), [2026-09-19-chat-scroll-hit-target-design.md](./2026-09-19-chat-scroll-hit-target-design.md)

## Problem

Filtering zero-height `stats` slots helped scroll metrics, but the scrollbar thumb still jumps wildly when scrolling threads that mix short bubbles with **long** assistant/markdown rows. Stock `ListView.builder` estimates `maxScrollExtent` from the average of currently measured children; large height variance makes that estimate thrash ([flutter#25652](https://github.com/flutter/flutter/issues/25652)).

Desired UX: first visit / streaming / expand-collapse may still move the thumb; after the user has visited the content once, revisiting without real layout changes should keep the thumb stable.

## Goals

- Replace the message `ListView.builder` with `SuperListView.builder` from `super_sliver_list`, which tracks measured extents for variable-height children.
- Preserve full-pane scroll hit-target, visible-list stats filtering, `_contentColumn`, composer, and auto-scroll-while-sending via the existing `ScrollController`.
- Land on the same PR/branch as the other #25 scroll fixes.

## Non-goals

- DIY height-cache / custom `estimateMaxScrollOffset`.
- Non-lazy full layout of the transcript.
- Perfect thumb stability during streaming or while expanding/collapsing activity rows.
- Indexed jump APIs (`ListController`) unless needed later.

## Approach

**Chosen:** B2 — `super_sliver_list` / `SuperListView.builder`.

1. Add `super_sliver_list` to `client/pubspec.yaml`.
2. Swap `ListView.builder` → `SuperListView.builder` in `chat_screen.dart` with the same builder, `itemCount`, padding, key, and `ScrollController`.
3. Keep `ListController` optional/out of v1; `_scrollToEnd` continues to `jumpTo(maxScrollExtent)`.
4. Leave stats filtering and content-column layout unchanged.

**Rejected:** DIY cache (more maintenance for the same mental model); non-lazy list (unnecessary for expected thread sizes once SuperListView is in).

## Testing

- Existing `chat_screen_test.dart` cases keep passing; finders that cast to `ListView` must target `SuperListView` or use `Key('message-list')` + size/delegate checks as appropriate.
- Manual: multi-turn thread with a long answer — scroll end-to-end once, then up/down without expand/collapse; thumb should be markedly more stable than stock `ListView`.

## Acceptance

- After a full first pass of a static thread, scrollbar thumb does not jump wildly on revisit.
- Streaming / first visit / expand-collapse may still revise extents.
- Prior #25 scroll-gutter and stats-filter behavior remain intact.
