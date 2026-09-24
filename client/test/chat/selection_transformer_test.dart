import 'package:agent_fabric_client/chat/selection_transformer.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

Offset _textOffsetToPosition(RenderParagraph paragraph, int offset) {
  const caret = Rect.fromLTWH(0.0, 0.0, 2.0, 20.0);
  final Offset localOffset = paragraph.getOffsetForCaret(
    TextPosition(offset: offset),
    caret,
  );
  return paragraph.localToGlobal(localOffset);
}

Future<void> _sendKeyCombination(
  WidgetTester tester,
  SingleActivator activator,
) async {
  final modifiers = <LogicalKeyboardKey>[
    if (activator.control) LogicalKeyboardKey.control,
    if (activator.shift) LogicalKeyboardKey.shift,
    if (activator.alt) LogicalKeyboardKey.alt,
    if (activator.meta) LogicalKeyboardKey.meta,
  ];
  for (final modifier in modifiers) {
    await tester.sendKeyDownEvent(modifier);
  }
  await tester.sendKeyDownEvent(activator.trigger);
  await tester.sendKeyUpEvent(activator.trigger);
  await tester.pump();
  for (final modifier in modifiers.reversed) {
    await tester.sendKeyUpEvent(modifier);
  }
}

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
      expect(joinSelectedPlainTexts(['a', 'b'], separator: '\n'), 'a\nb');
    });
  });

  testWidgets('SelectionTransformer.separated builds SelectionContainer', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: SelectionArea(
          child: SelectionTransformer.separated(child: const Text('hello')),
        ),
      ),
    );
    expect(find.byType(SelectionTransformer), findsOneWidget);
    expect(find.byType(SelectionContainer), findsWidgets);
    expect(find.text('hello'), findsOneWidget);
  });

  testWidgets(
    'SelectionTransformer.separated joins multi-text selection with double newline',
    (tester) async {
      SelectedContent? content;
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
          home: SelectionArea(
            onSelectionChanged: (SelectedContent? selected) =>
                content = selected,
            child: SelectionTransformer.separated(
              child: const Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[Text('a'), Text('b')],
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      final RenderParagraph paragraphA = tester.renderObject<RenderParagraph>(
        find.descendant(of: find.text('a'), matching: find.byType(RichText)),
      );
      // Focus region, then select-all (exercises delegate handleSelectAll +
      // getSelectedContent across both Text selectables).
      final TestGesture focus = await tester.startGesture(
        _textOffsetToPosition(paragraphA, 0),
        kind: PointerDeviceKind.mouse,
      );
      addTearDown(focus.removePointer);
      await tester.pump();
      await focus.up();
      await tester.pumpAndSettle();

      await _sendKeyCombination(
        tester,
        const SingleActivator(LogicalKeyboardKey.keyA, control: true),
      );
      await tester.pumpAndSettle();

      expect(content, isNotNull);
      expect(content!.plainText, 'a\n\nb');

      await _sendKeyCombination(
        tester,
        const SingleActivator(LogicalKeyboardKey.keyC, control: true),
      );
      await tester.pumpAndSettle();

      expect(copied, contains('a\n\nb'));
    },
  );
}
