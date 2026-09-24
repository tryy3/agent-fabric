import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../chat/chat_controller.dart';
import '../ui/theme/design_tokens.dart';
import 'project_config.dart';
import 'project_tabs_controller.dart';

/// Open-project tabs above the workspace.
///
/// Tabs are workspace switches, not documents: selecting one swaps the whole
/// workspace, and only the active project is live. The active tab connects
/// to the context bar below it; every open tab keeps a close affordance, and
/// the trailing + opens any project that is not tabbed yet.
class ProjectTabStrip extends StatelessWidget {
  const ProjectTabStrip({
    super.key,
    required this.controller,
    required this.tabs,
    required this.onSelectProject,
    required this.onCloseProjectTab,
    this.onOpenSidebar,
    this.onResetLayout,
  });

  final ChatController controller;
  final ProjectTabsController tabs;
  final ValueChanged<String> onSelectProject;
  final ValueChanged<String> onCloseProjectTab;
  final VoidCallback? onOpenSidebar;
  final VoidCallback? onResetLayout;

  static final ButtonStyle _iconStyle = IconButton.styleFrom(
    visualDensity: VisualDensity.compact,
    padding: EdgeInsets.zero,
    minimumSize: const Size(28, 28),
    fixedSize: const Size(28, 28),
    tapTargetSize: MaterialTapTargetSize.shrinkWrap,
  );

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return SizedBox(
      height: DesignTokens.tabHeight,
      width: double.infinity,
      child: ListenableBuilder(
        listenable: Listenable.merge([controller, tabs]),
        builder: (context, _) {
          final known = {
            for (final project in controller.projects) project.id: project,
          };
          final open = [
            for (final id in tabs.openProjectIds)
              if (known.containsKey(id)) known[id]!,
          ];
          return Row(
            children: [
              if (onOpenSidebar != null)
                IconButton(
                  key: const Key('sidebar-menu'),
                  tooltip: 'Projects',
                  onPressed: onOpenSidebar,
                  style: _iconStyle,
                  icon: Icon(Icons.menu, size: 18, color: tokens.textSecondary),
                ),
              Expanded(
                child: ListView(
                  scrollDirection: Axis.horizontal,
                  padding: EdgeInsets.zero,
                  children: [
                    if (open.isEmpty)
                      const _NoProjectTab()
                    else
                      for (final project in open)
                        _ProjectTab(
                          project: project,
                          active: project.id == controller.selectedProjectId,
                          onSelect: () => onSelectProject(project.id),
                          onClose: () => onCloseProjectTab(project.id),
                        ),
                    _NewProjectTabButton(
                      controller: controller,
                      tabs: tabs,
                      onSelectProject: onSelectProject,
                    ),
                  ],
                ),
              ),
              if (onResetLayout != null)
                IconButton(
                  key: const Key('reset-layout'),
                  tooltip: 'Reset pane layout',
                  onPressed: onResetLayout,
                  style: _iconStyle,
                  icon: Icon(
                    Icons.restore_outlined,
                    size: 18,
                    color: tokens.textMuted,
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}

class _ProjectTab extends StatelessWidget {
  const _ProjectTab({
    required this.project,
    required this.active,
    required this.onSelect,
    required this.onClose,
  });

  final Project project;
  final bool active;
  final VoidCallback onSelect;
  final VoidCallback onClose;

  static final ButtonStyle _closeStyle = IconButton.styleFrom(
    visualDensity: VisualDensity.compact,
    padding: EdgeInsets.zero,
    minimumSize: const Size(28, 28),
    fixedSize: const Size(28, 28),
    tapTargetSize: MaterialTapTargetSize.shrinkWrap,
  );

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final marker = Color(projectMarkerColor(project.id));
    final shape = const RoundedRectangleBorder(
      borderRadius: BorderRadius.vertical(
        top: Radius.circular(DesignTokens.radiusLg),
      ),
    );
    return Padding(
      padding: const EdgeInsets.only(right: 2),
      child: Material(
        color: active ? tokens.surfaceRaised : tokens.surface,
        shape: shape,
        child: InkWell(
          key: Key('project-tab-${project.id}'),
          onTap: onSelect,
          customBorder: shape,
          hoverColor: active ? null : tokens.surfaceActive.withValues(
            alpha: 0.35,
          ),
          focusColor: tokens.primary.withValues(alpha: 0.12),
          child: Container(
            height: DesignTokens.tabHeight,
            constraints: const BoxConstraints(maxWidth: 240),
            padding: const EdgeInsets.only(left: 14, right: 6),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(color: marker, shape: BoxShape.circle),
                ),
                const SizedBox(width: 10),
                ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 140),
                  child: Text(
                    project.name,
                    key: active ? const Key('active-project-tab') : null,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: tokens.labelMd().copyWith(
                      color: active ? tokens.textPrimary : tokens.textSecondary,
                      fontWeight: active ? FontWeight.w600 : FontWeight.w500,
                    ),
                  ),
                ),
                IconButton(
                  key: Key('close-project-tab-${project.id}'),
                  tooltip: 'Close ${project.name}',
                  onPressed: onClose,
                  style: _closeStyle,
                  icon: Icon(Icons.close, size: 16, color: tokens.textMuted),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Empty state while no project is active. Not a switch; the + button next
/// to it is how a project gets opened again.
class _NoProjectTab extends StatelessWidget {
  const _NoProjectTab();

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Container(
      height: DesignTokens.tabHeight,
      padding: const EdgeInsets.only(left: 14, right: 14),
      decoration: BoxDecoration(
        color: tokens.surface,
        borderRadius: const BorderRadius.vertical(
          top: Radius.circular(DesignTokens.radiusLg),
        ),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.workspaces_outlined, size: 16, color: tokens.textMuted),
          const SizedBox(width: 8),
          Text(
            'No project',
            key: const Key('no-project-tab'),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: tokens.labelMd().copyWith(color: tokens.textMuted),
          ),
        ],
      ),
    );
  }
}

/// Opens a project that is not tabbed yet.
class _NewProjectTabButton extends StatelessWidget {
  const _NewProjectTabButton({
    required this.controller,
    required this.tabs,
    required this.onSelectProject,
  });

  final ChatController controller;
  final ProjectTabsController tabs;
  final ValueChanged<String> onSelectProject;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final closed = [
      for (final project in controller.projects)
        if (!tabs.isOpen(project.id)) project,
    ];
    return PopupMenuButton<String>(
      key: const Key('new-project-tab'),
      tooltip: closed.isEmpty ? 'All projects are open' : 'Open project tab',
      enabled: closed.isNotEmpty,
      onSelected: onSelectProject,
      position: PopupMenuPosition.under,
      padding: EdgeInsets.zero,
      iconSize: 18,
      icon: Icon(Icons.add, size: 18, color: tokens.textSecondary),
      itemBuilder: (context) => [
        for (final project in closed)
          PopupMenuItem<String>(
            key: Key('new-project-tab-${project.id}'),
            value: project.id,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    color: Color(projectMarkerColor(project.id)),
                    shape: BoxShape.circle,
                  ),
                ),
                const SizedBox(width: 10),
                Text(project.name, style: tokens.labelMd()),
              ],
            ),
          ),
      ],
    );
  }
}
