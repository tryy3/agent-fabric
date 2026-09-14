import 'package:material_ui/material_ui.dart';

import 'agent_bubble.dart';
import 'chat_bubble.dart';
import 'chat_controller.dart';
import 'display_settings.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({
    super.key,
    required this.controller,
    required this.displaySettings,
  });

  final ChatController controller;
  final ChatDisplaySettings displaySettings;

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _input = TextEditingController();
  final _scroll = ScrollController();

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_scrollToEnd);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) {
        return;
      }
      widget.controller.connect();
    });
  }

  @override
  void dispose() {
    widget.controller.removeListener(_scrollToEnd);
    _scroll.dispose();
    _input.dispose();
    super.dispose();
  }

  void _scrollToEnd() {
    if (!widget.controller.sending) {
      return;
    }
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scroll.hasClients) {
        return;
      }
      _scroll.jumpTo(_scroll.position.maxScrollExtent);
    });
  }

  Future<void> _submit() async {
    final text = _input.text;
    _input.clear();
    await widget.controller.send(text);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: Listenable.merge([widget.controller, widget.displaySettings]),
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
                child: c.selectedThreadId == null
                    ? const Center(
                        child: Text('Create a thread to start chatting'),
                      )
                    : ListView.builder(
                        controller: _scroll,
                        padding: const EdgeInsets.all(16),
                        itemCount: c.messages.length,
                        itemBuilder: (context, index) {
                          final m = c.messages[index];
                          final display = widget.displaySettings;
                          if (m.kind == ChatBubbleKind.user) {
                            return Align(
                              alignment: Alignment.centerRight,
                              child: Container(
                                margin: const EdgeInsets.symmetric(vertical: 4),
                                padding: const EdgeInsets.all(12),
                                decoration: BoxDecoration(
                                  color: Colors.blue.shade100,
                                  borderRadius: BorderRadius.circular(8),
                                ),
                                child: Text(m.text),
                              ),
                            );
                          }
                          return AgentBubble(
                            bubble: m,
                            thinkingMode: display.thinking,
                            statsMode: display.stats,
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
    final items = <DropdownMenuItem<String>>[
      for (final agent in c.agents)
        DropdownMenuItem(
          value: agent.id,
          enabled: agent.isComplete,
          child: Text(
            agent.isComplete ? agent.name : '${agent.name} — needs provider',
          ),
        ),
    ];
    if (c.selectedAgentMissing && c.selectedAgentId != null) {
      items.add(
        DropdownMenuItem(
          value: c.selectedAgentId,
          enabled: false,
          child: const Text('(deleted)'),
        ),
      );
    }
    return DropdownButton<String>(
      key: const Key('agent-picker'),
      isExpanded: true,
      hint: const Text('Agent'),
      value: c.selectedAgentId,
      items: items,
      onChanged: c.canSelectAgent
          ? (id) {
              if (id != null) {
                c.selectAgent(id);
              }
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
    if (c.selectedAgentMissing) {
      return 'This agent was deleted';
    }
    if (c.selectedAgentId != null && !c.selectedAgentIsComplete) {
      return 'This agent needs a provider';
    }
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
