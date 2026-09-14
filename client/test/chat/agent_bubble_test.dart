import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('thought is collapsed; caption sits on the answer', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
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
        theme: AppTheme.light(),
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
            theme: AppTheme.light(),
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
    final colors = ChatColors.light();
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
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
    expect(thought.color, colors.thinking.fill);
    expect(
      thought.border,
      Border(left: BorderSide(color: colors.thinking.bar, width: 4)),
    );

    final answer = cardOf(find.text('hello'));
    expect(answer.color, colors.answer.fill);
    expect(
      answer.border,
      Border(left: BorderSide(color: colors.answer.bar, width: 4)),
    );

    final stats = cardOf(find.text('Stats'));
    expect(stats.color, colors.stats.fill);
    expect(
      stats.border,
      Border(left: BorderSide(color: colors.stats.bar, width: 4)),
    );
  });

  testWidgets('AgentBubble uses ChatColors from theme extension', (
    tester,
  ) async {
    final custom = ChatColors.light().withOverride(
      ChatColorRole.thinking,
      fill: const Color(0xFFABCDEF),
      bar: const Color(0xFF123456),
    );
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(chatColors: custom),
        home: const Scaffold(
          body: AgentBubble(
            bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 't'),
            thinkingMode: VisibilityMode.expanded,
          ),
        ),
      ),
    );

    final box = tester.widget<Container>(
      find
          .ancestor(
            of: find.text('Thinking'),
            matching: find.byWidgetPredicate((widget) {
              return widget is Container &&
                  widget.decoration is BoxDecoration;
            }),
          )
          .first,
    );
    final deco = box.decoration! as BoxDecoration;
    expect(deco.color, const Color(0xFFABCDEF));
    expect(
      deco.border,
      const Border(left: BorderSide(color: Color(0xFF123456), width: 4)),
    );
  });

  testWidgets('stats omitted without usage or stopReason', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(bubble: ChatBubble(kind: ChatBubbleKind.stats)),
        ),
      ),
    );
    expect(find.text('Stats'), findsNothing);
  });
}
