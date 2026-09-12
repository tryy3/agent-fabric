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
      chunk: ContentChunk(
        content: TextContentBlock(text: 'hello'),
      ),
    );
    expect(agentMessageText(update), 'hello');
    expect(
      agentMessageText(UserMessageChunk(
        chunk: ContentChunk(content: TextContentBlock(text: 'x')),
      )),
      isNull,
    );
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
          return const NewSessionResponse(sessionId: 'sess-1');
        })
        .onPrompt((ctx, request, cancellation) async {
          ctx.sessionUpdate(
            sessionId: request.sessionId,
            update: AgentMessageChunk(
              chunk: ContentChunk(
                content: TextContentBlock(text: 'hello'),
              ),
            ),
          );
          return const PromptResponse(stopReason: StopReason.endTurn);
        })
        .connect(agentTransport);

    final chunks = <String>[];
    final conn = AgentConnection();
    await conn.connect(transport: clientTransport);

    await conn.sendPrompt('ping', onChunk: chunks.add);
    expect(chunks, ['hello']);

    await conn.close();
    await agentConn.close();
    await clientTransport.close();
    await agentTransport.close();
  });
}
