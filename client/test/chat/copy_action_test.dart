import 'package:agent_fabric_client/chat/copy_action.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  tearDown(clearCopyToastForTest);

  testWidgets('copies text and shows top-right toast', (tester) async {
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
    await tester.pump();

    expect(copied, ['hello world']);
    expect(find.byKey(const Key('copy-toast')), findsOneWidget);
    expect(find.text('Copied'), findsOneWidget);
    expect(find.byType(SnackBar), findsNothing);

    final toast = tester.getTopLeft(find.byKey(const Key('copy-toast')));
    final size = tester.getSize(find.byType(MaterialApp));
    expect(toast.dx, greaterThan(size.width / 2));
    expect(toast.dy, lessThan(size.height / 2));

    clearCopyToastForTest();
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
    await tester.pump();

    expect(copied, isEmpty);
    expect(find.text('Copied'), findsNothing);
    expect(find.byKey(const Key('copy-toast')), findsNothing);
  });
}
