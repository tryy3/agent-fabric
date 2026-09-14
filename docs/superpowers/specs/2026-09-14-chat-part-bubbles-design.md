# Chat transcript as typed part bubbles

**Date:** 2026-09-14  
**Status:** draft for user review  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-14-chat-transparency-design.md](./2026-09-14-chat-transparency-design.md)  
**Supersedes:** that spec’s **Chat UI** section (answer-on-top turn tile). Persistence, ACP, and Settings keys are unchanged.

## Problem

The first transparency slice delivers thought, usage, model, and provider, but the cockpit still treats an assistant turn as one widget: **answer on top**, caption, then Thinking, then Stats. That inverts arrival order, so users read the reply before they see the thought that produced it. Collapsed extras also look like unlabeled text on the page, not chat.

Agent loops will interleave modes (thinking → tool → thinking → other events → answer). A single “assistant turn” tile cannot grow a timeline; each part must be its own row in the transcript as it arrives.

## Goals

- Render the transcript as a **flat stream of bubbles** in arrival order. The user prompt is the grouping; there is no extra rail around a turn.
- Start a new bubble when the ACP event **type changes**; append to the last bubble when the type is the same.
- Keep caption `model · provider · tok/s` on the **answer** bubble; put the dense usage dump in a separate **Stats** bubble that stays collapsible.
- Style agent bubbles as chat cards with a **colored left accent bar** so thinking, answer, and stats are distinguishable when collapsed.
- Reload from existing assistant `parts[]` into the same bubble sequence. Control plane, ACP, and `CommitTurn` stay as they are.

## Non-goals

- Tools, MCP, plans, `sent` parts, or permission UI (types are reserved in the list; no widgets this slice).
- Extra database rows per bubble (still one user row + one assistant row with `parts[]`).
- Changing LLM hydrate (still `role` + `content` only).
- Per-chat visibility override.
- Dark theme or a full design-system rewrite; light Material, accent-bar cards only.
- Auto-scroll settings; auto-scroll to the newest in-flight bubble is in scope.

## Approach

**Chosen:** Flatten `ChatController`’s list into typed bubbles (Approach 1).

The `ListView` is `[user, thought, message, stats, …]`. Live ACP events append or grow the last matching bubble. History maps `ThreadMessage.parts` in order to the same widgets.

**Rejected:**

- Keep `AssistantTurnTile` and only reorder children — the list does not grow when modes change, which is the loop problem.
- Dual model (`ChatMessage` plus a derived view) — extra mapping with no persistence benefit.
- Turn cluster / shared rail — the user prompt already marks the cutover.

## Transcript model

Bubble kinds this slice: `user`, `thought`, `message`, `stats`.

| Kind | Created from | Body | Extra |
| --- | --- | --- | --- |
| `user` | send / user catalog row | prompt text | none |
| `thought` | `AgentThoughtDelta` / `parts` type `thought` | thought text | `streamingThought` |
| `message` | `AgentMessageDelta` / `parts` type `message` | visible answer | `model`, `providerName`, caption tok/s |
| `stats` | `AgentUsageEvent` / `parts` type `usage` | field list + `stopReason` | `TurnUsage` |

Rules:

- New bubble when the incoming kind ≠ last bubble kind (or the list is empty).
- Same kind **appends** text (or replaces usage on the last stats bubble).
- A later thought after a different kind (e.g. tool, then thought) is a **new** thought bubble. Today’s Unsloth path is thought → message → stats.
- Unknown `parts[].type` values are skipped (same as today).
- `hidden` Thinking/Stats **omits** that bubble; data is still on the assistant row.
- Caption is **not** a bubble. It is a footer on the `message` bubble. It is never hidden when Stats is hidden. Omit tok/s until `predictedPerSecond` is known.
- `content` on the assistant catalog row remains the visible answer for LLM hydrate.

## UI

**User:** right-aligned, filled blue, current corner treatment.

**Agent:** left-aligned white card, 12px radii, 4px left bar:

- Thinking — amber bar, label `Thinking`, body = thought.
- Answer — near-black bar, **no** “Answer” label, body = text (`…` until the first chunk), caption footer.
- Stats — gray bar, label `Stats`, body = existing usage lines + `stopReason`.

Settings (unchanged keys and meanings): Thinking/Stats collapsed (default) / expanded / hidden. Thinking **opens** while `streamingThought` is true, then follows the setting. Collapsed still shows the labeled card + bar. ExpansionTile `ValueKey` includes `streamingThought` and the visibility mode, **not** thought text.

While a turn is in flight, auto-scroll the list to the newest bubble.

## Data flow

Control plane and ACP are unchanged: `agent_thought_chunk` / `agent_message_chunk` / `usage_update` → existing `AgentTurnEvent`s → `CommitTurn` with ordered `parts`.

**Live send**

1. Append `user`.
2. Thought deltas: grow last `thought` or append one (`streamingThought: true`).
3. Message deltas: grow last `message` or append one; stamp `model` / `providerName` on create.
4. Usage: append `stats` (or update last stats); copy `predictedPerSecond` onto the answer caption.
5. After `sendPrompt` returns, set `streamingThought: false` on the open thought bubble.

**Reload:** user row → `user` bubble; assistant `parts` in order → thought / message / stats. Message-level `model` / `providerName` / `stopReason` apply to the answer caption and stats.

**Cancel / error:** drop every uncommitted bubble after the last committed user message. No catalog rows.

**GET after send:** non-empty `detail.messages` replaces the list via the same mapping; empty GET leaves live bubbles. Stale GET / `selectThread` generation guards stay as they are.

## Components

| Piece | Job |
| --- | --- |
| `chat_bubble.dart` | `ChatBubble` + kind enum; replaces `ChatMessage` as the list element. |
| `agent_bubble.dart` | Shared accent-bar card (bar color, label, collapse, body). |
| `chat_controller.dart` | `List<ChatBubble> messages`; arrival rules above; GET mapping. |
| `chat_screen.dart` | `ListView` of user bubble vs agent bubble; auto-scroll in flight. |
| Remove | `AssistantTurnTile` as a one-row turn. |
| Unchanged | ACP forwarding, catalog HTTP, Settings → Chat, `VisibilityMode`, `ChatDisplaySettings`. |

## Errors and lifecycle

Same as the parent spec: cancel / stream error / thinking-without-content write **no** message rows. `max_tokens` / `refusal` still commit; `stopReason` lives on the stats bubble.

If the model emits only thought then content with no usage, there is no stats bubble (same gate: usage or stopReason present). Caption may omit tok/s.

## Testing

No new Go tests.

Controller: order `[user, thought, message, stats]`; two thought deltas stay one thought bubble; `streamingThought` false after send; persisted GET parts replace in order; empty GET does not wipe; cancel drops uncommitted bubbles.

Widgets: thought **above** answer; accent bars and labels; answer has caption and no “Answer” label; collapsed thought hides body; `thinkingMode: hidden` omits the thought bubble; caption remains if stats is hidden.

Screen: during `sendHang` after thought + chunk, thinking appears before answer.

## Success criteria

1. After “How are you?”, the user sees Thinking, then the reply, then optional Stats — never the reply stacked above thinking.
2. Caption sits on the reply; Stats is a separate collapsed card.
3. Refresh restores the same bubble order from `parts[]`.
4. Settings hidden/collapsed/expanded still apply; caption survives Stats hidden.
5. A future tool bubble can be inserted in the same list without redesigning a turn tile.
