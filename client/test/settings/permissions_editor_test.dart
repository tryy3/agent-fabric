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
}
