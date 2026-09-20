import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/sandbox_tab.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

class FakeSettingsCatalog extends CatalogClient {
  FakeSettingsCatalog({Map<String, dynamic>? sandbox})
    : sandbox = Map<String, dynamic>.from(
        sandbox ??
            {
              'kind': 'docker',
              'workspaceRoot': '/workspace',
              'image': 'alpine:3.20',
              'idleTTLSeconds': 3600,
              'containerName': 'agent-fabric-container-{projectID}',
            },
      ),
      super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  Map<String, dynamic> sandbox;
  Map<String, dynamic>? lastPatch;

  @override
  Future<List<Provider>> listProviders() async => const [];

  @override
  Future<List<Agent>> listAgents() async => const [];

  @override
  Future<PlaneSettings> getSettings() async {
    return PlaneSettings(sandbox: Map<String, dynamic>.from(sandbox));
  }

  @override
  Future<PlaneSettings> patchSettings({
    required Map<String, dynamic> sandbox,
  }) async {
    lastPatch = sandbox;
    this.sandbox = {...this.sandbox, ...sandbox};
    return PlaneSettings(sandbox: Map<String, dynamic>.from(this.sandbox));
  }
}

void main() {
  testWidgets('Sandbox tab loads global overlay and saves a patch', (
    tester,
  ) async {
    final catalog = FakeSettingsCatalog();
    await tester.pumpWidget(MaterialApp(home: SandboxTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Global sandbox defaults'), findsOneWidget);
    expect(find.text('alpine:3.20'), findsOneWidget);
    expect(find.text('/workspace'), findsOneWidget);
    expect(find.text('agent-fabric-container-{projectID}'), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('sandbox-image')),
      'golang:1.23',
    );
    await tester.tap(find.byKey(const Key('sandbox-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastPatch, isNotNull);
    expect(catalog.lastPatch!['image'], 'golang:1.23');
    expect(catalog.lastPatch!['kind'], 'docker');
    expect(catalog.lastPatch!['workspaceRoot'], '/workspace');
    expect(catalog.lastPatch!['idleTTLSeconds'], 3600);
    expect(
      catalog.lastPatch!['containerName'],
      'agent-fabric-container-{projectID}',
    );
  });

  testWidgets('Settings page includes a Sandbox tab', (tester) async {
    SharedPreferences.setMockInitialValues({});
    final displaySettings = await ChatDisplaySettings.load();
    final appearanceSettings = await AppearanceSettings.load();
    final catalog = FakeSettingsCatalog();

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

    expect(find.text('Sandbox'), findsOneWidget);
    await tester.tap(find.text('Sandbox'));
    await tester.pumpAndSettle();
    expect(find.byType(SandboxTab), findsOneWidget);
    expect(find.text('Global sandbox defaults'), findsOneWidget);
  });
}
