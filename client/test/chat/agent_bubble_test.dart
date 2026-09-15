import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('thought collapsed shows title + description; expands to body', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            bubble: ChatBubble(
              kind: ChatBubbleKind.thought,
              text: 'hmm\nmore detail',
            ),
          ),
        ),
      ),
    );
    expect(find.text('Thinking'), findsOneWidget);
    expect(find.text('hmm'), findsOneWidget);
    expect(find.text('hmm\nmore detail'), findsNothing);
    expect(find.text('more detail'), findsNothing);
    await tester.tap(find.byKey(const Key('activity-thinking')));
    await tester.pumpAndSettle();
    expect(find.text('hmm\nmore detail'), findsOneWidget);
    expect(find.textContaining('more detail'), findsOneWidget);
  });

  testWidgets('single-line thought expanded shows full body', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            thinkingMode: VisibilityMode.expanded,
            bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 'hmm'),
          ),
        ),
      ),
    );
    expect(find.text('hmm'), findsNWidgets(2));
  });

  testWidgets('hidden thinking omits the row', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            thinkingMode: VisibilityMode.hidden,
            bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 'hmm'),
          ),
        ),
      ),
    );
    expect(find.text('Thinking'), findsNothing);
  });

  testWidgets('message is plain prose with caption; Stats opens popover', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            bubble: const ChatBubble(
              kind: ChatBubbleKind.message,
              text: 'hello',
              model: 'm1',
              providerName: 'Local',
              predictedPerSecond: 35.5,
            ),
            stats: ChatBubble(
              kind: ChatBubbleKind.stats,
              usage: const TurnUsage(elapsedMs: 50, deltas: 1),
              stopReason: 'end_turn',
            ),
          ),
        ),
      ),
    );
    expect(find.text('hello'), findsOneWidget);
    expect(find.textContaining('m1'), findsOneWidget);
    expect(find.byKey(const Key('stats-action')), findsOneWidget);
    expect(find.text('Stats'), findsOneWidget);
    expect(find.byIcon(Icons.bar_chart), findsOneWidget);
    final statsMaterial = tester.widget<Material>(
      find
          .ancestor(
            of: find.byKey(const Key('stats-action')),
            matching: find.byWidgetPredicate(
              (widget) =>
                  widget is Material &&
                  widget.color == ChatColors.light().stats.fill,
            ),
          )
          .first,
    );
    expect(statsMaterial.color, ChatColors.light().stats.fill);
    await tester.tap(find.byKey(const Key('stats-action')));
    await tester.pumpAndSettle();
    expect(find.text('elapsedMs: 50'), findsOneWidget);
    expect(find.text('stopReason: end_turn'), findsOneWidget);
  });

  testWidgets('Stats action omitted without usage/stopReason', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            bubble: ChatBubble(kind: ChatBubbleKind.message, text: 'hello'),
          ),
        ),
      ),
    );
    expect(find.byKey(const Key('stats-action')), findsNothing);
  });

  testWidgets('stats kind widget is empty', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            bubble: ChatBubble(
              kind: ChatBubbleKind.stats,
              usage: const TurnUsage(elapsedMs: 1),
            ),
          ),
        ),
      ),
    );
    expect(find.text('Stats'), findsNothing);
    expect(find.byKey(const Key('stats-action')), findsNothing);
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
          .widget(
            find.byKey(
              const ValueKey('thinking-true-VisibilityMode.collapsed'),
            ),
          )
          .key;
      await pump(text: 'hmm more', streamingThought: true);
      expect(
        tester
            .widget(
              find.byKey(
                const ValueKey('thinking-true-VisibilityMode.collapsed'),
              ),
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
            .widget(
              find.byKey(
                const ValueKey('thinking-true-VisibilityMode.expanded'),
              ),
            )
            .key,
        isNot(first),
      );
    },
  );

  testWidgets('expanded thinking colors header and body together', (
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
                  widget.decoration is BoxDecoration &&
                  (widget.decoration! as BoxDecoration).color ==
                      const Color(0xFFABCDEF);
            }),
          )
          .first,
    );
    expect((box.decoration! as BoxDecoration).color, const Color(0xFFABCDEF));
    expect(find.text('t'), findsNWidgets(2));
  });
}
