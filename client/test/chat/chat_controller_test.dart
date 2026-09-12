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
