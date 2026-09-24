import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../ui/pane_header.dart';
import '../ui/theme/design_tokens.dart';
import 'chat_controller.dart';

import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';

class ThreadPane extends StatelessWidget {
  const ThreadPane({super.key, required this.controller});

  final ChatController controller;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final pane = _buildPane(context);
        // A parent Row may measure this pane with an unbounded width.
        if (constraints.maxWidth.isFinite) {
          return pane;
        }
        return SizedBox(width: 320, child: pane);
      },
    );
  }

  Widget _buildPane(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final threads = controller.visibleThreads;
        final showOfflineEmpty =
            controller.threads.isEmpty && _isOffline(controller.status);
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 8, 8),
              child: PaneHeader(
                title: Text(
                  'Threads',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                actions: [
                  Semantics(
                    button: true,
                    label: 'New thread',
                    child: IconButton(
                      key: const Key('new-thread'),
                      style: PaneHeader.actionStyle,
                      tooltip: 'New thread',
                      icon: const Icon(Icons.add, size: 20),
                      onPressed: controller.createThread,
                    ),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              child: TextField(
                key: const Key('thread-filter'),
                onChanged: controller.setThreadFilter,
                decoration: const InputDecoration(
                  hintText: 'Filter',
                  isDense: true,
                ),
              ),
            ),
            Expanded(
              child: showOfflineEmpty
                  ? const Center(child: Text("You're offline"))
                  : ListView.builder(
                      itemCount: threads.length,
                      itemBuilder: (context, index) {
                        final thread = threads[index];
                        return _ThreadRow(
                          controller: controller,
                          thread: thread,
                        );
                      },
                    ),
            ),
          ],
        );
      },
    );
  }

  bool _isOffline(ChatStatus status) {
    return status == ChatStatus.disconnected ||
        status == ChatStatus.reconnecting ||
        status == ChatStatus.error;
  }
}

class _ThreadRow extends StatefulWidget {
  const _ThreadRow({required this.controller, required this.thread});

  final ChatController controller;
  final ThreadSummary thread;

  @override
  State<_ThreadRow> createState() => _ThreadRowState();
}

class _ThreadRowState extends State<_ThreadRow> {
  bool _hovering = false;

  Future<void> _rename() async {
    final result = await showDialog<String>(
      context: context,
      builder: (context) =>
          _RenameThreadDialog(initialTitle: widget.thread.title),
    );
    if (!mounted) {
      return;
    }
    final title = result?.trim() ?? '';
    if (title.isNotEmpty) {
      await widget.controller.renameThread(widget.thread.id, title);
    }
  }

  @override
  Widget build(BuildContext context) {
    final thread = widget.thread;
    final selected = thread.id == widget.controller.selectedThreadId;
    final menuOpacity = (_hovering || selected) ? 1.0 : 0.4;
    final tokens = designTokensOf(context);

    // Own Material so selected fill / ink are not painted under the dock
    // content-area DecoratedBox (see buildDockTabTheme).
    return MouseRegion(
      onEnter: (_) => setState(() => _hovering = true),
      onExit: (_) => setState(() => _hovering = false),
      child: Material(
        type: MaterialType.transparency,
        child: Semantics(
          button: true,
          selected: selected,
          label: 'Thread ${thread.title}',
          child: ListTile(
            dense: true,
            selected: selected,
            selectedTileColor: tokens.surfaceActive,
            shape: Border(
              left: BorderSide(
                color: selected ? tokens.primary : Colors.transparent,
                width: 3,
              ),
            ),
            title: Text(thread.title, overflow: TextOverflow.ellipsis),
            subtitle: thread.messageCount == 0 ? const Text('empty') : null,
            onTap: () {
              unawaited(
                widget.controller.selectThread(thread.id).catchError((
                  Object e,
                  StackTrace s,
                ) {
                  AppLog.record('selectThread: $e', s);
                }),
              );
            },
            trailing: Opacity(
              opacity: menuOpacity,
              child: PopupMenuButton<String>(
                key: const Key('thread-overflow'),
                tooltip: 'Thread actions',
                onSelected: (value) {
                  if (value == 'rename') {
                    unawaited(
                      _rename().catchError((Object e, StackTrace s) {
                        AppLog.record('thread rename: $e', s);
                      }),
                    );
                  }
                },
                itemBuilder: (context) => const [
                  PopupMenuItem(value: 'rename', child: Text('Rename')),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _RenameThreadDialog extends StatefulWidget {
  const _RenameThreadDialog({required this.initialTitle});

  final String initialTitle;

  @override
  State<_RenameThreadDialog> createState() => _RenameThreadDialogState();
}

class _RenameThreadDialogState extends State<_RenameThreadDialog> {
  late final TextEditingController _field;

  @override
  void initState() {
    super.initState();
    _field = TextEditingController(text: widget.initialTitle);
  }

  @override
  void dispose() {
    _field.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Rename'),
      content: TextField(
        key: const Key('rename-thread-field'),
        controller: _field,
        autofocus: true,
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: () => Navigator.of(context).pop(_field.text),
          child: const Text('OK'),
        ),
      ],
    );
  }
}
