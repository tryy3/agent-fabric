import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_message.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  bool failConnect = false;
  final List<String> prompts = [];
  final List<String> startSessionIds = [];
  final List<String> setModels = [];
  List<String> chunksToEmit = ['hel', 'lo'];
  final _closed = StreamController<void>.broadcast(sync: true);

  @override
  List<ModelOption> modelOptions = const [];

  @override
  String? currentModel;

  @override
  Stream<void> get closed => _closed.stream;

  void simulateDisconnect() {
    connected = false;
    _closed.add(null);
  }

  @override
  Future<void> connect({Transport? transport}) async {
    if (failConnect) {
      throw StateError('dial failed');
    }
    connected = true;
  }

  @override
  Future<void> startSession(String agentId) async {
    startSessionIds.add(agentId);
    modelOptions = const [
      ModelOption(id: 'm1', name: 'Model 1'),
      ModelOption(id: 'm2', name: 'Model 2'),
    ];
    currentModel = 'm1';
  }

  @override
  Future<void> setModel(String modelId) async {
    setModels.add(modelId);
    currentModel = modelId;
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

class FakeCatalog extends CatalogClient {
  FakeCatalog(this.agents)
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient(
          (_) async => http.Response(
            '[]',
            200,
            headers: {'content-type': 'application/json'},
          ),
        ),
      );

  final List<Agent> agents;

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);
}

Agent _agent(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    version: 1,
    providerId: 'prov-1',
    defaultModel: 'm1',
    createdAt: now,
    updatedAt: now,
  );
}

void main() {
  test('connect moves status to connected', () async {
    final fake = FakeConn();
    final c = ChatController(session: fake);
    expect(c.status, ChatStatus.disconnected);
    await c.connect();
    expect(c.status, ChatStatus.connected);
    expect(c.canSend, isFalse);
  });

  test('send appends user message and streams assistant text', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.selectAgent('ag-1');
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

  test('idle peer disconnect sets disconnected and clears canSend', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.selectAgent('ag-1');
    expect(c.status, ChatStatus.connected);
    expect(c.canSend, isTrue);

    fake.simulateDisconnect();

    expect(c.status, ChatStatus.disconnected);
    expect(c.canSend, isFalse);
  });

  test('formatChatError includes RpcError data', () {
    final err = RpcError(
      code: -32603,
      message: 'Internal error',
      data: {'error': 'OpenAI HTTP 401 Unauthorized: Invalid token payload'},
    );
    expect(
      formatChatError(err),
      contains('OpenAI HTTP 401 Unauthorized: Invalid token payload'),
    );
  });

  test('connect loads agents from catalog without starting a session', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([
        _agent('ag-1', 'Alpha'),
        _agent('ag-2', 'Beta'),
      ]),
    );
    await c.connect();
    expect(c.agents.map((a) => a.id).toList(), ['ag-1', 'ag-2']);
    expect(fake.startSessionIds, isEmpty);
    expect(c.canSend, isFalse);
  });

  test('selectAgent starts session with agentId and clears transcript', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([
        _agent('ag-1', 'Alpha'),
        _agent('ag-2', 'Beta'),
      ]),
    );
    await c.connect();
    await c.selectAgent('ag-1');
    expect(fake.startSessionIds, ['ag-1']);
    expect(c.selectedAgentId, 'ag-1');
    expect(c.canSend, isTrue);
    expect(c.currentModel, 'm1');
    expect(c.modelOptions.map((m) => m.id).toList(), ['m1', 'm2']);

    await c.send('keep me');
    expect(c.messages, isNotEmpty);

    await c.selectAgent('ag-2');
    expect(fake.startSessionIds, ['ag-1', 'ag-2']);
    expect(c.selectedAgentId, 'ag-2');
    expect(c.messages, isEmpty);
  });

  test('selectModel forwards setModel to the session', () async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([_agent('ag-1', 'Alpha')]),
    );
    await c.connect();
    await c.selectAgent('ag-1');
    await c.selectModel('m2');
    expect(fake.setModels, ['m2']);
    expect(c.currentModel, 'm2');
    expect(c.messages, isEmpty);
  });
}
