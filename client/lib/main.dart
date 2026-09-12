import 'package:flutter/material.dart';

import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';

void main() {
  runApp(const AgentFabricApp());
}

class AgentFabricApp extends StatefulWidget {
  const AgentFabricApp({super.key, this.controller});

  /// Optional override for tests. Production leaves this null and owns the
  /// controller lifecycle.
  final ChatController? controller;

  @override
  State<AgentFabricApp> createState() => _AgentFabricAppState();
}

class _AgentFabricAppState extends State<AgentFabricApp> {
  late final ChatController _controller;
  late final bool _ownsController;

  @override
  void initState() {
    super.initState();
    _ownsController = widget.controller == null;
    _controller = widget.controller ?? ChatController();
  }

  @override
  void dispose() {
    if (_ownsController) {
      _controller.dispose();
    }
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
