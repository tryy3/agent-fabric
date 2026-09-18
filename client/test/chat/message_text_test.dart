import 'package:agent_fabric_client/chat/message_text.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter/services.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('plain mode uses Text', (tester) async {
    await tester.pumpWidget(
      MaterialApp(home: MessageText(text: '**x**', markdown: false)),
    );
    expect(find.byType(Text), findsWidgets);
    expect(find.byType(MarkdownBody), findsNothing);
  });

  testWidgets('markdown mode uses MarkdownBody', (tester) async {
    await tester.pumpWidget(
      MaterialApp(home: MessageText(text: '**x**', markdown: true)),
    );
    expect(find.byType(MarkdownBody), findsOneWidget);
  });

  testWidgets('GFM task list renders checkboxes under dark theme', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: MessageText(
            markdown: true,
            text:
                '## Task List\n\n'
                '- [x] Completed task\n'
                '- [ ] Pending task\n'
                '- [ ] In progress\n',
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byType(Checkbox), findsNWidgets(3));
    final boxes = tester.widgetList<Checkbox>(find.byType(Checkbox)).toList();
    expect(boxes.where((c) => c.value == true), hasLength(1));
    expect(boxes.where((c) => c.value == false), hasLength(2));
    expect(find.textContaining('Completed task'), findsOneWidget);
  });

  testWidgets('tapping link shows dialog with URL, Open and Copy', (
    tester,
  ) async {
    Uri? launched;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: MessageText(
            markdown: true,
            text: '[docs](https://example.com/path)',
            launchLink: (uri) async {
              launched = uri;
              return true;
            },
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('docs'));
    await tester.pumpAndSettle();

    expect(find.text('Open link?'), findsOneWidget);
    expect(find.text('https://example.com/path'), findsOneWidget);
    expect(find.text('Open'), findsOneWidget);
    expect(find.text('Copy'), findsOneWidget);

    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    expect(launched, Uri.parse('https://example.com/path'));
    expect(find.text('Open link?'), findsNothing);
  });

  testWidgets('link dialog warns on mismatched text and copies URL', (
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
        home: Scaffold(
          body: MessageText(
            markdown: true,
            text: '[https://openai.com](https://evil.example/x)',
            launchLink: (_) async => true,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('https://openai.com'));
    await tester.pumpAndSettle();

    expect(find.textContaining('openai.com'), findsWidgets);
    expect(find.textContaining('evil.example'), findsWidgets);

    await tester.tap(find.text('Copy'));
    await tester.pumpAndSettle();
    expect(copied, contains('https://evil.example/x'));
    expect(find.text('Link copied'), findsOneWidget);
  });
}
