import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/gate_instructions_editor.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class _FakeCatalog extends CatalogClient {
  _FakeCatalog({this.gate = '', this.permissions = const {}})
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  String gate;
  Map<String, dynamic> permissions;
  String? patchedGate;
  int patches = 0;
  CatalogException? failWith;

  @override
  Future<PlaneSettings> getSettings() async =>
      PlaneSettings(gateInstructions: gate, permissions: permissions);

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
    Map<String, dynamic>? skills,
  }) async {
    patches++;
    if (failWith case final CatalogException e) {
      throw e;
    }
    patchedGate = gateInstructions;
    gate = gateInstructions ?? gate;
    return PlaneSettings(gateInstructions: gate, permissions: this.permissions);
  }
}

Future<void> _pump(WidgetTester tester, _FakeCatalog catalog) async {
  tester.view.physicalSize = const Size(1000, 900);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(body: GateInstructionsSection(catalog: catalog)),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('loads the plane text and saves it through PATCH settings', (
    tester,
  ) async {
    final catalog = _FakeCatalog(gate: 'Never push to main.');
    await _pump(tester, catalog);

    expect(find.text('Never push to main.'), findsOneWidget);
    expect(find.textContaining('Only you write them'), findsOneWidget);
    expect(
      find.textContaining('changes apply from the next session'),
      findsOneWidget,
    );

    await tester.enterText(
      find.byKey(const Key('gate-instructions-field')),
      'Never force push.',
    );
    await tester.tap(find.byKey(const Key('gate-instructions-save')));
    await tester.pumpAndSettle();

    expect(catalog.patchedGate, 'Never force push.');
    expect(find.byKey(const Key('gate-instructions-saved')), findsOneWidget);
  });

  testWidgets('clearing the text saves an empty string', (tester) async {
    final catalog = _FakeCatalog(gate: 'Rules');
    await _pump(tester, catalog);

    await tester.enterText(
      find.byKey(const Key('gate-instructions-field')),
      '',
    );
    await tester.tap(find.byKey(const Key('gate-instructions-save')));
    await tester.pumpAndSettle();

    expect(catalog.patchedGate, '');
  });

  testWidgets('counter warns past the default scorer cap of 4000', (
    tester,
  ) async {
    final catalog = _FakeCatalog();
    await _pump(tester, catalog);
    final field = find.byKey(const Key('gate-instructions-field'));

    await tester.enterText(field, 'a' * 4000);
    await tester.pump();
    expect(find.text('4000 / 20000'), findsOneWidget);
    expect(find.byKey(const Key('gate-instructions-warning')), findsNothing);

    await tester.enterText(field, 'a' * 4001);
    await tester.pump();
    expect(find.textContaining('first 4000 characters'), findsOneWidget);
  });

  testWidgets('warning uses the configured instructionsMaxChars', (
    tester,
  ) async {
    final catalog = _FakeCatalog(
      permissions: {
        'scorers': {'instructionsMaxChars': 200},
      },
    );
    await _pump(tester, catalog);

    await tester.enterText(
      find.byKey(const Key('gate-instructions-field')),
      'a' * 201,
    );
    await tester.pump();
    expect(find.textContaining('first 200 characters'), findsOneWidget);
  });

  testWidgets('text over 20000 characters cannot be saved', (tester) async {
    final catalog = _FakeCatalog();
    await _pump(tester, catalog);

    await tester.enterText(
      find.byKey(const Key('gate-instructions-field')),
      'a' * 20001,
    );
    await tester.pump();
    expect(find.textContaining('at most 20000'), findsOneWidget);
    final save = tester.widget<FilledButton>(
      find.byKey(const Key('gate-instructions-save')),
    );
    expect(save.onPressed, isNull);
    expect(catalog.patches, 0);
  });

  testWidgets('a rejected save shows the error and keeps the text', (
    tester,
  ) async {
    final catalog = _FakeCatalog()
      ..failWith = CatalogException(
        statusCode: 400,
        message: 'gateInstructions is too long',
      );
    await _pump(tester, catalog);

    await tester.enterText(
      find.byKey(const Key('gate-instructions-field')),
      'Rules',
    );
    await tester.tap(find.byKey(const Key('gate-instructions-save')));
    await tester.pumpAndSettle();

    expect(find.textContaining('gateInstructions'), findsOneWidget);
    expect(find.byKey(const Key('gate-instructions-saved')), findsNothing);
    expect(find.text('Rules'), findsOneWidget);
  });
}
