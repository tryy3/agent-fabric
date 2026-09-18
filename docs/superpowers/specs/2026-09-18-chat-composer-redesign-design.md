# Chat composer redesign

**Date:** 2026-09-18  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md)  
**Builds on:** [2026-09-11-flutter-client-acp-chat-design.md](./2026-09-11-flutter-client-acp-chat-design.md), [2026-09-15-chat-document-layout-design.md](./2026-09-15-chat-document-layout-design.md)

## Problem

The Flutter chat input is a single-line `TextField` beside a send button, with agent and model pickers living in the AppBar. Long messages scroll inside a fixed field instead of growing. Empty threads waste vertical space on AppBar controls while the composer stays cramped. Modern agent UIs put selectors and chat actions inside the composer itself.

## Goals

- Ship an expandable multiline **chat composer** that grows with typed content up to a max height, then scrolls internally.
- Use a **roomy** default when the thread has no messages; **compact** (~1 line) once messages exist, expanding again on focus or when text needs more than one line.
- Move **agent** and **model** selection into a desktop toolbar inside the composer.
- Illustrate future chat actions with **disabled** attach and microphone controls (tooltip: “Coming soon”).
- Free AppBar space: keep title, view-mode menu, and connection status; remove agent/model from the AppBar.

## Non-goals

- Implementing file upload, voice input, or any new backend/API for attachments.
- Mobile compact-pill layout (deferred; desktop toolbar only in V1).
- Changing send/selection semantics on `ChatController`.
- Custom size-state machine beyond soft `AnimatedSize` (or equivalent) height transitions.
- A third placeholder action (prompt library, slash commands, etc.).

## Decisions

| Topic | Choice |
|-------|--------|
| Structure | Extract `ChatComposer` widget; wire from `ChatScreen` |
| Toolbar layout | Desktop toolbar: attach, mic, agent, model → spacer → send |
| Mobile pills | Out of scope for V1 |
| AppBar | Status + view-mode stay; agent/model leave |
| Roomy vs compact | Roomy when message count is 0; compact when ≥1 (unless focused / multi-line text) |
| Placeholders | Attach + mic only; disabled + “Coming soon” tooltip |
| Submit keys | Enter sends; Shift+Enter inserts newline |
| Backend | No API or control-plane changes |

## Approach

**Chosen:** Extract `ChatComposer` (Approach 1).

- One bordered, rounded container with multiline field on top and a toolbar row below.
- Agent/model reuse existing controller APIs and keep test keys (`agent-picker`, `model-picker`).
- Height modes derived from message emptiness, focus, and content line count — not scrollability of the transcript.

**Rejected:**

- Inline-only rewrite in `ChatScreen` — harder to test and grows an already large screen file.
- Full animated size controller — more complexity than V1 needs.

## Architecture

```text
ChatScreen
  AppBar
    title, view-mode menu
    slim status line (Connected / errors / agent incomplete)
  body
    message list (unchanged)
    ChatComposer
      TextField (multiline, maxLines + minLines by mode)
      toolbar
        IconButton attach (disabled, tooltip Coming soon)
        IconButton mic (disabled, tooltip Coming soon)
        agent menu (compact)
        model menu (compact)
        Spacer
        send (circular / IconButton)

ChatController  — unchanged contracts (canSend, selectAgent, selectModel, send)
```

### Height modes

| Mode | Condition | Field height |
|------|-----------|--------------|
| Roomy | `messages.isEmpty` | min ~3–4 lines |
| Compact | ≥1 message, unfocused, text fits one line | ~1 line |
| Expanded | focused, or text needs >1 line | grow with content up to ~8–10 lines or ~40% of viewport, then scroll inside field |

Blur + short text + messages present → return to compact with soft animation.

## UI details

```text
┌─────────────────────────────────────────────────┐
│  multiline TextField (hint: Message)            │
│                                                 │
├─────────────────────────────────────────────────┤
│  📎  🎤   │ Agent ▾ │ Model ▾ │           [↑]  │
└─────────────────────────────────────────────────┘
```

- Theme surfaces and outline; single composition, not nested cards.
- Agent/model: compact controls (icon + truncated label + chevron) with the same enablement rules as today (`canSelectAgent`, `canSelectModel`, incomplete agents, missing selection).
- Send uses existing `canSend` / `_submit` flow; clear input after send as today.

## Testing

- Migrate `chat_screen_test.dart` expectations from AppBar pickers to composer keys.
- Cover: roomy empty thread; compact after a message; expand on focus; Enter sends; Shift+Enter does not send; attach/mic disabled with tooltip; agent/model still selectable when allowed.
- Keep content-width constraint tests against the composer container.

## Implementation notes

- New file: `client/lib/chat/chat_composer.dart` (or equivalent under `client/lib/chat/`).
- Slim `ChatScreen` AppBar `bottom` PreferredSize to status-only (height reduced accordingly).
- Prefer `maxLines: null` with constrained height via `ConstrainedBox` / min-max line counts rather than a fixed pixel box alone.
- No new packages required unless keyboard shortcut handling needs a small helper already used elsewhere in the repo.
