import 'dart:async';

import 'package:flutter/foundation.dart';

import 'workspace_memory.dart';

import 'package:agent_fabric_client/core/app_log.dart';

/// Open project tabs shown in the workspace tab strip.
///
/// The list order is user-visible. Which tab is active is *not* stored here:
/// the active project stays the chat controller's selected project so the
/// workspace swap logic keeps a single source of truth. Every mutation
/// persists through [WorkspaceMemory] so the strip survives a restart.
class ProjectTabsController extends ChangeNotifier {
  ProjectTabsController({WorkspaceMemory? memory}) : _memory = memory;

  final WorkspaceMemory? _memory;
  final List<String> _open = [];

  /// Ids that left the strip this session. A [restore] that is still in
  /// flight must not resurrect a tab the user already closed.
  final Set<String> _removed = {};

  /// Hydration owns the store: tabs opened or closed while [restore] is
  /// loading must not clobber the persisted strip before it merges.
  bool _restoring = false;

  /// Queued so a newer strip never persists before an older snapshot.
  Future<void>? _pendingPersist;

  List<String> get openProjectIds => List.unmodifiable(_open);

  bool isOpen(String projectId) => _open.contains(projectId);

  bool get isEmpty => _open.isEmpty;

  /// Appends [projectId] when it is not open yet.
  ///
  /// Returns whether the strip changed.
  bool open(String projectId) {
    if (projectId.isEmpty || _open.contains(projectId)) {
      return false;
    }
    _removed.remove(projectId);
    _open.add(projectId);
    _persist();
    notifyListeners();
    return true;
  }

  /// Removes [projectId] and returns the project that should become active
  /// when the closed tab was the active one: the tab to its right, else the
  /// tab to its left, else null when no tabs remain.
  String? close(String projectId) {
    final index = _open.indexOf(projectId);
    if (index < 0) {
      return null;
    }
    _open.removeAt(index);
    _removed.add(projectId);
    final neighbor = index < _open.length
        ? _open[index]
        : index > 0
        ? _open[index - 1]
        : null;
    _persist();
    notifyListeners();
    return neighbor;
  }

  /// Drops tabs whose project no longer exists in the catalog.
  void retainProjects(Set<String> projectIds) {
    final stale = _open.where((id) => !projectIds.contains(id)).toList();
    if (stale.isEmpty) {
      return;
    }
    _open.removeWhere((id) => !projectIds.contains(id));
    _removed.addAll(stale);
    _persist();
    notifyListeners();
  }

  /// Loads the persisted strip. Restored ids keep their saved order in front
  /// of ids opened while the load was in flight; ids closed in that window
  /// stay closed.
  Future<void> restore() async {
    final memory = _memory;
    if (memory == null) {
      return;
    }
    _restoring = true;
    List<String> saved;
    try {
      saved = await memory.openProjects();
    } on Object catch (_) {
      // Tests without mocked preferences, or a missing plugin, skip restore.
      _restoring = false;
      return;
    }
    final liveOnly = _open.where((id) => !saved.contains(id)).toList();
    final merged = <String>[
      for (final id in saved)
        if (id.isNotEmpty && !_removed.contains(id)) id,
      ...liveOnly,
    ];
    final changed = !listEquals(merged, _open);
    if (changed) {
      _open
        ..clear()
        ..addAll(merged);
    }
    _restoring = false;
    _persist();
    if (changed) {
      notifyListeners();
    }
  }

  void _persist() {
    final memory = _memory;
    if (memory == null || _restoring) {
      return;
    }
    final previous = _pendingPersist;
    final next = _runPersist(memory, previous);
    _pendingPersist = next;
    unawaited(
      next.catchError((Object e, StackTrace s) {
        AppLog.record('tabs persist chain: $e', s);
      }),
    );
  }

  Future<void> _runPersist(
    WorkspaceMemory memory,
    Future<void>? previous,
  ) async {
    // Wait so a newer strip never persists before an older snapshot. The
    // snapshot is taken when this link runs, not when it was queued.
    await previous;
    try {
      await memory.rememberOpenProjects(List.of(_open));
    } on Object catch (e, s) {
      // Same policy as the dock layout: a failed preferences write is
      // skipped rather than breaking the interaction.
      AppLog.record('tabs persist: $e', s);
    }
  }
}
