import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/ui/connectivity_badge.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('shows Offline for disconnected', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: ConnectivityBadge(status: ChatStatus.disconnected),
      ),
    );

    expect(find.text('Offline'), findsOneWidget);
  });

  testWidgets('shows Reconnecting… while reconnecting', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: ConnectivityBadge(status: ChatStatus.reconnecting),
      ),
    );

    expect(find.text('Reconnecting…'), findsOneWidget);
  });

  testWidgets('shows Online for connected', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(home: ConnectivityBadge(status: ChatStatus.connected)),
    );

    expect(find.text('Online'), findsOneWidget);
  });
}
