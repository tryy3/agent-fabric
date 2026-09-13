import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/chat_screen.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class FakeConn implements AgentSessionApi {
  bool connected = false;
  bool failConnect = false;
  bool failStartSession = false;
  bool failSetModel = false;
  Completer<void>? startHang;
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
    final hang = startHang;
    if (hang != null) {
      await hang.future;
    }
    if (failStartSession) {
      throw StateError('session failed');
    }
    modelOptions = const [
      ModelOption(id: 'm1', name: 'Model 1'),
      ModelOption(id: 'm2', name: 'Model 2'),
    ];
    currentModel = 'm1';
  }

  @override
  Future<void> setModel(String modelId) async {
    setModels.add(modelId);
    if (failSetModel) {
      throw StateError('setModel failed');
    }
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

Agent _incomplete(String id, String name) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    version: 2,
    providerId: null,
    defaultModel: null,
    createdAt: now,
    updatedAt: now,
  );
}

void main() {
  testWidgets('incomplete agent is labeled and not selectable', (tester) async {
    final fake = FakeConn();
    final c = ChatController(
      session: fake,
      catalog: FakeCatalog([
        _agent('ag-1', 'Alpha'),
        _incomplete('ag-2', 'Work'),
      ]),
    );
    addTearDown(c.dispose);
    await c.connect();

    await tester.pumpWidget(MaterialApp(home: ChatScreen(controller: c)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('agent-picker')));
    await tester.pumpAndSettle();
    expect(find.text('Work — needs provider'), findsOneWidget);

    await tester.tap(find.text('Work — needs provider'));
    await tester.pumpAndSettle();
    expect(fake.startSessionIds, isEmpty);
    expect(c.selectedAgentId, isNull);
  });
}
