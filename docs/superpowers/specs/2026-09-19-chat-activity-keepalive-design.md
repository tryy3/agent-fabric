# Chat activity expand keep-alive

**Date:** 2026-09-19  
**Status:** approved  
**Issue:** [#25](https://github.com/tryy3/agent-fabric/issues/25) (extension)  
**Branch:** `fix/chat-scroll-hit-target`

## Problem

Expanding a thinking/tool activity, scrolling it off-screen, then scrolling back collapses it. `_expanded` lives in `State`; lazy lists dispose off-screen children.

## Goals

- Remember expand (and tool tab) state while scrolling within the same thread.
- Forgetting on thread switch is fine (list rebuild disposes keep-alives).

## Non-goals

- Changing the intentional “open while streaming, then follow collapsed default” behavior (driven by `ValueKey` including `streamingThought`).
- Persisting expand across app restarts.
- Lifting expand state into `ChatController`.

## Approach

`AutomaticKeepAliveClientMixin` with `wantKeepAlive => true` on `_ThoughtActivityState` and `_ToolCallActivityState`; call `super.build(context)` in `build`.

## Testing

Widget test: expand an activity, scroll it off the message list, scroll back — still expanded.
