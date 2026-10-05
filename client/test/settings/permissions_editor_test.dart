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
    await tester.tap(find.byKey(const Key('permission-rule-action-1')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Deny').last);
    await tester.pumpAndSettle();
    expect(rules[1].action, 'deny');

    await tester.tap(find.byKey(const Key('permission-rule-remove-0')));
    await tester.pumpAndSettle();
    expect(rules, hasLength(1));
    expect(rules.single.match.text, 'terraform *');
    expect(changes, greaterThan(0));
  });

  test('permissionsPatch drops blank rules and unset scorers', () {
    final rules = [
      PermissionRuleDraft(match: 'make *', action: 'allow'),
      PermissionRuleDraft(match: '   '),
    ];
    final patch = permissionsPatch(
      rules: rules,
      fast: PermissionScorerDraft(),
      deep: PermissionScorerDraft(),
    );
    expect(patch['rules'], [
      {'tool': 'run_command', 'match': 'make *', 'action': 'allow'},
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
