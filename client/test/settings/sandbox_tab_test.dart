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
              'volumes': [
                {
                  'id': 'vol_workspace',
                  'enabled': true,
                  'name': 'agent-fabric-vol-{projectID}',
                  'target': '/workspace',
                  'write': true,
                },
              ],
            },
      ),
      super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  Map<String, dynamic> sandbox;
  Map<String, dynamic>? lastPatch;

  @override
  Future<List<Project>> listProjects() async => const [];

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
    expect(find.text('/workspace'), findsNWidgets(2));
    expect(find.text('agent-fabric-container-{projectID}'), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('sandbox-image')),
      'golang:1.23',
    );
    await tester.ensureVisible(find.byKey(const Key('sandbox-save')));
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
    final volumes = catalog.lastPatch!['volumes'] as List<dynamic>;
    expect(volumes, isNotEmpty);
    expect((volumes.first as Map)['id'], 'vol_workspace');
    expect((volumes.first as Map)['name'], 'agent-fabric-vol-{projectID}');
  });

  testWidgets('Sandbox tab edits the global volume list', (tester) async {
    final catalog = FakeSettingsCatalog();
    await tester.pumpWidget(MaterialApp(home: SandboxTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Volumes'), findsOneWidget);
    expect(
      find.text(
        'Named Docker volumes. Projects that resolve the same name share files.',
      ),
      findsOneWidget,
    );
    expect(find.textContaining('bind'), findsNothing);
    expect(find.text('agent-fabric-vol-{projectID}'), findsOneWidget);

    await tester.ensureVisible(
      find.byKey(const Key('sandbox-volume-vol_workspace-write')),
    );
    await tester.tap(
      find.byKey(const Key('sandbox-volume-vol_workspace-write')),
    );
    await tester.pump();

    await tester.ensureVisible(find.byKey(const Key('sandbox-volume-add')));
    await tester.tap(find.byKey(const Key('sandbox-volume-add')));
    await tester.pump();

    late String addedID;
    for (final widget in tester.widgetList<TextField>(find.byType(TextField))) {
      final key = widget.key;
      if (key is ValueKey<String> &&
          key.value.startsWith('sandbox-volume-') &&
          key.value.endsWith('-name') &&
          key.value != 'sandbox-volume-vol_workspace-name') {
        addedID = key.value.substring(
          'sandbox-volume-'.length,
          key.value.length - '-name'.length,
        );
      }
    }
    expect(addedID, startsWith('vol_'));

    await tester.enterText(
      find.byKey(Key('sandbox-volume-$addedID-name')),
      'shared-files',
    );
    await tester.enterText(
      find.byKey(Key('sandbox-volume-$addedID-target')),
      '/data',
    );
    await tester.ensureVisible(find.byKey(const Key('sandbox-save')));
    await tester.tap(find.byKey(const Key('sandbox-save')));
    await tester.pumpAndSettle();

    final volumes = catalog.lastPatch!['volumes'] as List<dynamic>;
    expect(volumes, hasLength(2));
    final workspace = volumes.cast<Map>().firstWhere(
      (row) => row['id'] == 'vol_workspace',
    );
    expect(workspace['write'], isFalse);
    final extra = volumes.cast<Map>().firstWhere((row) => row['id'] == addedID);
    expect(extra['name'], 'shared-files');
    expect(extra['target'], '/data');
    expect(extra['enabled'], isTrue);
    expect(extra['write'], isTrue);
  });

  testWidgets('Sandbox tab edits extra paths and volume whitelist flags', (
    tester,
  ) async {
    final catalog = FakeSettingsCatalog();
    await tester.pumpWidget(MaterialApp(home: SandboxTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Extra paths'), findsOneWidget);
    expect(
      find.textContaining('Files pane still lists only workspace root'),
      findsOneWidget,
    );

    await tester.ensureVisible(
      find.byKey(const Key('sandbox-volume-vol_workspace-whitelisted')),
    );
    expect(
      find.byKey(const Key('sandbox-volume-vol_workspace-read')),
      findsOneWidget,
    );
    expect(
      find.byKey(const Key('sandbox-volume-vol_workspace-exec')),
      findsOneWidget,
    );

    await tester.ensureVisible(find.byKey(const Key('sandbox-path-add')));
    await tester.tap(find.byKey(const Key('sandbox-path-add')));
    await tester.pump();

    late String addedID;
    for (final widget in tester.widgetList<TextField>(find.byType(TextField))) {
      final key = widget.key;
      if (key is ValueKey<String> &&
          key.value.startsWith('sandbox-path-') &&
          key.value.endsWith('-path')) {
        addedID = key.value.substring(
          'sandbox-path-'.length,
          key.value.length - '-path'.length,
        );
      }
    }
    expect(addedID, startsWith('path_'));

    await tester.enterText(
      find.byKey(Key('sandbox-path-$addedID-path')),
      '/tmp',
    );
    await tester.ensureVisible(find.byKey(Key('sandbox-path-$addedID-write')));
    await tester.tap(find.byKey(Key('sandbox-path-$addedID-write')));
    await tester.pump();
    await tester.ensureVisible(find.byKey(const Key('sandbox-save')));
    await tester.tap(find.byKey(const Key('sandbox-save')));
    await tester.pumpAndSettle();

    final extraPaths = catalog.lastPatch!['extraPaths'] as List<dynamic>;
    expect(extraPaths, hasLength(1));
    final extra = extraPaths.first as Map;
    expect(extra['id'], addedID);
    expect(extra['path'], '/tmp');
    expect(extra['whitelisted'], isTrue);
    expect(extra['write'], isFalse);
    expect(extra['exec'], isFalse);
    final volumes = catalog.lastPatch!['volumes'] as List<dynamic>;
    expect((volumes.first as Map)['whitelisted'], isTrue);
    expect((volumes.first as Map)['read'], isTrue);
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
