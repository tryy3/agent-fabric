import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/environment_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

Resource _resource() {
  final now = DateTime.utc(2026, 9, 22);
  return Resource(
    id: 'res_work',
    name: 'Work',
    kind: 'container',
    spec: {
      'image': 'alpine:3.20',
      'containerName': 'work',
      'volumes': [
        {
          'id': 'vol_0123456789abcdef',
          'enabled': true,
          'name': 'disk',
          'target': '/workspace',
          'whitelisted': true,
          'read': true,
          'write': true,
          'exec': true,
        },
      ],
    },
    createdAt: now,
    updatedAt: now,
  );
}

class FakeEnvironmentCatalog extends CatalogClient {
  FakeEnvironmentCatalog()
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  Map<String, dynamic>? lastEnvironment;

  @override
  Future<List<Resource>> listResources() async => [_resource()];

  @override
  Future<PlaneSettings> getSettings() async {
    return const PlaneSettings(environment: {'workspaceRoot': '/workspace'});
  }

  @override
  Future<PlaneSettings> patchSettings({
    Map<String, dynamic>? sandbox,
    Map<String, dynamic>? environment,
  }) async {
    lastEnvironment = environment;
    return PlaneSettings(
      sandbox: sandbox ?? const {},
      environment: environment ?? const {},
    );
  }
}

void main() {
  testWidgets('environment dropdown includes an empty choice', (tester) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeEnvironmentCatalog();
    await tester.pumpWidget(
      MaterialApp(home: EnvironmentTab(catalog: catalog)),
    );
    await tester.pumpAndSettle();

    expect(find.text('Add volume'), findsNothing);

    await tester.tap(find.byKey(const Key('environment-resource')));
    await tester.pumpAndSettle();
    final menuItems = tester
        .widgetList<DropdownMenuItem<String>>(
          find.byType(DropdownMenuItem<String>),
        )
        .toList();
    expect(menuItems.map((item) => item.value), contains(''));
    await tester.tap(find.text('Work').last);
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('environment-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastEnvironment?['resourceId'], 'res_work');
    expect(catalog.lastEnvironment?.containsKey('sandbox'), isFalse);
  });

  testWidgets('environment saves workspace, extra path, and grant override', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeEnvironmentCatalog();
    await tester.pumpWidget(
      MaterialApp(home: EnvironmentTab(catalog: catalog)),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('environment-resource')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Work').last);
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const Key('environment-workspace-root')),
      '/srv',
    );
    await tester.ensureVisible(find.byKey(const Key('environment-path-add')));
    await tester.tap(find.byKey(const Key('environment-path-add')));
    await tester.pump();

    late String pathID;
    for (final widget in tester.widgetList<TextField>(find.byType(TextField))) {
      final key = widget.key;
      if (key is ValueKey<String> &&
          key.value.startsWith('environment-path-') &&
          key.value.endsWith('-path')) {
        pathID = key.value.substring(
          'environment-path-'.length,
          key.value.length - '-path'.length,
        );
      }
    }
    expect(pathID, matches(RegExp(r'^path_[0-9a-f]{16}$')));
    await tester.enterText(
      find.byKey(Key('environment-path-$pathID-path')),
      '/opt/tools',
    );
    await tester.ensureVisible(
      find.byKey(const Key('environment-grant-vol_0123456789abcdef-write')),
    );
    await tester.tap(
      find.byKey(const Key('environment-grant-vol_0123456789abcdef-write')),
    );
    await tester.pump();
    await tester.ensureVisible(find.byKey(const Key('environment-save')));
    await tester.tap(find.byKey(const Key('environment-save')));
    await tester.pumpAndSettle();

    final environment = catalog.lastEnvironment!;
    expect(environment['resourceId'], 'res_work');
    expect(environment['workspaceRoot'], '/srv');
    final extra = (environment['extraPaths'] as List).single as Map;
    expect(extra['id'], pathID);
    expect(extra['path'], '/opt/tools');
    expect(environment['grants'], [
      {'volumeId': 'vol_0123456789abcdef', 'write': false},
    ]);
  });
}
