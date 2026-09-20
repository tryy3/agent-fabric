import 'package:material_ui/material_ui.dart';

import 'catalog/catalog_client.dart';
import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';
import 'chat/display_settings.dart';
import 'chat/thread_pane.dart';
import 'settings/appearance_settings.dart';
import 'settings/settings_page.dart';
import 'ui/connectivity_badge.dart';
import 'workspace/workspace_controller.dart';
import 'workspace/workspace_pane.dart';

class AppShell extends StatefulWidget {
  const AppShell({
    super.key,
    required this.controller,
    required this.displaySettings,
    required this.appearanceSettings,
    this.catalog,
  });

  final ChatController controller;
  final ChatDisplaySettings displaySettings;
  final AppearanceSettings appearanceSettings;
  final CatalogClient? catalog;

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  int _selectedIndex = 0;
  late final CatalogClient _catalog;
  late final bool _ownsCatalog;
  late final WorkspaceController _workspace;

  @override
  void initState() {
    super.initState();
    _ownsCatalog = widget.catalog == null;
    _catalog = widget.catalog ?? CatalogClient(baseUri: defaultCatalogBase);
    _workspace = WorkspaceController(catalog: _catalog);
    widget.controller.addListener(_syncProject);
    widget.controller.onAgentTurnCommitted = _workspace.refreshAfterAgentTurn;
    _syncProject();
  }

  @override
  void dispose() {
    widget.controller.removeListener(_syncProject);
    if (widget.controller.onAgentTurnCommitted ==
        _workspace.refreshAfterAgentTurn) {
      widget.controller.onAgentTurnCommitted = null;
    }
    _workspace.dispose();
    if (_ownsCatalog) {
      _catalog.close();
    }
    super.dispose();
  }

  void _syncProject() {
    final id = widget.controller.selectedProjectId;
    if (_workspace.projectId != id) {
      _workspace.setProjectId(id);
    }
  }

  void _toggleFiles() {
    if (MediaQuery.sizeOf(context).width < 720) {
      Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) => WorkspacePage(controller: _workspace),
        ),
      );
      return;
    }
    _workspace.togglePane();
    setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _selectedIndex,
            onDestinationSelected: (index) {
              setState(() => _selectedIndex = index);
              if (index == 0) {
                widget.controller.reloadAgents();
              }
            },
            labelType: NavigationRailLabelType.all,
            destinations: const [
              NavigationRailDestination(
                icon: Icon(Icons.chat),
                label: Text('Chat'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.settings),
                label: Text('Settings'),
              ),
            ],
            trailing: Padding(
              padding: const EdgeInsets.only(top: 16),
              child: AnimatedBuilder(
                animation: widget.controller,
                builder: (context, _) =>
                    ConnectivityBadge(status: widget.controller.status),
              ),
            ),
          ),
          const VerticalDivider(thickness: 1, width: 1),
          if (_selectedIndex == 0) ...[
            ThreadPane(controller: widget.controller),
            const VerticalDivider(thickness: 1, width: 1),
          ],
          Expanded(
            child: IndexedStack(
              index: _selectedIndex,
              children: [
                ChatScreen(
                  controller: widget.controller,
                  displaySettings: widget.displaySettings,
                  filesOpen: _workspace.paneOpen,
                  onToggleFiles: _toggleFiles,
                ),
                SettingsPage(
                  catalog: _catalog,
                  displaySettings: widget.displaySettings,
                  appearanceSettings: widget.appearanceSettings,
                ),
              ],
            ),
          ),
          if (_selectedIndex == 0)
            ListenableBuilder(
              listenable: _workspace,
              builder: (context, _) {
                if (!_workspace.paneOpen) {
                  return const SizedBox.shrink();
                }
                return Row(
                  children: [
                    const VerticalDivider(thickness: 1, width: 1),
                    SizedBox(
                      width: 360,
                      child: WorkspacePane(controller: _workspace),
                    ),
                  ],
                );
              },
            ),
        ],
      ),
    );
  }
}
