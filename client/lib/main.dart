import 'package:flutter/material.dart';

import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';

void main() {
  runApp(const AgentFabricApp());
}

class AgentFabricApp extends StatefulWidget {
  const AgentFabricApp({super.key});

  @override
  State<AgentFabricApp> createState() => _AgentFabricAppState();
}

class _AgentFabricAppState extends State<AgentFabricApp> {
  late final ChatController _controller = ChatController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Agent Fabric',
      home: ChatScreen(controller: _controller),
    );
  }
}
