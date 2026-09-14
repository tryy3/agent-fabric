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
    const alignSlop = 2.0;
    expect(
      tester.getTopLeft(find.text('Thinking')).dx,
      closeTo(tester.getTopLeft(find.text('hello')).dx, alignSlop),
    );
    expect(
      tester.getTopLeft(find.text('hmm')).dx,
      closeTo(tester.getTopLeft(find.text('hello')).dx, alignSlop),
    );
    await tester.tap(find.text('Stats'));
    await tester.pumpAndSettle();
    expect(
      tester.getTopLeft(find.text('Stats')).dx,
      closeTo(tester.getTopLeft(find.text('hello')).dx, alignSlop),
    );
    expect(
      tester.getTopLeft(find.text('elapsedMs: 50')).dx,
      closeTo(tester.getTopLeft(find.text('hello')).dx, alignSlop),
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

  testWidgets('agent cards use stronger fills and a teal answer', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              const AgentBubble(
                bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 'hmm'),
              ),
              const AgentBubble(
                bubble: ChatBubble(kind: ChatBubbleKind.message, text: 'hello'),
              ),
              AgentBubble(
                bubble: ChatBubble(
                  kind: ChatBubbleKind.stats,
                  usage: const TurnUsage(elapsedMs: 50),
                  stopReason: 'end_turn',
                ),
              ),
            ],
          ),
        ),
      ),
    );

    BoxDecoration cardOf(Finder of) {
      return tester
              .widget<Container>(
                find
                    .ancestor(
                      of: of,
                      matching: find.byWidgetPredicate((widget) {
                        return widget is Container &&
                            widget.decoration is BoxDecoration;
                      }),
                    )
                    .first,
              )
              .decoration!
          as BoxDecoration;
    }

    final thought = cardOf(find.text('Thinking'));
    expect(thought.color, const Color(0xFFFEF3C7));
    expect(
      thought.border,
      const Border(left: BorderSide(color: Color(0xFFD97706), width: 4)),
    );

    final answer = cardOf(find.text('hello'));
    expect(answer.color, const Color(0xFFCCFBF1));
    expect(
      answer.border,
      const Border(left: BorderSide(color: Color(0xFF0F766E), width: 4)),
    );

    final stats = cardOf(find.text('Stats'));
    expect(stats.color, const Color(0xFFE4E4E7));
    expect(
      stats.border,
      const Border(left: BorderSide(color: Color(0xFF71717A), width: 4)),
    );
  });

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
