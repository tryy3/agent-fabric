import 'dart:async';

import 'package:acpd/acpd.dart' hide AgentConnection;

import 'ws_transport.dart';

typedef AgentChunkHandler = void Function(String text);

final defaultAcpUri = Uri.parse('ws://localhost:8080/acp');

/// Returns assistant text from an agent_message_chunk; otherwise null.
String? agentMessageText(SessionUpdate update) {
  if (update is! AgentMessageChunk) return null;
  final block = update.chunk.content;
  if (block is! TextContentBlock) return null;
  return block.text;
}

abstract class AgentSessionApi {
  Stream<void> get closed;
  Future<void> connect({Transport? transport});
  Future<void> sendPrompt(String text, {required AgentChunkHandler onChunk});
  Future<void> close();
}

class AgentConnection implements AgentSessionApi {
  ClientConnection? _client;
  Session? _session;
  Transport? _transport;
  AgentChunkHandler? _activeChunkHandler;
  final _closedController = StreamController<void>.broadcast(sync: true);

  @override
  Stream<void> get closed => _closedController.stream;

  @override
  Future<void> connect({Transport? transport}) async {
    await close();
    final t = transport ?? await WsTransport.connect(defaultAcpUri);
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

    final client = _client!;
    unawaited(client.closed.then((_) {
      if (!_closedController.isClosed) {
        _closedController.add(null);
      }
    }));

    await client.client.initialize(
      const InitializeRequest(
        protocolVersion: ProtocolVersion.v1,
        clientInfo: Implementation(
          name: 'agent-fabric-client',
          version: '0.1.0',
        ),
      ),
    );

    _session = await Session.create(
      client,
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
