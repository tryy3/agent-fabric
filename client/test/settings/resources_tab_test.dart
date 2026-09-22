import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/resources_tab.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

Resource _resource({
  required String id,
  required String name,
  Map<String, dynamic> spec = const {},
}) {
  final now = DateTime.utc(2026, 9, 22);
  return Resource(
    id: id,
    name: name,
    kind: 'container',
    spec: spec,
    createdAt: now,
    updatedAt: now,
  );
}

class FakeResourcesCatalog extends CatalogClient {
  FakeResourcesCatalog({List<Resource>? resources})
    : resources = List.of(resources ?? const []),
      super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  final List<Resource> resources;
  Map<String, dynamic>? lastCreate;
  Map<String, dynamic>? lastUpdate;
  String? lastUpdateId;
  String? lastDeleteId;
  Object? deleteError;

  @override
  Future<List<Resource>> listResources() async => List.of(resources);

  @override
  Future<List<Project>> listProjects() async => const [];

  @override
  Future<List<Provider>> listProviders() async => const [];

  @override
  Future<List<Agent>> listAgents() async => const [];

  @override
  Future<Resource> createResource({
    required String name,
    required String kind,
    required Map<String, dynamic> spec,
  }) async {
    lastCreate = {'name': name, 'kind': kind, 'spec': spec};
    final created = _resource(
      id: 'res_new',
      name: name,
      spec: Map<String, dynamic>.from(spec),
    );
    resources.add(created);
    return created;
  }

  @override
  Future<Resource> updateResource(
    String id, {
    String? name,
    Map<String, dynamic>? spec,
  }) async {
    lastUpdateId = id;
    lastUpdate = {
      if (name != null) 'name': name,
      if (spec != null) 'spec': spec,
    };
    final index = resources.indexWhere((resource) => resource.id == id);
    final current = resources[index];
    final updated = _resource(
      id: current.id,
      name: name ?? current.name,
      spec: spec ?? current.spec,
    );
    resources[index] = updated;
    return updated;
  }

  @override
  Future<void> deleteResource(String id) async {
    lastDeleteId = id;
    final error = deleteError;
    if (error != null) {
      throw error;
    }
    resources.removeWhere((resource) => resource.id == id);
  }
}

void main() {
  testWidgets('Resources tab lists a container by name and kind', (
    tester,
  ) async {
    final catalog = FakeResourcesCatalog(
      resources: [
        _resource(
          id: 'res_1',
          name: 'Dev',
          spec: {
            'image': 'alpine:3.20',
            'containerName': 'dev',
            'volumes': <Object>[],
          },
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: ResourcesTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Dev'), findsOneWidget);
    expect(find.text('container'), findsOneWidget);
  });

  testWidgets('adding a container calls createResource', (tester) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeResourcesCatalog();
    await tester.pumpWidget(MaterialApp(home: ResourcesTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Add resource'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('resource-name')), 'Work');
    await tester.enterText(
      find.byKey(const Key('resource-image')),
      'alpine:3.20',
    );
    await tester.enterText(
      find.byKey(const Key('resource-container-name')),
      'work',
    );
    await tester.tap(find.byKey(const Key('resource-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastCreate?['name'], 'Work');
    expect(catalog.lastCreate?['kind'], 'container');
    expect(catalog.lastCreate?['spec']['image'], 'alpine:3.20');
    expect(catalog.lastCreate?['spec']['containerName'], 'work');
  });

  testWidgets('adding a volume row sends a vol_ id', (tester) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeResourcesCatalog();
    await tester.pumpWidget(MaterialApp(home: ResourcesTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byTooltip('Add resource'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('resource-name')), 'Work');
    await tester.enterText(
      find.byKey(const Key('resource-image')),
      'alpine:3.20',
    );
    await tester.enterText(
      find.byKey(const Key('resource-container-name')),
      'work',
    );
    await tester.enterText(find.byKey(const Key('resource-idle-ttl')), '90');
    await tester.ensureVisible(find.byKey(const Key('resource-volume-add')));
    await tester.tap(find.byKey(const Key('resource-volume-add')));
    await tester.pump();

    late String addedID;
    for (final widget in tester.widgetList<TextField>(find.byType(TextField))) {
      final key = widget.key;
      if (key is ValueKey<String> &&
          key.value.startsWith('resource-volume-') &&
          key.value.endsWith('-name')) {
        addedID = key.value.substring(
          'resource-volume-'.length,
          key.value.length - '-name'.length,
        );
      }
    }
    expect(addedID, matches(RegExp(r'^vol_[0-9a-f]{16}$')));

    await tester.enterText(
      find.byKey(Key('resource-volume-$addedID-name')),
      'disk',
    );
    await tester.enterText(
      find.byKey(Key('resource-volume-$addedID-target')),
      '/workspace',
    );
    await tester.ensureVisible(
      find.byKey(Key('resource-volume-$addedID-write')),
    );
    await tester.tap(find.byKey(Key('resource-volume-$addedID-write')));
    await tester.pump();
    await tester.ensureVisible(find.byKey(const Key('resource-save')));
    await tester.tap(find.byKey(const Key('resource-save')));
    await tester.pumpAndSettle();

    final volumes = catalog.lastCreate?['spec']['volumes'] as List<dynamic>;
    expect(volumes, hasLength(1));
    final volume = volumes.single as Map;
    expect(volume['id'], addedID);
    expect(volume['name'], 'disk');
    expect(volume['target'], '/workspace');
    expect(volume['enabled'], isTrue);
    expect(volume['whitelisted'], isTrue);
    expect(volume['read'], isTrue);
    expect(volume['write'], isFalse);
    expect(volume['exec'], isTrue);
    expect(catalog.lastCreate?['spec']['idleTTLSeconds'], 90);
  });

  testWidgets('editing a container calls updateResource', (tester) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeResourcesCatalog(
      resources: [
        _resource(
          id: 'res_1',
          name: 'Dev',
          spec: {
            'image': 'alpine:3.20',
            'containerName': 'dev',
            'idleTTLSeconds': 3600,
            'volumes': <Object>[],
          },
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: ResourcesTab(catalog: catalog)));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('resource-res_1')));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byKey(const Key('resource-image')),
      'golang:1.23',
    );
    await tester.tap(find.byKey(const Key('resource-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastUpdateId, 'res_1');
    expect(catalog.lastUpdate?['spec']['image'], 'golang:1.23');
    expect(catalog.lastUpdate?['spec']['containerName'], 'dev');
  });

  testWidgets('delete confirms, and resource in use leaves the row', (
    tester,
  ) async {
    final catalog = FakeResourcesCatalog(
      resources: [_resource(id: 'res_1', name: 'Dev')],
    );
    catalog.deleteError = CatalogException(
      statusCode: 409,
      message: 'resource in use',
    );
    await tester.pumpWidget(MaterialApp(home: ResourcesTab(catalog: catalog)));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('delete-resource-res_1')));
    await tester.pumpAndSettle();
    expect(find.text('Delete resource?'), findsOneWidget);
    await tester.tap(find.widgetWithText(TextButton, 'Cancel'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, isNull);
    expect(find.text('Dev'), findsOneWidget);

    await tester.tap(find.byKey(const Key('delete-resource-res_1')));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();

    expect(catalog.lastDeleteId, 'res_1');
    expect(find.text('resource in use'), findsOneWidget);
    expect(find.text('Dev'), findsOneWidget);
  });

  testWidgets('Settings tabs are resources and environment, not sandbox', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    final display = await ChatDisplaySettings.load();
    final appearance = await AppearanceSettings.load();
    final catalog = FakeResourcesCatalog();
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: display,
          appearanceSettings: appearance,
        ),
      ),
    );
    await tester.pumpAndSettle();

    final labels = tester
        .widgetList<Tab>(find.byType(Tab))
        .map((tab) => tab.text)
        .toList();
    expect(labels, [
      'Providers',
      'Agents',
      'Projects',
      'Resources',
      'Environment',
      'Display',
    ]);
    expect(find.widgetWithText(Tab, 'Sandbox'), findsNothing);
  });
}
