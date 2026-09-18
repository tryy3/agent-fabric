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

final class AgentToolCallEvent extends AgentTurnEvent {
  const AgentToolCallEvent({
    required this.id,
    required this.title,
    required this.status,
    required this.rawInput,
    required this.rawOutput,
    required this.inProgress,
  });

  final String id;
  final String? title;
  final String? status;
  final Object? rawInput;
  final Object? rawOutput;
  final bool inProgress;
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
typedef TransportFactory = Future<Transport> Function(Uri uri);

final defaultAcpUri = Uri.parse('ws://localhost:8080/acp');

enum AcpConnectionState { disconnected, connecting, connected, reconnecting }

Duration defaultAcpBackoff(int attempt) {
  if (attempt >= 5) return const Duration(seconds: 30);
  return Duration(seconds: 1 << attempt);
}

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

/// Maps tool_call and tool_call_update session updates; otherwise null.
AgentToolCallEvent? agentToolCallEventFromUpdate(SessionUpdate update) {
  final tool = switch (update) {
    ToolCallUpdateSession(:final toolCall) => toolCall.toUpdate(),
    ToolCallStatusUpdate(:final update) => update,
    _ => null,
  };
  if (tool == null) return null;
  final status = tool.status?.toJson();
  return AgentToolCallEvent(
    id: tool.toolCallId,
    title: tool.title,
    status: status,
    rawInput: tool.rawInput,
    rawOutput: tool.rawOutput,
    inProgress: status != 'completed' && status != 'failed',
  );
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
  Stream<AcpConnectionState> get connectionState;
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
  static const _maxReconnectReplayFailures = 3;

  AgentConnection({
    TransportFactory? transportFactory,
    Uri? acpUri,
    Duration Function(int) backoffForAttempt = defaultAcpBackoff,
  }) : _transportFactory =
           transportFactory ?? ((uri) => WsTransport.connect(uri)),
       _acpUri = acpUri ?? defaultAcpUri,
       // Keep the public injection point free of a private-name prefix.
       // ignore: prefer_initializing_formals
       _backoffForAttempt = backoffForAttempt;

  ClientConnection? _client;
  Session? _session;
  Transport? _transport;
  AgentTurnHandler? _activeTurnHandler;
  final _closedController = StreamController<void>.broadcast(sync: true);
  final _stateController = StreamController<AcpConnectionState>.broadcast(
    sync: true,
  );
  final TransportFactory _transportFactory;
  final Uri _acpUri;
  final Duration Function(int) _backoffForAttempt;

  AcpConnectionState _state = AcpConnectionState.disconnected;
  bool _wanted = false;
  bool _autoReconnect = false;
  String? _lastAgentId;
  String? _lastThreadId;
  String? _lastModelId;
  int _reconnectAttempt = 0;
  int _reconnectGeneration = 0;
  Timer? _reconnectTimer;
  Completer<void>? _reconnectDelay;
  Future<void>? _reconnectTask;

  List<ModelOption> _modelOptions = const [];
  String? _currentModel;

  @override
  Stream<void> get closed => _closedController.stream;

  @override
  Stream<AcpConnectionState> get connectionState => _stateController.stream;

  @override
  List<ModelOption> get modelOptions => _modelOptions;

  @override
  String? get currentModel => _currentModel;

  @override
  Future<void> connect({Transport? transport}) async {
    _wanted = false;
    _cancelReconnect();
    await _tearDownConnection();
    _wanted = true;
    _autoReconnect = transport == null;
    final generation = _reconnectGeneration;
    _setState(AcpConnectionState.connecting);
    try {
      final t = transport ?? await _transportFactory(_acpUri);
      if (!_wanted || generation != _reconnectGeneration) {
        await t.close();
        return;
      }
      final adopted = await _initializeTransport(t, generation: generation);
      if (!adopted) {
        return;
      }
      _reconnectAttempt = 0;
      _setState(AcpConnectionState.connected);
    } catch (_) {
      if (generation == _reconnectGeneration) {
        _wanted = false;
        _setState(AcpConnectionState.disconnected);
        await _tearDownConnection();
      }
      rethrow;
    }
  }

  /// Wires [t], initializes ACP, and adopts into fields only if [generation]
  /// is still current. Returns false when the attempt was superseded (locals
  /// already closed). On initialize failure, closes attempt-local resources
  /// and rethrows without touching a newer adopted connection.
  Future<bool> _initializeTransport(
    Transport t, {
    required int generation,
  }) async {
    final client = ClientRole()
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
          final toolCall = agentToolCallEventFromUpdate(update);
          if (toolCall != null) {
            handler(toolCall);
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

    try {
      await client.client.initialize(
        const InitializeRequest(
          protocolVersion: ProtocolVersion.v1,
          clientInfo: Implementation(
            name: 'agent-fabric-client',
            version: '0.1.0',
          ),
        ),
      );
    } catch (_) {
      try {
        await client.close();
      } catch (_) {}
      try {
        await t.close();
      } catch (_) {}
      rethrow;
    }

    if (!_wanted || generation != _reconnectGeneration) {
      try {
        await client.close();
      } catch (_) {}
      try {
        await t.close();
      } catch (_) {}
      return false;
    }

    _transport = t;
    _client = client;
    unawaited(
      client.closed.then((_) {
        if (identical(_client, client)) {
          _handleConnectionClosed();
        }
        if (!_closedController.isClosed) {
          _closedController.add(null);
        }
      }),
    );
    return true;
  }

  void _handleConnectionClosed() {
    _client = null;
    _transport = null;
    _session?.dispose();
    _session = null;
    _modelOptions = const [];
    _currentModel = null;
    if (_wanted && _autoReconnect) {
      _scheduleReconnect();
    } else {
      _setState(AcpConnectionState.disconnected);
    }
  }

  void _scheduleReconnect() {
    if (_reconnectTask != null) return;
    final generation = _reconnectGeneration;
    late final Future<void> task;
    task = _reconnect(generation).whenComplete(() {
      if (identical(_reconnectTask, task)) {
        _reconnectTask = null;
      }
    });
    _reconnectTask = task;
  }

  Future<void> _reconnect(int generation) async {
    _setState(AcpConnectionState.reconnecting);
    var replayFailures = 0;
    while (_wanted && generation == _reconnectGeneration) {
      await _waitForReconnectDelay(
        _backoffForAttempt(_reconnectAttempt),
        generation,
      );
      if (!_wanted || generation != _reconnectGeneration) return;

      Transport? transport;
      try {
        transport = await _transportFactory(_acpUri);
        if (!_wanted || generation != _reconnectGeneration) {
          try {
            await transport.close();
          } catch (_) {}
          return;
        }
        final adopted = await _initializeTransport(
          transport,
          generation: generation,
        );
        transport = null;
        if (!adopted) return;
      } catch (_) {
        try {
          await transport?.close();
        } catch (_) {}
        if (!_wanted || generation != _reconnectGeneration) return;
        _reconnectAttempt++;
        continue;
      }

      try {
        final agentId = _lastAgentId;
        if (agentId != null) {
          await startSession(agentId, threadId: _lastThreadId);
          if (!_wanted || generation != _reconnectGeneration) return;
          final modelId = _lastModelId;
          if (modelId != null && currentModel != modelId) {
            await setModel(modelId);
          }
        }
        if (!_wanted || generation != _reconnectGeneration) return;
        _reconnectAttempt = 0;
        _setState(AcpConnectionState.connected);
        return;
      } catch (_) {
        replayFailures++;
        if (replayFailures >= _maxReconnectReplayFailures) {
          if (!_wanted || generation != _reconnectGeneration) return;
          _lastAgentId = null;
          _lastThreadId = null;
          _lastModelId = null;
          _reconnectAttempt = 0;
          _setState(AcpConnectionState.connected);
          return;
        }
        if (!_wanted || generation != _reconnectGeneration) return;
        await _tearDownConnection(bestEffort: true);
        if (!_wanted || generation != _reconnectGeneration) return;
        _reconnectAttempt++;
      }
    }
  }

  Future<void> _waitForReconnectDelay(Duration duration, int generation) async {
    if (duration == Duration.zero) return;
    final delay = Completer<void>();
    _reconnectDelay = delay;
    _reconnectTimer = Timer(duration, () {
      if (!delay.isCompleted) delay.complete();
    });
    await delay.future;
    if (generation == _reconnectGeneration) {
      _reconnectTimer = null;
      _reconnectDelay = null;
    }
  }

  void _cancelReconnect() {
    _reconnectGeneration++;
    _reconnectTimer?.cancel();
    _reconnectTimer = null;
    final delay = _reconnectDelay;
    _reconnectDelay = null;
    if (delay != null && !delay.isCompleted) delay.complete();
    _reconnectTask = null;
  }

  void _setState(AcpConnectionState state) {
    if (_state == state) return;
    _state = state;
    if (!_stateController.isClosed) {
      _stateController.add(state);
    }
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
    _lastAgentId = agentId;
    _lastThreadId = threadId;
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
    _lastModelId = modelId;
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
    _wanted = false;
    _cancelReconnect();
    _lastAgentId = null;
    _lastThreadId = null;
    _lastModelId = null;
    await _tearDownConnection();
    _setState(AcpConnectionState.disconnected);
  }

  Future<void> _tearDownConnection({bool bestEffort = false}) async {
    _activeTurnHandler = null;
    _session?.dispose();
    _session = null;
    _modelOptions = const [];
    _currentModel = null;
    final client = _client;
    _client = null;
    final transport = _transport;
    _transport = null;
    if (bestEffort) {
      try {
        await client?.close();
      } catch (_) {}
      try {
        await transport?.close();
      } catch (_) {}
      return;
    }
    if (client != null) {
      await client.close();
    }
    if (transport != null) {
      await transport.close();
    }
  }
}
