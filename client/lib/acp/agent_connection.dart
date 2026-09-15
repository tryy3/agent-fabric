import 'dart:async';

import 'package:acpd/acpd.dart' hide AgentConnection;

import 'ws_transport.dart';

sealed class AgentTurnEvent {
  const AgentTurnEvent();
}

final class AgentThoughtDelta extends AgentTurnEvent {
  const AgentThoughtDelta(this.text);
  final String text;
}

final class AgentMessageDelta extends AgentTurnEvent {
  const AgentMessageDelta(this.text);
  final String text;
}

final class AgentUsageEvent extends AgentTurnEvent {
  const AgentUsageEvent(this.usage);
  final TurnUsage usage;
}

class TurnUsage {
  const TurnUsage({
    this.promptTokens,
    this.completionTokens,
    this.totalTokens,
    this.ttftMs,
    this.elapsedMs,
    this.promptMs,
    this.predictedMs,
    this.promptPerSecond,
    this.predictedPerSecond,
    this.deltas,
    this.stopReason,
    this.extras = const {},
  });
  final int? promptTokens;
  final int? completionTokens;
  final int? totalTokens;
  final int? ttftMs;
  final int? elapsedMs;
  final double? promptMs;
  final double? predictedMs;
  final double? promptPerSecond;
  final double? predictedPerSecond;
  final int? deltas;
  final String? stopReason;

  /// Keys from ACP/catalog that are not mapped to typed fields.
  /// Preserved so new inference stats still appear in the Raw view.
  final Map<String, Object?> extras;
}

/// Known usage/meta field names on [TurnUsage].
const Set<String> kTurnUsageKnownKeys = {
  'promptTokens',
  'completionTokens',
  'totalTokens',
  'ttftMs',
  'elapsedMs',
  'promptMs',
  'predictedMs',
  'promptPerSecond',
  'predictedPerSecond',
  'deltas',
  'stopReason',
  'type', // catalog part discriminator, not a stat
};

typedef AgentTurnHandler = void Function(AgentTurnEvent event);

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

/// Returns thought text from an agent_thought_chunk; otherwise null.
String? agentThoughtText(SessionUpdate update) {
  if (update is! AgentThoughtChunk) return null;
  final block = update.chunk.content;
  if (block is! TextContentBlock) return null;
  return block.text;
}

/// Maps a usage_update to [TurnUsage]; otherwise null.
TurnUsage? turnUsageFromUpdate(SessionUpdate update) {
  if (update is! UsageSessionUpdate) return null;
  final meta = update.meta;
  return TurnUsage(
    promptTokens: _metaInt(meta, 'promptTokens'),
    completionTokens: _metaInt(meta, 'completionTokens'),
    totalTokens:
        _metaInt(meta, 'totalTokens') ??
        (update.used == 0 ? null : update.used),
    ttftMs: _metaInt(meta, 'ttftMs'),
    elapsedMs: _metaInt(meta, 'elapsedMs'),
    promptMs: _metaDouble(meta, 'promptMs'),
    predictedMs: _metaDouble(meta, 'predictedMs'),
    promptPerSecond: _metaDouble(meta, 'promptPerSecond'),
    predictedPerSecond: _metaDouble(meta, 'predictedPerSecond'),
    deltas: _metaInt(meta, 'deltas'),
    stopReason: meta['stopReason'] is String
        ? meta['stopReason'] as String
        : null,
    extras: _extrasFromMap(meta),
  );
}

Map<String, Object?> _extrasFromMap(Map<String, Object?> source) {
  return {
    for (final entry in source.entries)
      if (!kTurnUsageKnownKeys.contains(entry.key)) entry.key: entry.value,
  };
}

int? _metaInt(Map<String, Object?> meta, String key) {
  final value = meta[key];
  if (value is num) return value.toInt();
  return null;
}

double? _metaDouble(Map<String, Object?> meta, String key) {
  final value = meta[key];
  if (value is num) return value.toDouble();
  return null;
}

abstract class AgentSessionApi {
  Stream<void> get closed;
  Future<void> connect({Transport? transport});
  Future<void> startSession(String agentId, {String? threadId});
  Future<void> setModel(String modelId);
  List<ModelOption> get modelOptions;
  String? get currentModel;
  Future<void> sendPrompt(String text, {required AgentTurnHandler onEvent});
  Future<void> cancel();
  Future<void> close();
}

class AgentConnection implements AgentSessionApi {
  ClientConnection? _client;
  Session? _session;
  Transport? _transport;
  AgentTurnHandler? _activeTurnHandler;
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
          final handler = _activeTurnHandler;
          if (handler == null) return;
          final update = notification.update;
          final thought = agentThoughtText(update);
          if (thought != null) {
            handler(AgentThoughtDelta(thought));
            return;
          }
          final message = agentMessageText(update);
          if (message != null) {
            handler(AgentMessageDelta(message));
            return;
          }
          final usage = turnUsageFromUpdate(update);
          if (usage != null) {
            handler(AgentUsageEvent(usage));
          }
        })
        .connect(t);

    final client = _client!;
    unawaited(
      client.closed.then((_) {
        if (!_closedController.isClosed) {
          _closedController.add(null);
        }
      }),
    );

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
  Future<void> startSession(String agentId, {String? threadId}) async {
    final client = _client;
    if (client == null) {
      throw StateError('AgentConnection is not connected');
    }
    Session next;
    try {
      next = await Session.create(
        client,
        NewSessionRequest(
          cwd: '/',
          mcpServers: const [],
          meta: {'agentId': agentId, 'threadId': ?threadId},
        ),
      );
    } catch (_) {
      if (_session == null) {
        _modelOptions = const [];
        _currentModel = null;
      }
      rethrow;
    }
    final previous = _session;
    _session = next;
    if (previous != null) {
      try {
        await previous.close();
      } catch (_) {
        previous.dispose();
      }
    }
    _syncModels();
  }

  @override
  Future<void> cancel() async {
    _session?.cancel();
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
        [for (final v in values) ModelOption(id: v.value, name: v.name)],
        opt.currentValue,
      );
    }
    return (const [], null);
  }

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
  }) async {
    final session = _session;
    if (session == null) {
      throw StateError('AgentConnection is not connected');
    }
    _activeTurnHandler = onEvent;
    try {
      await session.sendPrompt([TextContentBlock(text: text)]);
    } finally {
      _activeTurnHandler = null;
    }
  }

  @override
  Future<void> close() async {
    _activeTurnHandler = null;
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
