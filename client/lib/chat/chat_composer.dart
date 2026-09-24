import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import 'chat_controller.dart';
import 'model_picker.dart';

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
    _focus.onKeyEvent = (node, event) {
      if (event is! KeyDownEvent) {
        return KeyEventResult.ignored;
      }
      if (event.logicalKey != LogicalKeyboardKey.enter &&
          event.logicalKey != LogicalKeyboardKey.numpadEnter) {
        return KeyEventResult.ignored;
      }
      final shift = HardwareKeyboard.instance.isShiftPressed;
      if (shift) {
        _insertNewline();
        return KeyEventResult.handled;
      }
      if (widget.controller.canSend && _input.text.trim().isNotEmpty) {
        _submit();
        return KeyEventResult.handled;
      }
      return KeyEventResult.handled;
    };
  }

  @override
  void dispose() {
    _input.dispose();
    _focus.dispose();
    super.dispose();
  }

  void _insertNewline() {
    final text = _input.text;
    final selection = _input.selection;
    final start = selection.isValid ? selection.start : text.length;
    final end = selection.isValid ? selection.end : text.length;
    final next = text.replaceRange(start, end, '\n');
    _input.value = TextEditingValue(
      text: next,
      selection: TextSelection.collapsed(offset: start + 1),
    );
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
                    IconButton(
                      key: const Key('composer-attach'),
                      tooltip: 'Coming soon',
                      onPressed: null,
                      icon: const Icon(Icons.attach_file),
                    ),
                    IconButton(
                      key: const Key('composer-mic'),
                      tooltip: 'Coming soon',
                      onPressed: null,
                      icon: const Icon(Icons.mic_none),
                    ),
                    Flexible(
                      child: DropdownButtonHideUnderline(
                        child: _agentPicker(c),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Flexible(
                      child: Tooltip(
                        message: 'Session model',
                        child: ModelPicker(controller: c),
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
            agent.isComplete ? agent.name : '${agent.name} - needs provider',
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
}
