import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

import 'workspace_document_ref.dart';

/// Per-project thread and explorer state kept on the client.
class WorkspaceMemory {
  static const threadPrefix = 'workspace_thread_v1:';
  static const expansionPrefix = 'workspace_expansion_v1:';
  static const documentsPrefix = 'workspace_documents_v1:';
  static const openProjectsKey = 'workspace_open_projects_v1';
  static const activeProjectKey = 'workspace_active_project_v1';

  /// Cached so concurrent first reads cannot end up on separate
  /// [SharedPreferences] instances with independent caches.
  SharedPreferences? _prefs;

  Future<SharedPreferences> _store() async {
    return _prefs ??= await SharedPreferences.getInstance();
  }

  /// Ordered ids of the project tabs that were open when the app closed.
  Future<List<String>> openProjects() async {
    final prefs = await _store();
    return prefs.getStringList(openProjectsKey) ?? const [];
  }

  Future<void> rememberOpenProjects(List<String> projectIds) async {
    final prefs = await _store();
    await prefs.setStringList(openProjectsKey, projectIds);
  }

  /// Project that was active when the app closed, for startup restoration.
  Future<String?> lastActiveProject() async {
    final prefs = await _store();
    final value = prefs.getString(activeProjectKey);
    if (value == null || value.isEmpty) {
      return null;
    }
    return value;
  }

  Future<void> rememberActiveProject(String projectId) async {
    if (projectId.isEmpty) {
      return;
    }
    final prefs = await _store();
    await prefs.setString(activeProjectKey, projectId);
  }

  /// Clears the remembered project so a closed-last-tab session does not
  /// resurrect it on the next launch.
  Future<void> forgetActiveProject() async {
    final prefs = await _store();
    await prefs.remove(activeProjectKey);
  }

  Future<String?> lastThread(String projectId) async {
    final prefs = await _store();
    final value = prefs.getString('$threadPrefix$projectId');
    if (value == null || value.isEmpty) {
      return null;
    }
    return value;
  }

  Future<void> rememberThread(String projectId, String threadId) async {
    if (projectId.isEmpty || threadId.isEmpty) {
      return;
    }
    final prefs = await _store();
    await prefs.setString('$threadPrefix$projectId', threadId);
  }

  Future<List<String>> expansion(String projectId) async {
    final prefs = await _store();
    return prefs.getStringList('$expansionPrefix$projectId') ?? const [];
  }

  Future<void> rememberExpansion(String projectId, List<String> paths) async {
    if (projectId.isEmpty) {
      return;
    }
    final prefs = await _store();
    await prefs.setStringList('$expansionPrefix$projectId', paths);
  }

  /// Open document tabs for [projectId], including view mode and focus.
  Future<List<WorkspaceDocumentRef>> documents(String projectId) async {
    final prefs = await _store();
    final raw = prefs.getString('$documentsPrefix$projectId');
    if (raw == null || raw.isEmpty) {
      return const [];
    }
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! List) {
        return const [];
      }
      return [
        for (final item in decoded)
          if (item is Map)
            WorkspaceDocumentRef.fromJson(Map<String, dynamic>.from(item)),
      ];
    } on FormatException {
      return const [];
    } on TypeError {
      return const [];
    }
  }

  Future<void> rememberDocuments(
    String projectId,
    List<WorkspaceDocumentRef> docs,
  ) async {
    if (projectId.isEmpty) {
      return;
    }
    final prefs = await _store();
    final key = '$documentsPrefix$projectId';
    if (docs.isEmpty) {
      await prefs.remove(key);
      return;
    }
    await prefs.setString(
      key,
      jsonEncode([for (final doc in docs) doc.toJson()]),
    );
  }
}
