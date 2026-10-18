import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/gate_questions.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/catalog/permission_tiers.dart';
import 'package:agent_fabric_client/settings/permissions_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

const _network = PermissionTier(
  id: 'command.network',
  title: 'Network commands',
  description: 'Programs that talk to other hosts.',
  action: 'ask',
  risk: 6,
  consult: false,
  programs: ['curl', 'wget'],
);

const _webRequest = PermissionTier(
  id: 'web.request',
  title: 'Web requests',
  description: 'fetch_page and web_search.',
  action: 'ask',
  risk: 2,
  consult: true,
  tools: ['fetch_page', 'web_search'],
);

class _FakePlane extends CatalogClient {
  _FakePlane({this.permissions = const {}, this.saveError})
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  final Map<String, dynamic> permissions;
  final CatalogException? saveError;
  Map<String, dynamic>? saved;

  @override
  Future<PlaneSettings> getSettings() async =>
      PlaneSettings(permissions: permissions);

  @override
  Future<List<PermissionTier>> listPermissionBuiltins() async => const [
    _network,
    _webRequest,
  ];

  @override
  Future<GateQuestionCatalog> listPermissionQuestions() async =>
      const GateQuestionCatalog(
        families: [
          GateQuestionFamily(id: 'web', tools: ['fetch_page', 'web_search']),
        ],
        questions: [
          GateQuestion(
            id: 'data_leak',
            family: 'web',
            kind: 'tripwire',
            text: 'Does this request send private data to an outside host?',
            risk: 8,
            enabled: true,
          ),
        ],
      );

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
    if (saveError case final error?) throw error;
    saved = permissions;
    return PlaneSettings(permissions: permissions ?? const {});
  }
}

Future<void> _pumpTab(WidgetTester tester, CatalogClient catalog) async {
  tester.view.physicalSize = const Size(1200, 5000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(body: PermissionsTab(catalog: catalog)),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('lists the web request tier like any other', (tester) async {
    await _pumpTab(tester, _FakePlane());
    expect(find.text('Web requests'), findsOneWidget);
    expect(find.byKey(const Key('builtin-tools-web.request')), findsOneWidget);
    expect(
      tester
          .widget<Text>(find.byKey(const Key('builtin-tools-web.request')))
          .data,
      'fetch_page, web_search',
    );
    expect(find.text('asks · score 2'), findsOneWidget);
  });

  testWidgets('saves question overrides with the rest of the permissions', (
    tester,
  ) async {
    final scorers = {
      'deep': {'connectionId': 'c1', 'model': 'm', 'style': 'questions'},
    };
    final catalog = _FakePlane(permissions: {'scorers': scorers});
    await _pumpTab(tester, catalog);
    await tester.tap(
      find.byKey(const Key('gate-question-enabled-web-data_leak')),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('permissions-save')));
    await tester.pumpAndSettle();

    expect(catalog.saved, {
      'rules': <Object?>[],
      'builtins': <String, Object?>{},
      'scorers': scorers,
      'questions': {
        'builtins': {
          'data_leak': {'enabled': false},
        },
      },
    });
  });

  testWidgets('shows a rejected save', (tester) async {
    final catalog = _FakePlane(
      saveError: CatalogException(
        statusCode: 400,
        message: 'permissions.questions: unknown question "nope"',
      ),
    );
    await _pumpTab(tester, catalog);
    await tester.tap(
      find.byKey(const Key('gate-question-enabled-web-data_leak')),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('permissions-save')));
    await tester.pumpAndSettle();
    expect(find.textContaining('unknown question "nope"'), findsOneWidget);
  });
}
