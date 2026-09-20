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
  String? lastIsolation;

  @override
  Future<List<Project>> listProjects() async => List.of(projects);

  @override
  Future<List<Agent>> listAgents() async => List.of(agents);

  @override
  Future<List<Provider>> listProviders() async => const [];

  @override
  Future<Map<String, dynamic>> resolvedProjectSandbox(String projectId) async {
    return {
      'image': 'alpine:3.20',
      'containerName': 'agent-fabric-container-$projectId',
    };
  }

  @override
  Future<Project> updateProject(
    String id, {
    String? name,
    String? description,
    String? isolation,
    Map<String, dynamic>? settings,
    List<dynamic>? remotes,
  }) async {
    lastSettings = settings;
    lastRemotes = remotes;
    lastIsolation = isolation;
    final index = projects.indexWhere((p) => p.id == id);
    final current = projects[index];
    final updated = Project(
      id: current.id,
      name: name ?? current.name,
      description: description ?? current.description,
      isolation: isolation ?? current.isolation,
      settings: settings ?? current.settings,
      remotes: remotes ?? current.remotes,
      createdAt: current.createdAt,
      updatedAt: DateTime.utc(2026, 9, 21),
    );
    projects[index] = updated;
    return updated;
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
    expect(find.text('isolated'), findsWidgets);
    await tester.tap(find.byKey(const Key('project-isolation')));
    await tester.pumpAndSettle();
    expect(find.text('shared (coming soon)'), findsOneWidget);
    await tester.tap(find.text('isolated').last);
    await tester.pumpAndSettle();
    expect(
      find.textContaining('GitHub and S3 remotes are stubs'),
      findsOneWidget,
    );
    expect(find.textContaining('Coming soon — extra MCP'), findsOneWidget);

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
    expect(catalog.lastIsolation, 'isolated');
  });

  testWidgets('Project editor saves sandbox overlay separately', (
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

    expect(find.byKey(const Key('project-resolved-sandbox')), findsOneWidget);
    await tester.ensureVisible(find.byKey(const Key('sandbox-image')));
    await tester.enterText(
      find.byKey(const Key('sandbox-image')),
      'golang:1.23',
    );
    await tester.ensureVisible(find.byKey(const Key('sandbox-save')));
    await tester.tap(find.byKey(const Key('sandbox-save')));
    await tester.pumpAndSettle();

    expect(catalog.lastSettings?['sandbox']['image'], 'golang:1.23');
    expect(catalog.lastSettings?.containsKey('allowedAgents'), isFalse);
    expect(
      (catalog.lastSettings?['sandbox'] as Map).containsKey('kind'),
      isFalse,
    );
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
}
