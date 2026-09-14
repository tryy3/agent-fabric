import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('thought is collapsed; caption sits on the answer', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              AgentBubble(
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.thought,
                  text: 'hmm',
                ),
              ),
              AgentBubble(
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.message,
                  text: 'hello',
                  model: 'm1',
                  providerName: 'Local',
                  predictedPerSecond: 35.5,
                ),
              ),
              AgentBubble(
                bubble: ChatBubble(
                  kind: ChatBubbleKind.stats,
                  usage: const TurnUsage(
                    predictedPerSecond: 35.5,
                    deltas: 1,
                    elapsedMs: 50,
                    stopReason: 'end_turn',
                  ),
                  stopReason: 'end_turn',
                ),
              ),
            ],
          ),
        ),
      ),
    );
    expect(find.text('hello'), findsOneWidget);
    expect(find.textContaining('m1'), findsOneWidget);
    expect(find.textContaining('Local'), findsOneWidget);
    expect(find.textContaining('35.5'), findsOneWidget);
    expect(find.text('hmm'), findsNothing);
    expect(find.text('Answer'), findsNothing);
    await tester.tap(find.text('Thinking'));
    await tester.pumpAndSettle();
    expect(find.text('hmm'), findsOneWidget);
    expect(
      tester.getTopLeft(find.text('Thinking')).dy,
      lessThan(tester.getTopLeft(find.text('hello')).dy),
    );
  });

  testWidgets('hidden thinking omits the thought bubble; caption remains', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              AgentBubble(
                thinkingMode: VisibilityMode.hidden,
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.thought,
                  text: 'hmm',
                ),
              ),
              AgentBubble(
                statsMode: VisibilityMode.hidden,
                bubble: const ChatBubble(
                  kind: ChatBubbleKind.message,
                  text: 'hello',
                  model: 'm1',
                  providerName: 'Local',
                  predictedPerSecond: 35.5,
                ),
              ),
            ],
          ),
        ),
      ),
    );
    expect(find.text('Thinking'), findsNothing);
    expect(find.text('Stats'), findsNothing);
    expect(find.text('hello'), findsOneWidget);
    expect(find.textContaining('35.5'), findsOneWidget);
  });

  testWidgets(
    'thinking key stable across text while streamingThought stays true',
    (tester) async {
      Future<void> pump({
        required String text,
        required bool streamingThought,
        VisibilityMode mode = VisibilityMode.collapsed,
      }) {
        return tester.pumpWidget(
          MaterialApp(
            home: Scaffold(
              body: AgentBubble(
                thinkingMode: mode,
                bubble: ChatBubble(
                  kind: ChatBubbleKind.thought,
                  text: text,
                  streamingThought: streamingThought,
                ),
              ),
            ),
          ),
        );
      }

      await pump(text: 'hmm', streamingThought: true);
      final first = tester
          .widget<ExpansionTile>(find.widgetWithText(ExpansionTile, 'Thinking'))
          .key;
      await pump(text: 'hmm more', streamingThought: true);
      expect(
        tester
            .widget<ExpansionTile>(
              find.widgetWithText(ExpansionTile, 'Thinking'),
            )
            .key,
        first,
      );
      await pump(
        text: 'hmm more',
        streamingThought: true,
        mode: VisibilityMode.expanded,
      );
      expect(
        tester
            .widget<ExpansionTile>(
              find.widgetWithText(ExpansionTile, 'Thinking'),
            )
            .key,
        isNot(first),
      );
    },
  );

  testWidgets('stats omitted without usage or stopReason', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: AgentBubble(bubble: ChatBubble(kind: ChatBubbleKind.stats)),
        ),
      ),
    );
    expect(find.text('Stats'), findsNothing);
  });
}
