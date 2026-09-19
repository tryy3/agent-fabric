import 'package:agent_fabric_client/chat/message_timestamp.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('formatMessageTimestamp joins locale date and time', (
    tester,
  ) async {
    late String formatted;
    await tester.pumpWidget(
      MaterialApp(
        locale: const Locale('en', 'US'),
        home: Builder(
          builder: (context) {
            formatted = formatMessageTimestamp(
              context,
              DateTime(2026, 9, 9, 10, 40),
            );
            return const SizedBox.shrink();
          },
        ),
      ),
    );
    expect(formatted, isNotEmpty);
    expect(formatted.toLowerCase(), contains('sep'));
    expect(formatted, contains('10:40'));
  });
}
