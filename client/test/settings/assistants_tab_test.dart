import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/assistants_tab.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

InferenceConnection _inferenceConnection({
  required String id,
  required String name,
  String type = providerTypeOpenAICompatible,
  List<ModelInfo> models = const [],
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return InferenceConnection(
    id: id,
    name: name,
    type: type,
    baseUrl: 'http://127.0.0.1:8888/v1',
    apiKey: 'sk-test',
    models: models,
    modelsUpdatedAt: now,
    createdAt: now,
    updatedAt: now,
  );
}

Assistant _assistant({
  required String id,
  required String name,
  String description = '',
  String instructions = '',
  String? inferenceConnectionId = 'prov-1',
  String? defaultModel = 'm1',
  Map<String, dynamic> settings = const {},
}) {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Assistant(
    id: id,
    name: name,
    description: description,
    instructions: instructions,
    version: 1,
    inferenceConnectionId: inferenceConnectionId,
    defaultModel: defaultModel,
    settings: settings,
    createdAt: now,
    updatedAt: now,
  );
}

class FakeCatalogClient extends CatalogClient {
  FakeCatalogClient({
    List<InferenceConnection>? inferenceConnections,
    List<Assistant>? assistants,
  }) : inferenceConnections = List.of(inferenceConnections ?? const []),
       assistants = List.of(assistants ?? const []),
       super(
         baseUri: Uri.parse('http://catalog.test'),
         httpClient: MockClient((_) async => http.Response('unused', 500)),
       );

  final List<InferenceConnection> inferenceConnections;
  final List<Assistant> assistants;
  Map<String, String>? lastCreate;
  String? lastDeleteId;
  Map<String, dynamic>? lastAssistantSettings;

  @override
  Future<List<InferenceConnection>> listInferenceConnections() async {
    return List.of(inferenceConnections);
  }

  @override
  Future<PlaneSettings> getSettings() async {
    return const PlaneSettings();
  }

  @override
  Future<List<ToolIntegration>> listToolIntegrations() async {
    return const [];
  }

  @override
  Future<List<Assistant>> listAssistants() async {
    return List.of(assistants);
  }

  @override
  Future<Assistant> createAssistant({
    required String name,
    String description = '',
    String instructions = '',
    required String inferenceConnectionId,
    required String defaultModel,
  }) async {
    lastCreate = {
      'name': name,
      'description': description,
      'instructions': instructions,
      'inferenceConnectionId': inferenceConnectionId,
      'defaultModel': defaultModel,
    };
    final created = _assistant(
      id: 'ag-new',
      name: name,
      description: description,
      instructions: instructions,
      inferenceConnectionId: inferenceConnectionId,
      defaultModel: defaultModel,
    );
    assistants.add(created);
    return created;
  }

  @override
  Future<void> deleteAssistant(String id) async {
    lastDeleteId = id;
    assistants.removeWhere((a) => a.id == id);
  }

  @override
  Future<Assistant> updateAssistant(
    String id, {
    String? name,
    String? description,
    String? instructions,
    String? inferenceConnectionId,
    String? defaultModel,
    Map<String, dynamic>? settings,
  }) async {
    lastAssistantSettings = settings;
    final index = assistants.indexWhere((a) => a.id == id);
    if (index < 0) {
      throw CatalogException(statusCode: 404, message: 'not found');
    }
    final current = assistants[index];
    final mergedSettings = Map<String, dynamic>.from(current.settings);
    if (settings != null) {
      for (final entry in settings.entries) {
        mergedSettings[entry.key] = entry.value;
      }
    }
    assistants[index] = Assistant(
      id: current.id,
      name: name ?? current.name,
      description: description ?? current.description,
      instructions: instructions ?? current.instructions,
      version: current.version + 1,
      inferenceConnectionId:
          inferenceConnectionId ?? current.inferenceConnectionId,
      inferenceConnectionName: current.inferenceConnectionName,
      defaultModel: defaultModel ?? current.defaultModel,
      settings: mergedSettings,
      createdAt: current.createdAt,
      updatedAt: DateTime.utc(2026, 9, 20),
    );
    return assistants[index];
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

  testWidgets('create form requires provider and model from cached data', (
    WidgetTester tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
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

    expect(find.byType(AssistantsTab), findsOneWidget);

    await tester.tap(find.byTooltip('Add assistant'));
    await tester.pumpAndSettle();

    expect(find.widgetWithText(TextButton, 'Create'), findsOneWidget);
    expect(
      tester
          .widget<TextButton>(find.widgetWithText(TextButton, 'Create'))
          .onPressed,
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
      'instructions': '',
      'inferenceConnectionId': 'prov-1',
      'defaultModel': 'm2',
    });
    expect(find.text('Helper'), findsOneWidget);
  });

  testWidgets('shows list and web tool bindings editor', (
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
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'Work',
          description: 'office',
          inferenceConnectionId: 'prov-1',
          defaultModel: 'm1',
        ),
      ],
    );

    await tester.pumpWidget(MaterialApp(home: AssistantsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Work'), findsOneWidget);
    expect(find.text('office'), findsOneWidget);

    await tester.tap(find.text('Work'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('agent-tool-bindings')), findsOneWidget);

    for (final label in ['MCP', 'Memory']) {
      final tile = tester.widget<ExpansionTile>(
        find.widgetWithText(ExpansionTile, label),
      );
      expect(tile.enabled, isFalse);
      expect(tile.subtitle, isA<Text>());
      expect((tile.subtitle! as Text).data, 'Coming soon');
    }
    expect(find.text('Sandbox'), findsNothing);
    expect(
      find.byWidgetPredicate(
        (widget) => widget.runtimeType.toString() == 'SandboxOverlayForm',
      ),
      findsNothing,
    );
  });

  testWidgets('incomplete agent shows Needs connection', (tester) async {
    final catalog = FakeCatalogClient(
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'Work',
          inferenceConnectionId: null,
          defaultModel: null,
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AssistantsTab(catalog: catalog)));
    await tester.pumpAndSettle();
    expect(find.text('Work'), findsOneWidget);
    expect(find.text('Needs connection'), findsOneWidget);
  });

  testWidgets('delete agent confirms then deletes', (tester) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'Work',
          inferenceConnectionId: 'prov-1',
          defaultModel: 'm1',
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AssistantsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-agent-ag-1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete assistant?'), findsOneWidget);

    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'ag-1');
    expect(find.text('Work'), findsNothing);
  });

  testWidgets('saves inference settings for custom provider', (tester) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'Work',
          inferenceConnectionId: 'prov-1',
          defaultModel: 'm1',
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AssistantsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Work'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('agent-inference')), findsOneWidget);
    expect(find.byKey(const Key('inference-top-p')), findsNothing);

    await tester.enterText(
      find.byKey(const Key('inference-temperature')),
      '0.8',
    );
    await tester.enterText(
      find.byKey(const Key('inference-max-tokens')),
      '512',
    );
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    final settings = catalog.lastAssistantSettings;
    expect(settings, isNotNull);
    final inference = settings!['inference'] as Map<String, dynamic>;
    expect(inference['temperature'], 0.8);
    expect(inference['maxTokens'], 512);
  });

  testWidgets('saves assistant instructions independently of description', (
    tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-1',
          name: 'Local',
          models: const [ModelInfo(id: 'm1', name: 'Model 1')],
        ),
      ],
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'Work',
          description: 'office',
          instructions: 'old role',
          inferenceConnectionId: 'prov-1',
          defaultModel: 'm1',
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AssistantsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Work'));
    await tester.pumpAndSettle();

    await tester.enterText(
      find.byKey(const Key('assistant-instructions')),
      'You review pull requests.',
    );
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();

    expect(catalog.assistants.single.instructions, 'You review pull requests.');
    expect(catalog.assistants.single.description, 'office');
  });

  testWidgets('unsloth agent editor shows advanced sampler fields', (
    tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-u',
          name: 'Unsloth',
          type: providerTypeUnslothStudio,
          models: const [ModelInfo(id: 'default', name: 'default')],
        ),
      ],
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'LocalCoder',
          inferenceConnectionId: 'prov-u',
          defaultModel: 'default',
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AssistantsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('LocalCoder'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('inference-top-p')), findsOneWidget);
    expect(find.byKey(const Key('inference-min-p')), findsOneWidget);
    expect(find.byKey(const Key('inference-enable-thinking')), findsOneWidget);
  });

  testWidgets('berget agent editor shows sampler and thinking fields', (
    tester,
  ) async {
    final catalog = FakeCatalogClient(
      inferenceConnections: [
        _inferenceConnection(
          id: 'prov-b',
          name: 'Berget',
          type: providerTypeBergetAI,
          models: const [ModelInfo(id: 'm', name: 'm')],
        ),
      ],
      assistants: [
        _assistant(
          id: 'ag-1',
          name: 'BergetCoder',
          inferenceConnectionId: 'prov-b',
          defaultModel: 'm',
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: AssistantsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.text('BergetCoder'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('inference-top-p')), findsOneWidget);
    expect(
      find.byKey(const Key('inference-frequency-penalty')),
      findsOneWidget,
    );
    expect(find.byKey(const Key('inference-thinking-type')), findsOneWidget);
    expect(find.byKey(const Key('inference-enable-thinking')), findsNothing);
  });
}
