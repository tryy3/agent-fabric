import 'package:agent_fabric_client/chat/mermaid_block.dart';
import 'package:agent_fabric_client/chat/message_text.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

Future<void> _pump(
  WidgetTester tester,
  String text, {
  ThemeData? theme,
  double width = 400,
}) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: theme,
      home: Scaffold(
        body: SizedBox(
          width: width,
          child: MessageText(text: text, markdown: true),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

const _flowchart = 'graph TD\n  A[Start] --> B{Works?}\n  B -->|yes| C[Ship]\n';

void main() {
  testWidgets('closed mermaid fence renders a diagram, not code text', (
    tester,
  ) async {
    await _pump(tester, 'Intro\n\n```mermaid\n$_flowchart```\n\nOutro');

    expect(find.byType(MermaidBlock), findsOneWidget);
    expect(find.textContaining('graph TD'), findsNothing);
    expect(find.text('Intro'), findsOneWidget);
    expect(find.text('Outro'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('exposes a semantics label and the source', (tester) async {
    final handle = tester.ensureSemantics();
    await _pump(tester, '```mermaid\n$_flowchart```');

    expect(
      tester.getSemantics(find.byType(MermaidBlock)),
      matchesSemantics(label: 'Mermaid diagram', value: _flowchart.trimRight()),
    );
    handle.dispose();
  });

  for (final entry in {
    'sequence': 'sequenceDiagram\n  Alice->>Bob: Hi\n  Bob-->>Alice: Hello\n',
    'class': 'classDiagram\n  Animal <|-- Duck\n  Animal : +int age\n',
    'pie': 'pie title Pets\n  "Dogs" : 5\n  "Cats" : 3\n',
  }.entries) {
    testWidgets('${entry.key} diagram renders in light and dark', (
      tester,
    ) async {
      for (final theme in [AppTheme.light(), AppTheme.dark()]) {
        await _pump(tester, '```mermaid\n${entry.value}```', theme: theme);
        expect(find.byType(MermaidBlock), findsOneWidget);
        expect(tester.takeException(), isNull);
      }
    });
  }

  testWidgets('invalid source falls back to the code text with a notice', (
    tester,
  ) async {
    await _pump(tester, '```mermaid\nthis is not a diagram @@@\n```');

    expect(find.byType(MermaidBlock), findsNothing);
    expect(find.text('this is not a diagram @@@'), findsOneWidget);
    expect(find.text('Diagram could not be rendered'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('non-mermaid fence is an unchanged code block', (tester) async {
    await _pump(tester, '```dart\nvoid main() {}\n```');

    expect(find.byType(MermaidBlock), findsNothing);
    expect(find.text('void main() {}'), findsOneWidget);
    expect(find.text('Diagram could not be rendered'), findsNothing);
  });

  testWidgets('unterminated fence stays plain until it closes', (tester) async {
    await _pump(tester, '```mermaid\n$_flowchart');

    expect(find.byType(MermaidBlock), findsNothing);
    expect(find.textContaining('graph TD'), findsOneWidget);
    expect(find.text('Diagram could not be rendered'), findsNothing);

    await _pump(tester, '```mermaid\n$_flowchart```');
    expect(find.byType(MermaidBlock), findsOneWidget);
  });

  testWidgets('wide diagram scrolls instead of overflowing', (tester) async {
    await _pump(
      tester,
      '```mermaid\ngraph LR\n'
      '  A[Alpha node] --> B[Bravo node] --> C[Charlie node] --> '
      'D[Delta node] --> E[Echo node] --> F[Foxtrot node]\n```',
      width: 200,
    );

    expect(find.byType(MermaidBlock), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
