import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import 'chat_controller.dart';

class ThreadPane extends StatelessWidget {
  const ThreadPane({super.key, required this.controller});

  final ChatController controller;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 240,
      child: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          final threads = controller.visibleThreads;
          final showOfflineEmpty =
              controller.threads.isEmpty && _isOffline(controller.status);
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 8, 4, 0),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        'Threads',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                    ),
                    IconButton(
                      key: const Key('new-thread'),
                      icon: const Icon(Icons.add),
                      onPressed: controller.createThread,
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
      ),
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
    final scheme = Theme.of(context).colorScheme;

    return MouseRegion(
      onEnter: (_) => setState(() => _hovering = true),
      onExit: (_) => setState(() => _hovering = false),
      child: ListTile(
        dense: true,
        selected: selected,
        selectedTileColor: scheme.primaryContainer,
        shape: Border(
          left: BorderSide(
            color: selected ? scheme.primary : Colors.transparent,
            width: 3,
          ),
        ),
        title: Text(thread.title, overflow: TextOverflow.ellipsis),
        subtitle: thread.messageCount == 0 ? const Text('empty') : null,
        onTap: () => widget.controller.selectThread(thread.id),
        trailing: Opacity(
          opacity: menuOpacity,
          child: PopupMenuButton<String>(
            onSelected: (value) {
              if (value == 'rename') {
                _rename();
              }
            },
            itemBuilder: (context) => const [
              PopupMenuItem(value: 'rename', child: Text('Rename')),
            ],
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
