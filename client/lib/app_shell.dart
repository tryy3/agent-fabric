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
import 'shell/project_context_bar.dart';
import 'shell/project_sidebar.dart';
import 'shell/project_tabs_controller.dart';
import 'shell/project_workspace_session.dart';
import 'shell/workspace_memory.dart';
import 'ui/theme/design_tokens.dart';
import 'workspace/file_document.dart';
import 'workspace/open_with.dart';
import 'workspace/workspace_controller.dart';
import 'workspace/workspace_pane.dart';

import 'package:agent_fabric_client/core/app_log.dart';

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
  bool _showSettings = false;
  late final CatalogClient _catalog;
  late final bool _ownsCatalog;
  late final ProjectTabsController _tabs;
  late final WorkspaceMemory _memory;
  late final ProjectSessionStore _sessions;
  late final Widget _threadsBody;
  late final Widget _chatBody;
  late final WorkspaceController _emptyWorkspace;
  late final DockLayoutController _emptyDock;
  late final DockItemWidgets _emptyItems;
  final GlobalKey<ScaffoldState> _scaffoldKey = GlobalKey<ScaffoldState>();
  ProjectWorkspaceSession? _active;
  String? _layoutProject;
  int _handoffGen = 0;
  final DockChatTabUnread _chatUnread = DockChatTabUnread();
  final Set<FileDocument> _dirtyDocs = {};
  final Map<String, bool> _appliedDirty = {};
  bool _dirtySyncScheduled = false;
  bool _wasSending = false;
  dynamic _chatFocusSeen;
  String? _dirtyCloseViewId;

  WorkspaceController? get _workspace => _active?.workspace;
  DockLayoutController get _dock => _active?.dock ?? _emptyDock;

  @override
  void initState() {
    super.initState();
    _ownsCatalog = widget.catalog == null;
    _catalog = widget.catalog ?? CatalogClient(baseUri: defaultCatalogBase);
    _memory = WorkspaceMemory();
    _sessions = ProjectSessionStore();
    _tabs = ProjectTabsController(memory: _memory);
    widget.controller.preferredThread = _memory.lastThread;
    widget.controller.preferredProject = _memory.lastActiveProject;
    unawaited(
      _tabs.restore().catchError((Object e, StackTrace s) {
        AppLog.record('tabs restore: $e', s);
      }),
    );
    _threadsBody = DockCardBody(
      child: ThreadPane(controller: widget.controller),
    );
    _chatBody = DockCardBody(
      child: ChatScreen(
        controller: widget.controller,
        displaySettings: widget.displaySettings,
      ),
    );
    _emptyWorkspace = WorkspaceController(catalog: _catalog);
    _emptyItems = DockItemWidgets(
      threads: _threadsBody,
      files: DockCardBody(child: FilesDockPanel(controller: _emptyWorkspace)),
      chat: _chatBody,
    );
    _emptyDock = DockLayoutController()..resetToDefault(widgets: _emptyItems);
    _layoutProject = widget.controller.selectedProjectId;
    _wasSending = widget.controller.sending;
    widget.controller.addListener(_onChatController);
    // Seed a live session immediately when the project is already known so the
    // first frame shows Files/Chat instead of waiting on the post-frame handoff.
    final initial = _layoutProject;
    if (initial != null) {
      final session = createProjectSession(
        projectId: initial,
        catalog: _catalog,
        itemWidgetsFor: _itemsFor,
      );
      _active = session;
      _wireSession(session);
    }
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) {
        return;
      }
      unawaited(
        _handoffProject(null, _layoutProject).catchError((
          Object e,
          StackTrace s,
        ) {
          AppLog.record('initial handoff: $e', s);
        }),
      );
    });
  }

  DockItemWidgets _itemsFor(WorkspaceController workspace) {
    return DockItemWidgets(
      threads: _threadsBody,
      files: DockCardBody(child: FilesDockPanel(controller: workspace)),
      chat: _chatBody,
    );
  }

  void _wireSession(ProjectWorkspaceSession session) {
    final workspace = session.workspace;
    final dock = session.dock;
    workspace.onViewOpened = (view, {required bool toSide}) {
      dock.openDocument(
        view: view,
        toSide: toSide,
        child: _documentChild(workspace, view),
      );
    };
    workspace.onViewClosed = (view) {
      dock.closeDocument(DockIds.doc(view.path, view.appId));
    };
    workspace.onDocumentsCleared = dock.clearDocuments;
    workspace.addListener(_syncDirtyDockTabs);
    dock.addListener(_onDockChanged);
    widget.controller.onAgentTurnCommitted = workspace.refreshAfterAgentTurn;
  }

  void _unwireSession(ProjectWorkspaceSession? session) {
    if (session == null) {
      return;
    }
    final workspace = session.workspace;
    workspace.removeListener(_syncDirtyDockTabs);
    session.dock.removeListener(_onDockChanged);
    if (widget.controller.onAgentTurnCommitted ==
        workspace.refreshAfterAgentTurn) {
      widget.controller.onAgentTurnCommitted = null;
    }
    workspace.onViewOpened = null;
    workspace.onViewClosed = null;
    workspace.onDocumentsCleared = null;
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onChatController);
    _unwireSession(_active);
    for (final doc in _dirtyDocs) {
      doc.removeListener(_syncDirtyDockTabs);
    }
    _dirtyDocs.clear();
    if (widget.controller.preferredThread != null) {
      widget.controller.preferredThread = null;
    }
    if (widget.controller.preferredProject != null) {
      widget.controller.preferredProject = null;
    }
    _active?.dispose();
    _active = null;
    _sessions.disposeAll();
    _emptyDock.dispose();
    _emptyWorkspace.dispose();
    _tabs.dispose();
    if (_ownsCatalog) {
      _catalog.close();
    }
    super.dispose();
  }

  Widget _documentChild(WorkspaceController workspace, OpenView view) {
    return DockCardBody(
      child: DockViewBody(controller: workspace, view: view),
    );
  }

  Widget _documentBuilder(WorkspaceController workspace, dynamic id) {
    final parsed = DockIds.parseDoc(id);
    if (parsed == null) {
      return const SizedBox.shrink();
    }
    final view = workspace.findView(parsed.path, parsed.appId);
    if (view == null) {
      return const SizedBox.shrink();
    }
    return _documentChild(workspace, view);
  }

  /// Parks the leaving live session and activates [to] instantly when it is
  /// already open. Cold start (first visit / after restart) restores from disk.
  Future<void> _handoffProject(String? from, String? to) async {
    final gen = ++_handoffGen;

    final leaving = _active;
    if (leaving != null && from != null && leaving.projectId == from) {
      _unwireSession(leaving);
      if (_tabs.isOpen(from)) {
        _sessions.put(leaving);
        // Disk persist is for cold start only; do not block the swap.
        unawaited(
          leaving.persist(_memory).catchError((Object e, StackTrace s) {
            AppLog.record('park persist: $e', s);
          }),
        );
      } else {
        unawaited(
          () async {
            await leaving.persist(_memory);
            leaving.dispose();
          }().catchError((Object e, StackTrace s) {
            AppLog.record('leave persist: $e', s);
          }),
        );
      }
      _active = null;
    }

    if (!mounted || gen != _handoffGen) {
      return;
    }

    if (to == null) {
      _active = null;
      _emptyDock.resetToDefault(widgets: _emptyItems);
      if (mounted) {
        setState(() {});
      }
      _syncChatTab();
      _syncDirtyDockTabs();
      return;
    }

    // Already on this live session (e.g. seeded in initState) — cold restore only.
    if (_active?.projectId == to) {
      await _coldRestore(_active!, gen);
      return;
    }

    final parked = _sessions.remove(to);
    if (parked != null) {
      _activateSession(parked);
      return;
    }

    // First open this session (or cold start): build fresh, then restore.
    final session = createProjectSession(
      projectId: to,
      catalog: _catalog,
      itemWidgetsFor: _itemsFor,
    );
    _activateSession(session);
    await _coldRestore(session, gen);
  }

  void _activateSession(ProjectWorkspaceSession session) {
    _unwireSession(_active);
    // If we were showing a different live session that wasn't parked, drop it.
    final previous = _active;
    if (previous != null &&
        previous.projectId != session.projectId &&
        !_sessions.contains(previous.projectId) &&
        !_tabs.isOpen(previous.projectId)) {
      previous.dispose();
    }
    _active = session;
    session.workspace.projectName = widget.controller.selectedProject?.name;
    _wireSession(session);
    if (mounted) {
      setState(() {});
    }
    _syncChatTab();
    _syncDirtyDockTabs();
  }

  Future<void> _coldRestore(ProjectWorkspaceSession session, int gen) async {
    final projectId = session.projectId;
    final expanded = await _memory.expansion(projectId);
    final refs = await _memory.documents(projectId);
    if (!mounted ||
        gen != _handoffGen ||
        !identical(_active, session) ||
        widget.controller.selectedProjectId != projectId) {
      return;
    }

    if (session.workspace.projectId != projectId) {
      await session.workspace.setProjectId(
        projectId,
        restoreExpanded: expanded,
        notifyDocumentsCleared: false,
      );
    }
    if (!mounted || gen != _handoffGen || !identical(_active, session)) {
      return;
    }

    await session.workspace.restoreViews(refs, notifyDock: false);
    if (!mounted || gen != _handoffGen || !identical(_active, session)) {
      return;
    }

    await session.dock.restore(
      widgets: session.itemWidgets,
      projectId: projectId,
      documentBuilder: (id) => _documentBuilder(session.workspace, id),
    );
    if (!mounted || gen != _handoffGen || !identical(_active, session)) {
      return;
    }

    _syncDocumentsToDock(session);
    if (mounted) {
      setState(() {});
    }
    _syncChatTab();
    _syncDirtyDockTabs();
  }

  void _syncDocumentsToDock(ProjectWorkspaceSession session) {
    final workspace = session.workspace;
    final dock = session.dock;
    final openIds = <String>{};
    for (final view in workspace.openViews) {
      final id = DockIds.doc(view.path, view.appId);
      openIds.add(id);
      final child = _documentChild(workspace, view);
      if (dock.hasItem(id)) {
        dock.bindDocument(dockId: id, name: view.tabLabel, child: child);
      } else {
        dock.openDocument(view: view, child: child);
      }
    }
    for (final id in dock.documentIds()) {
      if (!openIds.contains(id)) {
        dock.closeDocument(id);
      }
    }
    final focused = workspace.focusedView;
    if (focused != null) {
      final id = DockIds.doc(focused.path, focused.appId);
      if (dock.hasItem(id)) {
        dock.focusedItemId = id;
        dock.openDocument(
          view: focused,
          child: _documentChild(workspace, focused),
        );
      }
    }
  }

  void _onChatController() {
    final project = widget.controller.selectedProjectId;
    final thread = widget.controller.selectedThreadId;
    if (project != null && thread != null) {
      unawaited(
        _memory.rememberThread(project, thread).catchError((
          Object e,
          StackTrace s,
        ) {
          AppLog.record('rememberThread: $e', s);
        }),
      );
    }
    _syncProjectTabs(project);
    if (project != _layoutProject) {
      final from = _layoutProject;
      _layoutProject = project;
      if (project != null) {
        unawaited(
          _memory.rememberActiveProject(project).catchError((
            Object e,
            StackTrace s,
          ) {
            AppLog.record('rememberActiveProject: $e', s);
          }),
        );
      } else {
        unawaited(
          _memory.forgetActiveProject().catchError((Object e, StackTrace s) {
            AppLog.record('forgetActiveProject: $e', s);
          }),
        );
      }
      unawaited(
        _handoffProject(from, project).catchError((Object e, StackTrace s) {
          AppLog.record('project handoff: $e', s);
        }),
      );
    } else {
      _active?.workspace.projectName = widget.controller.selectedProject?.name;
    }
    if (widget.controller.sending == _wasSending) return;
    _syncChatTab();
  }

  void _syncProjectTabs(String? project) {
    if (project != null) {
      _tabs.open(project);
    }
    final projects = widget.controller.projects;
    if (projects.isNotEmpty) {
      final ids = {for (final entry in projects) entry.id};
      _tabs.retainProjects(ids);
      final keep = {
        ..._tabs.openProjectIds,
        if (_layoutProject != null) _layoutProject!,
      };
      _sessions.retain(keep);
    }
  }

  Future<void> _selectProject(String id) async {
    if (id == widget.controller.selectedProjectId) {
      return;
    }
    await widget.controller.selectProject(id);
  }

  Future<void> _closeProjectTab(String id) async {
    final neighbor = _tabs.close(id);
    final parked = _sessions.remove(id);
    if (parked != null) {
      unawaited(
        () async {
          await parked.persist(_memory);
          parked.dispose();
        }().catchError((Object e, StackTrace s) {
          AppLog.record('close-tab persist: $e', s);
        }),
      );
    }
    if (id != widget.controller.selectedProjectId) {
      return;
    }
    // Active tab: handoff will persist+dispose because tab is no longer open.
    if (neighbor == null) {
      await widget.controller.clearProjectSelection();
      return;
    }
    await widget.controller.selectProject(neighbor);
  }

  void _openWorkspace() {
    final leavingSettings = _showSettings;
    if (leavingSettings) {
      setState(() => _showSettings = false);
      unawaited(
        widget.controller.reloadAgents().catchError((Object e, StackTrace s) {
          AppLog.record('reloadAgents: $e', s);
        }),
      );
    }
    _dock.ensureCore(DockIds.chat);
  }

  void _openSettings() {
    setState(() => _showSettings = true);
  }

  void _resetLayout() {
    final session = _active;
    if (session != null) {
      session.dock.layoutScope = session.projectId;
      session.dock.resetToDefault(widgets: session.itemWidgets);
      return;
    }
    _emptyDock.resetToDefault(widgets: _emptyItems);
  }

  void _onToggleCore(String coreId) {
    final workspace = _workspace;
    if (coreId == DockIds.files &&
        workspace != null &&
        MediaQuery.sizeOf(context).width < 720) {
      Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) => WorkspacePage(controller: workspace),
        ),
      );
      return;
    }
    _dock.toggleCore(coreId);
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
    final workspace = _workspace;
    if (workspace == null) {
      _appliedDirty.clear();
      return;
    }
    final live = <String>{};
    for (final view in workspace.openViews) {
      final id = DockIds.doc(view.path, view.appId);
      live.add(id);
      if (!_dock.hasItem(id)) continue;
      final dirty = workspace.documentFor(view.path)?.isDirty ?? false;
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
    final workspace = _workspace;
    final live = <FileDocument>{};
    if (workspace != null) {
      for (final view in workspace.openViews) {
        final doc = workspace.documentFor(view.path);
        if (doc == null) continue;
        live.add(doc);
        if (_dirtyDocs.add(doc)) {
          doc.addListener(_syncDirtyDockTabs);
        }
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

  void _onItemSelection(DockingItem item) {
    _dock.focusedItemId = item.id;
    _chatFocusSeen = item.id;
    _syncChatTab();
    final id = item.id;
    if (id is! String || !DockIds.isDoc(id)) {
      return;
    }
    final view = _openViewForDockId(id);
    final workspace = _workspace;
    if (view != null && workspace != null) {
      workspace.focusView(view.viewId);
    }
  }

  void _onItemClose(DockingItem item) {
    final id = item.id;
    if (id is! String || !DockIds.isDoc(id)) {
      return;
    }
    final view = _openViewForDockId(id);
    final workspace = _workspace;
    if (view == null || workspace == null) {
      return;
    }
    unawaited(
      workspace.closeView(view.viewId).catchError((Object e, StackTrace s) {
        AppLog.record('closeView: $e', s);
      }),
    );
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
    final workspace = _workspace;
    if (view == null || workspace == null) {
      return true;
    }
    final doc = workspace.documentFor(view.path);
    if (doc == null || !doc.isDirty) {
      return true;
    }
    if (_dirtyCloseViewId != null) {
      return false;
    }
    _dirtyCloseViewId = view.viewId;
    unawaited(
      _confirmDirtyClose(view).catchError((Object e, StackTrace s) {
        AppLog.record('confirmDirtyClose: $e', s);
      }),
    );
    return false;
  }

  Future<void> _confirmDirtyClose(OpenView view) async {
    final workspace = _workspace;
    if (workspace == null) {
      _dirtyCloseViewId = null;
      return;
    }
    try {
      await confirmDirtyViewClose(context, workspace, view);
    } finally {
      if (_dirtyCloseViewId == view.viewId) {
        _dirtyCloseViewId = null;
      }
    }
  }

  OpenView? _openViewForDockId(String dockId) {
    final workspace = _workspace;
    if (workspace == null) {
      return null;
    }
    for (final view in workspace.openViews) {
      if (DockIds.doc(view.path, view.appId) == dockId) {
        return view;
      }
    }
    return null;
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return LayoutBuilder(
      builder: (context, constraints) {
        final wide = constraints.maxWidth >= 1024;
        final sidebar = ProjectSidebar(
          controller: widget.controller,
          settingsActive: _showSettings,
          onOpenSettings: _openSettings,
          onOpenWorkspace: _openWorkspace,
        );
        return Scaffold(
          key: _scaffoldKey,
          backgroundColor: tokens.background,
          drawer: wide
              ? null
              : Drawer(width: DesignTokens.sidebarWidth, child: sidebar),
          body: Row(
            children: [
              if (wide)
                SizedBox(width: DesignTokens.sidebarWidth, child: sidebar),
              Expanded(
                child: IndexedStack(
                  index: _showSettings ? 1 : 0,
                  children: [
                    Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        ProjectContextBar(
                          controller: widget.controller,
                          catalog: _catalog,
                          dock: _dock,
                          tabs: _tabs,
                          onSelectProject: _selectProject,
                          onCloseProjectTab: _closeProjectTab,
                          onOpenSettings: _openSettings,
                          onResetLayout: _resetLayout,
                          onOpenSidebar: wide
                              ? null
                              : () => _scaffoldKey.currentState?.openDrawer(),
                          onToggleCore: _onToggleCore,
                        ),
                        Expanded(
                          child: Padding(
                            padding: const EdgeInsets.fromLTRB(8, 0, 8, 8),
                            child: MultiSplitViewTheme(
                              data: MultiSplitViewThemeData(
                                dividerThickness: 8,
                                dividerPainter: DividerPainters.grooved1(
                                  animationEnabled: false,
                                  color: Colors.transparent,
                                  highlightedColor: tokens.borderStrong,
                                  thickness: 2,
                                  highlightedThickness: 2,
                                ),
                              ),
                              child: TabbedViewTheme(
                                data: buildDockTabTheme(
                                  _flutterColorScheme(
                                    Theme.of(context).colorScheme,
                                  ),
                                ),
                                child: Docking(
                                  key: ValueKey(_active?.projectId ?? 'empty'),
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
                          ),
                        ),
                      ],
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
      },
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
