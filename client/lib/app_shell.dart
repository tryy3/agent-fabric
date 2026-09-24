import 'dart:async';

import 'package:docking/docking.dart';
import 'package:flutter/material.dart' as flutter_material;
import 'package:material_ui/material_ui.dart';

import 'catalog/catalog_client.dart';
import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';
import 'chat/display_settings.dart';
import 'chat/thread_pane.dart';
import 'dock/dock_chat_tab_status.dart';
import 'dock/dock_ids.dart';
import 'dock/dock_layout_controller.dart';
import 'dock/dock_tab_theme.dart';
import 'dock/dock_view_body.dart';
import 'dock/files_dock_panel.dart';
import 'settings/appearance_settings.dart';
import 'settings/settings_page.dart';
import 'ui/connectivity_badge.dart';
import 'workspace/file_document.dart';
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
  final DockChatTabUnread _chatUnread = DockChatTabUnread();
  final Set<FileDocument> _dirtyDocs = {};
  final Map<String, bool> _appliedDirty = {};
  bool _dirtySyncScheduled = false;
  bool _wasSending = false;
  dynamic _chatFocusSeen;
  String? _dirtyCloseViewId;

  @override
  void initState() {
    super.initState();
    _ownsCatalog = widget.catalog == null;
    _catalog = widget.catalog ?? CatalogClient(baseUri: defaultCatalogBase);
    _workspace = WorkspaceController(catalog: _catalog);
    _dock = DockLayoutController();
    _wasSending = widget.controller.sending;
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
    widget.controller.addListener(_onChatController);
    _workspace.addListener(_syncDirtyDockTabs);
    _dock.addListener(_onDockChanged);
    widget.controller.onAgentTurnCommitted = _workspace.refreshAfterAgentTurn;
    _syncProject();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) {
        return;
      }
      unawaited(_restoreDock());
    });
  }

  Future<void> _restoreDock() async {
    await _dock.restore(
      widgets: DockItemWidgets(
        threads: ThreadPane(controller: widget.controller),
        files: FilesDockPanel(controller: _workspace),
        chat: ChatScreen(
          controller: widget.controller,
          displaySettings: widget.displaySettings,
        ),
      ),
    );
    if (!mounted) return;
    _syncChatTab();
    _syncDirtyDockTabs();
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onChatController);
    _workspace.removeListener(_syncDirtyDockTabs);
    _dock.removeListener(_onDockChanged);
    for (final doc in _dirtyDocs) {
      doc.removeListener(_syncDirtyDockTabs);
    }
    _dirtyDocs.clear();
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

  void _onChatController() {
    _syncProject();
    if (widget.controller.sending == _wasSending) return;
    _syncChatTab();
  }

  void _onDockChanged() {
    if (_dock.focusedItemId == _chatFocusSeen) return;
    _chatFocusSeen = _dock.focusedItemId;
    _syncChatTab();
  }

  void _syncChatTab() {
    final sending = widget.controller.sending;
    syncChatDockTabStatus(
      dock: _dock,
      unread: _chatUnread,
      sending: sending,
      wasSending: _wasSending,
    );
    _wasSending = sending;
  }

  void _syncDirtyDockTabs() {
    _bindDirtyDocuments();
    if (_dirtySyncScheduled) return;
    _dirtySyncScheduled = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _dirtySyncScheduled = false;
      if (!mounted) return;
      _applyDirtyDockTabs();
    });
  }

  void _applyDirtyDockTabs() {
    final live = <String>{};
    for (final view in _workspace.openViews) {
      final id = DockIds.doc(view.path, view.appId);
      live.add(id);
      if (!_dock.hasItem(id)) continue;
      final dirty = _workspace.documentFor(view.path)?.isDirty ?? false;
      if (_appliedDirty[id] == dirty) continue;
      _appliedDirty[id] = dirty;
      _dock.setDocumentDirtyClose(
        id,
        dirty: dirty,
        onClose: () => _onDirtyTabClose(id),
      );
    }
    _appliedDirty.removeWhere((id, _) => !live.contains(id));
  }

  void _bindDirtyDocuments() {
    final live = <FileDocument>{};
    for (final view in _workspace.openViews) {
      final doc = _workspace.documentFor(view.path);
      if (doc == null) continue;
      live.add(doc);
      if (_dirtyDocs.add(doc)) {
        doc.addListener(_syncDirtyDockTabs);
      }
    }
    for (final doc in _dirtyDocs.difference(live).toList()) {
      doc.removeListener(_syncDirtyDockTabs);
      _dirtyDocs.remove(doc);
    }
  }

  /// Dirty button stands in for the package close icon. See [invokeDirtyDockTabClose].
  void _onDirtyTabClose(String dockId) {
    invokeDirtyDockTabClose(
      item: _dock.layout.findDockingItem(dockId),
      interceptItemClose: _interceptItemClose,
      onItemClose: _onItemClose,
    );
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
    _chatFocusSeen = item.id;
    _syncChatTab();
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

  /// Blocks a dirty document close until Save, Discard, or Cancel.
  ///
  /// The docking interceptor is synchronous, so a dirty document returns
  /// false and the dialog closes the view afterward. Clean documents and
  /// cores return true and close immediately.
  bool _interceptItemClose(DockingItem item) {
    final id = item.id;
    if (id is! String || !DockIds.isDoc(id)) {
      return true;
    }
    final view = _openViewForDockId(id);
    if (view == null) {
      return true;
    }
    final doc = _workspace.documentFor(view.path);
    if (doc == null || !doc.isDirty) {
      return true;
    }
    if (_dirtyCloseViewId != null) {
      return false;
    }
    _dirtyCloseViewId = view.viewId;
    unawaited(_confirmDirtyClose(view));
    return false;
  }

  Future<void> _confirmDirtyClose(OpenView view) async {
    try {
      await confirmDirtyViewClose(context, _workspace, view);
    } finally {
      if (_dirtyCloseViewId == view.viewId) {
        _dirtyCloseViewId = null;
      }
    }
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
                    data: buildDockTabTheme(
                      _flutterColorScheme(Theme.of(context).colorScheme),
                    ),
                    child: Docking(
                      layout: _dock.layout,
                      onItemSelection: _onItemSelection,
                      onItemClose: _onItemClose,
                      itemCloseInterceptor: _interceptItemClose,
                      maximizableItem: false,
                      maximizableTab: false,
                      maximizableTabsArea: false,
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

/// Dirty-tab button path.
///
/// The package close icon runs [interceptItemClose], then removes the item
/// and calls [onItemClose] only when the interceptor returns true. This
/// button replaces that icon. A false result means confirm-and-close already
/// started, so the workspace removes the item once. A true result (the
/// document was saved before the click) closes through [onItemClose] only.
void invokeDirtyDockTabClose({
  required DockingItem? item,
  required bool Function(DockingItem item) interceptItemClose,
  required void Function(DockingItem item) onItemClose,
}) {
  if (item == null) return;
  if (!interceptItemClose(item)) return;
  onItemClose(item);
}

/// Maps the shell `material_ui` scheme onto Flutter's `ColorScheme`.
/// Dock chrome and its unit test use the Flutter type.
flutter_material.ColorScheme _flutterColorScheme(ColorScheme scheme) {
  return flutter_material.ColorScheme(
    brightness: scheme.brightness,
    primary: scheme.primary,
    onPrimary: scheme.onPrimary,
    primaryContainer: scheme.primaryContainer,
    onPrimaryContainer: scheme.onPrimaryContainer,
    secondary: scheme.secondary,
    onSecondary: scheme.onSecondary,
    secondaryContainer: scheme.secondaryContainer,
    onSecondaryContainer: scheme.onSecondaryContainer,
    tertiary: scheme.tertiary,
    onTertiary: scheme.onTertiary,
    tertiaryContainer: scheme.tertiaryContainer,
    onTertiaryContainer: scheme.onTertiaryContainer,
    error: scheme.error,
    onError: scheme.onError,
    errorContainer: scheme.errorContainer,
    onErrorContainer: scheme.onErrorContainer,
    surface: scheme.surface,
    onSurface: scheme.onSurface,
    surfaceDim: scheme.surfaceDim,
    surfaceBright: scheme.surfaceBright,
    surfaceContainerLowest: scheme.surfaceContainerLowest,
    surfaceContainerLow: scheme.surfaceContainerLow,
    surfaceContainer: scheme.surfaceContainer,
    surfaceContainerHigh: scheme.surfaceContainerHigh,
    surfaceContainerHighest: scheme.surfaceContainerHighest,
    onSurfaceVariant: scheme.onSurfaceVariant,
    outline: scheme.outline,
    outlineVariant: scheme.outlineVariant,
    shadow: scheme.shadow,
    scrim: scheme.scrim,
    inverseSurface: scheme.inverseSurface,
    onInverseSurface: scheme.onInverseSurface,
    inversePrimary: scheme.inversePrimary,
    surfaceTint: scheme.surfaceTint,
  );
}
