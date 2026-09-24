import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/model_picker.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class FakeConn implements AgentSessionApi {
  final List<String> setModels = [];

  @override
  List<ModelOption> modelOptions = const [];

  @override
  String? currentModel;

  final _connectionState = StreamController<AcpConnectionState>.broadcast(
    sync: true,
  );

  @override
  Stream<void> get closed => const Stream.empty();

  @override
  Stream<AcpConnectionState> get connectionState => _connectionState.stream;

  @override
  Future<void> connect({Transport? transport}) async {
    _connectionState.add(AcpConnectionState.connected);
  }

  @override
  Future<void> startSession(String agentId, {String? threadId}) async {
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
    required AgentTurnHandler onEvent,
  }) async {}

  @override
  Future<void> cancel() async {}

  @override
  Future<void> close() async {}
}

class FakeCatalog extends CatalogClient {
  FakeCatalog(this.agents)
    : threads = [],
      super(
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
  final List<ThreadSummary> threads;
  List<Provider> providers = [];

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);

  @override
  Future<List<Provider>> listProviders() async => List.of(providers);

  @override
  Future<List<ThreadSummary>> listThreads({String? projectId}) async =>
      List.of(threads);

  @override
  Future<ThreadSummary> createThread({String? projectId}) async {
    final t = ThreadSummary(
      id: 'th_1',
      title: 'Untitled',
      titleSource: 'auto',
      createdAt: DateTime.utc(2026, 9, 13),
      updatedAt: DateTime.utc(2026, 9, 13),
    );
    threads.insert(0, t);
    return t;
  }

  @override
  Future<ThreadDetail> getThread(String id) async {
    final thread = threads.firstWhere((t) => t.id == id);
    return ThreadDetail(thread: thread, messages: const []);
  }
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

Provider _localProvider() {
  final now = DateTime.utc(2026, 9, 18);
  return Provider(
    id: 'p-local',
    name: 'Local',
    type: 'openai_compatible',
    baseUrl: 'http://example',
    apiKey: 'k',
    models: const [
      ModelInfo(id: 'm1', name: 'Model 1'),
      ModelInfo(id: 'm2', name: 'Model 2'),
    ],
    createdAt: now,
    updatedAt: now,
  );
}

/// Composer sits at the bottom of chat; place the picker there so the
/// upward-opening popover stays on-screen in tests.
Widget _bottomPickerScaffold(ChatController c) {
  return MaterialApp(
    theme: AppTheme.light(),
    home: Scaffold(
      body: Align(
        alignment: Alignment.bottomCenter,
        child: SizedBox(
          height: 48,
          width: 320,
          child: ModelPicker(controller: c),
        ),
      ),
    ),
  );
}

void main() {
  testWidgets('opens popover, filters by search, selects model', (
    tester,
  ) async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')])
      ..providers = [_localProvider()];
    final c = ChatController(session: fake, catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();
    await c.selectAgent('ag-1');

    await tester.pumpWidget(_bottomPickerScaffold(c));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('model-picker')));
    await tester.pumpAndSettle();

    expect(find.text('Local'), findsOneWidget);
    expect(find.textContaining('(2)'), findsWidgets);

    await tester.enterText(
      find.byKey(const Key('model-picker-search')),
      'Model 2',
    );
    await tester.pumpAndSettle();
    final list = find.byType(ListView);
    expect(
      find.descendant(of: list, matching: find.text('Model 1')),
      findsNothing,
    );
    expect(
      find.descendant(of: list, matching: find.text('Model 2')),
      findsOneWidget,
    );

    await tester.tap(find.descendant(of: list, matching: find.text('Model 2')));
    await tester.pumpAndSettle();
    expect(fake.setModels, ['m2']);
    expect(find.byKey(const Key('model-picker-search')), findsNothing);
  });

  testWidgets('disabled when cannot select model', (tester) async {
    final fake = FakeConn();
    final catalog = FakeCatalog([_agent('ag-1', 'Alpha')])
      ..providers = [_localProvider()];
    final c = ChatController(session: fake, catalog: catalog);
    addTearDown(c.dispose);
    await c.connect();
    await c.createThread();

    await tester.pumpWidget(_bottomPickerScaffold(c));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('model-picker')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('model-picker-search')), findsNothing);
  });
}
