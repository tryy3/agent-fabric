import 'dart:async';

import 'package:acpd/acpd.dart' hide AgentConnection;

import 'ws_transport.dart';

typedef AgentChunkHandler = void Function(String text);

final defaultAcpUri = Uri.parse('ws://localhost:8080/acp');

class ModelOption {
  const ModelOption({required this.id, required this.name});

  final String id;
  final String name;
}

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
  Future<void> startSession(String agentId);
  Future<void> setModel(String modelId);
  List<ModelOption> get modelOptions;
  String? get currentModel;
  Future<void> sendPrompt(String text, {required AgentChunkHandler onChunk});
  Future<void> close();
}

class AgentConnection implements AgentSessionApi {
  ClientConnection? _client;
  Session? _session;
  Transport? _transport;
  AgentChunkHandler? _activeChunkHandler;
  final _closedController = StreamController<void>.broadcast(sync: true);

  List<ModelOption> _modelOptions = const [];
  String? _currentModel;

  @override
  Stream<void> get closed => _closedController.stream;

  @override
  List<ModelOption> get modelOptions => _modelOptions;

  @override
  String? get currentModel => _currentModel;

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
  }

  @override
  Future<void> startSession(String agentId) async {
    final client = _client;
    if (client == null) {
      throw StateError('AgentConnection is not connected');
    }
    final previous = _session;
    _session = null;
    if (previous != null) {
      try {
        await previous.close();
      } catch (_) {
        previous.dispose();
      }
    }
    _session = await Session.create(
      client,
      NewSessionRequest(
        cwd: '/',
        mcpServers: const [],
        meta: {'agentId': agentId},
      ),
    );
    _syncModels();
  }

  @override
  Future<void> setModel(String modelId) async {
    final session = _session;
    if (session == null) {
      throw StateError('AgentConnection has no session');
    }
    await session.setConfigOption(
      SetValueIdConfigOption(
        sessionId: session.sessionId,
        configId: 'model',
        value: modelId,
      ),
    );
    _syncModels();
  }

  void _syncModels() {
    final parsed = _parseModelConfig(_session?.configOptions ?? const []);
    _modelOptions = parsed.$1;
    _currentModel = parsed.$2;
  }

  static (List<ModelOption>, String?) _parseModelConfig(
    List<SessionConfigOption> options,
  ) {
    for (final opt in options) {
      if (opt is! SessionConfigSelectOptionValue) continue;
      if (opt.id != 'model' &&
          opt.category != SessionConfigOptionCategory.model) {
        continue;
      }
      final values = switch (opt.options) {
        SessionConfigUngroupedOptions(:final options) => options,
        SessionConfigGroupedOptions(:final groups) => [
          for (final g in groups) ...g.options,
        ],
      };
      return (
        [
          for (final v in values) ModelOption(id: v.value, name: v.name),
        ],
        opt.currentValue,
      );
    }
    return (const [], null);
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
    _modelOptions = const [];
    _currentModel = null;
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
