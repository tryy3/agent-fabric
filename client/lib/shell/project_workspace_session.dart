import 'package:flutter/foundation.dart';

import '../catalog/catalog_client.dart';
import '../dock/dock_layout_controller.dart';
import '../workspace/workspace_controller.dart';
import 'workspace_document_ref.dart';
import 'workspace_memory.dart';

/// Live dock + workspace for one open project tab.
///
/// Parked sessions stay fully hydrated so switching tabs is a pointer swap,
/// not an async restore. Persist to [WorkspaceMemory] / dock prefs only for
/// cold start after the process exits or the tab is closed.
class ProjectWorkspaceSession {
  ProjectWorkspaceSession({
    required this.projectId,
    required this.workspace,
    required this.dock,
    required this.itemWidgets,
  });

  final String projectId;
  final WorkspaceController workspace;
  final DockLayoutController dock;
  final DockItemWidgets itemWidgets;

  bool _disposed = false;

  List<WorkspaceDocumentRef> documentRefs() {
    final focused = workspace.focusedView;
    return [
      for (final view in workspace.openViews)
        WorkspaceDocumentRef(
          path: view.path,
          appId: view.appId,
          viewMode: workspace.viewModeFor(view.viewId),
          focused: focused?.viewId == view.viewId,
        ),
    ];
  }

  /// Writes layout, open docs, and explorer expansion for the next cold start.
  Future<void> persist(WorkspaceMemory memory) async {
    if (_disposed) {
      return;
    }
    await memory.rememberDocuments(projectId, documentRefs());
    await memory.rememberExpansion(projectId, workspace.expansionSnapshot());
    await dock.persist();
  }

  void dispose() {
    if (_disposed) {
      return;
    }
    _disposed = true;
    workspace.onViewOpened = null;
    workspace.onViewClosed = null;
    workspace.onDocumentsCleared = null;
    workspace.dispose();
    dock.dispose();
  }
}

/// In-memory parked sessions keyed by project id.
class ProjectSessionStore {
  final Map<String, ProjectWorkspaceSession> _byProject = {};

  ProjectWorkspaceSession? operator [](String projectId) =>
      _byProject[projectId];

  bool contains(String projectId) => _byProject.containsKey(projectId);

  void put(ProjectWorkspaceSession session) {
    final previous = _byProject[session.projectId];
    if (previous != null && !identical(previous, session)) {
      previous.dispose();
    }
    _byProject[session.projectId] = session;
  }

  ProjectWorkspaceSession? remove(String projectId) =>
      _byProject.remove(projectId);

  /// Drops sessions whose project tabs are gone.
  void retain(Set<String> projectIds) {
    final stale = [
      for (final id in _byProject.keys)
        if (!projectIds.contains(id)) id,
    ];
    for (final id in stale) {
      _byProject.remove(id)?.dispose();
    }
  }

  void disposeAll() {
    for (final session in _byProject.values) {
      session.dispose();
    }
    _byProject.clear();
  }

  @visibleForTesting
  int get length => _byProject.length;
}

/// Builds a session with default layout; cold restore fills docs afterward.
ProjectWorkspaceSession createProjectSession({
  required String projectId,
  required CatalogClient catalog,
  required DockItemWidgets Function(WorkspaceController workspace)
  itemWidgetsFor,
}) {
  final workspace = WorkspaceController(catalog: catalog);
  final dock = DockLayoutController();
  final items = itemWidgetsFor(workspace);
  dock.resetToDefault(widgets: items);
  dock.layoutScope = projectId;
  return ProjectWorkspaceSession(
    projectId: projectId,
    workspace: workspace,
    dock: dock,
    itemWidgets: items,
  );
}
