import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/app_shell.dart';
import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

Never _throwObject(Object error) {
  if (error is Error) throw error;
  if (error is Exception) throw error;
  throw Exception(error);
}

class _FakeConn implements AgentSessionApi {
  List<String> thoughtsToEmit = const [];
  TurnUsage? usageToEmit;
  final _closed = StreamController<void>.broadcast(sync: true);
  final _connectionState = StreamController<AcpConnectionState>.broadcast(
    sync: true,
  );
  AcpConnectionState currentState = AcpConnectionState.disconnected;

  @override
  Stream<void> get closed => _closed.stream;

  @override
  Stream<AcpConnectionState> get connectionState => _connectionState.stream;

  @override
  Future<void> connect({Transport? transport}) async {
    currentState = AcpConnectionState.connected;
    _connectionState.add(currentState);
  }

  @override
  void retryNow() {}

  @override
  Future<void> startSession(String agentId, {String? threadId}) async {}

  @override
  Future<void> setModel(String modelId) async {}

  @override
  List<ModelOption> get modelOptions => const [];

  @override
  String? get currentModel => null;

  String? get pinnedPrompt => null;

  @override
  Future<StopReason> sendPrompt(
    String text, {
    required AgentTurnHandler onEvent,
    bool retryLatest = false,
  }) async {
    for (final t in thoughtsToEmit) {
      onEvent(AgentThoughtDelta(t));
    }
    final usage = usageToEmit;
    if (usage != null) {
      onEvent(AgentUsageEvent(usage));
    }
    return StopReason.endTurn;
  }

  @override
  Future<void> cancel() async {}

  @override
  Future<void> close() async {
    currentState = AcpConnectionState.disconnected;
    _connectionState.add(currentState);
  }
}

InferenceConnection _inferenceConnection({
  required String id,
  required String name,
  String type = providerTypeOpenAICompatible,
  String baseUrl = 'http://127.0.0.1:8888/v1',
  List<ModelInfo> models = const [],
  DateTime? modelsUpdatedAt,
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return InferenceConnection(
    id: id,
    name: name,
    type: type,
    baseUrl: baseUrl,
    apiKey: 'sk-test',
    models: models,
    modelsUpdatedAt: modelsUpdatedAt,
    createdAt: now,
    updatedAt: now,
  );
}

Assistant _assistant({
  required String id,
  required String name,
  String? inferenceConnectionId,
  String? defaultModel,
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Assistant(
    id: id,
    name: name,
    version: 1,
    inferenceConnectionId: inferenceConnectionId,
    defaultModel: defaultModel,
    createdAt: now,
    updatedAt: now,
  );
}

class FakeCatalogClient extends CatalogClient {
  FakeCatalogClient({
    List<InferenceConnection>? inferenceConnections,
    List<Assistant>? assistants,
    this.refreshError,
  }) : inferenceConnections = List.of(inferenceConnections ?? const []),
       assistants = List.of(assistants ?? const []),
       super(
         baseUri: Uri.parse('http://catalog.test'),
         httpClient: MockClient((_) async => http.Response('unused', 500)),
       );

  final List<InferenceConnection> inferenceConnections;
  final List<Assistant> assistants;
  Object? listAssistantsError;
  Map<String, String>? lastCreate;
  Map<String, String?>? lastUpdate;
  String? lastDeleteId;
  String? lastRefreshId;
  final Object? refreshError;

  @override
  Future<List<Assistant>> listAssistants() async {
    if (listAssistantsError != null) {
      _throwObject(listAssistantsError!);
    }
    return List.of(assistants);
  }

  @override
  Future<PlaneSettings> getSettings() async {
    return const PlaneSettings();
  }

  @override
  Future<List<Project>> listProjects() async => const [];

  @override
  Future<InferenceConnection> updateInferenceConnection(
    String id, {
    String? name,
    String? baseUrl,
    String? apiKey,
  }) async {
    lastUpdate = {'id': id, 'name': name, 'baseUrl': baseUrl, 'apiKey': apiKey};
    final index = inferenceConnections.indexWhere((p) => p.id == id);
    final current = inferenceConnections[index];
    final updated = _inferenceConnection(id: id, name: name ?? current.name);
    inferenceConnections[index] = updated;
    return updated;
  }

  @override
  Future<void> deleteInferenceConnection(String id) async {
    lastDeleteId = id;
    inferenceConnections.removeWhere((p) => p.id == id);
  }

  @override
  Future<List<InferenceConnection>> listInferenceConnections() async =>
      List.of(inferenceConnections);

  @override
  Future<InferenceConnection> createInferenceConnection({
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
    final created = _inferenceConnection(
      id: 'prov-new',
      name: name,
      type: type,
      baseUrl: baseUrl.isEmpty ? 'https://opencode.ai/zen/v1' : baseUrl,
    );
    inferenceConnections.add(created);
    return created;
  }

  @override
  Future<InferenceConnection> refreshModels(String id) async {
    lastRefreshId = id;
    if (refreshError != null) {
      _throwObject(refreshError!);
    }
    final index = inferenceConnections.indexWhere((p) => p.id == id);
    final updated = _inferenceConnection(
      id: id,
      name: inferenceConnections[index].name,
      models: const [ModelInfo(id: 'm2', name: 'Model 2')],
      modelsUpdatedAt: DateTime.utc(2026, 9, 12, 12),
    );
    inferenceConnections[index] = updated;
    return updated;
  }
}

void main() {
  late ChatDisplaySettings displaySettings;
  late AppearanceSettings appearanceSettings;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    displaySettings = await ChatDisplaySettings.load();
    appearanceSettings = await AppearanceSettings.load();
  });

  testWidgets('loads and shows one provider with cached models', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
          modelsUpdatedAt: DateTime.utc(2026, 9, 12, 10),
        ),
      ],
    );

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Connections'), findsWidgets);
    expect(find.text('Assistants'), findsOneWidget);
    expect(find.text('Local'), findsOneWidget);
    expect(find.text('Custom'), findsOneWidget);
    expect(find.text('Model 1'), findsOneWidget);
  });

  testWidgets('Agents tab shows placeholder', (WidgetTester tester) async {
    final catalog = FakeCatalogClient();

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Assistants'));
    await tester.pumpAndSettle();

    expect(find.text('Assistants'), findsWidgets);
  });

  testWidgets('create dialog posts name, baseUrl, and apiKey', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient();

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Add connection'));
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

  testWidgets('OpenCode Zen create hides base URL and posts type', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient();

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Add connection'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('provider-type')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('OpenCode Zen').last);
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('provider-base-url')), findsNothing);

    await tester.enterText(find.widgetWithText(TextField, 'API key'), 'oc-key');
    await tester.tap(find.text('Create'));
    await tester.pumpAndSettle();

    expect(catalog.lastCreate?['type'], providerTypeOpenCodeZen);
    expect(catalog.lastCreate?['baseUrl'], '');
    expect(catalog.lastCreate?['apiKey'], 'oc-key');
    expect(find.text('OpenCode Zen'), findsWidgets);
  });

  testWidgets('Unsloth Studio create keeps base URL and posts type', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient();

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Add connection'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('provider-type')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Unsloth Studio').last);
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('provider-base-url')), findsOneWidget);
    await tester.enterText(
      find.byKey(const Key('provider-base-url')),
      'http://127.0.0.1:8888/v1',
    );
    await tester.enterText(find.widgetWithText(TextField, 'API key'), 'sk-u');
    await tester.tap(find.text('Create'));
    await tester.pumpAndSettle();

    expect(catalog.lastCreate?['type'], providerTypeUnslothStudio);
    expect(catalog.lastCreate?['baseUrl'], 'http://127.0.0.1:8888/v1');
    expect(catalog.lastCreate?['apiKey'], 'sk-u');
  });

  testWidgets('Berget AI create hides base URL and posts fixed type', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient();

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Add connection'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('provider-type')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Berget AI').last);
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('provider-base-url')), findsNothing);

    await tester.enterText(find.widgetWithText(TextField, 'API key'), 'bg-key');
    await tester.tap(find.text('Create'));
    await tester.pumpAndSettle();

    expect(catalog.lastCreate?['type'], providerTypeBergetAI);
    expect(catalog.lastCreate?['baseUrl'], '');
    expect(catalog.lastCreate?['apiKey'], 'bg-key');
    expect(find.text('Berget AI'), findsWidgets);
  });

  testWidgets('refresh models updates cached list', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
    );

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
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
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(id: 'prov-1', name: 'Injected'),
      ],
    );

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(
          controller: controller,
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();

    await tester.tap(find.byKey(const Key('nav-settings')));
    await tester.pumpAndSettle();

    expect(find.byType(SettingsPage), findsOneWidget);
    expect(find.text('Injected'), findsOneWidget);
  });

  testWidgets('refresh error shows banner while providers remain', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
      refreshError: StateError('refresh failed'),
    );

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Local'), findsOneWidget);
    expect(find.text('Model 1'), findsOneWidget);

    await tester.tap(find.text('Refresh models'));
    await tester.pumpAndSettle();

    expect(
      find.text('Something went wrong. Check the connection and try again.'),
      findsOneWidget,
    );
    expect(find.text('Local'), findsOneWidget);
    expect(find.text('Model 1'), findsOneWidget);
  });

  testWidgets('tap provider row opens editor and save patches', (tester) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [_inferenceConnection(id: 'prov-1', name: 'Local')],
    );
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Local'));
    await tester.pumpAndSettle();
    expect(find.text('Edit connection'), findsOneWidget);

    await tester.enterText(find.widgetWithText(TextField, 'Name'), 'Renamed');
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(catalog.lastUpdate?['id'], 'prov-1');
    expect(catalog.lastUpdate?['name'], 'Renamed');
    expect(find.text('Renamed'), findsOneWidget);
  });

  testWidgets('delete provider confirms then deletes', (tester) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [_inferenceConnection(id: 'prov-1', name: 'Local')],
    );
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-provider-prov-1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete connection?'), findsOneWidget);
    expect(find.textContaining('Local'), findsWidgets);

    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'prov-1');
    expect(find.text('Local'), findsNothing);
  });

  testWidgets('delete in-use provider lists agent names', (tester) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [_inferenceConnection(id: 'prov-1', name: 'Local')],
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'Work',
          inferenceConnectionId: 'prov-1',
          defaultModel: 'm1',
        ),
      ],
    );
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
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

  testWidgets('delete still confirms when listAssistants fails', (
    tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [_inferenceConnection(id: 'prov-1', name: 'Local')],
    )..listAssistantsError = StateError('agents unavailable');

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: displaySettings,
          appearanceSettings: appearanceSettings,
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-provider-prov-1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete connection?'), findsOneWidget);
    expect(find.textContaining('Could not load assistants'), findsOneWidget);

    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'prov-1');
    expect(find.text('Local'), findsNothing);
  });
}
