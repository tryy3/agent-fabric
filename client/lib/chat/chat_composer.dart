import 'package:material_ui/material_ui.dart';

import 'chat_controller.dart';

@visibleForTesting
int composerMinLines({required bool hasMessages, required bool focused}) {
  if (!hasMessages) return 4;
  if (focused) return 3;
  return 1;
}

class ChatComposer extends StatefulWidget {
  const ChatComposer({super.key, required this.controller});

  final ChatController controller;

  @override
  State<ChatComposer> createState() => _ChatComposerState();
}

class _ChatComposerState extends State<ChatComposer> {
  final _input = TextEditingController();
  final _focus = FocusNode();
  bool _focused = false;

  @override
  void initState() {
    super.initState();
    _focus.addListener(() {
      setState(() => _focused = _focus.hasFocus);
    });
  }

  @override
  void dispose() {
    _input.dispose();
    _focus.dispose();
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
        final scheme = Theme.of(context).colorScheme;
        return Material(
          color: scheme.surfaceContainerHighest,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
            side: BorderSide(color: scheme.outlineVariant),
          ),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(12, 8, 8, 8),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                AnimatedSize(
                  duration: const Duration(milliseconds: 150),
                  curve: Curves.easeInOut,
                  alignment: Alignment.topCenter,
                  child: TextField(
                    key: const Key('composer-input'),
                    controller: _input,
                    focusNode: _focus,
                    enabled: c.canSend,
                    minLines: composerMinLines(
                      hasMessages: c.messages.isNotEmpty,
                      focused: _focused,
                    ),
                    maxLines: 8,
                    keyboardType: TextInputType.multiline,
                    textInputAction: TextInputAction.newline,
                    decoration: const InputDecoration(
                      hintText: 'Message',
                      border: InputBorder.none,
                      isDense: true,
                    ),
                  ),
                ),
                Row(
                  children: [
                    Flexible(
                      child: DropdownButtonHideUnderline(
                        child: _agentPicker(c),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Flexible(
                      child: DropdownButtonHideUnderline(
                        child: _modelPicker(c),
                      ),
                    ),
                    const Spacer(),
                    IconButton(
                      key: const Key('composer-send'),
                      onPressed: c.canSend ? _submit : null,
                      icon: const Icon(Icons.arrow_upward),
                      style: IconButton.styleFrom(
                        backgroundColor: c.canSend
                            ? Theme.of(context).colorScheme.primary
                            : null,
                        foregroundColor: c.canSend
                            ? Theme.of(context).colorScheme.onPrimary
                            : null,
                      ),
                    ),
                  ],
                ),
              ],
            ),
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
      isDense: true,
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
      isDense: true,
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
}
