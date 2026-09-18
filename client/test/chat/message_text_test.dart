import 'package:agent_fabric_client/chat/message_text.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
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
}
