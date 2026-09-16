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

class _FailingTransport implements Transport {
  @override
  Stream<TransportFrame> get incoming => const Stream.empty();

  @override
  void send(TransportFrame frame) {
    throw StateError('send failed');
  }

  @override
  Future<void> close() async {
    throw StateError('close failed');
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
  test('defaultAcpBackoff caps at 30s', () {
    expect(defaultAcpBackoff(0), const Duration(seconds: 1));
    expect(defaultAcpBackoff(5), const Duration(seconds: 30));
    expect(defaultAcpBackoff(99), const Duration(seconds: 30));
  });

  test(
    'dialed connection reconnects and replays startSession after drop',
    () async {
      var dials = 0;
      final agentClosers = <Future<void> Function()>[];
      final clientTransports = <_End>[];
      final sessionRequests = <Map<String, Object?>>[];
      final modelRequests = <String>[];

      Future<Transport> factory(Uri uri) async {
        dials++;
        final (clientTransport, agentTransport) = linkedTransports();
        clientTransports.add(clientTransport);
        final agentConn = AgentRole()
            .onInitialize((ctx, request, cancellation) async {
              return const InitializeResponse(
                protocolVersion: ProtocolVersion.v1,
                agentInfo: Implementation(name: 'test', version: '0.0.1'),
              );
            })
            .onNewSession((ctx, request, cancellation) async {
              sessionRequests.add(request.meta);
              return NewSessionResponse(
                sessionId: 'sess-$dials',
                configOptions: const [
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
              modelRequests.add((request as SetValueIdConfigOption).value);
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
        agentClosers.add(agentConn.close);
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
      await conn.setModel('m2');
      expect(dials, 1);

      await clientTransports.single.close();

      for (var i = 0; i < 50 && dials < 2; i++) {
        await Future<void>.delayed(const Duration(milliseconds: 10));
      }
      expect(dials, 2);
      expect(sessionRequests, [
        {'agentId': 'ag-1', 'threadId': 'th-1'},
        {'agentId': 'ag-1', 'threadId': 'th-1'},
      ]);
      expect(modelRequests, ['m2', 'm2']);
      expect(states, contains(AcpConnectionState.reconnecting));
      expect(states.last, AcpConnectionState.connected);

      await conn.close();
      expect(states.last, AcpConnectionState.disconnected);
      await sub.cancel();
      for (final closeAgent in agentClosers) {
        await closeAgent();
      }
    },
  );

  test(
    'reconnect abandons session replay after three consecutive failures',
    () async {
      var dials = 0;
      final clientTransports = <_End>[];
      final agentClosers = <Future<void> Function()>[];
      final sessionRequests = <Map<String, Object?>>[];

      Future<Transport> factory(Uri uri) async {
        dials++;
        final (clientTransport, agentTransport) = linkedTransports();
        clientTransports.add(clientTransport);
        final agentConn = AgentRole()
            .onInitialize((ctx, request, cancellation) async {
              return const InitializeResponse(
                protocolVersion: ProtocolVersion.v1,
                agentInfo: Implementation(name: 'test', version: '0.0.1'),
              );
            })
            .onNewSession((ctx, request, cancellation) async {
              sessionRequests.add(request.meta);
              if (dials > 1) {
                throw StateError('thread was deleted');
              }
              return const NewSessionResponse(sessionId: 'sess-1');
            })
            .connect(agentTransport);
        agentClosers.add(agentConn.close);
        return clientTransport;
      }

      final conn = AgentConnection(
        transportFactory: factory,
        backoffForAttempt: (_) => Duration.zero,
      );
      final states = <AcpConnectionState>[];
      final sub = conn.connectionState.listen(states.add);

      await conn.connect();
      await conn.startSession('ag-1', threadId: 'deleted-thread');
      await clientTransports.single.close();

      for (
        var i = 0;
        i < 100 && (dials < 4 || states.last != AcpConnectionState.connected);
        i++
      ) {
        await Future<void>.delayed(const Duration(milliseconds: 10));
      }
      expect(dials, 4);
      expect(sessionRequests, hasLength(4));
      expect(states.last, AcpConnectionState.connected);

      await clientTransports.last.close();
      for (var i = 0; i < 100 && dials < 5; i++) {
        await Future<void>.delayed(const Duration(milliseconds: 10));
      }
      expect(dials, 5);
      expect(sessionRequests, hasLength(4));
      expect(states.last, AcpConnectionState.connected);

      await conn.close();
      await sub.cancel();
      for (final closeAgent in agentClosers) {
        await closeAgent();
      }
    },
  );

  test('injected transport does not auto-reconnect', () async {
    final (clientTransport, agentTransport) = linkedTransports();
    var dials = 0;
    final conn = AgentConnection(
      transportFactory: (_) async {
        dials++;
        throw StateError('should not dial');
      },
    );
    final states = <AcpConnectionState>[];
    final sub = conn.connectionState.listen(states.add);

    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'test', version: '0.0.1'),
          );
        })
        .connect(agentTransport);

    await conn.connect(transport: clientTransport);
    final closed = conn.closed.first;
    await clientTransport.close();
    await closed.timeout(const Duration(seconds: 2));
    expect(dials, 0);
    expect(states.last, AcpConnectionState.disconnected);

    await conn.close();
    await sub.cancel();
    await agentConn.close();
    await agentTransport.close();
  });

  test('close cancels reconnect loop', () async {
    var dials = 0;
    final firstClientReady = Completer<_End>();
    final secondDialStarted = Completer<void>();
    final releaseSecondDial = Completer<void>();

    final conn = AgentConnection(
      transportFactory: (_) async {
        dials++;
        if (dials == 1) {
          final (clientTransport, agentTransport) = linkedTransports();
          AgentRole()
              .onInitialize((ctx, request, cancellation) async {
                return const InitializeResponse(
                  protocolVersion: ProtocolVersion.v1,
                  agentInfo: Implementation(name: 'test', version: '0.0.1'),
                );
              })
              .connect(agentTransport);
          firstClientReady.complete(clientTransport);
          return clientTransport;
        }
        secondDialStarted.complete();
        await releaseSecondDial.future;
        throw StateError('released after close');
      },
      backoffForAttempt: (_) => Duration.zero,
    );

    await conn.connect();
    await (await firstClientReady.future).close();
    await secondDialStarted.future.timeout(const Duration(seconds: 2));
    await conn.close();
    releaseSecondDial.complete();
    final dialsAfterClose = dials;
    await Future<void>.delayed(const Duration(milliseconds: 50));
    expect(dials, dialsAfterClose);
  });

  test('stale reconnect does not tear down a newer connect', () async {
    var dials = 0;
    final clients = <_End>[];
    final holdSecondInit = Completer<void>();
    final secondInitStarted = Completer<void>();
    final agents = <Future<void> Function()>[];

    Future<Transport> factory(Uri uri) async {
      dials++;
      final dial = dials;
      final (clientTransport, agentTransport) = linkedTransports();
      clients.add(clientTransport);
      final agentConn = AgentRole()
          .onInitialize((ctx, request, cancellation) async {
            if (dial == 2) {
              secondInitStarted.complete();
              await holdSecondInit.future;
              throw StateError('stale init');
            }
            return const InitializeResponse(
              protocolVersion: ProtocolVersion.v1,
              agentInfo: Implementation(name: 'test', version: '0.0.1'),
            );
          })
          .onNewSession((ctx, request, cancellation) async {
            return NewSessionResponse(sessionId: 'sess-$dial');
          })
          .connect(agentTransport);
      agents.add(agentConn.close);
      return clientTransport;
    }

    final conn = AgentConnection(
      transportFactory: factory,
      backoffForAttempt: (_) => Duration.zero,
    );
    await conn.connect();
    expect(dials, 1);

    await clients.first.close();
    await secondInitStarted.future.timeout(const Duration(seconds: 2));
    expect(dials, 2);

    await conn.connect();
    expect(dials, 3);
    expect(conn.connectionState, isNotNull);
    await conn.startSession('ag-1');

    holdSecondInit.complete();
    await Future<void>.delayed(const Duration(milliseconds: 50));

    // Newer connection must still be usable after the stale attempt fails.
    await conn.startSession('ag-2');
    await conn.close();
    for (final closeAgent in agents) {
      try {
        await closeAgent();
      } catch (_) {}
    }
  });

  test('reconnect retries when failed transport cleanup throws', () async {
    var dials = 0;
    final clientTransports = <_End>[];
    final agentClosers = <Future<void> Function()>[];

    Future<Transport> factory(Uri uri) async {
      dials++;
      if (dials == 2) return _FailingTransport();

      final (clientTransport, agentTransport) = linkedTransports();
      clientTransports.add(clientTransport);
      final agentConn = AgentRole()
          .onInitialize((ctx, request, cancellation) async {
            return const InitializeResponse(
              protocolVersion: ProtocolVersion.v1,
              agentInfo: Implementation(name: 'test', version: '0.0.1'),
            );
          })
          .connect(agentTransport);
      agentClosers.add(agentConn.close);
      return clientTransport;
    }

    final conn = AgentConnection(
      transportFactory: factory,
      backoffForAttempt: (_) => Duration.zero,
    );
    await conn.connect();
    await clientTransports.single.close();

    for (var i = 0; i < 50 && dials < 3; i++) {
      await Future<void>.delayed(const Duration(milliseconds: 10));
    }
    expect(dials, 3);

    await conn.close();
    for (final closeAgent in agentClosers) {
      await closeAgent();
    }
  });

  test('close clears session replay state before a new lifecycle', () async {
    var dials = 0;
    final clientTransports = <_End>[];
    final agentClosers = <Future<void> Function()>[];
    final sessionRequests = <Map<String, Object?>>[];

    Future<Transport> factory(Uri uri) async {
      dials++;
      final (clientTransport, agentTransport) = linkedTransports();
      clientTransports.add(clientTransport);
      final agentConn = AgentRole()
          .onInitialize((ctx, request, cancellation) async {
            return const InitializeResponse(
              protocolVersion: ProtocolVersion.v1,
              agentInfo: Implementation(name: 'test', version: '0.0.1'),
            );
          })
          .onNewSession((ctx, request, cancellation) async {
            sessionRequests.add(request.meta);
            return const NewSessionResponse(sessionId: 'sess');
          })
          .connect(agentTransport);
      agentClosers.add(agentConn.close);
      return clientTransport;
    }

    final conn = AgentConnection(
      transportFactory: factory,
      backoffForAttempt: (_) => Duration.zero,
    );
    await conn.connect();
    await conn.startSession('ag-1', threadId: 'th-1');
    await conn.close();

    await conn.connect();
    await clientTransports.last.close();
    for (var i = 0; i < 50 && dials < 3; i++) {
      await Future<void>.delayed(const Duration(milliseconds: 10));
    }

    expect(dials, 3);
    expect(sessionRequests, [
      {'agentId': 'ag-1', 'threadId': 'th-1'},
    ]);

    await conn.close();
    for (final closeAgent in agentClosers) {
      await closeAgent();
    }
  });

  test('close clears model replay state before a new lifecycle', () async {
    var dials = 0;
    final clientTransports = <_End>[];
    final agentClosers = <Future<void> Function()>[];
    final modelRequests = <String>[];

    Future<Transport> factory(Uri uri) async {
      dials++;
      final (clientTransport, agentTransport) = linkedTransports();
      clientTransports.add(clientTransport);
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
              configOptions: const [
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
            modelRequests.add((request as SetValueIdConfigOption).value);
            return const SetSessionConfigOptionResponse(configOptions: []);
          })
          .connect(agentTransport);
      agentClosers.add(agentConn.close);
      return clientTransport;
    }

    final conn = AgentConnection(
      transportFactory: factory,
      backoffForAttempt: (_) => Duration.zero,
    );
    await conn.connect();
    await conn.startSession('ag-1');
    await conn.setModel('m2');
    await conn.close();

    await conn.connect();
    await conn.startSession('ag-2');
    await clientTransports.last.close();
    for (var i = 0; i < 50 && dials < 3; i++) {
      await Future<void>.delayed(const Duration(milliseconds: 10));
    }

    expect(dials, 3);
    expect(modelRequests, ['m2']);

    await conn.close();
    for (final closeAgent in agentClosers) {
      await closeAgent();
    }
  });

  test('sendPrompt fails when transport drops mid-turn', () async {
    final (clientTransport, agentTransport) = linkedTransports();
    final promptStarted = Completer<void>();
    final holdPrompt = Completer<PromptResponse>();

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
        .onPrompt((ctx, request, cancellation) {
          promptStarted.complete();
          return holdPrompt.future;
        })
        .connect(agentTransport);

    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);
    await conn.startSession('ag-1');
    final prompt = conn.sendPrompt('ping', onEvent: (_) {});
    await promptStarted.future.timeout(const Duration(seconds: 2));

    await clientTransport.close();
    await expectLater(prompt, throwsA(anything));

    holdPrompt.complete(const PromptResponse(stopReason: StopReason.cancelled));
    await conn.close();
    await agentConn.close();
    await agentTransport.close();
  });

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

  test('agentToolCallEventFromUpdate maps starts and partial updates', () {
    final start = agentToolCallEventFromUpdate(
      ToolCallUpdateSession(
        toolCall: const ToolCall(
          toolCallId: 'call_1',
          title: 'Read file',
          status: ToolCallStatus.inProgress,
          rawInput: {'path': 'notes.txt'},
        ),
      ),
    );
    expect(start, isNotNull);
    expect(start!.id, 'call_1');
    expect(start.title, 'Read file');
    expect(start.status, 'in_progress');
    expect(start.rawInput, {'path': 'notes.txt'});
    expect(start.inProgress, isTrue);

    final completed = agentToolCallEventFromUpdate(
      ToolCallStatusUpdate(
        update: const ToolCallUpdate(
          toolCallId: 'call_1',
          status: ToolCallStatus.completed,
          rawOutput: {'content': 'hello'},
        ),
      ),
    );
    expect(completed, isNotNull);
    expect(completed!.title, isNull);
    expect(completed.status, 'completed');
    expect(completed.rawInput, isNull);
    expect(completed.rawOutput, {'content': 'hello'});
    expect(completed.inProgress, isFalse);
    expect(
      agentToolCallEventFromUpdate(
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
