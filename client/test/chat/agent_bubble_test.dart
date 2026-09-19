import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/tool_format.dart';
import 'package:agent_fabric_client/chat/view_modes.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:agent_fabric_client/ui/theme/chat_colors.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:super_sliver_list/super_sliver_list.dart';

const hiddenTools = ViewMode(
  id: 't',
  label: 't',
  description: '',
  markdownRender: false,
  thinkingVisibility: VisibilityMode.collapsed,
  toolVisibility: VisibilityMode.hidden,
  toolIO: ToolIOMode.both,
);

const expandedThinking = ViewMode(
  id: 'expanded-thinking',
  label: 'Expanded thinking',
  description: '',
  markdownRender: false,
  thinkingVisibility: VisibilityMode.expanded,
  toolVisibility: VisibilityMode.collapsed,
  toolIO: ToolIOMode.both,
);

void main() {
  testWidgets('thought collapsed shows title + description; expands to body', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            viewMode: ViewMode(
              id: 'pretty',
              label: 'Pretty',
              description: '',
              markdownRender: true,
              thinkingVisibility: VisibilityMode.collapsed,
              toolVisibility: VisibilityMode.collapsed,
              toolIO: ToolIOMode.both,
            ),
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
            viewMode: expandedThinking,
            bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 'hmm'),
          ),
        ),
      ),
    );
    expect(find.text('hmm'), findsNWidgets(2));
  });

  testWidgets('tool call expands to Full/Output tabs; defaults to Full', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode('detailed'),
            bubble: const ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: 'call_1',
              toolTitle: 'Read file',
              toolStatus: 'completed',
              toolInput: {'path': 'notes.txt'},
              toolOutput: {'content': 'hello'},
            ),
          ),
        ),
      ),
    );

    expect(find.text('Read file'), findsOneWidget);
    await tester.tap(find.byKey(const Key('activity-tool-call_1')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('tool-tab-full')), findsOneWidget);
    expect(find.byKey(const Key('tool-tab-output')), findsOneWidget);
    // Default Full: args and output both visible as labeled sections.
    expect(find.text('Args'), findsOneWidget);
    expect(find.text('Output'), findsWidgets); // section label and/or tab
    expect(find.textContaining('"path": "notes.txt"'), findsOneWidget);
    expect(find.textContaining('"content": "hello"'), findsOneWidget);

    await tester.tap(find.byKey(const Key('tool-tab-output')));
    await tester.pumpAndSettle();
    expect(find.textContaining('"content": "hello"'), findsOneWidget);
    expect(find.textContaining('"path": "notes.txt"'), findsNothing);
  });

  testWidgets('hidden tool call omits the row', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            viewMode: hiddenTools,
            bubble: ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: 'call_1',
              toolTitle: 'Read file',
            ),
          ),
        ),
      ),
    );
    expect(find.text('Read file'), findsNothing);
    expect(find.byKey(const Key('activity-tool-call_1')), findsNothing);
  });

  testWidgets('hidden thinking omits the row', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            viewMode: ViewMode(
              id: 'hidden-thinking',
              label: 'Hidden thinking',
              description: '',
              markdownRender: false,
              thinkingVisibility: VisibilityMode.hidden,
              toolVisibility: VisibilityMode.collapsed,
              toolIO: ToolIOMode.both,
            ),
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
            viewMode: resolveViewMode('detailed'),
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
    expect(find.text('Elapsed time'), findsOneWidget);
    expect(find.text('50'), findsOneWidget);
    expect(find.text('Stop reason'), findsOneWidget);
    expect(find.text('end_turn'), findsOneWidget);
    await tester.tap(find.byKey(const Key('stats-tab-raw')));
    await tester.pumpAndSettle();
    expect(find.textContaining('"elapsedMs": 50'), findsOneWidget);
    expect(find.textContaining('"stopReason": "end_turn"'), findsOneWidget);
  });

  testWidgets('Stats action omitted without usage/stopReason', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode(null),
            bubble: const ChatBubble(
              kind: ChatBubbleKind.message,
              text: 'hello',
            ),
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
            viewMode: resolveViewMode(null),
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
        ViewMode mode = const ViewMode(
          id: 'pretty',
          label: 'Pretty',
          description: '',
          markdownRender: true,
          thinkingVisibility: VisibilityMode.collapsed,
          toolVisibility: VisibilityMode.collapsed,
          toolIO: ToolIOMode.both,
        ),
      }) {
        return tester.pumpWidget(
          MaterialApp(
            theme: AppTheme.light(),
            home: Scaffold(
              body: AgentBubble(
                viewMode: mode,
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
        mode: expandedThinking,
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
            viewMode: expandedThinking,
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

  testWidgets('expanded thinking survives scroll offscreen', (tester) async {
    final scroll = ScrollController();
    addTearDown(scroll.dispose);
    tester.view.physicalSize = const Size(400, 500);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: SuperListView.builder(
            controller: scroll,
            itemCount: 12,
            itemBuilder: (context, index) {
              if (index == 0) {
                return const AgentBubble(
                  viewMode: ViewMode(
                    id: 'pretty',
                    label: 'Pretty',
                    description: '',
                    markdownRender: true,
                    thinkingVisibility: VisibilityMode.collapsed,
                    toolVisibility: VisibilityMode.collapsed,
                    toolIO: ToolIOMode.both,
                  ),
                  bubble: ChatBubble(
                    kind: ChatBubbleKind.thought,
                    text: 'hmm\nmore detail',
                  ),
                );
              }
              return SizedBox(height: 180, child: Text('pad-$index'));
            },
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('activity-thinking')));
    await tester.pumpAndSettle();
    expect(find.text('hmm\nmore detail'), findsOneWidget);

    scroll.jumpTo(scroll.position.maxScrollExtent);
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('activity-thinking')), findsNothing);

    scroll.jumpTo(0);
    await tester.pumpAndSettle();
    expect(find.text('hmm\nmore detail'), findsOneWidget);
  });

  testWidgets('expanded tool survives scroll offscreen', (tester) async {
    final scroll = ScrollController();
    addTearDown(scroll.dispose);
    tester.view.physicalSize = const Size(400, 500);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: SuperListView.builder(
            controller: scroll,
            itemCount: 12,
            itemBuilder: (context, index) {
              if (index == 0) {
                return AgentBubble(
                  viewMode: resolveViewMode('detailed'),
                  bubble: const ChatBubble(
                    kind: ChatBubbleKind.toolCall,
                    toolCallId: 'call_keep',
                    toolTitle: 'Read file',
                    toolStatus: 'completed',
                    toolInput: {'path': 'notes.txt'},
                    toolOutput: {'content': 'hello-keep'},
                  ),
                );
              }
              return SizedBox(height: 180, child: Text('pad-$index'));
            },
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('activity-tool-call_keep')));
    await tester.pumpAndSettle();
    expect(find.textContaining('hello-keep'), findsOneWidget);

    scroll.jumpTo(scroll.position.maxScrollExtent);
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('activity-tool-call_keep')), findsNothing);

    scroll.jumpTo(0);
    await tester.pumpAndSettle();
    expect(find.textContaining('hello-keep'), findsOneWidget);
  });

  testWidgets('thinking copy copies full text without expanding', (
    tester,
  ) async {
    final copied = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>?;
          copied.add(args?['text'] as String? ?? '');
        }
        return null;
      },
    );
    addTearDown(() {
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      );
    });

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            viewMode: ViewMode(
              id: 'pretty',
              label: 'Pretty',
              description: '',
              markdownRender: true,
              thinkingVisibility: VisibilityMode.collapsed,
              toolVisibility: VisibilityMode.collapsed,
              toolIO: ToolIOMode.both,
            ),
            bubble: ChatBubble(
              kind: ChatBubbleKind.thought,
              text: 'hmm\nmore detail',
            ),
          ),
        ),
      ),
    );

    expect(find.text('hmm\nmore detail'), findsNothing);
    await tester.tap(find.byKey(const Key('copy-thinking')));
    await tester.pumpAndSettle();
    expect(copied, ['hmm\nmore detail']);
    expect(find.text('hmm\nmore detail'), findsNothing); // still collapsed
  });

  testWidgets('tool copy dumps structured text regardless of tab', (
    tester,
  ) async {
    final copied = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>?;
          copied.add(args?['text'] as String? ?? '');
        }
        return null;
      },
    );
    addTearDown(() {
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      );
    });

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode('detailed'),
            bubble: const ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: 'call_1',
              toolTitle: 'skill_view',
              toolInput: {'name': 'bike-maintenance'},
              toolOutput: {'success': true},
            ),
          ),
        ),
      ),
    );

    await tester.tap(find.byKey(const Key('activity-tool-call_1')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('tool-tab-output')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('copy-tool-call_1')));
    await tester.pumpAndSettle();

    expect(
      copied.single,
      formatToolCopyText(
        title: 'skill_view',
        input: {'name': 'bike-maintenance'},
        output: {'success': true},
      ),
    );
  });

  testWidgets('message copy copies source text next to caption', (
    tester,
  ) async {
    final copied = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>?;
          copied.add(args?['text'] as String? ?? '');
        }
        return null;
      },
    );
    addTearDown(() {
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      );
    });

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode('detailed'),
            bubble: const ChatBubble(
              kind: ChatBubbleKind.message,
              text: 'hello answer',
              model: 'm1',
            ),
          ),
        ),
      ),
    );

    expect(find.byKey(const Key('copy-message')), findsOneWidget);
    await tester.tap(find.byKey(const Key('copy-message')));
    await tester.pumpAndSettle();
    expect(copied, ['hello answer']);
  });

  testWidgets('message footer shows locale timestamp next to copy', (
    tester,
  ) async {
    final when = DateTime(2026, 9, 9, 10, 40);
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        locale: const Locale('en', 'US'),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode('detailed'),
            bubble: ChatBubble(
              kind: ChatBubbleKind.message,
              text: 'hello answer',
              model: 'm1',
              createdAt: when,
            ),
          ),
        ),
      ),
    );
    expect(find.byKey(const Key('copy-message')), findsOneWidget);
    expect(find.textContaining('Sep'), findsOneWidget);
    expect(find.textContaining('10:40'), findsOneWidget);
  });

  testWidgets('message footer omits timestamp when createdAt is null', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        locale: const Locale('en', 'US'),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode('detailed'),
            bubble: const ChatBubble(
              kind: ChatBubbleKind.message,
              text: 'hello answer',
              model: 'm1',
            ),
          ),
        ),
      ),
    );
    expect(find.byKey(const Key('copy-message')), findsOneWidget);
    expect(find.textContaining('Sep'), findsNothing);
    expect(find.textContaining('10:40'), findsNothing);
  });

  testWidgets(
    'tool completed uses stats chrome; failed tints icon+status amber',
    (tester) async {
      final light = ChatColors.light();
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: Scaffold(
            body: AgentBubble(
              viewMode: resolveViewMode('detailed'),
              bubble: const ChatBubble(
                kind: ChatBubbleKind.toolCall,
                toolCallId: 'ok',
                toolTitle: 'read',
                toolStatus: 'completed',
                streamingTool: false,
              ),
            ),
          ),
        ),
      );
      final completedIcon = tester.widget<Icon>(
        find.byIcon(Icons.build_outlined),
      );
      expect(completedIcon.color, light.stats.bar);

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.light(),
          home: Scaffold(
            body: AgentBubble(
              viewMode: resolveViewMode('detailed'),
              bubble: const ChatBubble(
                kind: ChatBubbleKind.toolCall,
                toolCallId: 'bad',
                toolTitle: 'bash',
                toolStatus: 'failed',
                streamingTool: false,
              ),
            ),
          ),
        ),
      );
      final failedIcon = tester.widget<Icon>(find.byIcon(Icons.build_outlined));
      expect(failedIcon.color, const Color(0xFFD97706));
      final status = tester.widget<Text>(find.text('failed'));
      expect(status.style?.color, const Color(0xFFD97706));

      await tester.tap(find.byKey(const Key('activity-tool-bad')));
      await tester.pumpAndSettle();
      final fill = tester
          .widgetList<Container>(find.byType(Container))
          .map((c) => c.decoration)
          .whereType<BoxDecoration>()
          .firstWhere(
            (d) => d.color == light.stats.fill,
            orElse: () => const BoxDecoration(),
          );
      expect(fill.color, light.stats.fill);
    },
  );

  testWidgets('tool pending uses primary icon color', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            viewMode: resolveViewMode('detailed'),
            bubble: const ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: 'run',
              toolTitle: 'search',
              toolStatus: 'pending',
              streamingTool: true,
            ),
          ),
        ),
      ),
    );
    final icon = tester.widget<Icon>(find.byIcon(Icons.build_outlined));
    expect(icon.color, AppTheme.light().colorScheme.primary);
  });

  testWidgets('thinking icon uses cyan thinking.bar', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: const Scaffold(
          body: AgentBubble(
            viewMode: ViewMode(
              id: 'pretty',
              label: 'Pretty',
              description: '',
              markdownRender: true,
              thinkingVisibility: VisibilityMode.collapsed,
              toolVisibility: VisibilityMode.collapsed,
              toolIO: ToolIOMode.both,
            ),
            bubble: ChatBubble(kind: ChatBubbleKind.thought, text: 'hmm'),
          ),
        ),
      ),
    );
    final icon = tester.widget<Icon>(find.byIcon(Icons.lightbulb_outline));
    expect(icon.color, ChatColors.light().thinking.bar);
    expect(icon.color, const Color(0xFF0891B2));
  });
}
