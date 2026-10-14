import 'package:agent_fabric_client/catalog/gate_questions.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/settings/permissions_editor.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('adds, edits and removes permission rules', (tester) async {
    final rules = permissionRuleDrafts({
      'permissions': {
        'mode': 'auto',
        'rules': [
          {'tool': 'run_command', 'match': 'git push *', 'action': 'ask'},
        ],
      },
    });
    var changes = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) => PermissionRulesEditor(
              rules: rules,
              onChanged: () => setState(() => changes++),
            ),
          ),
        ),
      ),
    );
    expect(rules.single.match.text, 'git push *');
    expect(find.byKey(const Key('permission-rule-match-0')), findsOneWidget);

    await tester.tap(find.byKey(const Key('permission-rule-add')));
    await tester.pumpAndSettle();
    expect(rules, hasLength(2));
    await tester.enterText(
      find.byKey(const Key('permission-rule-match-1')),
      'terraform *',
    );
    expect(rules[1].risk, permissionRuleDefaultScore);
    await tester.tap(find.byKey(const Key('permission-rule-score-1')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('10 · cancel').last);
    await tester.pumpAndSettle();
    expect(rules[1].risk, 10);

    expect(rules[1].consult, isFalse);
    await tester.tap(find.byKey(const Key('permission-rule-consult-1')));
    await tester.pumpAndSettle();
    expect(rules[1].consult, isTrue);
    expect(rules[1].toJson()['consult'], isTrue);
    expect(rules[0].toJson().containsKey('consult'), isFalse);

    await tester.tap(find.byKey(const Key('permission-rule-remove-0')));
    await tester.pumpAndSettle();
    expect(rules, hasLength(1));
    expect(rules.single.match.text, 'terraform *');
    expect(changes, greaterThan(0));
  });

  test('stored allow, ask and deny rules show as the score they imply', () {
    final rules = permissionRuleDrafts({
      'permissions': {
        'rules': [
          {'tool': 'run_command', 'match': 'a', 'action': 'allow'},
          {'tool': 'run_command', 'match': 'b', 'action': 'ask'},
          {'tool': 'run_command', 'match': 'c', 'action': 'deny'},
          {'tool': 'run_command', 'match': 'd', 'action': 'ask', 'risk': 8},
        ],
      },
    });
    expect(
      [for (final r in rules) r.risk],
      [1, permissionRuleDefaultScore, 10, 8],
    );
    expect(rules.first.toJson(), {
      'tool': 'run_command',
      'match': 'a',
      'action': 'score',
      'risk': 1,
    });
  });

  test('permissionsPatch writes max lowering and keeps the other options', () {
    final tuning = PermissionScorerTuning.fromJson({
      'maxLower': 2,
      'minConfidence': 0.7,
      'skipAtOrBelow': 3,
    });
    tuning.maxLower.text = '0';
    final patch = permissionsPatch(
      rules: const [],
      fast: PermissionScorerDraft(),
      deep: PermissionScorerDraft(connectionId: 'c2', model: 'm'),
      tuning: tuning,
    );
    final scorers = patch['scorers']! as Map;
    expect(scorers['maxLower'], 0);
    expect(scorers['minConfidence'], 0.7);
    expect(scorers['skipAtOrBelow'], 3);

    tuning.maxLower.text = '';
    expect(tuning.toJson().containsKey('maxLower'), isFalse);
    tuning.maxLower.text = '12';
    expect(tuning.toJson().containsKey('maxLower'), isFalse);
  });

  test('scorer draft keeps stored fields it has no control for', () {
    final stored = {
      'connectionId': 'c2',
      'model': 'big',
      'strategy': 'score',
      'style': 'bands',
      'enableThinking': false,
      'reasoningEffort': 'low',
      'maxTokens': 512,
      'structuredOutput': true,
    };
    final draft = PermissionScorerDraft.fromJson(stored);
    draft.model.text = 'bigger';
    draft.strategy = null;
    expect(draft.toJson(), {...stored, 'model': 'bigger'}..remove('strategy'));

    draft.model.text = '';
    expect(draft.toJson(), isNull);
  });

  test('permissionsPatch drops blank rules and unset scorers', () {
    final rules = [
      PermissionRuleDraft(match: 'make *', risk: 1),
      PermissionRuleDraft(match: '   '),
    ];
    final patch = permissionsPatch(
      rules: rules,
      fast: PermissionScorerDraft(),
      deep: PermissionScorerDraft(),
    );
    expect(patch['rules'], [
      {'tool': 'run_command', 'match': 'make *', 'action': 'score', 'risk': 1},
    ]);
    expect(patch.containsKey('mode'), isFalse);
    expect(patch['scorers'], isNull);

    final withScorer = permissionsPatch(
      rules: const [],
      fast: PermissionScorerDraft(
        connectionId: 'c1',
        model: 'jev-latest',
        strategy: 'score',
      ),
      deep: PermissionScorerDraft(connectionId: 'c2'),
    );
    expect(withScorer['scorers'], {
      'fast': {
        'connectionId': 'c1',
        'model': 'jev-latest',
        'strategy': 'score',
      },
      'deep': null,
    });
  });

  testWidgets('fast strategy dropdown lists every strategy with a help line', (
    tester,
  ) async {
    final now = DateTime.utc(2026, 9, 12);
    final scorer = PermissionScorerDraft(
      connectionId: 'c1',
      model: 'jev-latest',
      strategy: 'questions',
    );
    addTearDown(scorer.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) => PermissionScorerPicker(
              label: 'Fast',
              help: 'help',
              scorer: scorer,
              withStrategy: true,
              connections: [
                InferenceConnection(
                  id: 'c1',
                  name: 'Local',
                  type: providerTypeOpenAICompatible,
                  baseUrl: 'http://x/v1',
                  apiKey: '',
                  models: const [],
                  modelsUpdatedAt: now,
                  createdAt: now,
                  updatedAt: now,
                ),
              ],
              onChanged: () => setState(() {}),
            ),
          ),
        ),
      ),
    );

    expect(permissionScorerStrategies, [
      'score',
      'bands',
      'questions',
      'score+questions',
    ]);
    expect(
      find.textContaining('Asks yes/no questions about the call'),
      findsOneWidget,
    );

    await tester.tap(find.byKey(const Key('permission-scorer-Fast-strategy')));
    await tester.pumpAndSettle();
    for (final s in permissionScorerStrategies) {
      expect(find.text(s), findsWidgets);
    }
    await tester.tap(find.text('score+questions').last);
    await tester.pumpAndSettle();
    expect(scorer.strategy, 'score+questions');
    expect(
      find.textContaining('Scores the call and also asks'),
      findsOneWidget,
    );
  });

  testWidgets('deep style dropdown lists every style and stores the pick', (
    tester,
  ) async {
    final now = DateTime.utc(2026, 9, 12);
    final scorer = PermissionScorerDraft.fromJson({
      'connectionId': 'c1',
      'model': 'm1',
      'style': 'bands',
    });
    addTearDown(scorer.dispose);
    var changes = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) => PermissionScorerPicker(
              label: 'Deep',
              help: 'help',
              scorer: scorer,
              connections: [
                InferenceConnection(
                  id: 'c1',
                  name: 'Local',
                  type: providerTypeOpenAICompatible,
                  baseUrl: 'http://x/v1',
                  apiKey: '',
                  models: const [ModelInfo(id: 'm1', name: 'Model 1')],
                  modelsUpdatedAt: now,
                  createdAt: now,
                  updatedAt: now,
                ),
              ],
              onChanged: () => setState(() => changes++),
            ),
          ),
        ),
      ),
    );

    expect(permissionScorerStyles, permissionScorerStrategies);
    expect(find.textContaining('which risk band fits'), findsOneWidget);
    expect(
      find.byKey(const Key('permission-scorer-Deep-strategy')),
      findsNothing,
    );

    await tester.tap(find.byKey(const Key('permission-scorer-Deep-style')));
    await tester.pumpAndSettle();
    for (final s in permissionScorerStyles) {
      expect(find.text(s), findsWidgets);
    }
    await tester.tap(find.text('score+questions').last);
    await tester.pumpAndSettle();
    expect(scorer.style, 'score+questions');
    expect(scorer.strategy, isNull);
    expect(changes, 1);
    expect(
      find.textContaining('Scores the call and also asks'),
      findsOneWidget,
    );
    expect(scorer.toJson(), {
      'connectionId': 'c1',
      'model': 'm1',
      'style': 'score+questions',
    });

    await tester.tap(find.byKey(const Key('permission-scorer-Deep-style')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Default').last);
    await tester.pumpAndSettle();
    expect(scorer.style, isNull);
    expect(scorer.toJson()!.containsKey('style'), isFalse);
  });

  test('permissionsPatch sends the question overrides', () {
    final questions = GateQuestionsDraft.fromJson({
      'builtins': {
        'data_leak': {'risk': 9},
      },
    });
    final patch = permissionsPatch(
      rules: const [],
      fast: PermissionScorerDraft(),
      deep: PermissionScorerDraft(),
      questions: questions,
    );
    expect(patch['questions'], {
      'builtins': {
        'data_leak': {'risk': 9},
      },
    });
    // No draft leaves the stored questions alone; an empty one clears them.
    expect(
      permissionsPatch(
        rules: const [],
        fast: PermissionScorerDraft(),
        deep: PermissionScorerDraft(),
      ).containsKey('questions'),
      isFalse,
    );
    questions.reset('data_leak');
    expect(
      permissionsPatch(
        rules: const [],
        fast: PermissionScorerDraft(),
        deep: PermissionScorerDraft(),
        questions: questions,
      )['questions'],
      isNull,
    );
  });

  test('instructionsMaxChars is kept, validated and sent', () {
    final tuning = PermissionScorerTuning.fromJson({
      'instructionsMaxChars': 2500,
      'maxLower': 1,
    });
    addTearDown(tuning.dispose);
    expect(tuning.toJson(), {'maxLower': 1, 'instructionsMaxChars': 2500});

    for (final bad in ['99', '20001']) {
      tuning.instructionsMaxChars.text = bad;
      expect(tuning.instructionsMaxCharsInvalid, isTrue, reason: bad);
      expect(tuning.toJson().containsKey('instructionsMaxChars'), isFalse);
    }
    for (final ok in ['100', '20000']) {
      tuning.instructionsMaxChars.text = ok;
      expect(tuning.instructionsMaxCharsInvalid, isFalse, reason: ok);
    }
    tuning.instructionsMaxChars.text = '';
    expect(tuning.instructionsMaxCharsInvalid, isFalse);
    expect(tuning.toJson(), {'maxLower': 1});
  });
}
