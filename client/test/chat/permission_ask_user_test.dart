import 'package:acpd/acpd.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:agent_fabric_client/chat/ask_user_prompt.dart';
import 'package:agent_fabric_client/chat/permission_prompt.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    theme: AppTheme.dark(),
    home: Scaffold(body: child),
  );
}

void main() {
  testWidgets('permission prompt shows warning chrome and options', (
    tester,
  ) async {
    late RequestPermissionResponse response;
    await tester.pumpWidget(
      _wrap(
        Builder(
          builder: (context) {
            return TextButton(
              onPressed: () async {
                response = await showPermissionPrompt(
                  context,
                  const RequestPermissionRequest(
                    sessionId: 's1',
                    toolCall: ToolCallUpdate(
                      toolCallId: 'c1',
                      title: 'Read file',
                      rawInput: {
                        'reason': 'path escapes workspace',
                        'path': '/tmp/x',
                      },
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
                  ),
                );
              },
              child: const Text('open'),
            );
          },
        ),
      ),
    );

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    expect(find.text('Permission required'), findsNothing);
    expect(find.text('Read file'), findsOneWidget);
    expect(find.textContaining('path escapes workspace'), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_rounded), findsOneWidget);

    await tester.tap(find.text('Allow once'));
    await tester.pumpAndSettle();
    expect(response.outcome, isA<PermissionSelected>());
    expect((response.outcome as PermissionSelected).optionId, 'allow_once');
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

  testWidgets('ask_user prompt is calm clarification UI', (tester) async {
    Map<String, Object?>? content;
    await tester.pumpWidget(
      _wrap(
        Builder(
          builder: (context) {
            return TextButton(
              onPressed: () async {
                content = await showAskUserPrompt(
                  context,
                  message: 'Please answer',
                  questions: const [
                    AskUserQuestion(
                      id: 'approach',
                      question: 'Which approach?',
                      options: ['Safe', 'Fast'],
                    ),
                  ],
                );
              },
              child: const Text('open'),
            );
          },
        ),
      ),
    );

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
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
