import 'package:agent_fabric_client/chat/copy_action.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('copies text and shows snackbar', (tester) async {
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
          body: CopyAction(
            key: const Key('copy-test'),
            text: 'hello world',
            snackbarMessage: 'Copied',
          ),
        ),
      ),
    );

    await tester.tap(find.byKey(const Key('copy-test')));
    await tester.pumpAndSettle();

    expect(copied, ['hello world']);
    expect(find.text('Copied'), findsOneWidget);
    final bar = tester.widget<SnackBar>(find.byType(SnackBar));
    expect(bar.behavior, SnackBarBehavior.floating);
  });

  testWidgets('empty text is a no-op', (tester) async {
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
      const MaterialApp(
        home: Scaffold(
          body: CopyAction(key: Key('copy-empty'), text: ''),
        ),
      ),
    );

    final button = tester.widget<IconButton>(find.byType(IconButton));
    expect(button.onPressed, isNotNull);

    await tester.tap(find.byKey(const Key('copy-empty')));
    await tester.pumpAndSettle();

    expect(copied, isEmpty);
    expect(find.text('Copied'), findsNothing);
  });
}
