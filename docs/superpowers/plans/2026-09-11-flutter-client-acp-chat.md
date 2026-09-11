# Flutter Client ACP Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Flutter web chat client that speaks ACP v1 over WebSocket to the control plane echo agent at `ws://localhost:8080/acp`.

**Architecture:** `acpd` owns typed ACP Client/Session JSON-RPC. We own a browser WebSocket `Transport` (same role as Go `internal/transport/ws`) because `acpd_http` is not web-safe. A thin `AgentConnection` wraps initialize → session/new → prompt; `ChatController` owns UI state; one Material chat screen auto-connects and streams assistant chunks.

**Tech Stack:** Flutter (web), Dart 3.5+, `acpd` ^1.0.0, browser WebSocket via `package:web`, Nix flake `devShell` with `flutter` (+ existing `go` / `gopls`).

## Global Constraints

- ACP v1 only; no catalog, auth, editable URL, reconnect UI, cancel UI, permissions UX, tools UI.
- Hardcoded endpoint: `ws://localhost:8080/acp`.
- Web target first; do not polish iOS/Android/desktop runners.
- Do **not** depend on `acpd_http` or `acpd_io` (not web-safe).
- Pin: `acpd: ^1.0.0`.
- Offline unit tests only — no live network in `flutter test`.
- Prefer small files under `client/lib/`; follow TDD: failing test → implement → pass → commit per task.
- Control plane contract unchanged: one WS text frame ↔ one JSON-RPC value (Go bridge adds `\n` on read if missing).

## File Structure

| Path | Responsibility |
| --- | --- |
| `flake.nix` | Add `flutter` to `devShell` |
| `.gitignore` | Flutter/Dart ignore patterns |
| `client/pubspec.yaml` | Flutter app + `acpd` |
| `client/lib/acp/ws_transport.dart` | Browser WebSocket ↔ `acpd` `Transport` |
| `client/test/acp/ws_transport_test.dart` | Framing tests with fake socket |
| `client/lib/acp/agent_connection.dart` | `acpd` ClientRole + Session lifecycle |
| `client/test/acp/agent_connection_test.dart` | Fake-transport connect/prompt chunk tests |
| `client/lib/chat/chat_message.dart` | Simple message model (`role`, `text`) |
| `client/lib/chat/chat_controller.dart` | Status + messages + connect/send |
| `client/test/chat/chat_controller_test.dart` | Controller state tests with fake connection |
| `client/lib/chat/chat_screen.dart` | Transcript + input + status |
| `client/lib/main.dart` | App entry; wires controller + screen |
| `README.md` | Document Flutter web run path |

---

### Task 1: Nix Flutter tooling and client scaffold

**Files:**
- Modify: `flake.nix`
- Modify: `.gitignore`
- Create: `client/` (via `flutter create`)
- Modify: `client/pubspec.yaml` (add `acpd`)

**Interfaces:**
- Consumes: none
- Produces: runnable empty Flutter web project under `client/` with `acpd` resolvable

- [ ] **Step 1: Add Flutter to the Nix flake**

In `flake.nix`, extend `packages` to:

```nix
packages = with pkgs; [
  go
  gopls
  flutter
];
```

Reload the direnv / Nix shell so `flutter` is on PATH.

- [ ] **Step 2: Verify Flutter**

```bash
flutter --version
```

Expected: Flutter version printed (SDK from nixpkgs).

- [ ] **Step 3: Scaffold the Flutter web app**

```bash
cd /home/tryy3/src/agent-fabric
flutter create --platforms=web --project-name=agent_fabric_client client
```

Expected: `client/lib/main.dart`, `client/pubspec.yaml`, `client/web/` exist.

- [ ] **Step 4: Add `acpd` dependency**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter pub add acpd
```

Expected: `pubspec.yaml` lists `acpd: ^1.0.0` (or compatible 1.x); `flutter pub get` succeeds.

- [ ] **Step 5: Append Flutter ignores**

Append to `.gitignore` if not already present:

```gitignore
# Flutter / Dart
client/.dart_tool/
client/.flutter-plugins-dependencies
client/.packages
client/build/
client/.pub-cache/
client/**/*.iml
```

- [ ] **Step 6: Smoke-build web (no chat yet)**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter build web --debug
```

Expected: build completes without errors.

- [ ] **Step 7: Commit**

```bash
git add flake.nix .gitignore client
git commit -m "$(cat <<'EOF'
Scaffold Flutter web client and add Flutter to Nix shell.

EOF
)"
```

---

### Task 2: Browser WebSocket ACP transport

**Files:**
- Create: `client/lib/acp/ws_transport.dart`
- Create: `client/test/acp/ws_transport_test.dart`

**Interfaces:**
- Consumes: `acpd` `Transport`, `TransportFrame`, `decodeFrame` (top-level), `TransportFrame.toWire()`
- Produces:
  - `class WsTransport implements Transport`
  - `static Future<WsTransport> connect(Uri uri)`
  - `@visibleForTesting factory WsTransport.loopback({required StreamController<String> inbound, required void Function(String) outbound})` — or equivalent test constructor that avoids a real socket
  - `Stream<TransportFrame> get incoming`
  - `void send(TransportFrame frame)`
  - `Future<void> close()`

Framing must match `acpd_http`'s native `WebSocketClientTransport` (text frames; `send` uses `frame.toWire()`; receive uses `decodeFrame(data)`), which is compatible with the Go NDJSON bridge.

- [ ] **Step 1: Write the failing framing test**

Create `client/test/acp/ws_transport_test.dart`:

```dart
import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/ws_transport.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('send writes toWire payload to the socket', () async {
    final sent = <String>[];
    final inbound = StreamController<String>();
    final transport = WsTransport.loopback(
      inbound: inbound,
      outbound: sent.add,
    );

    final frame = TransportFrame.decode(
      '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}',
    );
    transport.send(frame);

    expect(sent, [frame.toWire()]);
    await transport.close();
    await inbound.close();
  });

  test('inbound text frames become TransportFrames on incoming', () async {
    final inbound = StreamController<String>();
    final transport = WsTransport.loopback(
      inbound: inbound,
      outbound: (_) {},
    );

    final future = transport.incoming.first;
    inbound.add('{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}');
    final frame = await future.timeout(const Duration(seconds: 1));

    expect(frame.messages, isNotEmpty);
    await transport.close();
    await inbound.close();
  });
}
```

Adjust imports if `flutter create` placed the package root differently; package name is `agent_fabric_client`.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter test test/acp/ws_transport_test.dart
```

Expected: FAIL — `ws_transport.dart` missing or `WsTransport` undefined.

- [ ] **Step 3: Implement `WsTransport`**

Create `client/lib/acp/ws_transport.dart`:

```dart
import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:web/web.dart' as web;

/// Browser WebSocket transport for acpd (web-safe stand-in for acpd_http).
class WsTransport implements Transport {
  WsTransport._({
    required Stream<String> inbound,
    required void Function(String data) outbound,
    required Future<void> Function() onClose,
  })  : _outbound = outbound,
        _onClose = onClose {
    _subscription = inbound.listen(
      _onData,
      onError: _fail,
      onDone: _onDone,
      cancelOnError: true,
    );
  }

  /// Test / loopback constructor — no real socket.
  factory WsTransport.loopback({
    required StreamController<String> inbound,
    required void Function(String) outbound,
  }) {
    return WsTransport._(
      inbound: inbound.stream,
      outbound: outbound,
      onClose: () async {
        if (!inbound.isClosed) {
          await inbound.close();
        }
      },
    );
  }

  static Future<WsTransport> connect(Uri uri) async {
    if (uri.scheme != 'ws' && uri.scheme != 'wss') {
      throw ArgumentError.value(uri, 'uri', 'must be ws or wss');
    }
    final socket = web.WebSocket(uri.toString());
    final ready = Completer<void>();
    final inbound = StreamController<String>();

    late final StreamController<web.Event> openEvents;
    openEvents = StreamController<web.Event>.broadcast();

    socket.onOpen.listen((event) {
      if (!ready.isCompleted) ready.complete();
    });
    socket.onError.listen((event) {
      if (!ready.isCompleted) {
        ready.completeError(StateError('WebSocket connection failed'));
      }
      inbound.addError(StateError('WebSocket error'));
    });
    socket.onClose.listen((event) {
      if (!inbound.isClosed) {
        inbound.close();
      }
    });
    socket.onMessage.listen((event) {
      final data = event.data;
      if (data is String) {
        inbound.add(data);
      } else {
        inbound.addError(
          const FormatException('ACP WebSocket messages must use text frames.'),
        );
      }
    });

    await ready.future.timeout(const Duration(seconds: 30));

    return WsTransport._(
      inbound: inbound.stream,
      outbound: (data) => socket.send(data.toJS),
      onClose: () async {
        socket.close(1000, 'normal');
        if (!inbound.isClosed) await inbound.close();
      },
    );
  }

  final void Function(String data) _outbound;
  final Future<void> Function() _onClose;
  final StreamController<TransportFrame> _incoming =
      StreamController<TransportFrame>();
  late final StreamSubscription<String> _subscription;
  bool _closed = false;

  @override
  Stream<TransportFrame> get incoming => _incoming.stream;

  @override
  void send(TransportFrame frame) {
    if (_closed) {
      throw StateError('WebSocket ACP transport is closed.');
    }
    _outbound(frame.toWire());
  }

  void _onData(String data) {
    if (_closed) return;
    try {
      _incoming.add(decodeFrame(data));
    } catch (error, stackTrace) {
      _fail(error, stackTrace);
    }
  }

  void _onDone() {
    if (_closed) return;
    _fail(StateError('ACP WebSocket closed by the peer.'));
  }

  void _fail(Object error, [StackTrace? stackTrace]) {
    if (_closed) return;
    if (!_incoming.isClosed) {
      _incoming.addError(error, stackTrace ?? StackTrace.current);
    }
    unawaited(close());
  }

  @override
  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    await _subscription.cancel();
    await _onClose();
    await _incoming.close();
  }
}
```

**Implementation note for the agent executing this task:** `package:web` WebSocket event APIs (`onOpen` / `onMessage` / `data.toJS`) vary slightly by Flutter/`web` version. If the snippet does not compile, adapt to the installed `package:web` WebSocket API while keeping the public `WsTransport` interface and framing (`toWire` / `decodeFrame`) identical. Prefer `dart:js_interop` helpers already used by the Flutter web template. Keep `WsTransport.loopback` stable for tests.

If `decodeFrame` is not exported under that name, use `TransportFrame.decode` instead (same framing).

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter test test/acp/ws_transport_test.dart
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/acp/ws_transport.dart client/test/acp/ws_transport_test.dart
git commit -m "$(cat <<'EOF'
Add browser WebSocket ACP transport for acpd.

EOF
)"
```

---

### Task 3: Agent connection (acpd ClientRole + Session)

**Files:**
- Create: `client/lib/acp/agent_connection.dart`
- Create: `client/test/acp/agent_connection_test.dart`

**Interfaces:**
- Consumes: `WsTransport` (or any `Transport`), `acpd` `ClientRole`, `Session`, `InitializeRequest`, `NewSessionRequest`, `TextContentBlock`, `AgentMessageChunk`, `TextContentBlock`
- Produces:
  - `typedef AgentChunkHandler = void Function(String text);`
  - `class AgentConnection`
  - `Future<void> connect({Transport? transport})` — if `transport` is null, dials `ws://localhost:8080/acp` via `WsTransport.connect`
  - `Future<void> sendPrompt(String text, {required AgentChunkHandler onChunk})`
  - `Future<void> close()`
  - Constant: `defaultAcpUri = Uri.parse('ws://localhost:8080/acp')`

Register stubs: `onRequestPermission` → `PermissionCancelled`; leave fs/terminal unregistered (echo never calls them). Use `onSessionUpdate` to forward `AgentMessageChunk` text to `onChunk` during an in-flight prompt (store the active handler on the connection).

- [ ] **Step 1: Write the failing tests**

Create `client/test/acp/agent_connection_test.dart` with two parts: (1) pure chunk text extraction, (2) end-to-end over linked in-memory transports.

```dart
import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:flutter_test/flutter_test.dart';

class _End implements Transport {
  final _incoming = StreamController<TransportFrame>();
  late _End peer;
  bool _closed = false;

  @override
  Stream<TransportFrame> get incoming => _incoming.stream;

  @override
  void send(TransportFrame frame) {
    if (_closed) throw StateError('closed');
    peer._incoming.add(frame);
  }

  @override
  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    await _incoming.close();
  }
}

(_End, _End) linkedTransports() {
  final a = _End();
  final b = _End();
  a.peer = b;
  b.peer = a;
  return (a, b);
}

void main() {
  test('agentMessageText extracts text from AgentMessageChunk', () {
    final update = AgentMessageChunk(
      chunk: ContentChunk(
        content: TextContentBlock(text: 'hello'),
      ),
    );
    expect(agentMessageText(update), 'hello');
    expect(agentMessageText(const UserMessageChunk(
      chunk: ContentChunk(content: TextContentBlock(text: 'x')),
    )), isNull);
  });

  test('connect + prompt forwards agent_message_chunk text', () async {
    final (clientTransport, agentTransport) = linkedTransports();

    // Build a minimal ACP agent peer with the typed AgentRole API from the
    // installed acpd version. Required behavior:
    // - initialize → protocolVersion v1 + agentInfo
    // - session/new → sessionId "sess-1"
    // - session/prompt → emit one session/update AgentMessageChunk("hello")
    //   then return stopReason end_turn
    //
    // Exact handler names differ across acpd minors; inspect
    // package:acpd ClientRole/AgentRole docs after pub get and wire the
    // typed equivalents of initialize / newSession / prompt + sessionUpdate.
    late final AgentConnection agentSide; // replace with AgentRole.connect result
    // Example shape (adapt to installed API):
    // final agentSide = AgentRole()
    //     .onInitialize(...)
    //     .onNewSession(...)
    //     .onPrompt((ctx, request, cancellation) async {
    //       await ctx.sessionUpdate(SessionUpdateNotification(
    //         sessionId: request.sessionId,
    //         update: AgentMessageChunk(
    //           chunk: ContentChunk(content: TextContentBlock(text: 'hello')),
    //         ),
    //       ));
    //       return const PromptResponse(stopReason: StopReason.endTurn);
    //     })
    //     .connect(agentTransport);

    // TEMPORARY compile gate for TDD red: call APIs that do not exist yet.
    final chunks = <String>[];
    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);

    // After wiring the fake agent above, uncomment:
    // await conn.sendPrompt('ping', onChunk: chunks.add);
    // expect(chunks, ['hello']);

    // Until the agent peer is wired, fail intentionally so the test stays red:
    fail('wire AgentRole peer then assert chunks == [hello]');

    await conn.close();
    await clientTransport.close();
    await agentTransport.close();
  });
}
```

**Executing-agent requirement:** Replace the intentional `fail(...)` with a real `AgentRole` peer using the installed `acpd` typed handlers so the test asserts `chunks == ['hello']`. Do not ship the task with `fail` or commented asserts. If `UserMessageChunk` / `StopReason` names differ, use the sealed `SessionUpdate` / prompt response types from the installed package.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter test test/acp/agent_connection_test.dart
```

Expected: FAIL — `AgentConnection` missing.

- [ ] **Step 3: Implement `AgentConnection`**

Create `client/lib/acp/agent_connection.dart`:

```dart
import 'package:acpd/acpd.dart';

import 'ws_transport.dart';

typedef AgentChunkHandler = void Function(String text);

const defaultAcpUri = 'ws://localhost:8080/acp';

/// Returns assistant text from an agent_message_chunk; otherwise null.
String? agentMessageText(SessionUpdate update) {
  if (update is! AgentMessageChunk) return null;
  final block = update.chunk.content;
  if (block is! TextContentBlock) return null;
  return block.text;
}

abstract class AgentSessionApi {
  Future<void> connect({Transport? transport});
  Future<void> sendPrompt(String text, {required AgentChunkHandler onChunk});
  Future<void> close();
}

class AgentConnection implements AgentSessionApi {
  ClientConnection? _client;
  Session? _session;
  Transport? _transport;
  AgentChunkHandler? _activeChunkHandler;

  @override
  Future<void> connect({Transport? transport}) async {
    await close();
    final t = transport ??
        await WsTransport.connect(Uri.parse(defaultAcpUri));
    _transport = t;

    _client = ClientRole()
        .onRequestPermission((context, request, cancellation) async {
          return const RequestPermissionResponse(
            outcome: PermissionCancelled(),
          );
        })
        .onSessionUpdate((context, notification) async {
          final handler = _activeChunkHandler;
          if (handler == null) return;
          final text = agentMessageText(notification.update);
          if (text != null) handler(text);
        })
        .connect(t);

    await _client!.client.initialize(
      const InitializeRequest(
        protocolVersion: ProtocolVersion.v1,
        clientInfo: Implementation(
          name: 'agent-fabric-client',
          version: '0.1.0',
        ),
      ),
    );

    _session = await Session.create(
      _client!,
      const NewSessionRequest(cwd: '/', mcpServers: []),
    );
  }

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentChunkHandler onChunk,
  }) async {
    final session = _session;
    if (session == null) {
      throw StateError('AgentConnection is not connected');
    }
    _activeChunkHandler = onChunk;
    try {
      await session.sendPrompt([
        TextContentBlock(text: text),
      ]);
    } finally {
      _activeChunkHandler = null;
    }
  }

  @override
  Future<void> close() async {
    _activeChunkHandler = null;
    _session?.dispose();
    _session = null;
    final client = _client;
    _client = null;
    if (client != null) {
      await client.close();
    }
    final transport = _transport;
    _transport = null;
    if (transport != null) {
      await transport.close();
    }
  }
}
```

**API drift note:** If `ClientRole.onSessionUpdate` / `RequestPermissionResponse` / `ProtocolVersion.v1` / `client.client.initialize` names differ slightly in the resolved `acpd` 1.x, match the installed package docs while preserving `AgentSessionApi`, `agentMessageText`, and chunk-forwarding behavior.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter test test/acp/agent_connection_test.dart
```

Expected: PASS (including chunk assertion once the fake agent emits updates).

- [ ] **Step 5: Commit**

```bash
git add client/lib/acp/agent_connection.dart client/test/acp/agent_connection_test.dart
git commit -m "$(cat <<'EOF'
Add acpd AgentConnection for initialize, session, and prompts.

EOF
)"
```

---

### Task 4: ChatController

**Files:**
- Create: `client/lib/chat/chat_message.dart`
- Create: `client/lib/chat/chat_controller.dart`
- Create: `client/test/chat/chat_controller_test.dart`

**Interfaces:**
- Consumes: `AgentConnection` (injectable for tests)
- Produces:
  - `enum ChatStatus { disconnected, connecting, connected, error }`
  - `enum ChatRole { user, assistant }`
  - `class ChatMessage { final ChatRole role; final String text; }`
  - `class ChatController extends ChangeNotifier`
  - `ChatStatus status`, `String? statusMessage`, `List<ChatMessage> messages`, `bool get canSend`
  - `Future<void> connect()`, `Future<void> send(String text)`, `Future<void> dispose()` / `close()`
  - Constructor: `ChatController({AgentSessionApi? session})` — default constructs a real `AgentConnection`

- [ ] **Step 1: Write failing controller tests**

Create `client/test/chat/chat_controller_test.dart`:

```dart
import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_message.dart';
import 'package:flutter_test/flutter_test.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  bool failConnect = false;
  final List<String> prompts = [];
  List<String> chunksToEmit = ['hel', 'lo'];

  @override
  Future<void> connect({Transport? transport}) async {
    if (failConnect) {
      throw StateError('dial failed');
    }
    connected = true;
  }

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentChunkHandler onChunk,
  }) async {
    prompts.add(text);
    for (final c in chunksToEmit) {
      onChunk(c);
    }
  }

  @override
  Future<void> close() async {
    connected = false;
  }
}

void main() {
  test('connect moves status to connected', () async {
    final fake = FakeConn();
    final c = ChatController(session: fake);
    expect(c.status, ChatStatus.disconnected);
    await c.connect();
    expect(c.status, ChatStatus.connected);
    expect(c.canSend, isTrue);
  });

  test('send appends user message and streams assistant text', () async {
    final fake = FakeConn();
    final c = ChatController(session: fake);
    await c.connect();
    await c.send('hi');
    expect(c.messages.map((m) => m.role).toList(), [
      ChatRole.user,
      ChatRole.assistant,
    ]);
    expect(c.messages[0].text, 'hi');
    expect(c.messages[1].text, 'hello');
    expect(fake.prompts, ['hi']);
  });

  test('connect failure sets error status', () async {
    final fake = FakeConn()..failConnect = true;
    final c = ChatController(session: fake);
    await c.connect();
    expect(c.status, ChatStatus.error);
    expect(c.canSend, isFalse);
  });
}
```

(`AgentSessionApi` is defined in Task 3.)

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter test test/chat/chat_controller_test.dart
```

Expected: FAIL — missing controller / interface.

- [ ] **Step 3: Implement message model + controller**

`client/lib/chat/chat_message.dart`:

```dart
enum ChatRole { user, assistant }

class ChatMessage {
  const ChatMessage({required this.role, required this.text});

  final ChatRole role;
  final String text;

  ChatMessage copyWith({String? text}) =>
      ChatMessage(role: role, text: text ?? this.text);
}
```

`client/lib/chat/chat_controller.dart`:

```dart
import 'package:flutter/foundation.dart';

import '../acp/agent_connection.dart';
import 'chat_message.dart';

enum ChatStatus { disconnected, connecting, connected, error }

class ChatController extends ChangeNotifier {
  ChatController({AgentSessionApi? session})
      : _session = session ?? AgentConnection();

  final AgentSessionApi _session;

  ChatStatus status = ChatStatus.disconnected;
  String? statusMessage;
  final List<ChatMessage> messages = [];
  bool _sending = false;

  bool get canSend =>
      status == ChatStatus.connected && !_sending;

  Future<void> connect() async {
    status = ChatStatus.connecting;
    statusMessage = null;
    notifyListeners();
    try {
      await _session.connect();
      status = ChatStatus.connected;
      statusMessage = null;
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = e.toString();
    }
    notifyListeners();
  }

  Future<void> send(String text) async {
    final trimmed = text.trim();
    if (!canSend || trimmed.isEmpty) return;

    messages.add(ChatMessage(role: ChatRole.user, text: trimmed));
    messages.add(const ChatMessage(role: ChatRole.assistant, text: ''));
    _sending = true;
    notifyListeners();

    try {
      await _session.sendPrompt(trimmed, onChunk: (chunk) {
        final last = messages.last;
        messages[messages.length - 1] = last.copyWith(text: last.text + chunk);
        notifyListeners();
      });
    } catch (e) {
      status = ChatStatus.error;
      statusMessage = e.toString();
    } finally {
      _sending = false;
      notifyListeners();
    }
  }

  @override
  void dispose() {
    _session.close();
    super.dispose();
  }
}
```

Extend `FakeConn` with `bool failConnect = false` throwing in `connect` for the error test.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter test test/chat/chat_controller_test.dart
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/lib/acp/agent_connection.dart client/lib/chat/ client/test/chat/
git commit -m "$(cat <<'EOF'
Add ChatController with injectable ACP session for streaming chat.

EOF
)"
```

---

### Task 5: Chat UI, main entry, README

**Files:**
- Create: `client/lib/chat/chat_screen.dart`
- Modify: `client/lib/main.dart`
- Modify: `README.md`

**Interfaces:**
- Consumes: `ChatController`
- Produces: runnable Flutter web chat against live control plane

- [ ] **Step 1: Implement `ChatScreen`**

Create `client/lib/chat/chat_screen.dart`:

```dart
import 'package:flutter/material.dart';

import 'chat_controller.dart';
import 'chat_message.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key, required this.controller});

  final ChatController controller;

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _input = TextEditingController();

  @override
  void initState() {
    super.initState();
    widget.controller.connect();
  }

  @override
  void dispose() {
    _input.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final text = _input.text;
    _input.clear();
    await widget.controller.send(text);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final c = widget.controller;
        return Scaffold(
          appBar: AppBar(
            title: const Text('Agent Fabric'),
            bottom: PreferredSize(
              preferredSize: const Size.fromHeight(24),
              child: Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Text(
                  _statusLabel(c),
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
            ),
          ),
          body: Column(
            children: [
              Expanded(
                child: ListView.builder(
                  padding: const EdgeInsets.all(16),
                  itemCount: c.messages.length,
                  itemBuilder: (context, index) {
                    final m = c.messages[index];
                    final isUser = m.role == ChatRole.user;
                    return Align(
                      alignment: isUser
                          ? Alignment.centerRight
                          : Alignment.centerLeft,
                      child: Container(
                        margin: const EdgeInsets.symmetric(vertical: 4),
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(
                          color: isUser
                              ? Colors.blue.shade100
                              : Colors.grey.shade200,
                          borderRadius: BorderRadius.circular(8),
                        ),
                        child: Text(m.text.isEmpty && !isUser ? '…' : m.text),
                      ),
                    );
                  },
                ),
              ),
              SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: _input,
                          enabled: c.canSend,
                          onSubmitted: (_) => _submit(),
                          decoration: const InputDecoration(
                            hintText: 'Message',
                            border: OutlineInputBorder(),
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      IconButton(
                        onPressed: c.canSend ? _submit : null,
                        icon: const Icon(Icons.send),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  String _statusLabel(ChatController c) {
    switch (c.status) {
      case ChatStatus.connecting:
        return 'Connecting…';
      case ChatStatus.connected:
        return 'Connected';
      case ChatStatus.error:
        return 'Error: ${c.statusMessage ?? 'unknown'}';
      case ChatStatus.disconnected:
        return 'Disconnected';
    }
  }
}
```

- [ ] **Step 2: Wire `main.dart`**

Replace `client/lib/main.dart` with:

```dart
import 'package:flutter/material.dart';

import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';

void main() {
  runApp(const AgentFabricApp());
}

class AgentFabricApp extends StatefulWidget {
  const AgentFabricApp({super.key});

  @override
  State<AgentFabricApp> createState() => _AgentFabricAppState();
}

class _AgentFabricAppState extends State<AgentFabricApp> {
  late final ChatController _controller = ChatController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Agent Fabric',
      home: ChatScreen(controller: _controller),
    );
  }
}
```

- [ ] **Step 3: Update README**

Update the Layout bullet for Flutter and add a run section. Replace the Flutter placeholder line and extend run docs:

```markdown
## Layout

- [`controlplane/`](controlplane/) — Go control plane (ACP agent, WebSocket `/acp`)
- [`client/`](client/) — Flutter web chat (ACP client over WebSocket)
- [`docs/`](docs/) — architecture and decisions

## Run (control plane + Flutter chat)

Requirements: Nix direnv shell (Go + Flutter) or local Go 1.22+ and Flutter 3.24+.

```bash
# terminal 1
go -C controlplane run ./cmd/controlplane

# terminal 2
cd client && flutter run -d chrome
```

Send a message in the browser; the echo agent streams the same text back.

Design: [`docs/superpowers/specs/2026-09-11-flutter-client-acp-chat-design.md`](docs/superpowers/specs/2026-09-11-flutter-client-acp-chat-design.md).

Client tests:

```bash
cd client && flutter test
```
```

Keep the existing control-plane-only commands as well (CLI smoke remains valid).

- [ ] **Step 4: Run all client tests**

```bash
cd /home/tryy3/src/agent-fabric/client
flutter test
```

Expected: all tests PASS.

- [ ] **Step 5: Manual smoke (requires control plane)**

```bash
# terminal 1
go -C /home/tryy3/src/agent-fabric/controlplane run ./cmd/controlplane

# terminal 2
cd /home/tryy3/src/agent-fabric/client && flutter run -d chrome
```

Expected: status becomes Connected; sending `hello` streams `hello` into the assistant bubble.

- [ ] **Step 6: Commit**

```bash
git add client/lib/main.dart client/lib/chat/chat_screen.dart README.md
git commit -m "$(cat <<'EOF'
Add Flutter web chat UI and document how to run it.

EOF
)"
```

---

## Self-review checklist (author)

1. **Spec coverage:** Scaffold + flake Flutter; `acpd` + owned WS transport; hardcoded URL; chat stream; offline tests; README — each maps to Tasks 1–5.
2. **Placeholders:** Task 3’s intentional `fail(...)` is a TDD red gate only; the task may not be committed until a real `AgentRole` peer asserts `chunks == ['hello']`. `package:web` WebSocket API may need local adaptation in Task 2 without changing the public `WsTransport` interface.
3. **Type consistency:** `AgentSessionApi` / `AgentChunkHandler` / `agentMessageText` / `ChatStatus` / `ChatRole` names aligned across Tasks 3–5.
