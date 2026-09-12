import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/app_shell.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class _FakeConn implements AgentSessionApi {
  final _closed = StreamController<void>.broadcast(sync: true);

  @override
  Stream<void> get closed => _closed.stream;

  @override
  Future<void> connect({Transport? transport}) async {}

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentChunkHandler onChunk,
  }) async {}

  @override
  Future<void> close() async {}
}

void main() {
  testWidgets('shows Chat and Settings destinations', (WidgetTester tester) async {
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(controller: controller),
      ),
    );
    await tester.pump();
    await tester.pump();

    expect(find.text('Chat'), findsOneWidget);
    expect(find.text('Settings'), findsOneWidget);
  });

  testWidgets('tapping Settings shows Settings placeholder', (WidgetTester tester) async {
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: AppShell(controller: controller),
      ),
    );
    await tester.pump();
    await tester.pump();

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.text('Settings'), findsWidgets);
    expect(find.byType(TextField), findsNothing);
  });
}
