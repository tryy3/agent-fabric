import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:agent_fabric_client/chat/ask_user_prompt.dart';
import 'package:agent_fabric_client/chat/pending_interaction.dart';
import 'package:agent_fabric_client/chat/permission_prompt.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    theme: AppTheme.dark(),
    home: Scaffold(body: child),
  );
}

RequestPermissionRequest _permissionRequest() {
  return const RequestPermissionRequest(
    sessionId: 's1',
    toolCall: ToolCallUpdate(
      toolCallId: 'c1',
      title: 'Read file',
      rawInput: {'reason': 'path escapes workspace', 'path': '/tmp/x'},
    ),
    options: [
      PermissionOption(
        optionId: 'allow_once',
        name: 'Allow once',
        kind: PermissionOptionKind.allowOnce,
      ),
      PermissionOption(
        optionId: 'reject_once',
        name: 'Reject',
        kind: PermissionOptionKind.rejectOnce,
      ),
    ],
  );
}

void main() {
  testWidgets('permission dock shows warning chrome and options', (
    tester,
  ) async {
    String? selected;
    await tester.pumpWidget(
      _wrap(
        PermissionDock(
          pending: PendingPermission(
            threadId: 'th_1',
            request: _permissionRequest(),
            completer: Completer<RequestPermissionResponse>(),
          ),
          onSelect: (id) => selected = id,
        ),
      ),
    );

    expect(find.text('Permission required'), findsNothing);
    expect(find.text('Read file'), findsOneWidget);
    expect(find.textContaining('path escapes workspace'), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_rounded), findsOneWidget);

    await tester.tap(find.text('Allow once'));
    await tester.pumpAndSettle();
    expect(selected, 'allow_once');
  });

  test('parseAskUserQuestions reads meta questions', () {
    final questions = parseAskUserQuestions({
      'message': 'Pick one',
      '_meta': {
        'questions': [
          {
            'id': 'approach',
            'question': 'Which approach?',
            'options': [
              {'label': 'Safe'},
              {'label': 'Fast'},
            ],
          },
        ],
      },
    });
    expect(questions, hasLength(1));
    expect(questions.single.id, 'approach');
    expect(questions.single.options, ['Safe', 'Fast']);
  });

  testWidgets('ask_user dock is calm clarification UI', (tester) async {
    Map<String, Object?>? content;
    await tester.pumpWidget(
      _wrap(
        AskUserDock(
          pending: PendingAskUser(
            threadId: 'th_1',
            message: 'Please answer',
            questions: const [
              AskUserQuestion(
                id: 'approach',
                question: 'Which approach?',
                options: ['Safe', 'Fast'],
              ),
            ],
            completer: Completer<Map<String, Object?>>(),
          ),
          onSubmit: (value) => content = value,
          onSkip: () {},
        ),
      ),
    );

    expect(find.text('Please answer'), findsOneWidget);
    expect(find.text('Which approach?'), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_rounded), findsNothing);
    expect(find.text('Other'), findsOneWidget);

    await tester.tap(find.text('Fast'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Submit'));
    await tester.pumpAndSettle();
    expect(content?['approach'], 'Fast');
  });
}
