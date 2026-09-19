# Tool call failed vs completed styling

**Date:** 2026-09-19  
**Status:** approved for planning  
**Parent:** GitHub issue #25 (tool call failed vs completed styling), PR #29  
**Builds on:** existing `ChatColors` / activity chrome in `AgentBubble`, chat status line in `ChatScreen`

## Problem

Tool activity rows show a status string, but failed and completed look the same: both reuse thinking’s amber chrome and muted status text. Issue #25 asks for a stronger visual signal for `failed` vs `completed` without treating a denied or bad tool path as a catastrophe. Loud red should mean the agent run actually stopped (control-plane / transport / inference failure), not that one tool returned failure while the model continues.

## Goals

- Make **failed** tools noticeable as soft attention (amber indication).
- Keep **completed** tools visually quiet (gray / neutral), distinct from thinking and prose.
- Keep **pending / in progress** distinct with a mild “active” cue.
- Move **thinking** off amber onto soft cyan so amber is free for tool failure.
- Style the existing chat **status line** in red when the run/connection fails.
- Keep accessibility: status **words** remain; color is secondary.
- Stay coherent in light and dark via `ChatColors` / `ColorScheme`.

## Non-goals

- Full amber chrome (fill + bar) for failed tools.
- Bright green success styling for completed tools.
- New Appearance pickers for failed / pending accents.
- New inline thread error banners separate from the existing status line.
- ACP / control-plane protocol changes.
- Changing default answer / user / stats role hues (except thinking retint and tool decoupling from thinking).

## Decision

| Topic | Choice |
|-------|--------|
| Approach | Hybrid: retint thinking in `ChatColors`; status accents in the tool widget; error color on status line |
| Thinking hue | Soft cyan (mock B: light fill `#ECFEFF`, bar `#0891B2`; dark equivalents) |
| Tool chrome | Reuse `ChatColors.stats` fill for expanded tool body; base icon uses `stats.bar` (not thinking) |
| Dedicated `tool` role | Out of v1 — Appearance “Stats” override may also retint tool chrome; split later if needed |
| Completed | Neutral icon (`stats.bar`) + muted status text |
| Failed | Amber icon + amber status text only; neutral fill |
| Pending / in progress | Neutral chrome; active icon via `colorScheme.primary` (no spinner in v1) |
| Red | Only run-stopping failures on the existing status line (`colorScheme.error`) |
| Status copy | Keep literal status strings (`completed`, `failed`, …) |

## Color semantics

| Signal | Treatment |
|--------|-----------|
| Thinking | Soft cyan fill/bar (replaces amber defaults) |
| Tool completed | Neutral gray chrome; muted status |
| Tool pending | Neutral chrome; primary-colored icon (no spinner in v1) |
| Tool failed | Neutral chrome; amber icon + status word |
| Agent / CP / inference stop | Red on existing status line — never on tool rows |

## Tokens & surfaces

### Tokens

- Update default `ChatColors.thinking` light/dark to soft cyan.
- Stop using `chat.thinking` for `_ToolCallActivity` chrome/icon base.
- Use `chat.stats` for expanded tool fill and the default (completed) icon tone.
- Failed amber is a fixed light/dark pair on a small helper (or `Color(0xFFD97706)` / dark amber equivalent); pending uses `colorScheme.primary`. No new Appearance rows.

### Tool row (`_ToolCallActivity`)

- Collapsed and expanded headers share the same status coloring.
- Map `toolStatus` (and streaming when status is open-ended) to the completed / pending / failed treatments above.
- Unknown terminal statuses other than `completed` / `failed` follow the pending/active treatment while streaming, muted when idle (match existing streaming rules in `chat_controller` / hydration).

### Run-stop errors (`ChatScreen` status line)

- When `ChatStatus.error`, or when showing `Error: …` from `statusMessage`, style that label with `colorScheme.error`.
- Do not add a separate thread banner in this change.

## Testing & acceptance

### Widget tests

- Completed tool: neutral/muted icon + status (not amber, not error red).
- Failed tool: amber icon + status; fill stays neutral.
- Pending/streaming tool: active non-amber icon cue.
- Thinking uses cyan `ChatColors.thinking` defaults (not previous amber).
- Status line uses error color when `ChatStatus.error` / error `statusMessage`.

### Manual

- Light + dark: thinking cyan remains distinct from answer teal.
- Failed tool reads as quiet indication; run-stop error reads as red in the header status line.
- Appearance “Thinking” override still applies only to thinking; tools stay on neutral chrome.

### Done when

- Tool fail ≠ catastrophe; agent/CP/inference stop = red status line; thinking no longer owns amber.

## Open follow-ups (out of this change)

- Add `ChatColorRole.tool` (and optional Appearance picker) if Stats overrides retinting tools becomes unwanted.
- Add a spinner / progress affordance for long-running tools if the primary-colored icon alone is too subtle.
