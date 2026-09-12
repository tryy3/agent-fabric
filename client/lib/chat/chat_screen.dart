import 'package:flutter/material.dart';

import 'chat_controller.dart';
import 'chat_message.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key, required this.controller});

  final ChatController controller;

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _input = TextEditingController();

  @override
  void initState() {
    super.initState();
    widget.controller.connect();
  }

  @override
  void dispose() {
    _input.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final text = _input.text;
    _input.clear();
    await widget.controller.send(text);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final c = widget.controller;
        return Scaffold(
          appBar: AppBar(
            title: const Text('Agent Fabric'),
            bottom: PreferredSize(
              preferredSize: const Size.fromHeight(64),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                child: Column(
                  children: [
                    Row(
                      children: [
                        Expanded(child: _agentPicker(c)),
                        const SizedBox(width: 12),
                        Expanded(child: _modelPicker(c)),
                      ],
                    ),
                    Text(
                      _statusLabel(c),
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
            ),
          ),
          body: Column(
            children: [
              Expanded(
                child: ListView.builder(
                  padding: const EdgeInsets.all(16),
                  itemCount: c.messages.length,
                  itemBuilder: (context, index) {
                    final m = c.messages[index];
                    final isUser = m.role == ChatRole.user;
                    return Align(
                      alignment: isUser
                          ? Alignment.centerRight
                          : Alignment.centerLeft,
                      child: Container(
                        margin: const EdgeInsets.symmetric(vertical: 4),
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(
                          color: isUser
                              ? Colors.blue.shade100
                              : Colors.grey.shade200,
                          borderRadius: BorderRadius.circular(8),
                        ),
                        child: Text(m.text.isEmpty && !isUser ? '…' : m.text),
                      ),
                    );
                  },
                ),
              ),
              SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: _input,
                          enabled: c.canSend,
                          onSubmitted: (_) => _submit(),
                          decoration: const InputDecoration(
                            hintText: 'Message',
                            border: OutlineInputBorder(),
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      IconButton(
                        onPressed: c.canSend ? _submit : null,
                        icon: const Icon(Icons.send),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _agentPicker(ChatController c) {
    final ids = {for (final a in c.agents) a.id};
    final value = ids.contains(c.selectedAgentId) ? c.selectedAgentId : null;
    return DropdownButton<String>(
      key: const Key('agent-picker'),
      isExpanded: true,
      hint: const Text('Agent'),
      value: value,
      items: [
        for (final agent in c.agents)
          DropdownMenuItem(value: agent.id, child: Text(agent.name)),
      ],
      onChanged: c.canSelectAgent
          ? (id) {
              if (id != null) c.selectAgent(id);
            }
          : null,
    );
  }

  Widget _modelPicker(ChatController c) {
    final ids = {for (final m in c.modelOptions) m.id};
    final value = ids.contains(c.currentModel) ? c.currentModel : null;
    return DropdownButton<String>(
      key: const Key('model-picker'),
      isExpanded: true,
      hint: const Text('Model'),
      value: value,
      items: [
        for (final model in c.modelOptions)
          DropdownMenuItem(value: model.id, child: Text(model.name)),
      ],
      onChanged: c.canSelectModel && c.modelOptions.isNotEmpty
          ? (id) {
              if (id != null) c.selectModel(id);
            }
          : null,
    );
  }

  String _statusLabel(ChatController c) {
    switch (c.status) {
      case ChatStatus.connecting:
        return 'Connecting…';
      case ChatStatus.connected:
        if (c.statusMessage != null) {
          return 'Error: ${c.statusMessage}';
        }
        return 'Connected';
      case ChatStatus.error:
        return 'Error: ${c.statusMessage ?? 'unknown'}';
      case ChatStatus.disconnected:
        return 'Disconnected';
    }
  }
}
