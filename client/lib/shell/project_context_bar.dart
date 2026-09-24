import 'dart:async';

import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../chat/chat_controller.dart';
import '../chat/model_picker.dart';
import '../dock/dock_layout_controller.dart';
import '../ui/theme/design_tokens.dart';
import 'core_view_toggles.dart';
import 'project_config.dart';
import 'project_tab_strip.dart';
import 'project_tabs_controller.dart';

import 'package:agent_fabric_client/core/app_log.dart';

class ProjectContextBar extends StatefulWidget {
  const ProjectContextBar({
    super.key,
    required this.controller,
    required this.catalog,
    required this.dock,
    required this.tabs,
    required this.onSelectProject,
    required this.onCloseProjectTab,
    required this.onOpenSettings,
    this.onResetLayout,
    this.onOpenSidebar,
    this.onToggleCore,
  });

  final ChatController controller;
  final CatalogClient catalog;
  final DockLayoutController dock;
  final ProjectTabsController tabs;
  final ValueChanged<String> onSelectProject;
  final ValueChanged<String> onCloseProjectTab;
  final VoidCallback onOpenSettings;
  final VoidCallback? onResetLayout;
  final VoidCallback? onOpenSidebar;
  final ValueChanged<String>? onToggleCore;

  @override
  State<ProjectContextBar> createState() => _ProjectContextBarState();
}

class _ProjectContextBarState extends State<ProjectContextBar> {
  String? _loadedFor;
  String _environment = 'Environment';
  Map<String, dynamic> _resolved = const {};
  int _loadGen = 0;

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_onController);
    _onController();
  }

  @override
  void didUpdateWidget(covariant ProjectContextBar oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller) {
      oldWidget.controller.removeListener(_onController);
      widget.controller.addListener(_onController);
      _onController();
    }
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onController);
    super.dispose();
  }

  void _onController() {
    final id = widget.controller.selectedProjectId;
    if (id == _loadedFor) {
      return;
    }
    _loadedFor = id;
    if (id == null) {
      setState(() {
        _environment = 'Environment';
        _resolved = const {};
      });
      return;
    }
    unawaited(
      _load(id).catchError((Object e, StackTrace s) {
        AppLog.record('context bar load: $e', s);
      }),
    );
  }

  Future<void> _load(String projectId) async {
    final gen = ++_loadGen;
    Map<String, dynamic> resolved = const {};
    List<Resource> resources = const [];
    try {
      resolved = await widget.catalog.resolvedEnvironment(projectId);
    } on Object catch (e, s) {
      AppLog.record('context bar resolvedEnvironment: $e', s);
    }
    try {
      resources = await widget.catalog.listResources();
    } on Object catch (e, s) {
      AppLog.record('context bar listResources: $e', s);
    }
    if (!mounted ||
        gen != _loadGen ||
        widget.controller.selectedProjectId != projectId) {
      return;
    }
    final summary = ProjectConfigSummary.fromSettings(
      widget.controller.selectedProject?.settings ?? const {},
    );
    Resource? resource;
    final resourceId = summary.resourceId;
    if (resourceId != null) {
      for (final item in resources) {
        if (item.id == resourceId) {
          resource = item;
          break;
        }
      }
    }
    setState(() {
      _resolved = resolved;
      _environment = environmentLabel(
        resolved: resolved,
        resourceName: resource?.name,
        resourceKind: resource?.kind,
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return ListenableBuilder(
      listenable: widget.controller,
      builder: (context, _) {
        final project = widget.controller.selectedProject;
        final summary = ProjectConfigSummary.fromSettings(
          project?.settings ?? const {},
        );
        return Padding(
          padding: const EdgeInsets.all(8),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              ProjectTabStrip(
                controller: widget.controller,
                tabs: widget.tabs,
                onSelectProject: widget.onSelectProject,
                onCloseProjectTab: widget.onCloseProjectTab,
                onOpenSidebar: widget.onOpenSidebar,
                onResetLayout: widget.onResetLayout,
              ),
              DecoratedBox(
                decoration: BoxDecoration(
                  color: tokens.surfaceRaised,
                  borderRadius: const BorderRadius.only(
                    topRight: Radius.circular(DesignTokens.radiusLg),
                    bottomLeft: Radius.circular(DesignTokens.radiusLg),
                    bottomRight: Radius.circular(DesignTokens.radiusLg),
                  ),
                ),
                child: SizedBox(
                  width: double.infinity,
                  height: DesignTokens.toolbarHeight,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 10),
                    child: Row(
                      children: [
                        Expanded(
                          child: ListView(
                            scrollDirection: Axis.horizontal,
                            padding: const EdgeInsets.symmetric(vertical: 10),
                            children: [
                              _ContextChip(
                                icon: Icons.inventory_2_outlined,
                                iconColor: tokens.secondary,
                                label: _environment,
                                chipKey: const Key('context-environment'),
                                onOpenSettings: widget.onOpenSettings,
                                details: _environmentDetails(),
                              ),
                              const SizedBox(width: 10),
                              SizedBox(
                                width: 240,
                                child: _ChipFrame(
                                  icon: Icons.view_in_ar_outlined,
                                  child: ModelPicker(
                                    controller: widget.controller,
                                    openUpward: false,
                                    activatorKey: const Key(
                                      'context-model-picker',
                                    ),
                                  ),
                                ),
                              ),
                              const SizedBox(width: 10),
                              _ContextChip(
                                icon: Icons.build_outlined,
                                label: 'Tools',
                                count: summary.tools.length,
                                chipKey: const Key('context-tools'),
                                onOpenSettings: widget.onOpenSettings,
                                details: summary.tools.isEmpty
                                    ? const [
                                        'No tools stored for this project.',
                                      ]
                                    : summary.tools,
                                footnote:
                                    'Stored only; tools are not filtered yet.',
                              ),
                              const SizedBox(width: 10),
                              _ContextChip(
                                icon: Icons.share_outlined,
                                label: 'MCP',
                                count: summary.mcpServers.length,
                                chipKey: const Key('context-mcp'),
                                onOpenSettings: widget.onOpenSettings,
                                details: summary.mcpServers.isEmpty
                                    ? const [
                                        'No MCP servers stored for this project.',
                                      ]
                                    : summary.mcpServers,
                                footnote: 'Coming soon — extra MCP servers are stored, not executed.',
                              ),
                            ],
                          ),
                        ),
                        CoreViewToggles(
                          dock: widget.dock,
                          onToggle: widget.onToggleCore,
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  List<String> _environmentDetails() {
    final lines = <String>[_environment];
    final root = _resolved['workspaceRoot'];
    if (root is String && root.isNotEmpty) {
      lines.add('Workspace root: $root');
    }
    lines.add('Changes are saved in Settings.');
    return lines;
  }
}

const double _chipHeight = 36;

BoxDecoration _chipDecoration(DesignTokens tokens) {
  return BoxDecoration(
    color: tokens.surface,
    border: Border.all(color: tokens.border),
    borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
  );
}

class _ChipFrame extends StatelessWidget {
  const _ChipFrame({required this.icon, required this.child});

  final IconData icon;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return DecoratedBox(
      decoration: _chipDecoration(tokens),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12),
        child: SizedBox(
          height: _chipHeight,
          child: Row(
            children: [
              Icon(icon, size: 18, color: tokens.textSecondary),
              const SizedBox(width: 8),
              Expanded(child: child),
            ],
          ),
        ),
      ),
    );
  }
}

class _ContextChip extends StatelessWidget {
  const _ContextChip({
    required this.icon,
    required this.label,
    required this.chipKey,
    required this.details,
    required this.onOpenSettings,
    this.iconColor,
    this.count,
    this.footnote,
  });

  final IconData icon;
  final Color? iconColor;
  final String label;
  final int? count;
  final Key chipKey;
  final List<String> details;
  final VoidCallback onOpenSettings;
  final String? footnote;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return PopupMenuButton<String>(
      key: chipKey,
      tooltip: label,
      onSelected: (value) {
        if (value == 'settings') {
          onOpenSettings();
        }
      },
      itemBuilder: (context) => [
        for (final line in details)
          PopupMenuItem<String>(
            enabled: false,
            child: Text(line, style: tokens.bodySm()),
          ),
        if (footnote != null)
          PopupMenuItem<String>(
            enabled: false,
            child: Text(
              footnote!,
              style: tokens.caption().copyWith(color: tokens.textMuted),
            ),
          ),
        const PopupMenuItem<String>(
          value: 'settings',
          child: Text('Edit in Settings'),
        ),
      ],
      child: DecoratedBox(
        decoration: _chipDecoration(tokens),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: SizedBox(
            height: _chipHeight,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon, size: 18, color: iconColor ?? tokens.textSecondary),
                const SizedBox(width: 8),
                Text(
                  label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: tokens.labelMd().copyWith(color: tokens.textPrimary),
                ),
                if (count != null) ...[
                  const SizedBox(width: 8),
                  _CountBadge(count: count!),
                ],
                const SizedBox(width: 6),
                Icon(Icons.expand_more, size: 16, color: tokens.textMuted),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _CountBadge extends StatelessWidget {
  const _CountBadge({required this.count});

  final int count;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return DecoratedBox(
      decoration: BoxDecoration(
        color: tokens.surfaceActive,
        borderRadius: BorderRadius.circular(DesignTokens.radiusXs),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
        child: Text(
          '$count',
          style: tokens.caption().copyWith(color: tokens.textPrimary),
        ),
      ),
    );
  }
}
