# Client WebSocket Connectivity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ACP WebSockets work on all Flutter targets via `web_socket_channel`, auto-reconnect with ACP session re-init, and show Online / Reconnecting / Offline in the shell while keeping navigation usable offline.

**Architecture:** Platform `openWsSocket` dials with `web_socket_channel` (IO gets `pingInterval` + `connectTimeout`; web gets `ready` timeout only). `WsTransport` stays framing-only. `AgentConnection` owns backoff reconnect + `initialize` / `startSession` replay. `ChatController` mirrors connection state; `AppShell` shows a persistent connectivity badge.

**Tech Stack:** Flutter, `web_socket_channel` ^3.0.3, `acpd` ^1.0.0, existing Material/`material_ui` shell.

## Global Constraints

- Spec: [`2026-09-15-client-websocket-connectivity-design.md`](../specs/2026-09-15-client-websocket-connectivity-design.md).
- IO keepalive: `pingInterval: 30s`, `connectTimeout: 30s`.
- Web: `channel.ready.timeout(30s)`; no protocol ping.
- Backoff: 1s → 2s → 4s → 8s → 16s → cap 30s; retry until `close()`.
- Injected `connect(transport: …)` does **not** auto-reconnect.
- Resume = new `session/new` with remembered `agentId` + `threadId` (no ACP `loadSession`).
- Drop `package:web` after socket migration (only consumer today).
- ACP WS state is the online/offline signal (no separate catalog probe).
- Follow TDD: failing test → implement → pass → commit per task.
- Work in repo root `client/` (not an unrelated worktree unless the user says otherwise).

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/pubspec.yaml` | Add `web_socket_channel`; remove `web` |
| `client/lib/acp/ws_socket_stub.dart` | `WsSocket` type + stub `openWsSocket` |
| `client/lib/acp/ws_socket_bind.dart` | Shared channel → `WsSocket` mapping (text-only) |
| `client/lib/acp/ws_socket_io.dart` | `IOWebSocketChannel.connect` + keepalive |
| `client/lib/acp/ws_socket_web.dart` | `WebSocketChannel.connect` + ready timeout |
| `client/lib/acp/ws_socket.dart` | Conditional export (`io` then `js_interop`) |
| `client/lib/acp/ws_transport.dart` | Unchanged public API (still uses `openWsSocket`) |
| `client/lib/acp/agent_connection.dart` | `AcpConnectionState`, reconnect, dialer |
| `client/lib/chat/chat_controller.dart` | `ChatStatus.reconnecting`; wire state |
| `client/lib/ui/connectivity_badge.dart` | Online / Reconnecting / Offline chip |
| `client/lib/app_shell.dart` | Host badge; keep rail navigable offline |
| `client/lib/chat/chat_screen.dart` | Offline empty copy; status label includes reconnecting |
| `client/test/acp/ws_socket_bind_test.dart` | Text-only + bind behavior |
| `client/test/acp/agent_connection_test.dart` | Reconnect / cancel / session replay |
| `client/test/chat/chat_controller_test.dart` | Status + `canSend` while reconnecting |
| `client/test/app_shell_test.dart` | Badge + Settings while offline |
| Fakes implementing `AgentSessionApi` | Add `connectionState` stream |

**Interfaces this plan adds** (later tasks consume these names exactly):

```dart
enum AcpConnectionState { disconnected, connecting, connected, reconnecting }

abstract class AgentSessionApi {
  Stream<AcpConnectionState> get connectionState;
  // existing members unchanged…
}

typedef TransportFactory = Future<Transport> Function(Uri uri);

class AgentConnection implements AgentSessionApi {
  AgentConnection({
    Uri? acpUri,
    TransportFactory? transportFactory,
    Duration Function(int attempt)? backoffForAttempt,
  });
}

const kWsConnectTimeout = Duration(seconds: 30);
const kWsPingInterval = Duration(seconds: 30);

Future<WsSocket> bindWsChannel(
  WebSocketChannel channel, {
  Duration? readyTimeout,
});
```

Backoff helper (exact):

```dart
Duration defaultAcpBackoff(int attempt) {
  // attempt is 0-based after the first failure
  const steps = [1, 2, 4, 8, 16, 30];
  final seconds = steps[attempt < steps.length ? attempt : steps.length - 1];
  return Duration(seconds: seconds);
}
```

---

### Task 1: Shared channel bind + platform `openWsSocket`

**Files:**
- Create: `client/lib/acp/ws_socket_bind.dart`
- Create: `client/test/acp/ws_socket_bind_test.dart`
- Modify: `client/pubspec.yaml`
- Modify: `client/lib/acp/ws_socket_stub.dart`
- Modify: `client/lib/acp/ws_socket_io.dart` (create; replace stub-only path)
- Modify: `client/lib/acp/ws_socket_web.dart` (rewrite)
- Modify: `client/lib/acp/ws_socket.dart`
- Delete usage of `package:web` (remove from pubspec after rewrite)

**Interfaces:**
- Consumes: `web_socket_channel` `WebSocketChannel` / `IOWebSocketChannel`
- Produces: `bindWsChannel`, platform `openWsSocket`, constants `kWsConnectTimeout` / `kWsPingInterval`; existing `WsSocket` + `WsTransport.connect` keep working

- [ ] **Step 1: Add dependency**

In `client/pubspec.yaml` under `dependencies`:

```yaml
  web_socket_channel: ^3.0.3
```

Remove:

```yaml
  web: ^1.1.1
```

Run: `cd client && flutter pub get`  
Expected: exit 0; lockfile updated.

- [ ] **Step 2: Write failing bind tests**

Create `client/test/acp/ws_socket_bind_test.dart`:

```dart
import 'dart:async';

import 'package:agent_fabric_client/acp/ws_socket_bind.dart';
import 'package:agent_fabric_client/acp/ws_socket_stub.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

class _FakeChannel implements WebSocketChannel {
  _FakeChannel(this._stream, this.sink);

  final Stream<dynamic> _stream;
  @override
  final WebSocketSink sink;
  final _ready = Completer<void>()..complete();

  @override
  Stream get stream => _stream;

  @override
  Future<void> get ready => _ready.future;

  @override
  int? get closeCode => null;

  @override
  String? get closeReason => null;

  @override
  String? get protocol => null;
}

class _FakeSink implements WebSocketSink {
  final sent = <dynamic>[];
  int? closeCode;
  String? closeReason;

  @override
  void add(dynamic data) => sent.add(data);

  @override
  void addError(Object error, [StackTrace? stackTrace]) {}

  @override
  Future addStream(Stream stream) => throw UnimplementedError();

  @override
  Future close([int? closeCode, String? closeReason]) async {
    this.closeCode = closeCode;
    this.closeReason = closeReason;
  }

  @override
  Future get done => Future.value();
}

void main() {
  test('bindWsChannel forwards text and rejects binary', () async {
    final controller = StreamController<dynamic>();
    final sink = _FakeSink();
    final socket = await bindWsChannel(_FakeChannel(controller.stream, sink));

    final first = socket.inbound.first;
    controller.add('hello');
    expect(await first, 'hello');

    final err = expectLater(
      socket.inbound,
      emitsError(isA<FormatException>()),
    );
    controller.add(<int>[1, 2, 3]);
    await err;

    socket.outbound('out');
    expect(sink.sent, ['out']);
    await socket.close();
    expect(sink.closeCode, isNotNull);
    await controller.close();
  });
}
```

If `_FakeChannel` cannot implement `WebSocketChannel` cleanly (abstract members differ by version), use a small test-only wrapper API instead: extract `WsSocket mapChannelStreams({required Stream stream, required void Function(String) add, required Future<void> Function() close})` in `ws_socket_bind.dart` and test that. Prefer testing the pure mapper if the interface is awkward.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd client && flutter test test/acp/ws_socket_bind_test.dart`  
Expected: FAIL — missing library / `bindWsChannel` undefined.

- [ ] **Step 4: Implement bind + platform opens**

`client/lib/acp/ws_socket_bind.dart`:

```dart
import 'dart:async';

import 'package:web_socket_channel/status.dart' as status;
import 'package:web_socket_channel/web_socket_channel.dart';

import 'ws_socket_stub.dart';

const kWsConnectTimeout = Duration(seconds: 30);
const kWsPingInterval = Duration(seconds: 30);

Future<WsSocket> bindWsChannel(
  WebSocketChannel channel, {
  Duration? readyTimeout,
}) async {
  final ready = channel.ready;
  if (readyTimeout != null) {
    await ready.timeout(readyTimeout);
  } else {
    await ready;
  }

  final inbound = StreamController<String>();
  final sub = channel.stream.listen(
    (event) {
      if (event is String) {
        inbound.add(event);
      } else {
        inbound.addError(
          const FormatException(
            'ACP WebSocket messages must use text frames.',
          ),
        );
      }
    },
    onError: inbound.addError,
    onDone: () {
      if (!inbound.isClosed) inbound.close();
    },
    cancelOnError: false,
  );

  return WsSocket(
    inbound: inbound.stream,
    outbound: (data) => channel.sink.add(data),
    close: () async {
      await sub.cancel();
      await channel.sink.close(status.normalClosure);
      if (!inbound.isClosed) await inbound.close();
    },
  );
}
```

`client/lib/acp/ws_socket_io.dart`:

```dart
import 'package:web_socket_channel/io.dart';

import 'ws_socket_bind.dart';
import 'ws_socket_stub.dart';

Future<WsSocket> openWsSocket(Uri uri) async {
  if (uri.scheme != 'ws' && uri.scheme != 'wss') {
    throw ArgumentError.value(uri, 'uri', 'must be ws or wss');
  }
  final channel = IOWebSocketChannel.connect(
    uri,
    pingInterval: kWsPingInterval,
    connectTimeout: kWsConnectTimeout,
  );
  return bindWsChannel(channel);
}
```

`client/lib/acp/ws_socket_web.dart`:

```dart
import 'package:web_socket_channel/web_socket_channel.dart';

import 'ws_socket_bind.dart';
import 'ws_socket_stub.dart';

Future<WsSocket> openWsSocket(Uri uri) async {
  if (uri.scheme != 'ws' && uri.scheme != 'wss') {
    throw ArgumentError.value(uri, 'uri', 'must be ws or wss');
  }
  final channel = WebSocketChannel.connect(uri);
  return bindWsChannel(channel, readyTimeout: kWsConnectTimeout);
}
```

`client/lib/acp/ws_socket.dart`:

```dart
export 'ws_socket_stub.dart'
    if (dart.library.io) 'ws_socket_io.dart'
    if (dart.library.js_interop) 'ws_socket_web.dart';
```

Keep `WsSocket` class in `ws_socket_stub.dart`. Change stub `openWsSocket` message to: `'WsTransport.connect is not supported on this platform.'`

- [ ] **Step 5: Run bind tests + existing transport tests**

Run:

```bash
cd client && flutter test test/acp/ws_socket_bind_test.dart test/acp/ws_transport_test.dart
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add client/pubspec.yaml client/pubspec.lock client/lib/acp/ws_socket*.dart client/test/acp/ws_socket_bind_test.dart
git commit -m "$(cat <<'EOF'
feat(client): cross-platform ACP sockets via web_socket_channel

EOF
)"
```

---

### Task 2: `AcpConnectionState` + reconnect on `AgentConnection`

**Files:**
- Modify: `client/lib/acp/agent_connection.dart`
- Modify: `client/test/acp/agent_connection_test.dart`
- Modify: every `AgentSessionApi` fake (`chat_controller_test.dart`, `chat_screen_test.dart`, `app_shell_test.dart`, `widget_test.dart`, `providers_tab_test.dart`) — add `connectionState` stub so the package analyzes

**Interfaces:**
- Consumes: `WsTransport.connect` / injectable `TransportFactory`
- Produces: `AcpConnectionState`, `connectionState` stream, `defaultAcpBackoff`, reconnect that replays `startSession`

- [ ] **Step 1: Write failing reconnect tests**

Add to `client/test/acp/agent_connection_test.dart`:

```dart
test('dialed connection reconnects and replays startSession after drop', () async {
  var dials = 0;
  final agents = <AgentRole>[];
  late _End agentTransport;

  Future<Transport> factory(Uri uri) async {
    dials++;
    final (clientTransport, agentEnd) = linkedTransports();
    agentTransport = agentEnd;
    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          return NewSessionResponse(
            sessionId: 'sess-$dials',
            meta: request.meta,
          );
        })
        .connect(agentEnd);
    agents.add(agentConn);
    return clientTransport;
  }

  final conn = AgentConnection(
    transportFactory: factory,
    backoffForAttempt: (_) => Duration.zero,
  );

  final states = <AcpConnectionState>[];
  final sub = conn.connectionState.listen(states.add);

  await conn.connect();
  await conn.startSession('ag-1', threadId: 'th-1');
  expect(dials, 1);

  await agentTransport.close();

  await Future<void>.delayed(Duration.zero);
  await pumpEventQueue(); // flutter_test
  // Wait until second dial completes
  for (var i = 0; i < 50 && dials < 2; i++) {
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
  expect(dials, greaterThanOrEqualTo(2));
  expect(states, contains(AcpConnectionState.reconnecting));
  expect(states.last, AcpConnectionState.connected);

  await conn.close();
  expect(states.last, AcpConnectionState.disconnected);
  await sub.cancel();
  for (final a in agents) {
    await a.close();
  }
});

test('injected transport does not auto-reconnect', () async {
  final (clientTransport, agentTransport) = linkedTransports();
  var dials = 0;
  final conn = AgentConnection(
    transportFactory: (_) async {
      dials++;
      throw StateError('should not dial');
    },
  );

  final agentConn = AgentRole()
      .onInitialize((ctx, request, cancellation) async {
        return const InitializeResponse(
          protocolVersion: ProtocolVersion.v1,
          agentInfo: Implementation(name: 'test', version: '0.0.1'),
        );
      })
      .connect(agentTransport);

  await conn.connect(transport: clientTransport);
  await clientTransport.close();
  await conn.closed.first.timeout(const Duration(seconds: 2));
  expect(dials, 0);
  expect(conn.connectionState /* last via listen */, /* disconnected */);

  await conn.close();
  await agentConn.close();
  await agentTransport.close();
});

test('close cancels reconnect loop', () async {
  var dials = 0;
  final conn = AgentConnection(
    transportFactory: (_) async {
      dials++;
      if (dials == 1) {
        final (c, a) = linkedTransports();
        // minimal agent that accepts initialize then we drop
        AgentRole()
            .onInitialize((ctx, request, cancellation) async {
              return const InitializeResponse(
                protocolVersion: ProtocolVersion.v1,
                agentInfo: Implementation(name: 'test', version: '0.0.1'),
              );
            })
            .connect(a);
        return c;
      }
      await Future<void>.delayed(const Duration(seconds: 30));
      throw StateError('hung dial');
    },
    backoffForAttempt: (_) => const Duration(seconds: 30),
  );

  await conn.connect();
  // Force drop by closing underlying — use factory that returns closable ends.
  // Simpler approach: call close() while reconnecting after first drop.
  await conn.close();
  final dialsAfterClose = dials;
  await Future<void>.delayed(const Duration(milliseconds: 50));
  expect(dials, dialsAfterClose);
});
```

Tighten the third test during implementation so it reliably closes mid-backoff without flaking (use `Completer` gates on dial #2).

Also add a small unit test for `defaultAcpBackoff`:

```dart
test('defaultAcpBackoff caps at 30s', () {
  expect(defaultAcpBackoff(0), const Duration(seconds: 1));
  expect(defaultAcpBackoff(5), const Duration(seconds: 30));
  expect(defaultAcpBackoff(99), const Duration(seconds: 30));
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/acp/agent_connection_test.dart`  
Expected: FAIL — `AcpConnectionState` / `connectionState` / factory ctor missing.

- [ ] **Step 3: Implement reconnect**

In `agent_connection.dart`:

1. Add `enum AcpConnectionState { disconnected, connecting, connected, reconnecting }`.
2. Add `defaultAcpBackoff` as above.
3. Extend `AgentSessionApi` with `Stream<AcpConnectionState> get connectionState`.
4. `AgentConnection` fields:
   - `_stateController` broadcast
   - `_wanted` bool
   - `_autoReconnect` bool (true only when dialed via factory)
   - `_lastAgentId` / `_lastThreadId` / `_lastModelId` updated in `startSession` / `setModel`
   - `_reconnectAttempt`
   - `TransportFactory? _transportFactory` (default: `(uri) => WsTransport.connect(uri)`)
   - `Uri _acpUri`
   - `Duration Function(int) _backoffForAttempt`
5. `_setState(AcpConnectionState s)` emit if changed.
6. `connect({Transport? transport})`:
   - cancel any reconnect loop
   - `_wanted = true`
   - `_autoReconnect = transport == null`
   - `_setState(connecting)`
   - dial via transport ?? factory
   - wire client as today
   - on `client.closed`: emit on existing `closed` stream **and** if `_wanted && _autoReconnect` schedule `_reconnect()` else `_setState(disconnected)`
   - after initialize: `_setState(connected)`
7. `_reconnect()`:
   - `_setState(reconnecting)`
   - loop while `_wanted`: delay backoff; try dial+initialize; on success, if `_lastAgentId != null` call `startSession`; if `_lastModelId != null && currentModel != _lastModelId` call `setModel`; `_setState(connected)`; return. On failure increment attempt and continue.
8. `close()`: `_wanted = false`; cancel pending timer/future; `_setState(disconnected)`; existing teardown.
9. In-flight `sendPrompt`: when transport dies, existing session errors should surface to caller (fail the turn). Do not swallow in `_onSessionClosed`-style ignores.

Update all fakes:

```dart
final _connectionState =
    StreamController<AcpConnectionState>.broadcast(sync: true);
AcpConnectionState currentState = AcpConnectionState.disconnected;

@override
Stream<AcpConnectionState> get connectionState => _connectionState.stream;

// in connect():
currentState = AcpConnectionState.connected;
_connectionState.add(currentState);

// in close() / simulateDisconnect for tests that need it:
currentState = AcpConnectionState.disconnected;
_connectionState.add(currentState);
```

- [ ] **Step 4: Run agent_connection + full client tests affected**

Run:

```bash
cd client && flutter test test/acp/agent_connection_test.dart test/chat/chat_controller_test.dart test/app_shell_test.dart test/widget_test.dart
```

Expected: agent reconnect tests PASS; other suites compile/pass with fake stubs (behavior changes come in Task 3).

- [ ] **Step 5: Commit**

```bash
git add client/lib/acp/agent_connection.dart client/test/acp/agent_connection_test.dart \
  client/test/chat/chat_controller_test.dart client/test/chat/chat_screen_test.dart \
  client/test/app_shell_test.dart client/test/widget_test.dart \
  client/test/settings/providers_tab_test.dart
git commit -m "$(cat <<'EOF'
feat(client): auto-reconnect ACP with session re-init

EOF
)"
```

---

### Task 3: `ChatController` reconnecting status + offline gates

**Files:**
- Modify: `client/lib/chat/chat_controller.dart`
- Modify: `client/test/chat/chat_controller_test.dart`
- Modify: `client/lib/chat/chat_screen.dart` (status label)

**Interfaces:**
- Consumes: `AgentSessionApi.connectionState`
- Produces: `ChatStatus.reconnecting`; `canSend` / select gates false unless `connected`

- [ ] **Step 1: Write failing controller tests**

```dart
test('connectionState reconnecting maps to ChatStatus.reconnecting', () async {
  final fake = FakeConn();
  final c = ChatController(session: fake, catalog: FakeCatalog([]));
  await c.connect();
  expect(c.status, ChatStatus.connected);

  fake.emitState(AcpConnectionState.reconnecting);
  expect(c.status, ChatStatus.reconnecting);
  expect(c.canSend, isFalse);

  fake.emitState(AcpConnectionState.connected);
  expect(c.status, ChatStatus.connected);
});

test('disconnect while idle sets disconnected and keeps threads', () async {
  final catalog = FakeCatalog([
    // existing helpers: one thread with messages if available
  ]);
  final fake = FakeConn();
  final c = ChatController(session: fake, catalog: catalog);
  await c.connect();
  // select thread if connect did not
  final threadId = c.selectedThreadId;
  fake.emitState(AcpConnectionState.disconnected);
  fake.simulateDisconnect();
  expect(c.status, ChatStatus.disconnected);
  expect(c.selectedThreadId, threadId);
  expect(c.canSend, isFalse);
});
```

Add `emitState` on `FakeConn` that updates `connectionState`. Change `_onSessionClosed` so it does **not** force disconnected when the session reports `reconnecting` (prefer listening to `connectionState` as source of truth; keep `closed` for logging only or remove the hard disconnect if redundant).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && flutter test test/chat/chat_controller_test.dart --name reconnecting`  
Expected: FAIL — no `ChatStatus.reconnecting` / no wiring.

- [ ] **Step 3: Implement controller + label**

```dart
enum ChatStatus { disconnected, connecting, connected, reconnecting, error }
```

In `connect()` after `_session.connect()`:

```dart
await _stateSub?.cancel();
_stateSub = _session.connectionState.listen((state) {
  switch (state) {
    case AcpConnectionState.connecting:
      status = ChatStatus.connecting;
    case AcpConnectionState.reconnecting:
      status = ChatStatus.reconnecting;
      _sessionReady = false; // live session is gone until rebound
    case AcpConnectionState.connected:
      status = ChatStatus.connected;
      // if thread+agent pinned, ChatController should re-call startSession
      // ONLY if AgentConnection did not already replay — prefer AgentConnection replay.
      // After reconnect, refresh catalog lists:
      unawaited(_refreshCatalogAfterReconnect());
    case AcpConnectionState.disconnected:
      status = ChatStatus.disconnected;
      _sessionReady = false;
  }
  notifyListeners();
});
```

`_refreshCatalogAfterReconnect`: if `_catalog != null`, `listAgents` + `listThreads` best-effort; on failure leave last known data and set `statusMessage`.

Remove or narrow `_onSessionClosed` so it does not fight `connectionState` (spec: AgentConnection owns reconnect; controller mirrors state). Prefer deleting the disconnect-on-`closed` path once `connectionState` covers it; update the existing test that expects disconnect via `simulateDisconnect` to also `emitState(disconnected)`.

`canSend` / `canSelectAgent` / `canSelectModel`: require `status == ChatStatus.connected` (reconnecting ≠ connected).

`chat_screen.dart` `_statusLabel`:

```dart
case ChatStatus.reconnecting:
  return 'Reconnecting…';
```

- [ ] **Step 4: Run controller + screen tests**

Run:

```bash
cd client && flutter test test/chat/chat_controller_test.dart test/chat/chat_screen_test.dart
```

Expected: PASS (fix any FakeConn / switch exhaustiveness breakages).

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/chat_controller.dart client/lib/chat/chat_screen.dart client/test/chat/chat_controller_test.dart
git commit -m "$(cat <<'EOF'
feat(client): mirror ACP reconnect state in chat controller

EOF
)"
```

---

### Task 4: Shell connectivity badge + offline empty UI

**Files:**
- Create: `client/lib/ui/connectivity_badge.dart`
- Create: `client/test/ui/connectivity_badge_test.dart`
- Modify: `client/lib/app_shell.dart`
- Modify: `client/test/app_shell_test.dart`
- Modify: `client/lib/chat/chat_screen.dart` and/or `thread_pane.dart` for offline empty copy

**Interfaces:**
- Consumes: `ChatController.status`
- Produces: `ConnectivityBadge` widget; shell shows Online / Reconnecting… / Offline

- [ ] **Step 1: Write failing badge + shell tests**

`client/test/ui/connectivity_badge_test.dart`:

```dart
testWidgets('shows Offline for disconnected', (tester) async {
  await tester.pumpWidget(
    const MaterialApp(home: ConnectivityBadge(status: ChatStatus.disconnected)),
  );
  expect(find.text('Offline'), findsOneWidget);
});

testWidgets('shows Reconnecting…', (tester) async {
  await tester.pumpWidget(
    const MaterialApp(home: ConnectivityBadge(status: ChatStatus.reconnecting)),
  );
  expect(find.text('Reconnecting…'), findsOneWidget);
});

testWidgets('shows Online for connected', (tester) async {
  await tester.pumpWidget(
    const MaterialApp(home: ConnectivityBadge(status: ChatStatus.connected)),
  );
  expect(find.text('Online'), findsOneWidget);
});
```

`app_shell_test.dart`:

```dart
testWidgets('shows Offline badge and can open Settings', (tester) async {
  SharedPreferences.setMockInitialValues({});
  final session = _FakeConn()..currentState = AcpConnectionState.disconnected;
  final controller = ChatController(session: session);
  // do not connect
  await tester.pumpWidget(/* existing AppShell harness */);
  expect(find.text('Offline'), findsOneWidget);

  await tester.tap(find.text('Settings'));
  await tester.pumpAndSettle();
  expect(find.byType(SettingsPage), findsOneWidget);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd client && flutter test test/ui/connectivity_badge_test.dart test/app_shell_test.dart`  
Expected: FAIL — missing widget / no Offline badge.

- [ ] **Step 3: Implement badge + shell placement**

`ConnectivityBadge`: small row with a colored `Icon(Icons.circle, size: 10)` + label:

| Status | Label | Color intent |
| --- | --- | --- |
| `connected` | Online | success/green from theme |
| `connecting` / `reconnecting` | Reconnecting… | secondary/orange |
| `disconnected` / `error` | Offline | error/outline |

Place in `AppShell` below the rail destinations (or above the body column) so it remains visible on Chat and Settings. Use `AnimatedBuilder` on `controller`.

Offline empty copy when `status` is offline/reconnecting and there is no useful local data:

- Chat body: if no `selectedThreadId` or empty messages **and** not connected → `Text('You\'re offline')` (keep last messages if present).
- Thread pane: keep list if non-empty; if empty and offline, show the same short empty hint.

Do not block `NavigationRail` selection.

- [ ] **Step 4: Run UI tests + full client suite**

Run:

```bash
cd client && flutter test
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/ui/connectivity_badge.dart client/test/ui/connectivity_badge_test.dart \
  client/lib/app_shell.dart client/test/app_shell_test.dart \
  client/lib/chat/chat_screen.dart client/lib/chat/thread_pane.dart
git commit -m "$(cat <<'EOF'
feat(client): show Online/Offline connectivity in the shell

EOF
)"
```

---

### Task 5: Manual smoke + README note

**Files:**
- Modify: `README.md` (client run section — note desktop/IO WS + reconnect)

- [ ] **Step 1: Manual smoke (document results in commit message if issues found)**

With control plane running:

1. `cd client && flutter run -d linux` (or macos/windows) → connect → send a prompt.
2. Stop control plane → badge becomes Reconnecting… → send disabled → Settings still opens.
3. Start control plane → Online → bound thread session works again.

Also spot-check `flutter run -d chrome` still connects.

- [ ] **Step 2: README**

Add 2–4 lines under the Flutter client section: WS works on web and desktop/mobile; IO uses protocol keepalive; UI shows Online/Reconnecting/Offline; cleartext `ws://localhost` is for local dev.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "$(cat <<'EOF'
docs: note cross-platform ACP WebSocket connectivity

EOF
)"
```

---

## Spec coverage (self-review)

| Spec requirement | Task |
| --- | --- |
| `web_socket_channel` + drop `package:web` | 1 |
| IO pingInterval 30s + connectTimeout 30s | 1 |
| Web ready timeout 30s | 1 |
| Text-only frames | 1 |
| `WsTransport` framing unchanged | 1 |
| Reconnect in `AgentConnection` with backoff | 2 |
| No reconnect for injected transport | 2 |
| Re-`initialize` + `startSession` replay | 2 |
| Fail in-flight turn on drop | 2 |
| `ChatStatus.reconnecting` + gates | 3 |
| Shell Online/Reconnecting/Offline | 4 |
| Navigate while offline | 4 |
| Keep last-known threads/messages | 3–4 |
| README / smoke | 5 |

No placeholders left in task steps. Types aligned: `AcpConnectionState`, `connectionState`, `TransportFactory`, `bindWsChannel`, `ChatStatus.reconnecting`.
