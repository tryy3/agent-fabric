import 'package:agent_fabric_client/catalog/permission_tiers.dart';
import 'package:agent_fabric_client/settings/permissions_tab.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

const _forbidden = PermissionTier(
  id: 'command.forbidden',
  title: 'Forbidden programs',
  description: 'Listed programs are refused.',
  action: 'deny',
  risk: 10,
  consult: false,
  programs: ['docker', 'sudo'],
);

const _network = PermissionTier(
  id: 'command.network',
  title: 'Network commands',
  description: 'Programs that talk to other hosts.',
  action: 'ask',
  risk: 6,
  consult: false,
  programs: ['curl', 'wget'],
);

void main() {
  test('tier draft tracks overrides against the defaults', () {
    final draft = PermissionTierDraft(_network);
    expect(draft.isModified, isFalse);
    expect(draft.toJson(), isNull);

    draft.setRisk(4);
    draft.setConsult(value: true);
    expect(draft.addProgram('httpie'), isTrue);
    expect(draft.addProgram('/usr/bin/nc'), isFalse);
    expect(draft.addProgram('two words'), isFalse);
    draft.removeProgram('wget');
    expect(draft.programs, ['curl', 'httpie']);
    expect(draft.toJson(), {
      'risk': 4,
      'consult': true,
      'add': ['httpie'],
      'remove': ['wget'],
    });

    // Undoing each change returns to the default with no override left.
    draft.setRisk(6);
    draft.setConsult(value: false);
    draft.removeProgram('httpie');
    expect(draft.addProgram('wget'), isTrue);
    expect(draft.isModified, isFalse);
    expect(draft.programs, ['curl', 'wget']);
  });

  test('tier draft loads a stored override', () {
    final draft = PermissionTierDraft(_forbidden, {
      'add': ['terraform'],
      'remove': ['docker'],
    });
    expect(draft.programs, ['sudo', 'terraform']);
    expect(draft.isModified, isTrue);
    draft.reset();
    expect(draft.programs, ['docker', 'sudo']);
  });

  testWidgets('tile shows programs and edits them', (tester) async {
    final draft = PermissionTierDraft(_forbidden);
    var changes = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: BuiltinTierTile(draft: draft, onChanged: () => changes++),
          ),
        ),
      ),
    );
    expect(find.text('refuses · risk 10'), findsOneWidget);
    await tester.tap(find.text('Forbidden programs'));
    await tester.pumpAndSettle();
    expect(find.text('sudo'), findsOneWidget);
    // A refusing tier has no score or scorer controls.
    expect(
      find.byKey(const Key('builtin-risk-command.forbidden')),
      findsNothing,
    );

    await tester.enterText(
      find.byKey(const Key('builtin-add-program-command.forbidden')),
      'terraform',
    );
    await tester.tap(
      find.byKey(const Key('builtin-add-program-button-command.forbidden')),
    );
    await tester.pumpAndSettle();
    expect(draft.programs, contains('terraform'));
    expect(find.text('refuses · risk 10 · modified'), findsOneWidget);
    expect(changes, 1);

    await tester.tap(find.byKey(const Key('builtin-reset-command.forbidden')));
    await tester.pumpAndSettle();
    expect(draft.isModified, isFalse);
  });
}
