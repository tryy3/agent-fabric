import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/ui/connectivity_badge.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('shows Offline for disconnected', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: ConnectivityBadge(status: ChatStatus.disconnected),
        ),
      ),
    );

    expect(find.text('Offline'), findsOneWidget);
    expect(find.text('Retry'), findsNothing);
  });

  testWidgets('shows Reconnecting... while reconnecting', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: ConnectivityBadge(status: ChatStatus.reconnecting),
        ),
      ),
    );

    expect(find.text('Reconnecting...'), findsOneWidget);
  });

  testWidgets('shows Online for connected', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: ConnectivityBadge(status: ChatStatus.connected)),
      ),
    );

    expect(find.text('Online'), findsOneWidget);
    expect(find.text('Retry'), findsNothing);
  });

  testWidgets('shows Retry while reconnecting when onRetry is set', (
    tester,
  ) async {
    var taps = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ConnectivityBadge(
            status: ChatStatus.reconnecting,
            onRetry: () => taps++,
          ),
        ),
      ),
    );

    expect(find.text('Retry'), findsOneWidget);
    await tester.tap(find.text('Retry'));
    expect(taps, 1);
  });

  testWidgets('shows Retry while offline when onRetry is set', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ConnectivityBadge(
            status: ChatStatus.disconnected,
            onRetry: () {},
          ),
        ),
      ),
    );

    expect(find.text('Retry'), findsOneWidget);
  });
}
