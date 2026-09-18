import 'package:agent_fabric_client/chat/message_text.dart';
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
}
