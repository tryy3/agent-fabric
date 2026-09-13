import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/app_shell.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class _FakeConn implements AgentSessionApi {
  final _closed = StreamController<void>.broadcast(sync: true);

  @override
  Stream<void> get closed => _closed.stream;

  @override
  Future<void> connect({Transport? transport}) async {}

  @override
  Future<void> startSession(String agentId) async {}

  @override
  Future<void> setModel(String modelId) async {}

  @override
  List<ModelOption> get modelOptions => const [];

  @override
  String? get currentModel => null;

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentChunkHandler onChunk,
  }) async {}

  @override
  Future<void> close() async {}
}

Provider _provider({
  required String id,
  required String name,
  List<ModelInfo> models = const [],
  DateTime? modelsUpdatedAt,
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Provider(
    id: id,
    name: name,
    type: 'openai_compatible',
    baseUrl: 'http://127.0.0.1:8888/v1',
    apiKey: 'sk-test',
    models: models,
    modelsUpdatedAt: modelsUpdatedAt,
    createdAt: now,
    updatedAt: now,
  );
}

Agent _agent({
  required String id,
  required String name,
  String? providerId,
  String? defaultModel,
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Agent(
    id: id,
    name: name,
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
    this.refreshError,
  }) : providers = List.of(providers ?? const []),
       agents = List.of(agents ?? const []),
       super(
         baseUri: Uri.parse('http://catalog.test'),
         httpClient: MockClient((_) async => http.Response('unused', 500)),
       );

  final List<Provider> providers;
  final List<Agent> agents;
  Map<String, String>? lastCreate;
  Map<String, String?>? lastUpdate;
  String? lastDeleteId;
  String? lastRefreshId;
  final Object? refreshError;

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);

  @override
  Future<Provider> updateProvider(
    String id, {
    String? name,
    String? baseUrl,
    String? apiKey,
  }) async {
    lastUpdate = {'id': id, 'name': name, 'baseUrl': baseUrl, 'apiKey': apiKey};
    final index = providers.indexWhere((p) => p.id == id);
    final current = providers[index];
    final updated = _provider(id: id, name: name ?? current.name);
    providers[index] = updated;
    return updated;
  }

  @override
  Future<void> deleteProvider(String id) async {
    lastDeleteId = id;
    providers.removeWhere((p) => p.id == id);
  }

  @override
  Future<List<Provider>> listProviders() async => List.of(providers);

  @override
  Future<Provider> createProvider({
    required String name,
    required String type,
    required String baseUrl,
    required String apiKey,
  }) async {
    lastCreate = {
      'name': name,
      'type': type,
      'baseUrl': baseUrl,
      'apiKey': apiKey,
    };
    final created = _provider(id: 'prov-new', name: name);
    providers.add(created);
    return created;
  }

  @override
  Future<Provider> refreshModels(String id) async {
    lastRefreshId = id;
    if (refreshError != null) {
      throw refreshError!;
    }
    final index = providers.indexWhere((p) => p.id == id);
    final updated = _provider(
      id: id,
      name: providers[index].name,
      models: const [ModelInfo(id: 'm2', name: 'Model 2')],
      modelsUpdatedAt: DateTime.utc(2026, 9, 12, 12),
    );
    providers[index] = updated;
    return updated;
  }
}

void main() {
  testWidgets('loads and shows one provider with cached models', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient(
      providers: [
        _provider(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
          modelsUpdatedAt: DateTime.utc(2026, 9, 12, 10),
        ),
      ],
    );

    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Providers'), findsWidgets);
    expect(find.text('Agents'), findsOneWidget);
    expect(find.text('Local'), findsOneWidget);
    expect(find.text('Model 1'), findsOneWidget);
  });

  testWidgets('Agents tab shows placeholder', (WidgetTester tester) async {
    final catalog = FakeCatalogClient();

    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Agents'));
    await tester.pumpAndSettle();

    expect(find.text('Agents'), findsWidgets);
  });

  testWidgets('create dialog posts name, baseUrl, and apiKey', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient();

    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Add provider'));
    await tester.pumpAndSettle();

    await tester.enterText(find.widgetWithText(TextField, 'Name'), 'Cloud');
    await tester.enterText(
      find.widgetWithText(TextField, 'Base URL'),
      'http://api.example/v1',
    );
    await tester.enterText(
      find.widgetWithText(TextField, 'API key'),
      'sk-live',
    );
    await tester.tap(find.text('Create'));
    await tester.pumpAndSettle();

    expect(catalog.lastCreate, {
      'name': 'Cloud',
      'type': 'openai_compatible',
      'baseUrl': 'http://api.example/v1',
      'apiKey': 'sk-live',
    });
    expect(find.text('Cloud'), findsOneWidget);
  });

  testWidgets('refresh models updates cached list', (
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
    );

    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Model 1'), findsOneWidget);

    await tester.tap(find.text('Refresh models'));
    await tester.pumpAndSettle();

    expect(catalog.lastRefreshId, 'prov-1');
    expect(find.text('Model 2'), findsOneWidget);
    expect(find.text('Model 1'), findsNothing);
  });

  testWidgets('Settings destination shows SettingsPage with injected catalog', (
    WidgetTester tester,
  ) async {
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);
    final catalog = FakeCatalogClient(
      providers: [_provider(id: 'prov-1', name: 'Injected')],
    );

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(controller: controller, catalog: catalog),
      ),
    );
    await tester.pump();
    await tester.pump();

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.byType(SettingsPage), findsOneWidget);
    expect(find.text('Injected'), findsOneWidget);
  });

  testWidgets('refresh error shows banner while providers remain', (
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
      refreshError: StateError('refresh failed'),
    );

    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Local'), findsOneWidget);
    expect(find.text('Model 1'), findsOneWidget);

    await tester.tap(find.text('Refresh models'));
    await tester.pumpAndSettle();

    expect(find.textContaining('refresh failed'), findsOneWidget);
    expect(find.text('Local'), findsOneWidget);
    expect(find.text('Model 1'), findsOneWidget);
  });

  testWidgets('tap provider row opens editor and save patches', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [_provider(id: 'prov-1', name: 'Local')],
    );
    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Local'));
    await tester.pumpAndSettle();
    expect(find.text('Edit provider'), findsOneWidget);

    await tester.enterText(find.widgetWithText(TextField, 'Name'), 'Renamed');
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(catalog.lastUpdate?['id'], 'prov-1');
    expect(catalog.lastUpdate?['name'], 'Renamed');
    expect(find.text('Renamed'), findsOneWidget);
  });

  testWidgets('delete provider confirms then deletes', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [_provider(id: 'prov-1', name: 'Local')],
    );
    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-provider-prov-1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete provider?'), findsOneWidget);
    expect(find.textContaining('Local'), findsWidgets);

    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'prov-1');
    expect(find.text('Local'), findsNothing);
  });

  testWidgets('delete in-use provider lists agent names', (tester) async {
    final catalog = FakeCatalogClient(
      providers: [_provider(id: 'prov-1', name: 'Local')],
      agents: [
        _agent(
          id: 'ag-1',
          name: 'Work',
          providerId: 'prov-1',
          defaultModel: 'm1',
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: SettingsPage(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-provider-prov-1')));
    await tester.pumpAndSettle();
    expect(find.textContaining('Work'), findsOneWidget);
    expect(find.textContaining('unset'), findsOneWidget);

    await tester.tap(find.widgetWithText(TextButton, 'Cancel'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, isNull);
    expect(find.text('Local'), findsOneWidget);
  });
}
