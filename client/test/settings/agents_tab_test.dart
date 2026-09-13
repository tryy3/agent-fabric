import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/agents_tab.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

Provider _provider({
  required String id,
  required String name,
  List<ModelInfo> models = const [],
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Provider(
    id: id,
    name: name,
    type: 'openai_compatible',
    baseUrl: 'http://127.0.0.1:8888/v1',
    apiKey: 'sk-test',
    models: models,
    modelsUpdatedAt: now,
    createdAt: now,
    updatedAt: now,
  );
}

Agent _agent({
  required String id,
  required String name,
  String description = '',
  String? providerId = 'prov-1',
  String? defaultModel = 'm1',
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
    description: description,
    version: 1,
    providerId: providerId,
    defaultModel: defaultModel,
    createdAt: now,
    updatedAt: now,
  );
}

class FakeCatalogClient extends CatalogClient {
  FakeCatalogClient({
    List<Provider>? providers,
    List<Agent>? agents,
  }) : providers = List.of(providers ?? const []),
       agents = List.of(agents ?? const []),
       super(
         baseUri: Uri.parse('http://catalog.test'),
         httpClient: MockClient(
           (_) async => http.Response('unused', 500),
         ),
       );

  final List<Provider> providers;
  final List<Agent> agents;
  Map<String, String>? lastCreate;
  String? lastDeleteId;

  @override
  Future<List<Provider>> listProviders() async {
    return List.of(providers);
  }

  @override
  Future<List<Agent>> listAgents() async {
    return List.of(agents);
  }

  @override
  Future<Agent> createAgent({
    required String name,
    String description = '',
    required String providerId,
    required String defaultModel,
  }) async {
    lastCreate = {
      'name': name,
      'description': description,
      'providerId': providerId,
      'defaultModel': defaultModel,
    };
    final created = _agent(
      id: 'ag-new',
      name: name,
      description: description,
      providerId: providerId,
      defaultModel: defaultModel,
    );
    agents.add(created);
    return created;
  }

  @override
  Future<void> deleteAgent(String id) async {
    lastDeleteId = id;
    agents.removeWhere((a) => a.id == id);
  }
}

void main() {
  testWidgets('create form requires provider and model from cached data', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient(
      providers: [
        _provider(
          id: 'prov-1',
          name: 'Local',
          models: const [
            ModelInfo(id: 'm1', name: 'Model 1'),
            ModelInfo(id: 'm2', name: 'Model 2'),
          ],
        ),
      ],
    );

    await tester.pumpWidget(
      MaterialApp(home: SettingsPage(catalog: catalog)),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Agents'));
    await tester.pumpAndSettle();

    expect(find.byType(AgentsTab), findsOneWidget);

    await tester.tap(find.byTooltip('Add agent'));
    await tester.pumpAndSettle();

    expect(find.widgetWithText(TextButton, 'Create'), findsOneWidget);
    expect(
      tester.widget<TextButton>(find.widgetWithText(TextButton, 'Create')).onPressed,
      isNull,
    );

    await tester.enterText(find.widgetWithText(TextField, 'Name'), 'Helper');
    await tester.enterText(
      find.widgetWithText(TextField, 'Description'),
      'desc',
    );

    await tester.tap(find.byKey(const Key('agent-provider')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Local').last);
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('agent-model')));
    await tester.pumpAndSettle();
    expect(find.text('Model 1'), findsWidgets);
    expect(find.text('Model 2'), findsWidgets);
    await tester.tap(find.text('Model 2').last);
    await tester.pumpAndSettle();

    await tester.tap(find.text('Create'));
    await tester.pumpAndSettle();

    expect(catalog.lastCreate, {
      'name': 'Helper',
      'description': 'desc',
      'providerId': 'prov-1',
      'defaultModel': 'm2',
    });
    expect(find.text('Helper'), findsOneWidget);
  });

  testWidgets('shows list and disabled Coming soon placeholders', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient(
      providers: [
        _provider(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
      agents: [
        _agent(
          id: 'ag-1',
          name: 'Work',
          description: 'office',
          providerId: 'prov-1',
          defaultModel: 'm1',
        ),
      ],
    );

    await tester.pumpWidget(
      MaterialApp(home: AgentsTab(catalog: catalog)),
    );
    await tester.pumpAndSettle();

    expect(find.text('Work'), findsOneWidget);
    expect(find.text('office'), findsOneWidget);

    await tester.tap(find.text('Work'));
    await tester.pumpAndSettle();

    for (final label in ['Tools', 'MCP', 'Sandbox', 'Memory']) {
      final tile = tester.widget<ExpansionTile>(
        find.widgetWithText(ExpansionTile, label),
      );
      expect(tile.enabled, isFalse);
      expect(tile.subtitle, isA<Text>());
      expect((tile.subtitle as Text).data, 'Coming soon');
    }
  });

  testWidgets('incomplete agent shows Needs provider', (tester) async {
    final catalog = FakeCatalogClient(
      agents: [
        _agent(
          id: 'ag-1',
          name: 'Work',
          providerId: null,
          defaultModel: null,
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AgentsTab(catalog: catalog)));
    await tester.pumpAndSettle();
    expect(find.text('Work'), findsOneWidget);
    expect(find.text('Needs provider'), findsOneWidget);
  });

  testWidgets('delete agent confirms then deletes', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [
        _provider(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
      agents: [
        _agent(id: 'ag-1', name: 'Work', providerId: 'prov-1', defaultModel: 'm1'),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AgentsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-agent-ag-1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete agent?'), findsOneWidget);

    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'ag-1');
    expect(find.text('Work'), findsNothing);
  });
}
