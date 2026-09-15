# Client WebSocket connectivity (cross-platform + reconnect + online UI)

**Date:** 2026-09-15  
**Status:** approved for planning  
**Parent docs:** [architecture.md](../../architecture.md), [flutter client ACP chat](./2026-09-11-flutter-client-acp-chat-design.md), [threads history](./2026-09-13-threads-history-design.md)

## Goal

Make the Flutter client’s ACP WebSocket path work on **all Flutter targets** (web, desktop, mobile), add **automatic reconnect with ACP session re-init**, and show a clear **online / reconnecting / offline** indicator so the app stays navigable while offline.

## Non-goals

- Replacing ACP with another protocol
- ACP `loadSession` / persisting live ACP session ids (resume remains `session/new` + `threadId` per threads design)
- Separate HTTP “catalog reachability” probe (ACP WS state is the connectivity signal)
- Application-level ping/pong text messages (use protocol pings on IO only)
- Shipping polished offline caching of catalog beyond “keep last known in-memory data”

## Decision: `web_socket_channel` + thin owned layers

Use `package:web_socket_channel` for the raw socket. Do **not** rely on `WebSocketChannel.connect` alone for keepalive: that cross-platform entry point does not expose `pingInterval`.

Own a thin stack above the library for platform gaps and product policy:

| Layer | Responsibility |
|--------|----------------|
| Platform `openWsSocket` | Dial via channel; IO keepalive + connect timeout; web connect + ready timeout; text-only frames; clean close. **No** reconnect. |
| `WsTransport` | Unchanged role: channel ↔ `acpd` `Transport` framing (`toWire` / `decodeFrame`). |
| `AgentConnection` | Unexpected drop → backoff reconnect → `initialize` → re-`startSession` when a session was bound. |
| `ChatController` + shell UI | Surface connectivity; keep navigation working offline; disable send / session actions when not connected. |

Drop `package:web` once the browser-only `ws_socket_web.dart` implementation is removed (it is the only current consumer).

## Socket layer

### Conditional connect

```
ws_socket.dart
  export stub
    if (dart.library.io) ws_socket_io.dart
    if (dart.library.js_interop) ws_socket_web.dart
```

- **IO:** `IOWebSocketChannel.connect(uri, pingInterval: 30s, connectTimeout: 30s)` then `await channel.ready`.
- **Web:** `WebSocketChannel.connect(uri)` then `await channel.ready.timeout(30s)`. No protocol `pingInterval` (browser API limitation).
- **Stub:** `UnsupportedError` fallback for exotic targets (should not run on normal Flutter builds).

### Shared mapping (`WsSocket`)

Keep the existing `WsSocket` shape (`inbound` / `outbound` / `close`):

- Inbound: accept `String` only; non-text → same `FormatException` as today.
- Outbound: `channel.sink.add(data)`.
- Close: normal closure status via `package:web_socket_channel/status.dart`.

### Keepalive

Enable **`pingInterval: 30s` on IO only**. Protocol pings are answered by typical servers (including gorilla/websocket on the control plane) without app messages. Web remains without protocol keepalive until a future app-level strategy is needed.

## Reconnect + ACP re-init

Reconnect belongs in **`AgentConnection`**, not in the socket helper.

### Trigger

- Unexpected transport/client close while the connection is still wanted (user has connected and has not called `close()`).
- Injected test `Transport` does **not** auto-reconnect unless tests opt in.

### Policy

- Exponential backoff: 1s → 2s → 4s → 8s → 16s → cap **30s**; retry until success or explicit `close()`.
- Each successful dial: create new `WsTransport` → `ClientRole` → `initialize` (same client info as today).
- If a session was bound, remember last `agentId` and `threadId` and call `startSession` again after initialize (throwaway ACP session id; thread history from catalog — same as threads design).
- Re-apply model only if `session/new` did not restore it.
- In-flight prompt: **fail the turn** (do not ignore the drop while `_sending`); after recovery the user can send again. Uncommitted local bubbles remain `ChatController`’s concern.

### Status signals

Expose connectivity clearly enough for UI (either by extending what `ChatController` already mirrors, or a small connection-state stream from `AgentConnection`). Minimum states:

- `connecting` — first connect
- `connected` — ACP online
- `reconnecting` — backoff in progress
- `disconnected` / `error` — offline

## Online / offline UI

### Indicator

- Persistent badge on **`AppShell`** (rail footer or top of main column): dot + short label — **Online** / **Reconnecting…** / **Offline**.
- Chat screen may keep a detail line; shell badge is the source of truth while switching Chat ↔ Settings.

### Offline behavior

- Navigation (rail, settings, thread pane chrome) always works — no blocking dialogs.
- Composer / send / new ACP session actions stay disabled unless `connected` (existing `canSend` / `canSelect*` gates, updated for `reconnecting`).
- Threads / transcript: keep last known in-memory data when present; otherwise empty with a short “You’re offline” empty state. Failed refresh must not crash.
- Settings: pages remain reachable; catalog fetch/mutation failures show inline errors.
- On return to `connected` after reconnect + session rebind, refresh agents/threads as today’s successful connect path does.

ACP WebSocket state is the connectivity signal for the indicator (no separate catalog probe in this work).

## Testing

- Keep existing `WsTransport.loopback` framing tests.
- Unit-test IO/web mapping logic where practical (text rejection, ready/timeout behavior with fakes).
- `AgentConnection` reconnect: fake transport that drops → assert backoff re-init + `startSession` replay of remembered `agentId`/`threadId`; assert `close()` cancels retries.
- `ChatController` / shell: status drives Online/Reconnecting/Offline; `canSend` false while reconnecting/offline; navigation still builds.

## Risks / notes

- Browser WS still cannot set arbitrary handshake headers; auth differences between web and IO remain a future concern.
- Unbounded backoff means a dead server keeps retrying until the user leaves or we later add a give-up — acceptable for this pass; UI must show **Reconnecting…** so it is not silent.
- Cleartext `ws://` may need platform network exceptions outside local desktop; prefer documenting `wss` for non-local deploys (no Android cleartext config in this pass unless required to run).

## Success criteria

1. `WsTransport.connect` works on Flutter web **and** a desktop/IO target against the control plane `/acp` endpoint.
2. Killing the server then bringing it back recovers ACP (`initialize` + bound `session/new`) without restarting the app.
3. Shell shows Online / Reconnecting / Offline correctly.
4. While offline/reconnecting, user can open Settings and Chat chrome; send stays disabled.
5. Existing loopback transport tests still pass; new reconnect tests cover re-init policy.
