import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import 'chat_controller.dart';

class ThreadPane extends StatelessWidget {
  const ThreadPane({super.key, required this.controller});

  final ChatController controller;

  @override
  Widget build(BuildContext context) {
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
              padding: const EdgeInsets.fromLTRB(12, 8, 4, 0),
              child: Row(
                children: [
                  Expanded(
                    child: DropdownButtonHideUnderline(
                      child: DropdownButton<String>(
                        key: const Key('project-switcher'),
                        isExpanded: true,
                        isDense: true,
                        value:
                            controller.projects.any(
                              (p) => p.id == controller.selectedProjectId,
                            )
                            ? controller.selectedProjectId
                            : null,
                        hint: const Text('Project'),
                        items: [
                          for (final project in controller.projects)
                            DropdownMenuItem(
                              value: project.id,
                              child: Text(
                                project.name,
                                overflow: TextOverflow.ellipsis,
                              ),
                            ),
                        ],
                        onChanged: (id) {
                          if (id != null) {
                            controller.selectProject(id);
                          }
                        },
                      ),
                    ),
                  ),
                  IconButton(
                    key: const Key('new-project'),
                    tooltip: 'New project',
                    icon: const Icon(Icons.create_new_folder_outlined),
                    onPressed: () => _createProject(context),
                  ),
                  PopupMenuButton<String>(
                    key: const Key('export-project'),
                    tooltip: 'Export',
                    enabled: controller.selectedProjectId != null,
                    onSelected: (id) {
                      controller.exportSelectedProject(method: id);
                    },
                    itemBuilder: (context) {
                      final methods = controller.exporters.isEmpty
                          ? ExportMethod.defaults
                          : controller.exporters;
                      return [
                        for (final method in methods)
                          PopupMenuItem(
                            key: Key('export-${method.id}'),
                            value: method.id,
                            enabled: method.enabled,
                            child: Text(_exportLabel(method)),
                          ),
                      ];
                    },
                    icon: const Icon(Icons.ios_share_outlined),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 0, 4, 0),
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
    );
  }

  bool _isOffline(ChatStatus status) {
    return status == ChatStatus.disconnected ||
        status == ChatStatus.reconnecting ||
        status == ChatStatus.error;
  }

  Future<void> _createProject(BuildContext context) async {
    final result = await showDialog<String>(
      context: context,
      builder: (context) => const _NewProjectDialog(),
    );
    final name = result?.trim() ?? '';
    if (name.isNotEmpty) {
      await controller.createProject(name);
    }
  }

  String _exportLabel(ExportMethod method) {
    final reason = method.reason?.trim();
    if (reason == null || reason.isEmpty || method.enabled) {
      return method.label;
    }
    return '${method.label} ($reason)';
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
            key: const Key('thread-overflow'),
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

class _NewProjectDialog extends StatefulWidget {
  const _NewProjectDialog();

  @override
  State<_NewProjectDialog> createState() => _NewProjectDialogState();
}

class _NewProjectDialogState extends State<_NewProjectDialog> {
  late final TextEditingController _field;

  @override
  void initState() {
    super.initState();
    _field = TextEditingController();
  }

  @override
  void dispose() {
    _field.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('New project'),
      content: TextField(
        key: const Key('new-project-field'),
        controller: _field,
        autofocus: true,
        decoration: const InputDecoration(hintText: 'Name'),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: () => Navigator.of(context).pop(_field.text),
          child: const Text('Create'),
        ),
      ],
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
