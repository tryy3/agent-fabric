import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class _FakeConn implements AgentSessionApi {
  final _closed = StreamController<void>.broadcast(sync: true);

  @override
  Stream<void> get closed => _closed.stream;

  @override
  Future<void> connect({Transport? transport}) async {}

  @override
  Future<void> startSession(String agentId) async {}

  @override
  Future<void> setModel(String modelId) async {}

  @override
  List<ModelOption> get modelOptions => const [];

  @override
  String? get currentModel => null;

  @override
  Future<void> sendPrompt(
    String text, {
    required AgentChunkHandler onChunk,
  }) async {}

  @override
  Future<void> close() async {}
}

void main() {
  testWidgets('shows chat shell', (WidgetTester tester) async {
    final controller = ChatController(session: _FakeConn());
    addTearDown(controller.dispose);

    await tester.pumpWidget(AgentFabricApp(controller: controller));
    await tester.pump();
    await tester.pump();

    expect(find.text('Agent Fabric'), findsOneWidget);
    expect(find.byKey(const Key('agent-picker')), findsOneWidget);
    expect(find.byKey(const Key('model-picker')), findsOneWidget);
    expect(find.byType(TextField), findsOneWidget);
    expect(find.byIcon(Icons.send), findsOneWidget);
  });
}
