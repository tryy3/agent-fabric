# Flutter client ACP chat slice

**Date:** 2026-09-11  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [decisions.md](../../decisions.md), [controlplane ACP echo design](./2026-09-11-controlplane-acp-echo-design.md)

## Problem

The control plane speaks ACP v1 over WebSocket `/acp` with a scripted echo agent. There is no first-party cockpit yet. We need a Flutter web vertical slice that connects as an ACP Client and proves a simple chat UI against that same endpoint.

## Goals

- Scaffold a Flutter app under `client/` targeting **web** first.
- Speak ACP via the community **`acpd`** SDK (`ClientRole` / `Session`).
- Own a thin **WebSocket transport** adapter if `acpd` / `acpd_http` does not cover browser WebSocket with our NDJSON framing (same role as Go `internal/transport/ws`).
- Hardcode `ws://localhost:8080/acp`; one transcript; text send; stream `agent_message_chunk` updates into an assistant bubble.
- Add Flutter to the Nix flake `devShell` so the client builds reproducibly.
- Prove the path with offline unit tests and a documented manual smoke against the running control plane.

## Non-goals

- Catalog HTTP API / agent picker
- Auth / multi-user
- Editable server URL, reconnect button, cancel-in-flight UI
- Permissions / tool-call / plan UX (stub handlers only)
- Mobile / desktop shipping targets (keep project structure web-capable; do not polish those runners)
- Hand-rolled JSON-RPC without an ACP SDK
- ACP v2 wire format
- Real LLM or provider UI

## Approach

**Chosen:** `acpd` for typed ACP Client + session helpers, plus a thin browser WebSocket `Transport` we own if the package does not already expose a web-safe one that matches control-plane framing (one WS text frame ↔ one NDJSON line).

**Rejected for this slice:**

- Hand-rolled JSON-RPC only — duplicates protocol edge cases the Go side already avoided with an SDK.
- Heavier client architecture (Riverpod/BLoC shell, multi-route settings) — premature for an echo cockpit.
- Pinning `acp_dart` or `acp` instead — less clear Flutter-web / ACP-v1 fit than `acpd` for this monorepo.

## Architecture

```text
Flutter web (client/)              controlplane process
─────────────────────              ────────────────────
ChatScreen
   │
   ▼
ChatController
   │  connect / send / stream text
   ▼
acpd ClientRole + Session
   │
   ▼
WsTransport (browser WS ↔ Transport)
   │
   ▼
ws://localhost:8080/acp  ─────────►  /acp → Go Agent (echo)
```

### Components

| Piece | Responsibility |
| --- | --- |
| `client/` | Flutter web app (Material), monorepo sibling to `controlplane/` |
| `lib/acp/ws_transport.dart` | Implement `acpd` `Transport` over browser WebSocket; NDJSON framing aligned with Go bridge |
| `lib/acp/agent_connection.dart` | `initialize` → `session/new`; expose prompt + update handling |
| `lib/chat/chat_controller.dart` | Connection lifecycle + message list (user + streaming assistant) |
| `lib/chat/chat_screen.dart` | Transcript + input; auto-connect on start; status line |
| flake `devShell` | Add Flutter so `flutter run -d chrome` / `flutter test` work in-repo |

### Runtime flow

1. App starts → `ChatController.connect()` opens WebSocket to `ws://localhost:8080/acp`.
2. `WsTransport` feeds `acpd` `ClientRole`.
3. Client calls `initialize` (ACP v1) then `session/new` (cwd stub + empty `mcpServers`; plane pins echo).
4. User sends text → append user bubble → `session/prompt` with a text content block.
5. Incoming `session/update` `agent_message_chunk`s append/stream into one assistant bubble until the prompt turn completes.
6. Further sends reuse the same session until disconnect.

### UI behavior

- Status: connecting / connected / error / disconnected.
- Disable send while connecting or while a turn is in flight.
- No reconnect control in this slice — refresh the page to retry.

### Errors and lifecycle

- Dial / upgrade failure → error status; empty transcript; send disabled.
- WebSocket drop mid-session → disconnected status; no auto-reconnect.
- Protocol / prompt errors → surface on the status line; UI stays up.
- Stub client handlers for permissions / fs / terminal (no-op or refuse) so the SDK is satisfied even though echo never calls them.

## Repo layout

```text
client/                   # Flutter project root
  lib/
    acp/
      ws_transport.dart
      agent_connection.dart
    chat/
      chat_controller.dart
      chat_screen.dart
    main.dart
  test/
  pubspec.yaml
controlplane/             # existing Go module (unchanged contract)
flake.nix                 # add flutter to devShell
docs/
README.md                 # document server + Flutter web run
```

## Tooling

- Nix flake `devShell`: keep `go` / `gopls`; add Flutter (and any packages required for `flutter test` / Chrome web in this environment).
- Dependency pin: `acpd` from pub.dev; add `acpd_http` only if it is web-safe and matches our framing — otherwise keep WS adapter local.
- No new Cursor skills required for this slice.

## Verification

**Manual**

```text
go -C controlplane run ./cmd/controlplane
# separate terminal, from client/
flutter run -d chrome
# send a message; echoed text streams into the assistant bubble
```

**Automated**

- Unit: `WsTransport` framing against a fake socket / stream pair.
- Unit: `ChatController` (or connection helper) maps fake `session/update` chunks into the message list.
- `flutter test` with no live network or API keys.

## Success criteria

1. Flutter web builds in the Nix shell.
2. Chat completes initialize → session/new → session/prompt against the echo agent.
3. Assistant text streams via `session/update`.
4. Offline unit tests pass.
5. README explains how to run server + Flutter web client.

## Follow-ups (explicitly later)

- Editable server URL, reconnect, cancel-in-flight
- Catalog-driven agent picker
- Permissions / tool-call / plan UI
- Mobile and desktop runners as first-class targets
- Auth
