import 'package:agent_fabric_client/chat/selection_transformer.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  group('joinSelectedPlainTexts', () {
    test('joins with double newline by default', () {
      expect(
        joinSelectedPlainTexts(['H1 test', 'H2 Test']),
        'H1 test\n\nH2 Test',
      );
    });

    test('single fragment unchanged', () {
      expect(joinSelectedPlainTexts(['only']), 'only');
    });

    test('empty iterable yields empty string', () {
      expect(joinSelectedPlainTexts(const <String>[]), '');
    });

    test('custom separator', () {
      expect(
        joinSelectedPlainTexts(['a', 'b'], separator: '\n'),
        'a\nb',
      );
    });
  });

  testWidgets('SelectionTransformer.separated builds SelectionContainer', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: SelectionArea(
          child: SelectionTransformer.separated(
            child: const Text('hello'),
          ),
        ),
      ),
    );
    expect(find.byType(SelectionTransformer), findsOneWidget);
    expect(find.byType(SelectionContainer), findsWidgets);
    expect(find.text('hello'), findsOneWidget);
  });
}
