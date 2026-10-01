import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/instructions_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

class FakeInstructionsCatalog extends CatalogClient {
  FakeInstructionsCatalog({this.harnessInstructions = ''})
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  String harnessInstructions;
  String? lastHarnessInstructions;

  @override
  Future<PlaneSettings> getSettings() async {
    return PlaneSettings(harnessInstructions: harnessInstructions);
  }

  @override
  Future<PlaneSettings> patchSettings({
    Map<String, dynamic>? sandbox,
    Map<String, dynamic>? environment,
    Map<String, dynamic>? integrations,
    Object? webSearchIntegrationId = CatalogClient.fieldUnset,
    Object? fetchPageIntegrationId = CatalogClient.fieldUnset,
    String? harnessInstructions,
  }) async {
    lastHarnessInstructions = harnessInstructions;
    if (harnessInstructions != null) {
      this.harnessInstructions = harnessInstructions;
    }
    return PlaneSettings(harnessInstructions: this.harnessInstructions);
  }
}

void main() {
  testWidgets('loads and saves Harness instructions', (tester) async {
    tester.view.physicalSize = const Size(1200, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeInstructionsCatalog(
      harnessInstructions: 'Be careful with tools.',
    );
    await tester.pumpWidget(
      MaterialApp(home: InstructionsTab(catalog: catalog)),
    );
    await tester.pumpAndSettle();

    expect(find.text('Harness instructions'), findsOneWidget);
    expect(find.text('Be careful with tools.'), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('harness-instructions')),
      'Prefer search before fetch.',
    );
    await tester.tap(find.byKey(const Key('harness-instructions-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastHarnessInstructions, 'Prefer search before fetch.');
  });
}
