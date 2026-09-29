import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

import 'project_document_ref.dart';

/// Per-project thread and explorer state kept on the client.
class WorkbenchStateStore {
  static const threadPrefix = 'workbench_thread_v1:';
  static const expansionPrefix = 'workbench_expansion_v1:';
  static const documentsPrefix = 'workbench_documents_v1:';
  static const openProjectsKey = 'workbench_open_projects_v1';
  static const activeProjectKey = 'workbench_active_project_v1';

  static const _legacyThreadPrefix = 'workspace_thread_v1:';
  static const _legacyExpansionPrefix = 'workspace_expansion_v1:';
  static const _legacyDocumentsPrefix = 'workspace_documents_v1:';
  static const _legacyOpenProjectsKey = 'workspace_open_projects_v1';
  static const _legacyActiveProjectKey = 'workspace_active_project_v1';

  /// Cached so concurrent first reads cannot end up on separate
  /// [SharedPreferences] instances with independent caches.
  SharedPreferences? _prefs;

  Future<SharedPreferences> _store() async {
    return _prefs ??= await SharedPreferences.getInstance();
  }

  Future<void> _migrateStringKey(
    SharedPreferences prefs,
    String newKey,
    String legacyKey,
  ) async {
    if (prefs.containsKey(newKey)) {
      return;
    }
    final legacy = prefs.getString(legacyKey);
    if (legacy == null) {
      return;
    }
    await prefs.setString(newKey, legacy);
    await prefs.remove(legacyKey);
  }

  Future<void> _migrateStringListKey(
    SharedPreferences prefs,
    String newKey,
    String legacyKey,
  ) async {
    if (prefs.containsKey(newKey)) {
      return;
    }
    if (!prefs.containsKey(legacyKey)) {
      return;
    }
    await prefs.setStringList(newKey, prefs.getStringList(legacyKey)!);
    await prefs.remove(legacyKey);
  }

  Future<void> _migratePrefixedKeys(
    SharedPreferences prefs,
    String newPrefix,
    String legacyPrefix, {
    required bool stringList,
  }) async {
    final keys = prefs
        .getKeys()
        .where((k) => k.startsWith(legacyPrefix))
        .toList();
    for (final legacyKey in keys) {
      final suffix = legacyKey.substring(legacyPrefix.length);
      final newKey = '$newPrefix$suffix';
      if (prefs.containsKey(newKey)) {
        await prefs.remove(legacyKey);
        continue;
      }
      if (stringList) {
        final value = prefs.getStringList(legacyKey);
        if (value != null) {
          await prefs.setStringList(newKey, value);
        }
      } else {
        final value = prefs.getString(legacyKey);
        if (value != null) {
          await prefs.setString(newKey, value);
        }
      }
      await prefs.remove(legacyKey);
    }
  }

  Future<void> _ensureMigrated() async {
    final prefs = await _store();
    await _migrateStringListKey(prefs, openProjectsKey, _legacyOpenProjectsKey);
    await _migrateStringKey(prefs, activeProjectKey, _legacyActiveProjectKey);
    await _migratePrefixedKeys(
      prefs,
      threadPrefix,
      _legacyThreadPrefix,
      stringList: false,
    );
    await _migratePrefixedKeys(
      prefs,
      expansionPrefix,
      _legacyExpansionPrefix,
      stringList: true,
    );
    await _migratePrefixedKeys(
      prefs,
      documentsPrefix,
      _legacyDocumentsPrefix,
      stringList: false,
    );
  }

  /// Ordered ids of the project tabs that were open when the app closed.
  Future<List<String>> openProjects() async {
    await _ensureMigrated();
    final prefs = await _store();
    return prefs.getStringList(openProjectsKey) ?? const [];
  }

  Future<void> rememberOpenProjects(List<String> projectIds) async {
    final prefs = await _store();
    await prefs.setStringList(openProjectsKey, projectIds);
  }

  /// Project that was active when the app closed, for startup restoration.
  Future<String?> lastActiveProject() async {
    await _ensureMigrated();
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
    await _ensureMigrated();
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
    await _ensureMigrated();
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
  Future<List<ProjectDocumentRef>> documents(String projectId) async {
    await _ensureMigrated();
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
            ProjectDocumentRef.fromJson(Map<String, dynamic>.from(item)),
      ];
    } on FormatException {
      return const [];
    } on TypeError {
      return const [];
    }
  }

  Future<void> rememberDocuments(
    String projectId,
    List<ProjectDocumentRef> docs,
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
