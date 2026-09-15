# Chat document layout (Hermes-style agent turns)

**Date:** 2026-09-15  
**Status:** implemented  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-14-chat-part-bubbles-design.md](./2026-09-14-chat-part-bubbles-design.md), [2026-09-14-chat-transparency-design.md](./2026-09-14-chat-transparency-design.md)  
**Supersedes:** the **UI** section of chat-part-bubbles (accent-bar agent cards and the Stats bubble as a transcript row). Transcript model kinds, ACP, persistence, and arrival-order rules stay unless noted below.

## Problem

Agent parts currently render as full-width tinted cards (Thinking / answer / Stats). On a wide cockpit that reads as three banners, not a conversation. Colored fills fight the transcript instead of supporting it.

Hermes-style UIs treat the agent turn as a **document stream**: user prompt stays a bubble; agent prose sits on the page; secondary work (thinking, later tools) collapses to slim activity rows; dense metadata lives behind actions, not as another card.

## Goals

- Constrain the chat transcript (and composer) to a centered **content column**, default **720px**, adjustable in Settings.
- Keep the **user** message as a right-aligned bubble.
- Render **agent message** text as plain left-aligned prose on the page background — no card, no accent bar, no tinted fill. (Markdown styling is a later slice.)
- Render **Thinking** as a Hermes-style **activity row**: collapsed = icon + title + short description + chevron; expanded = detail under the row.
- Remove the **Stats** transcript bubble. Keep `model · provider · tok/s` on the answer. Put the dense usage dump behind a **Stats** footer action that opens a popover.
- **Retire** the Stats visibility setting (`collapsed` / `expanded` / `hidden`). Thinking visibility stays.
- Preserve arrival-order transcript parts and control-plane/ACP behavior.

## Non-goals

- Markdown rendering for agent prose (explicit follow-up).
- Tool / MCP / terminal activity rows (reserve the pattern; no widgets this slice).
- Retry, fork, copy, or other footer actions beyond Stats (placeholders out of UI).
- Agent avatar / name header / “Trace: N tools” chrome from Hermes.
- Dark-theme redesign or a full design-system rewrite beyond what this layout needs.
- Changing `parts[]` persistence, ACP event types, or LLM hydrate rules.
- Per-thread width override (one global setting only).

## Approach

**Chosen:** Document stream inside a readable column (Approach A+C from design exploration).

1. Wrap the message list and composer in a centered `ConstrainedBox` whose max width comes from `ChatDisplaySettings`.
2. Replace `AgentBubble` card chrome for `message` with plain `Text` (plus caption).
3. Replace Thinking’s `ExpansionTile` card with a slim activity header + expandable body.
4. Stop emitting a `stats` **widget** in the list. Keep usage data on the bubble model (or on the preceding message) so the footer can open a popover. Prefer **not** inserting a visible list row for stats.

**Rejected:**

- Asymmetric messenger bubbles for agent replies — still feels like chat cards and stretches awkwardly for long prose.
- Grouped turn shell with a shared border around thinking+message+stats — heavier than needed once Stats leaves the stream; activity rows + prose already group visually under the user bubble.
- Keep Stats visibility modes — user chose to retire them in favor of an always-available Stats action when data exists.

## Layout

```text
┌──────────────────── chat pane ────────────────────┐
│                                                   │
│          ┌──── content column (≤ W px) ────┐      │
│          │  [ user bubble ───────────── ]  │      │
│          │  ▸ Thinking  short description  │      │
│          │  Agent prose on page background │      │
│          │  model · provider · tok/s        │      │
│          │  [ Stats ]                      │      │
│          │  ┌ composer ─────────────────┐  │      │
│          │  └───────────────────────────┘  │      │
│          └─────────────────────────────────┘      │
└───────────────────────────────────────────────────┘
```

**Column width `W`:** default `720`. Settings slider range **560–1200**, step **20**. Persist as `chat.contentWidth` (int) in `SharedPreferences`. Apply to both the `ListView` content and the composer row so they share the same reading measure.

## Transcript model

Bubble kinds remain: `user`, `thought`, `message`, `stats` (data still produced by `bubblesFromThreadMessage` / live send).

| Kind | UI this slice |
| --- | --- |
| `user` | Right-aligned bubble (unchanged role; restyle only if needed for the column). |
| `thought` | Activity row. Title `Thinking`. Description = first line of thought text, truncated with ellipsis (~1 line). Body = full thought when expanded. |
| `message` | Plain prose + caption `model · provider · tok/s`. No card. |
| `stats` | **Not a list card.** Controller and `bubblesFromThreadMessage` still produce a `stats` bubble for usage/stopReason. `chat_screen` **skips** rendering that kind as a row. The preceding `message` widget looks ahead (or the screen passes the following stats bubble into the message footer) so the Stats action can open a popover. Do not fold usage onto the message model in this slice. |

Rules unchanged from chat-part-bubbles: arrival order; same-kind append; unknown parts skipped; Thinking **opens** while `streamingThought` is true, then follows Thinking visibility; `thinkingMode: hidden` omits the thought row (data still on the assistant catalog row).

**Stats action:** show only when usage or stopReason is present. Tap opens a popover/dialog listing the same fields `_statsLines` shows today. No separate Settings control.

## Activity row

Collapsed row (single line):

- Leading affordance (icon or compact marker — Material icon is fine this slice).
- **Title** (semibold): `Thinking` for thought parts; later tool rows use the tool name.
- **Description** (muted, ellipsis): type-specific short string. Thinking → first line of thought. (Future: terminal → command; MCP → server/tool name.)
- Trailing chevron for expand/collapse.

Expanded: full body below the row, recessed slightly (fill + padding), still inside the content column — **not** a colored accent-bar card.

Thinking Settings: `collapsed` / `expanded` / `hidden` keep current meanings for activity rows.

## Footer actions

Under each `message` that has completed (or whenever caption/usage exist):

- Inline caption remains: `model · provider · tok/s` (omit missing segments; omit tok/s until known).
- **Stats** control when usage/stopReason exist → popover with dense fields.
- Do **not** ship Retry / Fork placeholders in this slice.

## Settings

| Setting | Behavior |
| --- | --- |
| Thinking | unchanged: collapsed / expanded / hidden |
| Stats visibility | **Removed** from UI and from `ChatDisplaySettings`. Delete `_statsKey` reads/writes; ignore legacy prefs values. |
| Content width | **New** slider: 560–1200, default 720, key `chat.contentWidth` |

## Components

| Piece | Job |
| --- | --- |
| `display_settings.dart` | Drop `stats`; add `contentWidth` + setter; keep `thinking`. |
| `chat_tab.dart` | Remove Stats dropdown; add content-width slider. |
| `agent_bubble.dart` / successor | Thought → activity row; message → plain prose + caption + Stats action; no stats card. |
| `chat_screen.dart` | Center column for list + composer; skip rendering stats rows; wire width from settings. |
| `chat_bubble.dart` / controller | Prefer minimal change: keep `stats` kind for mapping; UI skips the row. Optional cleanup to fold usage onto `message` can be a follow-up if it simplifies tests. |
| Tests | Update widget/settings tests; remove stats-visibility cases; add width + activity-row + Stats popover coverage. |

## Errors and lifecycle

Unchanged: cancel/error drops uncommitted bubbles; GET refresh mapping unchanged; empty GET does not wipe live bubbles.

If a turn has thought + message but no usage/stopReason, there is no Stats action (same gate as today’s missing stats bubble).

## Testing

- Settings: Stats dropdown gone; width persists and rebuilds the column; Thinking modes still affect activity rows.
- Layout: list + composer share max width; user bubble still right-aligned inside the column.
- Thought: collapsed shows title + truncated description; expanded shows full text; `hidden` omits the row; streaming opens the row.
- Message: no accent-bar card; caption present; Stats opens popover with usage lines when data exists; no Stats control when no usage/stopReason.
- Controller: bubble order and GET mapping unchanged (`[…, message, stats]` still). Widget tests assert no Stats **card** in the tree; Stats **action** on the message opens the popover when a following stats bubble has data.

## Out of scope follow-ups

- Markdown in agent prose.
- Tool / MCP activity rows using the same row pattern.
- Retry / Fork / copy actions on the footer.
- Folding `stats` out of the public bubble list entirely (model cleanup).
