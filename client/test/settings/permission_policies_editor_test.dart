import 'package:agent_fabric_client/settings/permission_policies_editor.dart';
import 'package:agent_fabric_client/settings/permissions_editor.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  test('policy drafts send only changed thresholds', () {
    final drafts = permissionPolicyDrafts({
      'auto': {'askAt': 5},
      'full': {'askAt': 3, 'cancelAt': 9},
    });
    expect([for (final d in drafts) d.askAt], [3, 5, 5, null]);
    expect(drafts.last.cancelAt, 9);
    expect(permissionPoliciesJson(drafts), {
      'auto': {'askAt': 5},
      'full': {'cancelAt': 9},
    });

    drafts[2].reset();
    drafts[3].reset();
    expect(permissionPoliciesJson(drafts), isNull);

    drafts[0].askAt = 10;
    expect(drafts[0].isValid, isFalse);
    drafts[0].askAt = 9;
    expect(drafts[0].isValid, isTrue);
    expect(drafts[0].asks, isFalse);
  });

  testWidgets('editor changes a threshold and flags a mode that never asks', (
    tester,
  ) async {
    final drafts = permissionPolicyDrafts(null);
    var changes = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: StatefulBuilder(
              builder: (context, setState) => PermissionPoliciesEditor(
                drafts: drafts,
                onChanged: () => setState(() => changes++),
              ),
            ),
          ),
        ),
      ),
    );
    expect(find.byKey(const Key('policy-ask-full')), findsNothing);
    expect(find.text('Never'), findsOneWidget);

    await tester.tap(find.byKey(const Key('policy-ask-auto')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('9 · cancel').last);
    await tester.pumpAndSettle();
    expect(changes, 1);
    expect(drafts[2].askAt, 9);
    expect(find.byKey(const Key('policy-never-asks-auto')), findsOneWidget);

    await tester.tap(find.byKey(const Key('policy-reset-auto')));
    await tester.pumpAndSettle();
    expect(drafts[2].askAt, 7);
    expect(find.byKey(const Key('policy-never-asks-auto')), findsNothing);
  });

  test('assistant mode reads and writes the storable modes only', () {
    expect(permissionModeOf(null), 'ask');
    expect(
      permissionModeOf({
        'permissions': {'mode': 'auto'},
      }),
      'auto',
    );
    expect(
      permissionModeOf({
        'permissions': {'mode': 'full'},
      }),
      'ask',
    );
    expect(storablePermissionModes.map((m) => m.value), [
      'ask',
      'auto_approve',
      'auto',
    ]);
    final patch = permissionsPatch(
      mode: 'auto_approve',
      rules: const [],
      fast: PermissionScorerDraft(),
      deep: PermissionScorerDraft(),
    );
    expect(patch['mode'], 'auto_approve');
  });
}
