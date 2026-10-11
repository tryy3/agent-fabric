import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/gate_questions.dart';
import 'package:agent_fabric_client/settings/gate_questions_editor.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';

const _families = [
  GateQuestionFamily(id: 'command', tools: ['run_command']),
  GateQuestionFamily(id: 'web', tools: ['fetch_page', 'web_search']),
  GateQuestionFamily(id: 'file_read', tools: ['read_file', 'list_files']),
];

const _questions = [
  GateQuestion(
    id: 'destructive',
    family: 'command',
    kind: 'tripwire',
    text: 'Does this command delete data irreversibly?',
    risk: 8,
    enabled: true,
  ),
  GateQuestion(
    id: 'policy_violation',
    family: 'command',
    kind: 'policy',
    text: 'Does this command break the gate instructions?',
    risk: 7,
    enabled: true,
  ),
  GateQuestion(
    id: 'policy_exception',
    family: 'command',
    kind: 'policy',
    text: 'Do the instructions explicitly allow this exact command?',
    enabled: true,
  ),
  GateQuestion(
    id: 'data_leak',
    family: 'web',
    kind: 'tripwire',
    text: 'Does this request send private data to an outside host?',
    risk: 8,
    enabled: true,
  ),
  GateQuestion(
    id: 'policy_violation',
    family: 'web',
    kind: 'policy',
    text: 'Does this web request break the gate instructions?',
    risk: 7,
    enabled: true,
  ),
  GateQuestion(
    id: 'bulk_gathering',
    family: 'file_read',
    kind: 'tripwire',
    text: 'Does this read gather many files at once?',
    risk: 6,
    enabled: false,
  ),
];

class _FakeCatalog extends CatalogClient {
  _FakeCatalog({this.failWith})
    : super(
        baseUri: Uri.parse('http://catalog.test'),
        httpClient: MockClient((_) async => http.Response('unused', 500)),
      );

  CatalogException? failWith;
  int loads = 0;

  @override
  Future<GateQuestionCatalog> listPermissionQuestions() async {
    loads++;
    if (failWith case final error?) throw error;
    return const GateQuestionCatalog(
      questions: _questions,
      families: _families,
    );
  }
}

Future<void> _pump(
  WidgetTester tester,
  _FakeCatalog catalog,
  GateQuestionsDraft draft,
  VoidCallback onChanged,
) async {
  tester.view.physicalSize = const Size(1200, 4000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(
        body: SingleChildScrollView(
          child: StatefulBuilder(
            builder: (context, setState) => GateQuestionsEditor(
              catalog: catalog,
              draft: draft,
              onChanged: () {
                onChanged();
                setState(() {});
              },
            ),
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

TextField _field(WidgetTester tester, String key) =>
    tester.widget<TextField>(find.byKey(Key(key)));

void main() {
  group('GateQuestionsDraft', () {
    test('stores only the difference from the defaults', () {
      final draft = GateQuestionsDraft();
      const q = GateQuestion(
        id: 'destructive',
        family: 'command',
        kind: 'tripwire',
        text: 'Delete?',
        risk: 8,
        enabled: true,
      );
      expect(draft.toJson(), isNull);

      draft.setRisk(q, 9);
      expect(draft.toJson(), {
        'builtins': {
          'destructive': {'risk': 9},
        },
      });

      draft.setText(q, 'Deletes data?');
      draft.setEnabled(q, value: false);
      expect(draft.toJson(), {
        'builtins': {
          'destructive': {'enabled': false, 'risk': 9, 'text': 'Deletes data?'},
        },
      });

      draft.setRisk(q, 8);
      draft.setText(q, 'Delete?');
      draft.setEnabled(q, value: true);
      expect(draft.isModified('destructive'), isFalse);
      expect(draft.toJson(), isNull);
    });

    test('round-trips stored settings', () {
      final stored = {
        'builtins': {
          'data_leak': {'enabled': false},
        },
        'custom': [
          {
            'id': 'prod_db',
            'tools': ['run_command'],
            'kind': 'tripwire',
            'text': 'Does this touch the production database?',
            'risk': 9,
            'enabled': true,
          },
        ],
      };
      expect(GateQuestionsDraft.fromJson(stored).toJson(), stored);
      expect(GateQuestionsDraft.fromJson(null).toJson(), isNull);
    });
  });

  testWidgets('groups questions by family with kind badges', (tester) async {
    await _pump(tester, _FakeCatalog(), GateQuestionsDraft(), () {});

    expect(find.text('Commands'), findsOneWidget);
    expect(find.text('Web'), findsOneWidget);
    expect(find.text('File reads'), findsOneWidget);
    expect(find.byKey(const Key('gate-family-file_write')), findsNothing);
    for (final key in [
      'command-destructive',
      'command-policy_violation',
      'command-policy_exception',
      'web-data_leak',
      'web-policy_violation',
      'file_read-bulk_gathering',
    ]) {
      expect(
        find.byKey(Key('gate-question-$key')),
        findsOneWidget,
        reason: key,
      );
    }
    // The policy pair is listed once per family.
    expect(find.text('policy_violation'), findsNWidgets(2));
    expect(
      find.descendant(
        of: find.byKey(const Key('gate-question-kind-command-destructive')),
        matching: find.text('tripwire'),
      ),
      findsOneWidget,
    );
    expect(
      find.descendant(
        of: find.byKey(const Key('gate-question-kind-web-policy_violation')),
        matching: find.text('policy'),
      ),
      findsOneWidget,
    );
    // The exception carries no score; the others do.
    expect(
      find.byKey(const Key('gate-question-risk-command-policy_exception')),
      findsNothing,
    );
    expect(
      find.byKey(const Key('gate-question-risk-command-destructive')),
      findsOneWidget,
    );
    expect(find.textContaining('a yes sets a'), findsOneWidget);
    expect(
      find.textContaining('only run when gate instructions'),
      findsOneWidget,
    );
  });

  testWidgets('edits write only the diff and reset removes the override', (
    tester,
  ) async {
    final draft = GateQuestionsDraft();
    var changes = 0;
    await _pump(tester, _FakeCatalog(), draft, () => changes++);
    const modified = Key('gate-question-modified-command-destructive');
    expect(find.byKey(modified), findsNothing);
    expect(
      find.byKey(const Key('gate-question-reset-command-destructive')),
      findsNothing,
    );

    await tester.enterText(
      find.byKey(const Key('gate-question-text-command-destructive')),
      'Does this command wipe data?',
    );
    await tester.pumpAndSettle();
    expect(draft.toJson(), {
      'builtins': {
        'destructive': {'text': 'Does this command wipe data?'},
      },
    });
    expect(find.byKey(modified), findsOneWidget);

    await tester.tap(
      find.byKey(const Key('gate-question-risk-command-destructive')),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('9 · cancel').last);
    await tester.pumpAndSettle();
    expect(draft.builtins['destructive']!.risk, 9);

    await tester.tap(
      find.byKey(const Key('gate-question-enabled-command-destructive')),
    );
    await tester.pumpAndSettle();
    expect(draft.builtins['destructive']!.enabled, isFalse);
    expect(changes, greaterThanOrEqualTo(3));

    await tester.tap(
      find.byKey(const Key('gate-question-reset-command-destructive')),
    );
    await tester.pumpAndSettle();
    expect(draft.toJson(), isNull);
    expect(find.byKey(modified), findsNothing);
    expect(
      _field(tester, 'gate-question-text-command-destructive').controller!.text,
      'Does this command delete data irreversibly?',
    );
    expect(
      tester
          .widget<Switch>(
            find.byKey(const Key('gate-question-enabled-command-destructive')),
          )
          .value,
      isTrue,
    );
  });

  testWidgets('a disabled question shows its switch off and can be enabled', (
    tester,
  ) async {
    final draft = GateQuestionsDraft();
    await _pump(tester, _FakeCatalog(), draft, () {});
    const key = Key('gate-question-enabled-file_read-bulk_gathering');
    expect(tester.widget<Switch>(find.byKey(key)).value, isFalse);

    await tester.tap(find.byKey(key));
    await tester.pumpAndSettle();
    expect(draft.toJson(), {
      'builtins': {
        'bulk_gathering': {'enabled': true},
      },
    });
  });

  testWidgets('a policy edit shows in every family', (tester) async {
    final draft = GateQuestionsDraft();
    await _pump(tester, _FakeCatalog(), draft, () {});
    expect(find.text('applies to every tool family'), findsNWidgets(3));

    await tester.enterText(
      find.byKey(const Key('gate-question-text-command-policy_violation')),
      'Does this go against what the user wrote?',
    );
    await tester.pumpAndSettle();
    expect(
      _field(
        tester,
        'gate-question-text-web-policy_violation',
      ).controller!.text,
      'Does this go against what the user wrote?',
    );
    expect(
      find.byKey(const Key('gate-question-modified-web-policy_violation')),
      findsOneWidget,
    );
    expect(draft.builtins.keys, ['policy_violation']);
  });

  testWidgets('text outside the limits shows an error', (tester) async {
    await _pump(tester, _FakeCatalog(), GateQuestionsDraft(), () {});
    await tester.enterText(
      find.byKey(const Key('gate-question-text-command-destructive')),
      'short',
    );
    await tester.pumpAndSettle();
    expect(find.text('10-600 characters'), findsOneWidget);
  });

  group('custom tripwires', () {
    Future<void> openDialog(WidgetTester tester) async {
      await tester.tap(find.byKey(const Key('gate-custom-add')));
      await tester.pumpAndSettle();
    }

    testWidgets('validates, adds, edits and removes', (tester) async {
      final draft = GateQuestionsDraft();
      await _pump(tester, _FakeCatalog(), draft, () {});

      await openDialog(tester);
      await tester.tap(find.byKey(const Key('gate-custom-save')));
      await tester.pumpAndSettle();
      expect(find.textContaining('Lowercase letters'), findsOneWidget);
      expect(find.text('Pick at least one tool'), findsOneWidget);
      expect(find.text('10-600 characters'), findsOneWidget);
      expect(draft.custom, isEmpty);

      await tester.enterText(find.byKey(const Key('gate-custom-id')), 'Bad-Id');
      await tester.pumpAndSettle();
      expect(find.textContaining('Lowercase letters'), findsOneWidget);
      await tester.enterText(
        find.byKey(const Key('gate-custom-id')),
        'data_leak',
      );
      await tester.pumpAndSettle();
      expect(
        find.text('Another question already uses this id'),
        findsOneWidget,
      );

      await tester.enterText(
        find.byKey(const Key('gate-custom-id')),
        'prod_db',
      );
      await tester.tap(find.byKey(const Key('gate-custom-tool-run_command')));
      await tester.enterText(
        find.byKey(const Key('gate-custom-text')),
        'Does this touch the production database?',
      );
      await tester.tap(find.byKey(const Key('gate-custom-risk')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('9 · cancel').last);
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('gate-custom-save')));
      await tester.pumpAndSettle();

      expect(draft.toJson(), {
        'custom': [
          {
            'id': 'prod_db',
            'tools': ['run_command'],
            'kind': 'tripwire',
            'text': 'Does this touch the production database?',
            'risk': 9,
            'enabled': true,
          },
        ],
      });
      expect(find.text('prod_db · score 9'), findsOneWidget);

      await tester.tap(find.byKey(const Key('gate-custom-enabled-0')));
      await tester.pumpAndSettle();
      expect(draft.custom.single.enabled, isFalse);

      await tester.tap(find.byKey(const Key('gate-custom-edit-0')));
      await tester.pumpAndSettle();
      expect(
        tester
            .widget<TextField>(find.byKey(const Key('gate-custom-id')))
            .enabled,
        isFalse,
      );
      await tester.enterText(
        find.byKey(const Key('gate-custom-text')),
        'Does this write to the production database?',
      );
      await tester.tap(find.byKey(const Key('gate-custom-save')));
      await tester.pumpAndSettle();
      expect(
        draft.custom.single.text,
        'Does this write to the production database?',
      );
      expect(draft.custom.single.enabled, isFalse);

      await tester.tap(find.byKey(const Key('gate-custom-remove-0')));
      await tester.pumpAndSettle();
      expect(draft.custom, isEmpty);
      expect(draft.toJson(), isNull);
    });

    testWidgets('enforces the per-family maximum', (tester) async {
      final draft = GateQuestionsDraft(
        custom: [
          for (var i = 0; i < 8; i++)
            CustomGateQuestion(
              id: 'extra_$i',
              tools: const ['run_command'],
              text: 'Is this extra check number $i a yes?',
              risk: 5,
            ),
        ],
      );
      await _pump(tester, _FakeCatalog(), draft, () {});

      await openDialog(tester);
      await tester.enterText(find.byKey(const Key('gate-custom-id')), 'ninth');
      await tester.tap(find.byKey(const Key('gate-custom-tool-run_command')));
      await tester.enterText(
        find.byKey(const Key('gate-custom-text')),
        'Is this the ninth command question?',
      );
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('gate-custom-tools-error')), findsOneWidget);
      await tester.tap(find.byKey(const Key('gate-custom-save')));
      await tester.pumpAndSettle();
      expect(draft.custom, hasLength(8));

      // Another family still has room.
      await tester.tap(find.byKey(const Key('gate-custom-tool-run_command')));
      await tester.tap(find.byKey(const Key('gate-custom-tool-fetch_page')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('gate-custom-tools-error')), findsNothing);
      await tester.tap(find.byKey(const Key('gate-custom-save')));
      await tester.pumpAndSettle();
      expect(draft.custom, hasLength(9));
      expect(draft.custom.last.tools, ['fetch_page']);
    });
  });

  testWidgets('shows the load error and retries', (tester) async {
    final catalog = _FakeCatalog(
      failWith: CatalogException(statusCode: 500, message: 'questions down'),
    );
    await _pump(tester, catalog, GateQuestionsDraft(), () {});
    expect(find.byKey(const Key('gate-questions-load-error')), findsOneWidget);
    expect(find.textContaining('questions down'), findsOneWidget);
    expect(find.byKey(const Key('gate-custom-add')), findsNothing);

    catalog.failWith = null;
    await tester.tap(find.byKey(const Key('gate-questions-retry')));
    await tester.pumpAndSettle();
    expect(catalog.loads, 2);
    expect(find.byKey(const Key('gate-custom-add')), findsOneWidget);
  });
}
