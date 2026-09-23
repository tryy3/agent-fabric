import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/settings/appearance_settings.dart';
import 'package:agent_fabric_client/settings/projects_tab.dart';
import 'package:agent_fabric_client/settings/settings_page.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

Project _project({
  required String id,
  required String name,
  Map<String, dynamic> settings = const {},
  List<dynamic> remotes = const [],
}) {
  final now = DateTime.utc(2026, 9, 20);
  return Project(
    id: id,
    name: name,
    settings: settings,
    remotes: remotes,
    createdAt: now,
    updatedAt: now,
  );
}

Agent _agent({required String id, required String name}) {
  final now = DateTime.utc(2026, 9, 20);
  return Agent(
    id: id,
    name: name,
    version: 1,
    providerId: 'prov-1',
    defaultModel: 'm1',
    createdAt: now,
    updatedAt: now,
  );
}

class FakeProjectsCatalog extends CatalogClient {
  FakeProjectsCatalog({List<Project>? projects, List<Agent>? agents})
    : projects = List.of(projects ?? const []),
      agents = List.of(agents ?? const []),
      super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  final List<Project> projects;
  final List<Agent> agents;
  Map<String, dynamic>? lastSettings;
  List<dynamic>? lastRemotes;
  String? lastDeleteId;

  @override
  Future<List<Project>> listProjects() async => List.of(projects);

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);

  @override
  Future<List<Provider>> listProviders() async => const [];

  @override
  Future<List<Resource>> listResources() async {
    final now = DateTime.utc(2026, 9, 22);
    return [
      Resource(
        id: 'res_1',
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
      ),
    ];
  }

  @override
  Future<Map<String, dynamic>> resolvedEnvironment(String projectId) async {
    return {
      'resourceId': null,
      'resource': null,
      'workspaceRoot': '/workspace',
      'volumes': <Object>[],
      'extraPaths': <Object>[],
    };
  }

  @override
  Future<Project> updateProject(
    String id, {
    String? name,
    String? description,
    Map<String, dynamic>? settings,
    List<dynamic>? remotes,
  }) async {
    lastSettings = settings;
    lastRemotes = remotes;
    final index = projects.indexWhere((p) => p.id == id);
    final current = projects[index];
    final updated = Project(
      id: current.id,
      name: name ?? current.name,
      description: description ?? current.description,
      settings: settings ?? current.settings,
      remotes: remotes ?? current.remotes,
      createdAt: current.createdAt,
      updatedAt: DateTime.utc(2026, 9, 21),
    );
    projects[index] = updated;
    return updated;
  }

  @override
  Future<void> deleteProject(String id) async {
    lastDeleteId = id;
    projects.removeWhere((p) => p.id == id);
  }
}

void main() {
  testWidgets('Projects tab lists projects and saves allowed agents', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeProjectsCatalog(
      projects: [_project(id: 'proj_1', name: 'Landing')],
      agents: [_agent(id: 'ag-1', name: 'Coder')],
    );
    await tester.pumpWidget(MaterialApp(home: ProjectsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.text('Landing'), findsOneWidget);
    await tester.tap(find.byKey(const Key('project-proj_1')));
    await tester.pumpAndSettle();

    expect(find.text('Allowed agents'), findsOneWidget);
    expect(
      find.textContaining('GitHub and S3 remotes are stubs'),
      findsOneWidget,
    );
    expect(find.textContaining('Coming soon - extra MCP'), findsOneWidget);

    await tester.tap(find.byKey(const Key('project-agent-ag-1')));
    await tester.pump();
    await tester.enterText(
      find.byKey(const Key('project-tools-allow')),
      'read_file, write_file',
    );
    await tester.enterText(
      find.byKey(const Key('project-mcp-servers')),
      'github, search',
    );
    await tester.ensureVisible(find.byKey(const Key('project-memory-enabled')));
    await tester.tap(find.byKey(const Key('project-memory-enabled')));
    await tester.pump();
    await tester.enterText(
      find.byKey(const Key('project-context-items')),
      'https://example.com/docs',
    );
    await tester.ensureVisible(find.byKey(const Key('project-save')));
    await tester.tap(find.byKey(const Key('project-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastSettings?['allowedAgents'], ['ag-1']);
    expect(catalog.lastSettings?['tools']['allow'], [
      'read_file',
      'write_file',
    ]);
    expect(catalog.lastSettings?['mcp']['servers'], [
      {'name': 'github'},
      {'name': 'search'},
    ]);
    expect(catalog.lastSettings?['memory']['enabled'], isTrue);
    expect(
      (catalog.lastSettings?['context']['items'] as List).single['uri'],
      'https://example.com/docs',
    );
  });

  testWidgets('project environment clears resource to the global default', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeProjectsCatalog(
      projects: [
        _project(
          id: 'proj_1',
          name: 'Landing',
          settings: {
            'environment': {'resourceId': 'res_1'},
          },
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: ProjectsTab(catalog: catalog)));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Landing'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('project-isolation')), findsNothing);
    expect(find.text('isolated'), findsNothing);
    expect(find.text('shared (coming soon)'), findsNothing);
    expect(find.text('Add volume'), findsNothing);
    expect(find.text('no resource'), findsOneWidget);

    await tester.tap(find.byKey(const Key('project-resource')));
    await tester.pumpAndSettle();
      final menuItems = tester
          .widgetList<DropdownMenuItem<String>>(
            find.descendant(
              of: find.byType(ListView),
              matching: find.byType(DropdownMenuItem<String>),
            ),
          )
          .toList();
      expect((menuItems.first.child as Text).data, 'Use global default');
    await tester.tap(find.text('Use global default').last);
    await tester.pumpAndSettle();
    await tester.ensureVisible(
      find.byKey(const Key('project-environment-save')),
    );
    await tester.tap(find.byKey(const Key('project-environment-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastSettings?.containsKey('allowedAgents'), isFalse);
    final environment = catalog.lastSettings?['environment'] as Map;
    expect(environment.containsKey('resourceId'), isTrue);
    expect(environment['resourceId'], isNull);
  });

  testWidgets('project environment saves workspace, path, and grant', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeProjectsCatalog(
      projects: [_project(id: 'proj_1', name: 'Landing')],
    );
    await tester.pumpWidget(MaterialApp(home: ProjectsTab(catalog: catalog)));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Landing'));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('project-resource')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Work').last);
    await tester.pumpAndSettle();

    await tester.enterText(
      find.byKey(const Key('project-workspace-root')),
      '/srv',
    );
    await tester.ensureVisible(find.byKey(const Key('project-path-add')));
    await tester.tap(find.byKey(const Key('project-path-add')));
    await tester.pump();

    late String pathID;
    for (final widget in tester.widgetList<TextField>(find.byType(TextField))) {
      final key = widget.key;
      if (key is ValueKey<String> &&
          key.value.startsWith('project-path-') &&
          key.value.endsWith('-path')) {
        pathID = key.value.substring(
          'project-path-'.length,
          key.value.length - '-path'.length,
        );
      }
    }
    expect(pathID, matches(RegExp(r'^path_[0-9a-f]{16}$')));
    await tester.enterText(
      find.byKey(Key('project-path-$pathID-path')),
      '/opt/tools',
    );
    await tester.ensureVisible(
      find.byKey(const Key('project-grant-vol_0123456789abcdef-write')),
    );
    await tester.tap(
      find.byKey(const Key('project-grant-vol_0123456789abcdef-write')),
    );
    await tester.pump();
    await tester.ensureVisible(
      find.byKey(const Key('project-environment-save')),
    );
    await tester.tap(find.byKey(const Key('project-environment-save')));
    await tester.pumpAndSettle();

    final environment = catalog.lastSettings?['environment'] as Map;
    expect(environment['resourceId'], 'res_1');
    expect(environment['workspaceRoot'], '/srv');
    final extra = (environment['extraPaths'] as List).single as Map;
    expect(extra['id'], pathID);
    expect(extra['path'], '/opt/tools');
    expect(environment['grants'], [
      {'volumeId': 'vol_0123456789abcdef', 'write': false},
    ]);
  });

  testWidgets('Project editor saves remotes stubs by id', (tester) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final catalog = FakeProjectsCatalog(
      projects: [_project(id: 'proj_1', name: 'Landing')],
    );
    await tester.pumpWidget(MaterialApp(home: ProjectsTab(catalog: catalog)));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Landing'));
    await tester.pumpAndSettle();

    await tester.ensureVisible(find.byKey(const Key('project-remote-add')));
    await tester.tap(find.byKey(const Key('project-remote-add')));
    await tester.pump();

    late String remoteID;
    for (final widget in tester.widgetList<TextField>(find.byType(TextField))) {
      final key = widget.key;
      if (key is ValueKey<String> &&
          key.value.startsWith('project-remote-') &&
          key.value.endsWith('-url')) {
        remoteID = key.value.substring(
          'project-remote-'.length,
          key.value.length - '-url'.length,
        );
      }
    }
    expect(remoteID, startsWith('rmt_'));
    await tester.enterText(
      find.byKey(Key('project-remote-$remoteID-url')),
      'https://github.com/acme/landing',
    );
    await tester.ensureVisible(find.byKey(const Key('project-save')));
    await tester.tap(find.byKey(const Key('project-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastRemotes, isNotNull);
    final remote = (catalog.lastRemotes as List).single as Map;
    expect(remote['id'], remoteID);
    expect(remote['kind'], 'github');
    expect(remote['urlOrBucket'], 'https://github.com/acme/landing');
  });

  testWidgets('Settings page includes a Projects tab', (tester) async {
    SharedPreferences.setMockInitialValues({});
    final display = await ChatDisplaySettings.load();
    final appearance = await AppearanceSettings.load();
    final catalog = FakeProjectsCatalog();
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsPage(
          catalog: catalog,
          displaySettings: display,
          appearanceSettings: appearance,
        ),
      ),
    );
    expect(find.text('Projects'), findsOneWidget);
    await tester.tap(find.text('Projects'));
    await tester.pumpAndSettle();
    expect(find.byType(ProjectsTab), findsOneWidget);
  });

  testWidgets('delete project confirms, and Default has no delete icon', (
    tester,
  ) async {
    final catalog = FakeProjectsCatalog(
      projects: [
        _project(id: 'proj_default', name: 'Default'),
        _project(id: 'proj_land', name: 'Landing'),
      ],
    );
    await tester.pumpWidget(MaterialApp(home: ProjectsTab(catalog: catalog)));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('delete-project-proj_default')), findsNothing);
    expect(find.byKey(const Key('delete-project-proj_land')), findsOneWidget);

    await tester.tap(find.byKey(const Key('delete-project-proj_land')));
    await tester.pumpAndSettle();
    expect(find.text('Delete project?'), findsOneWidget);
    expect(
      find.text(
        'Delete Landing? This deletes the project, its settings, and its threads. Workspace files and the linked environment are left in place.',
      ),
      findsOneWidget,
    );

    await tester.tap(find.widgetWithText(TextButton, 'Cancel'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, isNull);
    expect(find.text('Landing'), findsOneWidget);

    await tester.tap(find.byKey(const Key('delete-project-proj_land')));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(TextButton, 'Delete'));
    await tester.pumpAndSettle();
    expect(catalog.lastDeleteId, 'proj_land');
    expect(find.text('Landing'), findsNothing);
    expect(find.text('Default'), findsOneWidget);
  });
}
