import 'dart:async';

import 'package:docking/docking.dart';
import 'package:material_ui/material_ui.dart';

import 'catalog/catalog_client.dart';
import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';
import 'chat/display_settings.dart';
import 'chat/thread_pane.dart';
import 'dock/dock_ids.dart';
import 'dock/dock_layout_controller.dart';
import 'dock/dock_view_body.dart';
import 'dock/files_dock_panel.dart';
import 'settings/appearance_settings.dart';
import 'settings/settings_page.dart';
import 'ui/connectivity_badge.dart';
import 'workspace/open_with.dart';
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
  int _railIndex = 0;
  late final CatalogClient _catalog;
  late final bool _ownsCatalog;
  late final WorkspaceController _workspace;
  late final DockLayoutController _dock;

  @override
  void initState() {
    super.initState();
    _ownsCatalog = widget.catalog == null;
    _catalog = widget.catalog ?? CatalogClient(baseUri: defaultCatalogBase);
    _workspace = WorkspaceController(catalog: _catalog);
    _dock = DockLayoutController();
    _workspace.onViewOpened = (view, {required bool toSide}) {
      _dock.openDocument(
        view: view,
        toSide: toSide,
        child: DockViewBody(controller: _workspace, view: view),
      );
    };
    _workspace.onViewClosed = (view) {
      _dock.closeDocument(DockIds.doc(view.path, view.appId));
    };
    _workspace.onDocumentsCleared = _dock.clearDocuments;
    widget.controller.addListener(_syncProject);
    widget.controller.onAgentTurnCommitted = _workspace.refreshAfterAgentTurn;
    _syncProject();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) {
        return;
      }
      unawaited(
        _dock.restore(
          widgets: DockItemWidgets(
            threads: ThreadPane(controller: widget.controller),
            files: FilesDockPanel(controller: _workspace),
            chat: ChatScreen(
              controller: widget.controller,
              displaySettings: widget.displaySettings,
            ),
          ),
        ),
      );
    });
  }

  @override
  void dispose() {
    widget.controller.removeListener(_syncProject);
    if (widget.controller.onAgentTurnCommitted ==
        _workspace.refreshAfterAgentTurn) {
      widget.controller.onAgentTurnCommitted = null;
    }
    _workspace.onViewOpened = null;
    _workspace.onViewClosed = null;
    _workspace.onDocumentsCleared = null;
    _workspace.dispose();
    _dock.dispose();
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

  void _onRail(int index) {
    if (index == 2 && MediaQuery.sizeOf(context).width < 720) {
      Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) => WorkspacePage(controller: _workspace),
        ),
      );
      return;
    }
    setState(() => _railIndex = index);
    switch (index) {
      case 0:
        widget.controller.reloadAgents();
        _dock.ensureCore(DockIds.chat);
      case 1:
        _dock.toggleCore(DockIds.threads);
      case 2:
        _dock.toggleCore(DockIds.files);
    }
  }

  void _onItemSelection(DockingItem item) {
    _dock.focusedItemId = item.id;
    final id = item.id;
    if (id is! String || !DockIds.isDoc(id)) {
      return;
    }
    final view = _openViewForDockId(id);
    if (view != null) {
      _workspace.focusView(view.viewId);
    }
  }

  void _onItemClose(DockingItem item) {
    final id = item.id;
    if (id is! String || !DockIds.isDoc(id)) {
      return;
    }
    final view = _openViewForDockId(id);
    if (view == null) {
      return;
    }
    _workspace.closeView(view.viewId);
  }

  OpenView? _openViewForDockId(String dockId) {
    for (final view in _workspace.openViews) {
      if (DockIds.doc(view.path, view.appId) == dockId) {
        return view;
      }
    }
    return null;
  }

  @override
  Widget build(BuildContext context) {
    final dividerColor = Theme.of(context).colorScheme.outlineVariant;
    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _railIndex,
            onDestinationSelected: _onRail,
            labelType: NavigationRailLabelType.all,
            destinations: const [
              NavigationRailDestination(
                icon: Icon(Icons.chat, key: Key('rail-chat')),
                label: Text('Chat'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.forum_outlined, key: Key('rail-threads')),
                label: Text('Threads'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.folder_outlined, key: Key('rail-files')),
                label: Text('Files'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.settings, key: Key('rail-settings')),
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
          Expanded(
            child: IndexedStack(
              index: _railIndex == 3 ? 1 : 0,
              children: [
                MultiSplitViewTheme(
                  data: MultiSplitViewThemeData(
                    dividerThickness: 4,
                    dividerPainter: DividerPainters.background(
                      animationEnabled: false,
                      color: dividerColor,
                    ),
                  ),
                  child: TabbedViewTheme(
                    data: TabbedViewThemeData(
                      menu: TabbedViewMenuThemeData(
                        dividerThickness: 4,
                        dividerColor: dividerColor,
                      ),
                    ),
                    child: Docking(
                      layout: _dock.layout,
                      onItemSelection: _onItemSelection,
                      onItemClose: _onItemClose,
                      itemCloseInterceptor: (_) => true,
                    ),
                  ),
                ),
                SettingsPage(
                  catalog: _catalog,
                  displaySettings: widget.displaySettings,
                  appearanceSettings: widget.appearanceSettings,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
