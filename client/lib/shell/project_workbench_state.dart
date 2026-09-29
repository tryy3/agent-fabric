import 'package:flutter/foundation.dart';

import '../catalog/catalog_client.dart';
import '../dock/dock_layout_controller.dart';
import '../workspace/project_files_controller.dart';
import 'project_document_ref.dart';
import 'workbench_state_store.dart';

/// Live dock + project files for one open project tab.
///
/// Parked sessions stay fully hydrated so switching tabs is a pointer swap,
/// not an async restore. Persist to [WorkbenchStateStore] / dock prefs only for
/// cold start after the process exits or the tab is closed.
class ProjectWorkbenchState {
  ProjectWorkbenchState({
    required this.projectId,
    required this.projectFiles,
    required this.dock,
    required this.itemWidgets,
  });

  final String projectId;
  final ProjectFilesController projectFiles;
  final DockLayoutController dock;
  final DockItemWidgets itemWidgets;

  bool _disposed = false;

  List<ProjectDocumentRef> documentRefs() {
    final focused = projectFiles.focusedView;
    return [
      for (final view in projectFiles.openViews)
        ProjectDocumentRef(
          path: view.path,
          appId: view.appId,
          viewMode: projectFiles.viewModeFor(view.viewId),
          focused: focused?.viewId == view.viewId,
        ),
    ];
  }

  /// Writes layout, open docs, and explorer expansion for the next cold start.
  Future<void> persist(WorkbenchStateStore store) async {
    if (_disposed) {
      return;
    }
    await store.rememberDocuments(projectId, documentRefs());
    await store.rememberExpansion(projectId, projectFiles.expansionSnapshot());
    await dock.persist();
  }

  void dispose() {
    if (_disposed) {
      return;
    }
    _disposed = true;
    projectFiles.onViewOpened = null;
    projectFiles.onViewClosed = null;
    projectFiles.onDocumentsCleared = null;
    projectFiles.dispose();
    dock.dispose();
  }
}

/// In-memory parked sessions keyed by project id.
class ProjectSessionStore {
  final Map<String, ProjectWorkbenchState> _byProject = {};

  ProjectWorkbenchState? operator [](String projectId) => _byProject[projectId];

  bool contains(String projectId) => _byProject.containsKey(projectId);

  void put(ProjectWorkbenchState session) {
    final previous = _byProject[session.projectId];
    if (previous != null && !identical(previous, session)) {
      previous.dispose();
    }
    _byProject[session.projectId] = session;
  }

  ProjectWorkbenchState? remove(String projectId) =>
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
ProjectWorkbenchState createProjectSession({
  required String projectId,
  required CatalogClient catalog,
  required DockItemWidgets Function(ProjectFilesController projectFiles)
  itemWidgetsFor,
}) {
  final projectFiles = ProjectFilesController(catalog: catalog);
  final dock = DockLayoutController();
  final items = itemWidgetsFor(projectFiles);
  dock.resetToDefault(widgets: items);
  dock.layoutScope = projectId;
  return ProjectWorkbenchState(
    projectId: projectId,
    projectFiles: projectFiles,
    dock: dock,
    itemWidgets: items,
  );
}
