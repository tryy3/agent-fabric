import 'dart:async';

import 'package:acpd/acpd.dart' hide AgentConnection;
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
      chunk: ContentChunk(content: TextContentBlock(text: 'hello')),
    );
    expect(agentMessageText(update), 'hello');
    expect(
      agentMessageText(
        UserMessageChunk(
          chunk: ContentChunk(content: TextContentBlock(text: 'x')),
        ),
      ),
      isNull,
    );
  });

  test('agentThoughtText extracts text from AgentThoughtChunk', () {
    expect(
      agentThoughtText(
        AgentThoughtChunk(
          chunk: ContentChunk(content: TextContentBlock(text: 'why')),
        ),
      ),
      'why',
    );
    expect(
      agentThoughtText(
        AgentMessageChunk(
          chunk: ContentChunk(content: TextContentBlock(text: 'x')),
        ),
      ),
      isNull,
    );
  });

  test('turnUsageFromUpdate maps used and numeric meta', () {
    final usage = turnUsageFromUpdate(
      UsageSessionUpdate(
        used: 4,
        meta: {
          'predictedPerSecond': 35.5,
          'stopReason': 'end_turn',
          'deltas': 1,
          'ttftMs': 12.0,
          'promptTokens': 2,
        },
      ),
    );
    expect(usage, isNotNull);
    expect(usage!.totalTokens, 4);
    expect(usage.predictedPerSecond, 35.5);
    expect(usage.stopReason, 'end_turn');
    expect(usage.deltas, 1);
    expect(usage.ttftMs, 12);
    expect(usage.promptTokens, 2);
    expect(
      turnUsageFromUpdate(
        AgentMessageChunk(
          chunk: ContentChunk(content: TextContentBlock(text: 'x')),
        ),
      ),
      isNull,
    );
  });

  test(
    'turnUsageFromUpdate treats used 0 as unknown without meta totalTokens',
    () {
      final usage = turnUsageFromUpdate(
        UsageSessionUpdate(used: 0, meta: {'stopReason': 'end_turn'}),
      );
      expect(usage, isNotNull);
      expect(usage!.totalTokens, isNull);
      expect(usage.stopReason, 'end_turn');
    },
  );

  test('turnUsageFromUpdate keeps meta totalTokens even when used is 0', () {
    final usage = turnUsageFromUpdate(
      UsageSessionUpdate(used: 0, meta: {'totalTokens': 0}),
    );
    expect(usage!.totalTokens, 0);
  });

  test('turnUsageFromUpdate prefers meta totalTokens over used', () {
    final usage = turnUsageFromUpdate(
      UsageSessionUpdate(used: 4, meta: {'totalTokens': 9}),
    );
    expect(usage!.totalTokens, 9);
  });

  test('connect + prompt forwards agent_message_chunk text', () async {
    final (clientTransport, agentTransport) = linkedTransports();

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          expect(request.meta['agentId'], 'ag-1');
          return const NewSessionResponse(
            sessionId: 'sess-1',
            configOptions: [
              SessionConfigSelectOptionValue(
                id: 'model',
                name: 'Model',
                category: SessionConfigOptionCategory.model,
                currentValue: 'm1',
                options: SessionConfigUngroupedOptions([
                  SessionConfigSelectOption(value: 'm1', name: 'M1'),
                  SessionConfigSelectOption(value: 'm2', name: 'M2'),
                ]),
              ),
            ],
          );
        })
        .onPrompt((ctx, request, cancellation) async {
          ctx.sessionUpdate(
            sessionId: request.sessionId,
            update: AgentMessageChunk(
              chunk: ContentChunk(content: TextContentBlock(text: 'hello')),
            ),
          );
          return const PromptResponse(stopReason: StopReason.endTurn);
        })
        .connect(agentTransport);

    final events = <AgentTurnEvent>[];
    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await conn.startSession('ag-1');
    expect(conn.currentModel, 'm1');
    expect(conn.modelOptions.map((m) => m.id).toList(), ['m1', 'm2']);

    await conn.sendPrompt('ping', onEvent: events.add);
    expect(events, hasLength(1));
    expect(events[0], isA<AgentMessageDelta>());
    expect((events[0] as AgentMessageDelta).text, 'hello');

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });

  test('connect + prompt forwards thought, message, and usage', () async {
    final (clientTransport, agentTransport) = linkedTransports();

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          return const NewSessionResponse(sessionId: 'sess-1');
        })
        .onPrompt((ctx, request, cancellation) async {
          ctx.sessionUpdate(
            sessionId: request.sessionId,
            update: AgentThoughtChunk(
              chunk: ContentChunk(content: TextContentBlock(text: 'why')),
            ),
          );
          ctx.sessionUpdate(
            sessionId: request.sessionId,
            update: AgentMessageChunk(
              chunk: ContentChunk(content: TextContentBlock(text: 'hello')),
            ),
          );
          ctx.sessionUpdate(
            sessionId: request.sessionId,
            update: UsageSessionUpdate(
              used: 4,
              meta: {
                'predictedPerSecond': 35.5,
                'stopReason': 'end_turn',
                'deltas': 1,
              },
            ),
          );
          return const PromptResponse(stopReason: StopReason.endTurn);
        })
        .connect(agentTransport);

    final events = <AgentTurnEvent>[];
    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await conn.startSession('ag-1');

    await conn.sendPrompt('ping', onEvent: events.add);
    expect(events[0], isA<AgentThoughtDelta>());
    expect((events[0] as AgentThoughtDelta).text, 'why');
    expect(events[1], isA<AgentMessageDelta>());
    expect((events[1] as AgentMessageDelta).text, 'hello');
    expect(events[2], isA<AgentUsageEvent>());
    expect((events[2] as AgentUsageEvent).usage.predictedPerSecond, 35.5);
    expect((events[2] as AgentUsageEvent).usage.totalTokens, 4);
    expect((events[2] as AgentUsageEvent).usage.stopReason, 'end_turn');
    expect((events[2] as AgentUsageEvent).usage.deltas, 1);

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });

  test('closed emits when peer transport closes after connect', () async {
    final (clientTransport, agentTransport) = linkedTransports();

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          return const NewSessionResponse(sessionId: 'sess-1');
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    final closed = conn.closed.first;
    await conn.connect(transport: clientTransport);

    // Peer drop surfaces as the client transport closing.
    await clientTransport.close();
    await closed.timeout(const Duration(seconds: 2));

    await conn.close();
    await agentConn.close();
    await agentTransport.close();
  });

  test('connect does not create a session', () async {
    final (clientTransport, agentTransport) = linkedTransports();
    var newSessions = 0;

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          newSessions++;
          return const NewSessionResponse(sessionId: 'sess-1');
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    expect(newSessions, 0);
    expect(conn.currentModel, isNull);
    expect(conn.modelOptions, isEmpty);

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });

  test('failed startSession keeps previous session models', () async {
    final (clientTransport, agentTransport) = linkedTransports();
    var attempts = 0;

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          attempts++;
          if (attempts > 1) {
            throw StateError('nope');
          }
          return const NewSessionResponse(
            sessionId: 'sess-1',
            configOptions: [
              SessionConfigSelectOptionValue(
                id: 'model',
                name: 'Model',
                category: SessionConfigOptionCategory.model,
                currentValue: 'm1',
                options: SessionConfigUngroupedOptions([
                  SessionConfigSelectOption(value: 'm1', name: 'M1'),
                ]),
              ),
            ],
          );
        })
        .onPrompt((ctx, request, cancellation) async {
          return const PromptResponse(stopReason: StopReason.endTurn);
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await conn.startSession('ag-1');
    expect(conn.currentModel, 'm1');

    await expectLater(conn.startSession('ag-2'), throwsA(anything));
    expect(conn.currentModel, 'm1');
    await conn.sendPrompt('still-alive', onEvent: (_) {});

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });

  test('failed first startSession clears model cache', () async {
    final (clientTransport, agentTransport) = linkedTransports();

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          throw StateError('nope');
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await expectLater(conn.startSession('ag-1'), throwsA(anything));
    expect(conn.currentModel, isNull);
    expect(conn.modelOptions, isEmpty);

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });

  test('setModel calls session setConfigOption for model', () async {
    final (clientTransport, agentTransport) = linkedTransports();
    Object? captured;

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          return const NewSessionResponse(
            sessionId: 'sess-1',
            configOptions: [
              SessionConfigSelectOptionValue(
                id: 'model',
                name: 'Model',
                category: SessionConfigOptionCategory.model,
                currentValue: 'm1',
                options: SessionConfigUngroupedOptions([
                  SessionConfigSelectOption(value: 'm1', name: 'M1'),
                  SessionConfigSelectOption(value: 'm2', name: 'M2'),
                ]),
              ),
            ],
          );
        })
        .onSetSessionConfigOption((ctx, request, cancellation) async {
          captured = request;
          return const SetSessionConfigOptionResponse(
            configOptions: [
              SessionConfigSelectOptionValue(
                id: 'model',
                name: 'Model',
                category: SessionConfigOptionCategory.model,
                currentValue: 'm2',
                options: SessionConfigUngroupedOptions([
                  SessionConfigSelectOption(value: 'm1', name: 'M1'),
                  SessionConfigSelectOption(value: 'm2', name: 'M2'),
                ]),
              ),
            ],
          );
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await conn.startSession('ag-1');
    await conn.setModel('m2');

    expect(captured, isA<SetValueIdConfigOption>());
    final req = captured! as SetValueIdConfigOption;
    expect(req.configId, 'model');
    expect(req.value, 'm2');
    expect(conn.currentModel, 'm2');

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });

  test('startSession passes threadId in meta', () async {
    final (clientTransport, agentTransport) = linkedTransports();
    Map<String, Object?> recordedMeta = const {};

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          recordedMeta = request.meta;
          return const NewSessionResponse(sessionId: 'sess-1');
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await conn.startSession('ag-1', threadId: 'th_1');
    expect(recordedMeta['agentId'], 'ag-1');
    expect(recordedMeta['threadId'], 'th_1');

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });

  test('cancel sends session/cancel', () async {
    final (clientTransport, agentTransport) = linkedTransports();
    final cancelled = Completer<CancelNotification>();

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          return const NewSessionResponse(sessionId: 'sess-1');
        })
        .onSessionCancel((ctx, notification) {
          cancelled.complete(notification);
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await conn.startSession('ag-1');
    await conn.cancel();
    final notification = await cancelled.future.timeout(
      const Duration(seconds: 2),
    );
    expect(notification.sessionId, 'sess-1');

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });
}
