import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../chat/chat_controller.dart';
import '../ui/connectivity_badge.dart';
import '../ui/theme/design_tokens.dart';
import 'project_config.dart';

class ProjectSidebar extends StatefulWidget {
  const ProjectSidebar({
    super.key,
    required this.controller,
    required this.settingsActive,
    required this.onOpenSettings,
    required this.onOpenWorkspace,
  });

  final ChatController controller;
  final bool settingsActive;
  final VoidCallback onOpenSettings;
  final VoidCallback onOpenWorkspace;

  @override
  State<ProjectSidebar> createState() => _ProjectSidebarState();
}

class _ProjectSidebarState extends State<ProjectSidebar> {
  final Set<String> _expanded = {};

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_expandSelected);
    _expandSelected();
  }

  @override
  void didUpdateWidget(covariant ProjectSidebar oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller) {
      oldWidget.controller.removeListener(_expandSelected);
      widget.controller.addListener(_expandSelected);
    }
  }

  @override
  void dispose() {
    widget.controller.removeListener(_expandSelected);
    super.dispose();
  }

  void _expandSelected() {
    final id = widget.controller.selectedProjectId;
    if (id == null || _expanded.contains(id)) {
      return;
    }
    setState(() => _expanded.add(id));
  }

  Future<void> _toggleProject(Project project) async {
    final open = _expanded.contains(project.id);
    setState(() {
      if (open) {
        _expanded.remove(project.id);
      } else {
        _expanded.add(project.id);
      }
    });
    if (!open) {
      await widget.controller.ensureProjectThreads(project.id);
    }
  }

  Future<void> _openThread(Project project, ThreadSummary thread) async {
    widget.onOpenWorkspace();
    if (widget.controller.selectedProjectId != project.id) {
      await widget.controller.selectProject(
        project.id,
        preferThreadId: thread.id,
      );
      return;
    }
    await widget.controller.selectThread(thread.id);
  }

  /// Row clicks activate the project; the group chevron only expands.
  Future<void> _openProject(Project project) async {
    widget.onOpenWorkspace();
    if (widget.controller.selectedProjectId == project.id) {
      return;
    }
    await widget.controller.selectProject(project.id);
  }

  Future<void> _createProject() async {
    final name = await showDialog<String>(
      context: context,
      builder: (context) => const _NewProjectDialog(),
    );
    final trimmed = name?.trim() ?? '';
    if (trimmed.isEmpty) {
      return;
    }
    widget.onOpenWorkspace();
    await widget.controller.createProject(trimmed);
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Material(
      color: tokens.sidebar,
      child: ListenableBuilder(
        listenable: widget.controller,
        builder: (context, _) {
          final projects = widget.controller.projects;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _Identity(
                tokens: tokens,
                onOpenWorkspace: widget.onOpenWorkspace,
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 4, 12, 12),
                child: FilledButton.icon(
                  key: const Key('sidebar-new-thread'),
                  onPressed: () {
                    widget.onOpenWorkspace();
                    widget.controller.createThread();
                  },
                  icon: const Icon(Icons.add, size: 18),
                  label: const Text('New thread'),
                  style: FilledButton.styleFrom(
                    backgroundColor: tokens.primary,
                    foregroundColor: tokens.background,
                    minimumSize: const Size.fromHeight(40),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(
                        DesignTokens.radiusLg,
                      ),
                    ),
                    textStyle: tokens.labelMd(),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 4, 8, 4),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        'Projects',
                        style: tokens.labelSm().copyWith(
                          color: tokens.textMuted,
                        ),
                      ),
                    ),
                    IconButton(
                      key: const Key('new-project'),
                      tooltip: 'New project',
                      onPressed: _createProject,
                      icon: Icon(
                        Icons.create_new_folder_outlined,
                        size: 18,
                        color: tokens.textSecondary,
                      ),
                    ),
                  ],
                ),
              ),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  children: [
                    for (final project in projects)
                      _ProjectGroup(
                        project: project,
                        expanded: _expanded.contains(project.id),
                        selected:
                            project.id == widget.controller.selectedProjectId,
                        threads: widget.controller.threadsFor(project.id),
                        selectedThreadId: widget.controller.selectedThreadId,
                        onToggle: () => _toggleProject(project),
                        onOpenProject: () => _openProject(project),
                        onOpenThread: (thread) => _openThread(project, thread),
                        onExport:
                            project.id == widget.controller.selectedProjectId
                            ? (method) => widget.controller
                                  .exportSelectedProject(method: method)
                            : null,
                        exporters: widget.controller.exporters,
                      ),
                  ],
                ),
              ),
              Divider(height: 1, color: tokens.border),
              _Footer(
                tokens: tokens,
                settingsActive: widget.settingsActive,
                status: widget.controller.status,
                onOpenSettings: widget.onOpenSettings,
              ),
            ],
          );
        },
      ),
    );
  }
}

class _Identity extends StatelessWidget {
  const _Identity({required this.tokens, required this.onOpenWorkspace});

  final DesignTokens tokens;
  final VoidCallback onOpenWorkspace;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 12, 8, 8),
      child: InkWell(
        key: const Key('nav-workspace'),
        borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
        onTap: onOpenWorkspace,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
          child: Row(
            children: [
              Icon(Icons.hub_outlined, color: tokens.primary, size: 20),
              const SizedBox(width: 8),
              Text('Agent Fabric', style: tokens.labelMd()),
            ],
          ),
        ),
      ),
    );
  }
}

class _ProjectGroup extends StatelessWidget {
  const _ProjectGroup({
    required this.project,
    required this.expanded,
    required this.selected,
    required this.threads,
    required this.selectedThreadId,
    required this.onToggle,
    required this.onOpenProject,
    required this.onOpenThread,
    required this.onExport,
    required this.exporters,
  });

  final Project project;
  final bool expanded;
  final bool selected;
  final List<ThreadSummary> threads;
  final String? selectedThreadId;
  final VoidCallback onToggle;
  final VoidCallback onOpenProject;
  final ValueChanged<ThreadSummary> onOpenThread;
  final void Function(String method)? onExport;
  final List<ExportMethod> exporters;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final marker = Color(projectMarkerColor(project.id));
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Material(
          color: selected ? tokens.surfaceActive : Colors.transparent,
          borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
          child: SizedBox(
            height: 40,
            child: Row(
              children: [
                SizedBox(
                  width: 28,
                  height: 40,
                  child: InkWell(
                    key: Key('project-toggle-${project.id}'),
                    onTap: onToggle,
                    borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
                    child: Icon(
                      expanded ? Icons.expand_more : Icons.chevron_right,
                      size: 16,
                      color: tokens.textMuted,
                    ),
                  ),
                ),
                Expanded(
                  child: InkWell(
                    key: Key('project-row-${project.id}'),
                    onTap: onOpenProject,
                    borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
                    child: Padding(
                      padding: const EdgeInsets.only(right: 4),
                      child: Row(
                        children: [
                          Container(
                            width: 8,
                            height: 8,
                            decoration: BoxDecoration(
                              color: marker,
                              shape: BoxShape.circle,
                            ),
                          ),
                          const SizedBox(width: 8),
                          Expanded(
                            child: Text(
                              project.name,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: tokens.labelMd().copyWith(
                                color: selected
                                    ? tokens.textPrimary
                                    : tokens.textSecondary,
                              ),
                            ),
                          ),
                          if (onExport != null)
                            PopupMenuButton<String>(
                              key: const Key('export-project'),
                              tooltip: 'Export',
                              padding: EdgeInsets.zero,
                              iconSize: 18,
                              onSelected: onExport,
                              itemBuilder: (context) {
                                final methods = exporters.isEmpty
                                    ? ExportMethod.defaults
                                    : exporters;
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
                              icon: Icon(
                                Icons.more_horiz,
                                size: 18,
                                color: tokens.textMuted,
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
        ),
        if (expanded)
          for (final thread in threads)
            _ThreadNavRow(
              thread: thread,
              selected: selected && thread.id == selectedThreadId,
              onTap: () => onOpenThread(thread),
            ),
      ],
    );
  }
}

class _ThreadNavRow extends StatelessWidget {
  const _ThreadNavRow({
    required this.thread,
    required this.selected,
    required this.onTap,
  });

  final ThreadSummary thread;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Padding(
      padding: const EdgeInsets.only(left: 28),
      child: Material(
        color: selected ? tokens.surfaceActive : Colors.transparent,
        borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
        child: InkWell(
          key: Key('sidebar-thread-${thread.id}'),
          borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
          onTap: onTap,
          child: SizedBox(
            height: DesignTokens.rowHeight,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 8),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      thread.title,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: tokens.bodySm().copyWith(
                        color: selected
                            ? tokens.textPrimary
                            : tokens.textSecondary,
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Text(
                    _stamp(thread.updatedAt),
                    style: tokens.caption().copyWith(color: tokens.textMuted),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _Footer extends StatelessWidget {
  const _Footer({
    required this.tokens,
    required this.settingsActive,
    required this.status,
    required this.onOpenSettings,
  });

  final DesignTokens tokens;
  final bool settingsActive;
  final ChatStatus status;
  final VoidCallback onOpenSettings;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 8, 8, 12),
      child: Column(
        children: [
          Material(
            color: settingsActive ? tokens.surfaceActive : Colors.transparent,
            borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
            child: InkWell(
              key: const Key('nav-settings'),
              borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
              onTap: onOpenSettings,
              child: SizedBox(
                height: 40,
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 10),
                  child: Row(
                    children: [
                      Icon(
                        Icons.settings_outlined,
                        size: 18,
                        color: tokens.textSecondary,
                      ),
                      const SizedBox(width: 8),
                      Text(
                        'Settings',
                        style: tokens.labelMd().copyWith(
                          color: tokens.textSecondary,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
          const SizedBox(height: 8),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10),
            child: Row(
              children: [
                Expanded(child: ConnectivityBadge(status: status)),
                const SizedBox(width: 8),
                const _LocalAccount(),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _LocalAccount extends StatelessWidget {
  const _LocalAccount();

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Tooltip(
      message: 'Local session',
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          CircleAvatar(
            radius: 12,
            backgroundColor: tokens.surfaceActive,
            child: Text(
              'L',
              style: tokens.caption().copyWith(color: tokens.textPrimary),
            ),
          ),
          const SizedBox(width: 8),
          Text(
            'Local',
            key: const Key('local-account'),
            style: tokens.labelSm().copyWith(color: tokens.textMuted),
          ),
        ],
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

String _exportLabel(ExportMethod method) {
  final reason = method.reason?.trim();
  if (reason == null || reason.isEmpty || method.enabled) {
    return method.label;
  }
  return '${method.label} ($reason)';
}

String _stamp(DateTime when) {
  final local = when.toLocal();
  final now = DateTime.now();
  final sameDay =
      local.year == now.year &&
      local.month == now.month &&
      local.day == now.day;
  if (sameDay) {
    final hour12 = local.hour % 12 == 0 ? 12 : local.hour % 12;
    final minute = local.minute.toString().padLeft(2, '0');
    final suffix = local.hour >= 12 ? 'PM' : 'AM';
    return '$hour12:$minute $suffix';
  }
  const months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  return '${months[local.month - 1]} ${local.day}';
}
