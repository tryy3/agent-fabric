import 'dart:async';

import 'package:acpd/acpd.dart' hide AgentConnection;
import 'package:http/http.dart' as http;

import 'gate_info.dart';
import 'ws_transport.dart';

import 'package:agent_fabric_client/core/app_log.dart';

sealed class AgentTurnEvent {
  const AgentTurnEvent();
}

final class AgentThoughtDelta extends AgentTurnEvent {
  const AgentThoughtDelta(this.text);
  final String text;
}

final class AgentSentEvent extends AgentTurnEvent {
  const AgentSentEvent(this.text);
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
    this.kind,
    this.gate,
  });

  final String id;
  final String? title;
  final String? status;
  final Object? rawInput;
  final Object? rawOutput;
  final bool inProgress;

  /// ACP tool kind (`edit`, `move`, `delete`, ...); null on updates that omit it.
  final String? kind;

  /// Tool gate decision from `_meta.gate`; null when the update carries none.
  final GateInfo? gate;
}

final class AgentUsageEvent extends AgentTurnEvent {
  const AgentUsageEvent(this.usage);
  final TurnUsage usage;
}

/// Estimated cost of a turn (or round) in [currency], from the plane's model
/// specs. An estimate, never billing; [partial] means some prices were unknown.
class TurnCost {
  const TurnCost({
    required this.total,
    this.currency = 'USD',
    this.estimated = true,
    this.partial = false,
    this.input = 0,
    this.cacheRead = 0,
    this.cacheWrite = 0,
    this.output = 0,
    this.reasoning = 0,
  });

  final double total;
  final String currency;
  final bool estimated;
  final bool partial;
  final double input;
  final double cacheRead;
  final double cacheWrite;
  final double output;
  final double reasoning;

  static TurnCost? tryParse(Object? raw) {
    if (raw is! Map) return null;
    final total = raw['total'];
    if (total is! num) return null;
    double n(String key) =>
        (raw[key] is num) ? (raw[key]! as num).toDouble() : 0;
    return TurnCost(
      total: total.toDouble(),
      currency: raw['currency'] as String? ?? 'USD',
      estimated: raw['estimated'] as bool? ?? true,
      partial: raw['partial'] as bool? ?? false,
      input: n('input'),
      cacheRead: n('cacheRead'),
      cacheWrite: n('cacheWrite'),
      output: n('output'),
      reasoning: n('reasoning'),
    );
  }

  Map<String, Object?> toJson() => {
    'currency': currency,
    'estimated': estimated,
    'total': total,
    'input': input,
    'cacheRead': cacheRead,
    'cacheWrite': cacheWrite,
    'output': output,
    'reasoning': reasoning,
    if (partial) 'partial': true,
  };
}

/// Per-round entries of a tool-using turn, as sent by the plane.
List<Map<String, Object?>> parseTurnRounds(Object? raw) {
  if (raw is! List) return const [];
  return [
    for (final r in raw)
      if (r is Map) r.cast<String, Object?>(),
  ];
}

class TurnUsage {
  const TurnUsage({
    this.cachedTokens,
    this.cacheWriteTokens,
    this.reasoningTokens,
    this.cost,
    this.reportedCostUsd,
    this.rounds = const [],
    this.isPartial = false,
    this.round,
    this.promptTokens,
    this.completionTokens,
    this.totalTokens,
    this.ttftMs,
    this.elapsedMs,
    this.promptMs,
    this.predictedMs,
    this.promptPerSecond,
    this.predictedPerSecond,
    this.co2Grams,
    this.gpuEnergyJoules,
    this.deltas,
    this.stopReason,
    this.extras = const {},
  });
  final int? promptTokens;
  final int? completionTokens;
  final int? totalTokens;

  /// Input tokens served from the provider's prompt cache (part of
  /// [promptTokens]).
  final int? cachedTokens;
  final int? cacheWriteTokens;

  /// Reasoning tokens (part of [completionTokens]).
  final int? reasoningTokens;

  /// Estimated cost so far (running total on a [isPartial] update).
  final TurnCost? cost;

  /// Cost the provider itself reported, when it did (USD).
  final double? reportedCostUsd;

  /// Per-round usage and cost of the turn, when it had more than one round.
  final List<Map<String, Object?>> rounds;

  /// True for a mid-turn update sent after a tool-calling round; the turn is
  /// not finished and no stats bubble should be created from it.
  final bool isPartial;

  /// Zero-based round index of a partial update.
  final int? round;
  final int? ttftMs;
  final int? elapsedMs;
  final double? promptMs;
  final double? predictedMs;
  final double? promptPerSecond;
  final double? predictedPerSecond;
  final double? co2Grams;
  final double? gpuEnergyJoules;
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
  'co2Grams',
  'gpuEnergyJoules',
  'deltas',
  'stopReason',
  'cachedTokens',
  'cacheWriteTokens',
  'reasoningTokens',
  'cost',
  'roundCost',
  'reportedCostUsd',
  'rounds',
  'partial',
  'round',
  'type', // catalog part discriminator, not a stat
};

typedef AgentTurnHandler = void Function(AgentTurnEvent event);
typedef TransportFactory = Future<Transport> Function(Uri uri);

/// Returns true when the control plane looks reachable (catalog HTTP up).
typedef ReachabilityProbe = Future<bool> Function();

/// Handles ACP [session/request_permission] from the UI layer.
typedef PermissionRequestHandler = Future<RequestPermissionResponse> Function(
  RequestPermissionRequest request,
  RequestCancellation cancellation,
);

/// Handles ACP [elicitation/create] from the UI layer; return response JSON.
typedef ElicitationRequestHandler = Future<Map<String, Object?>> Function(
  Map<String, Object?> params,
  RequestCancellation cancellation,
);

/// Local-dev ACP WebSocket URI when `/config.json` is absent.
final defaultAcpUri = Uri.parse('ws://localhost:8080/acp');

/// Bound for ACP [initialize] during connect and reconnect.
const kAcpInitializeTimeout = Duration(seconds: 15);

/// Bound for session replay ([startSession] / [setModel]) during reconnect.
const kAcpSessionReplayTimeout = Duration(seconds: 15);

/// Outer bound around dial during reconnect (shorter than platform connect).
const kAcpReconnectDialTimeout = Duration(seconds: 5);

/// How often [ReachabilityProbe] runs while waiting on reconnect backoff.
const kAcpReachabilityPollInterval = Duration(seconds: 1);

enum AcpConnectionState { disconnected, connecting, connected, reconnecting }

Duration defaultAcpBackoff(int attempt) {
  if (attempt >= 5) return const Duration(seconds: 30);
  return Duration(seconds: 1 << attempt);
}

/// Catalog HTTP probe used to punch through reconnect backoff when the plane
/// returns mid-wait. Any completed response means the listen socket is up.
ReachabilityProbe catalogHttpProbe(Uri catalogBase, {http.Client? httpClient}) {
  return () async {
    final ownsClient = httpClient == null;
    final client = httpClient ?? http.Client();
    try {
      final response = await client
          .get(catalogBase.resolve('/v1/settings'))
          .timeout(const Duration(seconds: 2));
      // Any HTTP response (including 4xx/5xx) proves the plane accepted TCP.
      return response.statusCode > 0;
    } on Object {
      return false;
    } finally {
      if (ownsClient) {
        client.close();
      }
    }
  };
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

/// Returns sent-prompt text from agentFabric extension meta; otherwise null.
String? agentSentTextFromMeta(Map<String, Object?> meta) {
  final fabric = meta['agentFabric'];
  if (fabric is! Map) {
    return null;
  }
  if (fabric['kind'] != 'sent') {
    return null;
  }
  final text = fabric['text'];
  return text is String && text.isNotEmpty ? text : null;
}

/// Returns sent-prompt text from a session_info_update extension; otherwise null.
String? agentSentText(SessionUpdate update) {
  if (update is! SessionInfoSessionUpdate) {
    return null;
  }
  return agentSentTextFromMeta(update.meta);
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
    kind: tool.kind?.toJson(),
    status: status,
    rawInput: tool.rawInput,
    rawOutput: tool.rawOutput,
    inProgress: status != 'completed' && status != 'failed',
    gate: GateInfo.tryParse(tool.meta['gate']),
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
    co2Grams: _metaDouble(meta, 'co2Grams'),
    gpuEnergyJoules: _metaDouble(meta, 'gpuEnergyJoules'),
    deltas: _metaInt(meta, 'deltas'),
    cachedTokens: _metaInt(meta, 'cachedTokens'),
    cacheWriteTokens: _metaInt(meta, 'cacheWriteTokens'),
    reasoningTokens: _metaInt(meta, 'reasoningTokens'),
    cost: TurnCost.tryParse(meta['cost']),
    reportedCostUsd: _metaDouble(meta, 'reportedCostUsd'),
    rounds: parseTurnRounds(meta['rounds']),
    isPartial: meta['partial'] == true,
    round: _metaInt(meta, 'round'),
    stopReason: () {
      final value = meta['stopReason'];
      return value is String ? value : null;
    }(),
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

  /// Interrupt reconnect backoff and dial immediately when reconnecting.
  void retryNow();
  Future<void> startSession(String assistantId, {String? threadId});

  /// Effective instructions pinned at [startSession], when the plane provided them.
  String? get pinnedPrompt;
  Future<void> setModel(String modelId);
  List<ModelOption> get modelOptions;
  String? get currentModel;
  Future<StopReason> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
    bool retryLatest = false,
  });
  Future<void> cancel();
  Future<void> close();
}

class AgentConnection implements AgentSessionApi {
  static const _maxReconnectReplayFailures = 3;

  AgentConnection({
    TransportFactory? transportFactory,
    Uri? acpUri,
    Duration Function(int) backoffForAttempt = defaultAcpBackoff,
    ReachabilityProbe? reachabilityProbe,
    Duration initializeTimeout = kAcpInitializeTimeout,
    Duration sessionReplayTimeout = kAcpSessionReplayTimeout,
  }) : _transportFactory =
           transportFactory ?? ((uri) => WsTransport.connect(uri)),
       _acpUri = acpUri ?? defaultAcpUri,
       // Keep the public injection point free of a private-name prefix.
       // ignore: prefer_initializing_formals
       _backoffForAttempt = backoffForAttempt,
       _reachabilityProbe = reachabilityProbe,
       _initializeTimeout = initializeTimeout,
       _sessionReplayTimeout = sessionReplayTimeout;

  ClientConnection? _client;
  Session? _session;
  Transport? _transport;
  AgentTurnHandler? _activeTurnHandler;
  String? _pinnedPrompt;
  final _closedController = StreamController<void>.broadcast(sync: true);
  final _stateController = StreamController<AcpConnectionState>.broadcast(
    sync: true,
  );
  final TransportFactory _transportFactory;
  final Uri _acpUri;
  final Duration Function(int) _backoffForAttempt;
  final ReachabilityProbe? _reachabilityProbe;
  final Duration _initializeTimeout;
  final Duration _sessionReplayTimeout;

  AcpConnectionState _state = AcpConnectionState.disconnected;
  bool _wanted = false;
  bool _autoReconnect = false;
  String? _lastAssistantId;
  String? _lastThreadId;
  String? _lastModelId;
  int _reconnectAttempt = 0;
  int _reconnectGeneration = 0;
  Timer? _reconnectTimer;
  Completer<void>? _reconnectDelay;
  Future<void>? _reconnectTask;
  bool _skipNextDelay = false;

  List<ModelOption> _modelOptions = const [];
  String? _currentModel;

  /// Set by the chat UI to present permission and ask_user prompts.
  PermissionRequestHandler? permissionHandler;
  ElicitationRequestHandler? elicitationHandler;

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
    } on Object catch (_) {
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
          final handler = permissionHandler;
          if (handler == null) {
            return const RequestPermissionResponse(
              outcome: PermissionCancelled(),
            );
          }
          return handler(request, cancellation);
        })
        .handleCancellableRequest('elicitation/create', (
          params,
          ctx,
          cancellation,
        ) async {
          final handler = elicitationHandler;
          if (handler == null) {
            return <String, Object?>{'action': 'cancel'};
          }
          return handler(params, cancellation);
        })
        .onSessionUpdate((context, notification) async {
          final handler = _activeTurnHandler;
          if (handler == null) return;
          final update = notification.update;
          final sent = agentSentText(update);
          if (sent != null) {
            handler(AgentSentEvent(sent));
            return;
          }
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
      await client.client
          .initialize(
            InitializeRequest(
              protocolVersion: ProtocolVersion.v1,
              clientInfo: const Implementation(
                name: 'agent-fabric-client',
                version: '0.1.0',
              ),
              // acpd 1.0.0 has no typed elicitation capability field.
              meta: const {
                'elicitation': {'form': <String, Object?>{}},
              },
            ),
          )
          .timeout(_initializeTimeout);
    } on Object catch (_) {
      try {
        await client.close();
      } on Object catch (e, s) {
        AppLog.record('teardown: $e', s);
      }
      try {
        await t.close();
      } on Object catch (e, s) {
        AppLog.record('teardown: $e', s);
      }
      rethrow;
    }

    if (!_wanted || generation != _reconnectGeneration) {
      try {
        await client.close();
      } on Object catch (e, s) {
        AppLog.record('teardown: $e', s);
      }
      try {
        await t.close();
      } on Object catch (e, s) {
        AppLog.record('teardown: $e', s);
      }
      return false;
    }

    _transport = t;
    _client = client;
    unawaited(
      client.closed
          .then((_) {
            if (identical(_client, client)) {
              _handleConnectionClosed();
            }
            if (!_closedController.isClosed) {
              _closedController.add(null);
            }
          })
          .catchError((Object e, StackTrace s) {
            AppLog.record('client.closed handler: $e', s);
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
      Transport? transport;
      try {
        transport = await _transportFactory(_acpUri)
            .timeout(kAcpReconnectDialTimeout);
        if (!_wanted || generation != _reconnectGeneration) {
          try {
            await transport.close();
          } on Object catch (e, s) {
            AppLog.record('teardown: $e', s);
          }
          return;
        }
        final adopted = await _initializeTransport(
          transport,
          generation: generation,
        );
        transport = null;
        if (!adopted) return;
      } on Object catch (_) {
        try {
          await transport?.close();
        } on Object catch (e, s) {
          AppLog.record('teardown: $e', s);
        }
        if (!_wanted || generation != _reconnectGeneration) return;
        _reconnectAttempt++;
        await _waitForReconnectDelay(
          _backoffForAttempt(_reconnectAttempt - 1),
          generation,
        );
        continue;
      }

      try {
        final assistantId = _lastAssistantId;
        if (assistantId != null) {
          await startSession(
            assistantId,
            threadId: _lastThreadId,
          ).timeout(_sessionReplayTimeout);
          if (!_wanted || generation != _reconnectGeneration) return;
          final modelId = _lastModelId;
          if (modelId != null && currentModel != modelId) {
            await setModel(modelId).timeout(_sessionReplayTimeout);
          }
        }
        if (!_wanted || generation != _reconnectGeneration) return;
        _reconnectAttempt = 0;
        _setState(AcpConnectionState.connected);
        return;
      } on Object catch (_) {
        replayFailures++;
        if (replayFailures >= _maxReconnectReplayFailures) {
          if (!_wanted || generation != _reconnectGeneration) return;
          _lastAssistantId = null;
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
        await _waitForReconnectDelay(
          _backoffForAttempt(_reconnectAttempt - 1),
          generation,
        );
      }
    }
  }

  /// Interrupt the current reconnect backoff so the next dial starts now.
  ///
  /// No-op unless actively reconnecting. Does not cancel an in-flight dial;
  /// if called mid-dial, the following backoff wait is skipped instead.
  @override
  void retryNow() {
    if (_state != AcpConnectionState.reconnecting) return;
    _reconnectAttempt = 0;
    _skipNextDelay = true;
    final delay = _reconnectDelay;
    if (delay != null && !delay.isCompleted) {
      _reconnectTimer?.cancel();
      _reconnectTimer = null;
      delay.complete();
    }
  }

  Future<void> _waitForReconnectDelay(Duration duration, int generation) async {
    if (_skipNextDelay) {
      _skipNextDelay = false;
      return;
    }
    if (duration == Duration.zero) return;
    if (!_wanted || generation != _reconnectGeneration) return;
    final delay = Completer<void>();
    _reconnectDelay = delay;
    _reconnectTimer = Timer(duration, () {
      if (!delay.isCompleted) delay.complete();
    });

    final probe = _reachabilityProbe;
    if (probe != null) {
      unawaited(
        _pollReachability(probe, delay, generation).catchError((
          Object e,
          StackTrace s,
        ) {
          AppLog.record('reachability probe: $e', s);
        }),
      );
    }

    await delay.future;
    if (generation == _reconnectGeneration) {
      _reconnectTimer = null;
      _reconnectDelay = null;
    }
  }

  Future<void> _pollReachability(
    ReachabilityProbe probe,
    Completer<void> delay,
    int generation,
  ) async {
    while (!delay.isCompleted &&
        _wanted &&
        generation == _reconnectGeneration) {
      await Future<void>.delayed(kAcpReachabilityPollInterval);
      if (delay.isCompleted || !_wanted || generation != _reconnectGeneration) {
        return;
      }
      final reachable = await probe();
      if (reachable && !delay.isCompleted) {
        _reconnectTimer?.cancel();
        _reconnectTimer = null;
        delay.complete();
        return;
      }
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
    _skipNextDelay = false;
  }

  void _setState(AcpConnectionState state) {
    if (_state == state) return;
    _state = state;
    if (!_stateController.isClosed) {
      _stateController.add(state);
    }
  }

  @override
  Future<void> startSession(String assistantId, {String? threadId}) async {
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
          meta: {'assistantId': assistantId, 'threadId': ?threadId},
        ),
      );
    } on Object catch (_) {
      if (_session == null) {
        _modelOptions = const [];
        _currentModel = null;
        _pinnedPrompt = null;
      }
      rethrow;
    }
    final previous = _session;
    _session = next;
    _pinnedPrompt = agentSentTextFromMeta(next.meta);
    if (previous != null) {
      try {
        await previous.close();
      } on Object catch (_) {
        previous.dispose();
      }
    }
    _syncModels();
    _lastAssistantId = assistantId;
    _lastThreadId = threadId;
  }

  @override
  String? get pinnedPrompt => _pinnedPrompt;

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
  Future<StopReason> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
    bool retryLatest = false,
  }) async {
    final session = _session;
    final client = _client;
    if (session == null || client == null) {
      throw StateError('AgentConnection is not connected');
    }
    _activeTurnHandler = onEvent;
    try {
      final response = await client.client.prompt(
        PromptRequest(
          sessionId: session.sessionId,
          prompt: [TextContentBlock(text: text)],
          meta: retryLatest ? const {'retryLatest': true} : null,
        ),
      );
      return response.stopReason;
    } finally {
      _activeTurnHandler = null;
    }
  }

  @override
  Future<void> close() async {
    _wanted = false;
    _cancelReconnect();
    _lastAssistantId = null;
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
      } on Object catch (e, s) {
        AppLog.record('teardown: $e', s);
      }
      try {
        await transport?.close();
      } on Object catch (e, s) {
        AppLog.record('teardown: $e', s);
      }
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
