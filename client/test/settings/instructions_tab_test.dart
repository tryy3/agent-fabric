import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/instructions_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class FakeInstructionsCatalog extends CatalogClient {
  FakeInstructionsCatalog({
    this.platformInstructions = '',
    this.runtimeContext = '',
    this.gateInstructions = '',
  }) : super(
         baseUri: Uri.parse('http://catalog.test'),
         httpClient: MockClient((_) async => http.Response('unused', 500)),
       );

  String platformInstructions;
  String runtimeContext;
  String? lastPlatformInstructions;
  String? lastRuntimeContext;
  String gateInstructions;

  /// The non-null fields of each PATCH, in call order.
  final patches = <Map<String, String>>[];

  @override
  Future<PlaneSettings> getSettings() async {
    return PlaneSettings(
      platformInstructions: platformInstructions,
      runtimeContext: runtimeContext,
      gateInstructions: gateInstructions,
    );
  }

  @override
  Future<PlaneSettings> patchSettings({
    Map<String, dynamic>? sandbox,
    Map<String, dynamic>? environment,
    Map<String, dynamic>? integrations,
    Object? webSearchIntegrationId = CatalogClient.fieldUnset,
    Object? fetchPageIntegrationId = CatalogClient.fieldUnset,
    String? platformInstructions,
    String? runtimeContext,
    String? gateInstructions,
    Map<String, dynamic>? permissions,
    Map<String, dynamic>? limits,
  }) async {
    lastPlatformInstructions = platformInstructions;
    lastRuntimeContext = runtimeContext;
    patches.add({
      'platformInstructions': ?platformInstructions,
      'runtimeContext': ?runtimeContext,
      'gateInstructions': ?gateInstructions,
    });
    if (gateInstructions != null) {
      this.gateInstructions = gateInstructions;
    }
    if (platformInstructions != null) {
      this.platformInstructions = platformInstructions;
    }
    if (runtimeContext != null) {
      this.runtimeContext = runtimeContext;
    }
    return PlaneSettings(
      platformInstructions: this.platformInstructions,
      runtimeContext: this.runtimeContext,
      gateInstructions: this.gateInstructions,
    );
  }
}

void main() {
  testWidgets('loads and saves platform instructions and runtime context', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1200, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeInstructionsCatalog(
      platformInstructions: 'Be careful with tools.',
      runtimeContext: 'Date: {{currentDate}}',
    );
    await tester.pumpWidget(
      MaterialApp(home: InstructionsTab(catalog: catalog)),
    );
    await tester.pumpAndSettle();

    expect(find.text('Platform instructions'), findsOneWidget);
    expect(find.text('Runtime context'), findsOneWidget);
    expect(find.text('Variables'), findsOneWidget);
    expect(find.text('Be careful with tools.'), findsOneWidget);
    expect(find.text('Date: {{currentDate}}'), findsOneWidget);
    expect(find.byKey(const Key('instruction-variables-rail')), findsOneWidget);
    expect(find.textContaining('{{modelId}}'), findsWidgets);

    await tester.enterText(
      find.byKey(const Key('platform-instructions')),
      'Prefer search before fetch.',
    );
    await tester.enterText(
      find.byKey(const Key('runtime-context')),
      'Workspace: {{workspaceRoot}}',
    );
    await tester.tap(find.byKey(const Key('instructions-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastPlatformInstructions, 'Prefer search before fetch.');
    expect(catalog.lastRuntimeContext, 'Workspace: {{workspaceRoot}}');
  });

  testWidgets('variables rail selects meaning and inserts into runtime', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1200, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeInstructionsCatalog(runtimeContext: 'Prefix ');
    await tester.pumpWidget(
      MaterialApp(home: InstructionsTab(catalog: catalog)),
    );
    await tester.pumpAndSettle();

    await tester.tap(
      find.byKey(const Key('instruction-variable-{{timezone}}')),
    );
    await tester.pumpAndSettle();

    expect(find.textContaining('IANA / zone name'), findsOneWidget);
    expect(find.text('Europe/Stockholm'), findsOneWidget);

    await tester.tap(find.byKey(const Key('runtime-context')));
    await tester.pumpAndSettle();
    // Move caret to end so insert appends.
    final runtimeField = tester.widget<TextField>(
      find.descendant(
        of: find.byKey(const Key('runtime-context')),
        matching: find.byType(TextField),
      ),
    );
    runtimeField.controller!.selection = TextSelection.collapsed(
      offset: runtimeField.controller!.text.length,
    );

    await tester.tap(find.byKey(const Key('instruction-variable-insert')));
    await tester.pumpAndSettle();

    expect(runtimeField.controller!.text, 'Prefix {{timezone}}');
  });

  Future<FakeInstructionsCatalog> pumpTab(
    WidgetTester tester, {
    String platform = '',
    String gate = '',
  }) async {
    tester.view.physicalSize = const Size(1200, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final catalog = FakeInstructionsCatalog(
      platformInstructions: platform,
      gateInstructions: gate,
    );
    await tester.pumpWidget(
      MaterialApp(home: InstructionsTab(catalog: catalog)),
    );
    await tester.pumpAndSettle();
    return catalog;
  }

  Future<void> selectGate(WidgetTester tester) async {
    await tester.tap(find.text('Gate instructions'));
    await tester.pumpAndSettle();
  }

  testWidgets('opens on Effective instructions with two labelled segments', (
    tester,
  ) async {
    await pumpTab(tester);

    final control = tester.widget<SegmentedButton<Object?>>(
      find.byKey(const Key('instructions-section')),
    );
    expect(control.segments.map((s) => (s.label! as Text).data), [
      'Effective instructions',
      'Gate instructions',
    ]);
    expect(find.byKey(const Key('platform-instructions')), findsOneWidget);
    expect(find.byKey(const Key('gate-instructions-field')), findsNothing);
  });

  testWidgets('the Gate instructions segment shows the plane text', (
    tester,
  ) async {
    await pumpTab(tester, gate: 'Never push to main.');
    await selectGate(tester);

    expect(find.byKey(const Key('gate-instructions-field')), findsOneWidget);
    expect(find.text('Never push to main.'), findsOneWidget);
    expect(find.byKey(const Key('platform-instructions')), findsNothing);
  });

  testWidgets('unsaved text survives switching sections', (tester) async {
    await pumpTab(tester, platform: 'Saved platform.');
    await tester.enterText(
      find.byKey(const Key('platform-instructions')),
      'Unsaved platform.',
    );
    await selectGate(tester);
    await tester.tap(find.text('Effective instructions'));
    await tester.pumpAndSettle();

    expect(find.text('Unsaved platform.'), findsOneWidget);
  });

  testWidgets('each section saves only its own fields', (tester) async {
    final catalog = await pumpTab(tester);

    await tester.enterText(
      find.byKey(const Key('platform-instructions')),
      'Be brief.',
    );
    await tester.tap(find.byKey(const Key('instructions-save')));
    await tester.pumpAndSettle();
    expect(catalog.patches, [
      {'platformInstructions': 'Be brief.', 'runtimeContext': ''},
    ]);

    await selectGate(tester);
    await tester.enterText(
      find.byKey(const Key('gate-instructions-field')),
      'No force pushes.',
    );
    await tester.tap(find.byKey(const Key('gate-instructions-save')));
    await tester.pumpAndSettle();
    expect(catalog.patches.last, {'gateInstructions': 'No force pushes.'});
    expect(catalog.patches, hasLength(2));
  });
}
