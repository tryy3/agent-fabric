import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../acp/slash_commands.dart';
import 'chat_controller.dart';
import 'model_picker.dart';
import 'permission_mode_picker.dart';

import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';

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
  int _pickerIndex = 0;

  @override
  void initState() {
    super.initState();
    _input.addListener(() {
      if (mounted) setState(() {});
    });
    _focus.addListener(() {
      setState(() => _focused = _focus.hasFocus);
    });
    _focus.onKeyEvent = (node, event) {
      if (event is! KeyDownEvent) {
        return KeyEventResult.ignored;
      }
      final matches = _pickerMatches();
      if (matches.isNotEmpty) {
        final key = event.logicalKey;
        if (key == LogicalKeyboardKey.arrowDown) {
          setState(() => _pickerIndex = (_pickerIndex + 1) % matches.length);
          return KeyEventResult.handled;
        }
        if (key == LogicalKeyboardKey.arrowUp) {
          setState(
            () => _pickerIndex =
                (_pickerIndex - 1 + matches.length) % matches.length,
          );
          return KeyEventResult.handled;
        }
        if (key == LogicalKeyboardKey.tab ||
            key == LogicalKeyboardKey.enter ||
            key == LogicalKeyboardKey.numpadEnter) {
          _pick(matches[_pickerIndex.clamp(0, matches.length - 1)]);
          return KeyEventResult.handled;
        }
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
        unawaited(
          _submit().catchError((Object e, StackTrace s) {
            AppLog.record('composer submit: $e', s);
          }),
        );
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

  List<SlashCommand> _pickerMatches() =>
      matchSlashCommands(widget.controller.slashCommands, _input.text);

  void _pick(SlashCommand command) {
    final text = command.insertText;
    _input.value = TextEditingValue(
      text: text,
      selection: TextSelection.collapsed(offset: text.length),
    );
    _pickerIndex = 0;
    _focus.requestFocus();
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
                if (_pickerMatches() case final matches when matches.isNotEmpty)
                  _SlashCommandList(
                    commands: matches,
                    selected: _pickerIndex.clamp(0, matches.length - 1),
                    onPick: _pick,
                  ),
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
                    if (c.permissionModes.isNotEmpty) ...[
                      const SizedBox(width: 8),
                      Flexible(child: PermissionModePicker(controller: c)),
                    ],
                    const Spacer(),
                    if (c.canCancelTurn)
                      IconButton(
                        key: const Key('composer-stop'),
                        tooltip: 'Stop',
                        onPressed: () {
                          unawaited(
                            c.cancelTurn().catchError((Object e, StackTrace s) {
                              AppLog.record('composer stop: $e', s);
                            }),
                          );
                        },
                        icon: const Icon(Icons.stop),
                        style: IconButton.styleFrom(
                          backgroundColor: Theme.of(context)
                              .colorScheme
                              .errorContainer,
                          foregroundColor: Theme.of(context)
                              .colorScheme
                              .onErrorContainer,
                        ),
                      )
                    else
                      IconButton(
                        key: const Key('composer-send'),
                        tooltip: 'Send',
                        onPressed: c.canSend
                            ? () {
                                unawaited(
                                  _submit().catchError((
                                    Object e,
                                    StackTrace s,
                                  ) {
                                    AppLog.record('composer submit: $e', s);
                                  }),
                                );
                              }
                            : null,
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
      for (final agent in c.assistants)
        DropdownMenuItem(
          value: agent.id,
          enabled: agent.isComplete,
          child: Text(
            agent.isComplete ? agent.name : '${agent.name} - needs connection',
          ),
        ),
    ];
    if (c.selectedAssistantMissing && c.selectedAssistantId != null) {
      items.add(
        DropdownMenuItem(
          value: c.selectedAssistantId,
          enabled: false,
          child: const Text('(deleted)'),
        ),
      );
    }
    return DropdownButton<String>(
      key: const Key('agent-picker'),
      isDense: true,
      isExpanded: true,
      hint: const Text('Assistant'),
      value: c.selectedAssistantId,
      items: items,
      onChanged: c.canSelectAssistant
          ? (id) {
              if (id != null) {
                unawaited(
                  c.selectAssistant(id).catchError((Object e, StackTrace s) {
                    AppLog.record('selectAssistant: $e', s);
                  }),
                );
              }
            }
          : null,
    );
  }
}

/// The `/` picker above the message field: the enabled skills that match what
/// the user has typed, with their descriptions and argument hints.
class _SlashCommandList extends StatelessWidget {
  const _SlashCommandList({
    required this.commands,
    required this.selected,
    required this.onPick,
  });

  final List<SlashCommand> commands;
  final int selected;
  final ValueChanged<SlashCommand> onPick;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final text = Theme.of(context).textTheme;
    return Semantics(
      container: true,
      label: 'Commands',
      child: Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Column(
          key: const Key('slash-command-list'),
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (var i = 0; i < commands.length; i++)
              Material(
                color: i == selected
                    ? scheme.secondaryContainer
                    : Colors.transparent,
                borderRadius: BorderRadius.circular(8),
                child: InkWell(
                  key: Key('slash-command-${commands[i].name}'),
                  borderRadius: BorderRadius.circular(8),
                  onTap: () => onPick(commands[i]),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 6,
                    ),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('/${commands[i].name}', style: text.labelLarge),
                        if (commands[i].hint != null) ...[
                          const SizedBox(width: 6),
                          Text(
                            commands[i].hint!,
                            style: text.bodySmall?.copyWith(
                              color: scheme.onSurfaceVariant,
                            ),
                          ),
                        ],
                        const SizedBox(width: 12),
                        Expanded(
                          child: Text(
                            commands[i].description,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: text.bodySmall?.copyWith(
                              color: scheme.onSurfaceVariant,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
