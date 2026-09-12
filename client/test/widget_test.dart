import 'package:agent_fabric_client/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('shows chat shell', (WidgetTester tester) async {
    await tester.pumpWidget(const AgentFabricApp());
    await tester.pump();

    expect(find.text('Agent Fabric'), findsOneWidget);
    expect(find.byType(TextField), findsOneWidget);
    expect(find.byIcon(Icons.send), findsOneWidget);
  });
}
