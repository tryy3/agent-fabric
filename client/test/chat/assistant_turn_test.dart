import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/assistant_turn.dart';
import 'package:agent_fabric_client/chat/chat_message.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('assistant turn shows caption and collapsed thinking', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: AssistantTurnTile(
            message: ChatMessage(
              role: ChatRole.assistant,
              text: 'hello',
              thought: 'hmm',
              model: 'm1',
              providerName: 'Local',
              usage: TurnUsage(
                predictedPerSecond: 35.5,
                deltas: 1,
                elapsedMs: 50,
                stopReason: 'end_turn',
              ),
            ),
          ),
        ),
      ),
    );
    expect(find.text('hello'), findsOneWidget);
    expect(find.textContaining('m1'), findsOneWidget);
    expect(find.textContaining('Local'), findsOneWidget);
    expect(find.textContaining('35.5'), findsOneWidget); // tok/s in caption
    expect(find.text('hmm'), findsNothing); // collapsed
    await tester.tap(find.text('Thinking'));
    await tester.pumpAndSettle();
    expect(find.text('hmm'), findsOneWidget);
  });
}
