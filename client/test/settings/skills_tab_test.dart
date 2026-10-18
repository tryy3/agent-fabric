import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/skills_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

final _now = DateTime.utc(2026, 10, 18);

class _FakeSkillsCatalog extends CatalogClient {
  _FakeSkillsCatalog()
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  Map<String, dynamic> planeSkills = {};
  Map<String, dynamic> projectSettings = {};
  final queries = <String>[];
  Map<String, dynamic>? lastPlanePatch;
  Map<String, dynamic>? lastProjectPatch;

  @override
  Future<PlaneSettings> getSettings() async =>
      PlaneSettings(skills: planeSkills);

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
    lastPlanePatch = skills;
    planeSkills = skills ?? planeSkills;
    return PlaneSettings(skills: planeSkills);
  }

  @override
  Future<List<Project>> listProjects() async => [
    Project(
      id: 'proj_1',
      name: 'Site',
      settings: projectSettings,
      createdAt: _now,
      updatedAt: _now,
    ),
  ];

  @override
  Future<Project> updateProject(
    String id, {
    String? name,
    String? description,
    Map<String, dynamic>? settings,
    List<dynamic>? remotes,
  }) async {
    lastProjectPatch = settings;
    projectSettings = {...projectSettings, ...?settings};
    return Project(
      id: id,
      name: 'Site',
      settings: projectSettings,
      createdAt: _now,
      updatedAt: _now,
    );
  }

  @override
  Future<List<Assistant>> listAssistants() async => [
    Assistant(
      id: 'asst_1',
      name: 'Coder',
      version: 1,
      inferenceConnectionId: 'c',
      defaultModel: 'm',
      createdAt: _now,
      updatedAt: _now,
    ),
  ];

  List<SkillEntry> _entries(String? projectId) {
    final disabled = {
      ...SkillList.disabledIn(planeSkills),
      if (projectId != null) ...SkillList.disabledIn(projectSettings['skills']),
    };
    return [
      SkillEntry(
        name: 'deep-research',
        description: 'Research a question',
        source: 'builtin',
        enabled: !disabled.contains('deep-research'),
        requires: 'sandbox',
        maxToolRounds: 80,
        flags: const ['depth', 'save'],
      ),
      if (projectId != null)
        SkillEntry(
          name: 'hello',
          description: 'Says hello',
          source: 'project',
          enabled: !disabled.contains('hello'),
        ),
    ];
  }

  @override
  Future<SkillList> listSkills({String? projectId, String? assistantId}) async {
    queries.add('project=$projectId assistant=$assistantId');
    return SkillList(
      skills: _entries(projectId),
      problems: projectId == null
          ? const []
          : const [
              SkillProblem(
                source: 'project',
                path: '.agents/skills/bad',
                error: 'description is required',
              ),
            ],
    );
  }

  @override
  Future<SkillEntry> getSkill(
    String name, {
    String? projectId,
    String? assistantId,
  }) async {
    return SkillEntry(
      name: name,
      description: '',
      source: 'builtin',
      enabled: true,
      body: '# Deep research\n\nFull instructions.',
    );
  }
}

Future<void> _open(WidgetTester tester, _FakeSkillsCatalog catalog) async {
  tester.view.physicalSize = const Size(1200, 1600);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(body: SkillsTab(catalog: catalog)),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('lists built-in skills with source and details', (tester) async {
    final catalog = _FakeSkillsCatalog();
    await _open(tester, catalog);

    expect(find.byKey(const Key('skill-deep-research')), findsOneWidget);
    expect(find.text('/deep-research'), findsOneWidget);
    expect(find.text('builtin'), findsOneWidget);
    expect(find.textContaining('needs a sandbox'), findsOneWidget);
    expect(find.textContaining('up to 80 tool rounds'), findsOneWidget);
    expect(find.textContaining('--depth --save'), findsOneWidget);
    expect(find.byKey(const Key('skill-hello')), findsNothing);
    expect(catalog.queries.last, 'project=null assistant=null');
  });

  testWidgets('toggling at plane level saves the disabled list', (
    tester,
  ) async {
    final catalog = _FakeSkillsCatalog();
    await _open(tester, catalog);

    await tester.tap(find.byKey(const Key('skill-toggle-deep-research')));
    await tester.pumpAndSettle();

    expect(catalog.lastPlanePatch, {
      'disabled': ['deep-research'],
    });
    expect(find.byKey(const Key('skill-off-deep-research')), findsOneWidget);
    final toggle = tester.widget<Switch>(
      find.byKey(const Key('skill-toggle-deep-research')),
    );
    expect(toggle.value, isFalse);

    await tester.tap(find.byKey(const Key('skill-toggle-deep-research')));
    await tester.pumpAndSettle();
    expect(catalog.lastPlanePatch, {'disabled': <String>[]});
    expect(find.byKey(const Key('skill-off-deep-research')), findsNothing);
  });

  testWidgets('project scope includes project skills and skipped folders', (
    tester,
  ) async {
    final catalog = _FakeSkillsCatalog();
    await _open(tester, catalog);

    await tester.tap(find.text('Project'));
    await tester.pumpAndSettle();

    expect(catalog.queries.last, 'project=proj_1 assistant=null');
    expect(find.byKey(const Key('skill-hello')), findsOneWidget);
    expect(find.text('project'), findsOneWidget);
    expect(find.textContaining('description is required'), findsOneWidget);

    await tester.tap(find.byKey(const Key('skill-toggle-hello')));
    await tester.pumpAndSettle();
    expect(catalog.lastProjectPatch, {
      'skills': {
        'disabled': ['hello'],
      },
    });
    // A plane toggle is untouched by a project toggle.
    expect(catalog.lastPlanePatch, isNull);
  });

  testWidgets('tapping a skill shows its full text', (tester) async {
    await _open(tester, _FakeSkillsCatalog());

    await tester.tap(find.text('/deep-research'));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('skill-text')), findsOneWidget);
    expect(find.textContaining('Full instructions.'), findsOneWidget);
    await tester.tap(find.text('Close'));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('skill-text')), findsNothing);
  });
}
