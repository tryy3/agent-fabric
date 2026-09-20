import 'package:flutter/foundation.dart';

import '../catalog/catalog_client.dart';
import 'file_document.dart';
import 'open_with.dart';
import 'text_editor_session.dart';

class WorkspaceController extends ChangeNotifier {
  WorkspaceController({required CatalogClient catalog}) : _catalog = catalog;

  final CatalogClient _catalog;

  String? projectId;
  bool paneOpen = false;
  String? error;
  bool loading = false;
  List<GitCommit> commits = const [];
  String? lastDiff;

  final Map<String, List<FsEntry>> children = {};
  final Set<String> expanded = {'.'};

  final Map<String, FileDocument> documents = {};
  final Map<String, _DocSession> _sessions = {};
  final List<EditorGroup> groups = [];
  String? focusedGroupId;

  int _viewSeq = 0;

  CatalogClient get catalog => _catalog;

  EditorGroup? get focusedGroup {
    final id = focusedGroupId;
    if (id == null) {
      return groups.isEmpty ? null : groups.first;
    }
    for (final g in groups) {
      if (g.groupId == id) {
        return g;
      }
    }
    return groups.isEmpty ? null : groups.first;
  }

  OpenView? get focusedView => focusedGroup?.active;

  Uri previewUriFor(String path) {
    final id = projectId;
    if (id == null) {
      return Uri.parse('about:blank');
    }
    return _catalog.previewUri(id, path);
  }

  void togglePane() {
    paneOpen = !paneOpen;
    notifyListeners();
    if (paneOpen && projectId != null && !children.containsKey('.')) {
      refreshTree();
    }
  }

  Future<void> setProjectId(String? id) async {
    if (id == projectId) {
      return;
    }
    projectId = id;
    children.clear();
    expanded
      ..clear()
      ..add('.');
    documents.clear();
    for (final s in _sessions.values) {
      s.dispose();
    }
    _sessions.clear();
    groups.clear();
    focusedGroupId = null;
    error = null;
    notifyListeners();
    if (id != null && paneOpen) {
      await refreshTree();
    }
  }

  Future<void> refreshTree() async {
    final id = projectId;
    if (id == null) {
      return;
    }
    loading = true;
    error = null;
    notifyListeners();
    try {
      final listing = await _catalog.listProjectFs(id, path: '/');
      children['.'] = listing.entries;
      for (final path in expanded.where((p) => p != '.').toList()) {
        try {
          final nested = await _catalog.listProjectFs(id, path: path);
          children[path] = nested.entries;
        } catch (_) {
          // Folder may have been deleted by the agent.
          children.remove(path);
          expanded.remove(path);
        }
      }
    } catch (e) {
      error = e.toString();
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  Future<void> refreshAfterAgentTurn() async {
    await refreshTree();
    for (final doc in documents.values.toList()) {
      await _reloadDocument(doc, force: false);
    }
    notifyListeners();
  }

  Future<void> expand(String path) async {
    final id = projectId;
    if (id == null) {
      return;
    }
    if (expanded.contains(path)) {
      expanded.remove(path);
      notifyListeners();
      return;
    }
    expanded.add(path);
    try {
      final listing = await _catalog.listProjectFs(
        id,
        path: path == '.' ? '/' : path,
      );
      children[path] = listing.entries;
    } catch (e) {
      error = e.toString();
    }
    notifyListeners();
  }

  Future<void> openDefault(String path, {bool toSide = false}) async {
    final app = associationFor(path).defaultApp;
    if (app == null) {
      return;
    }
    await openWith(path, app, toSide: toSide);
  }

  Future<void> openWith(
    String path,
    WorkspaceAppId app, {
    bool toSide = false,
  }) async {
    final existing = _findView(path, app);
    if (existing != null) {
      _focusView(existing.viewId);
      notifyListeners();
      return;
    }
    if (app == WorkspaceAppId.textEditor ||
        app == WorkspaceAppId.imagePreview) {
      await _ensureDocument(path);
    }
    var split = toSide;
    if (!split &&
        app == WorkspaceAppId.webPreview &&
        _findView(path, WorkspaceAppId.textEditor) != null) {
      split = true;
    }
    final view = OpenView(viewId: 'view-${++_viewSeq}', path: path, appId: app);
    _placeView(view, toSide: split);
    notifyListeners();
  }

  Future<void> closeView(String viewId) async {
    for (final group in groups) {
      final i = group.tabs.indexWhere((t) => t.viewId == viewId);
      if (i < 0) {
        continue;
      }
      final removed = group.tabs.removeAt(i);
      if (group.activeViewId == viewId) {
        group.activeViewId = group.tabs.isEmpty ? null : group.tabs.last.viewId;
      }
      if (group.tabs.isEmpty) {
        groups.remove(group);
        if (focusedGroupId == group.groupId) {
          focusedGroupId = groups.isEmpty ? null : groups.first.groupId;
        }
      }
      _maybeCloseDocument(removed.path);
      notifyListeners();
      return;
    }
  }

  void focusGroup(String groupId) {
    focusedGroupId = groupId;
    notifyListeners();
  }

  void focusTab(String groupId, String viewId) {
    for (final g in groups) {
      if (g.groupId == groupId) {
        g.activeViewId = viewId;
        focusedGroupId = groupId;
        notifyListeners();
        return;
      }
    }
  }

  Future<void> saveFocused() async {
    final view = focusedView;
    if (view == null) {
      return;
    }
    await savePath(view.path);
  }

  Future<void> savePath(String path) async {
    final id = projectId;
    final doc = documents[path];
    if (id == null || doc == null) {
      return;
    }
    await _catalog.putProjectFile(id, path, doc.bytes);
    doc.markClean();
    notifyListeners();
  }

  Future<void> createFile(String dir, String name) async {
    final id = projectId;
    if (id == null || name.trim().isEmpty) {
      return;
    }
    final path = _join(dir, name.trim());
    await _catalog.putProjectFile(id, path, Uint8List(0));
    await refreshTree();
    await openDefault(path);
  }

  Future<void> createDir(String dir, String name) async {
    final id = projectId;
    if (id == null || name.trim().isEmpty) {
      return;
    }
    await _catalog.createProjectDir(id, _join(dir, name.trim()));
    await refreshTree();
  }

  Future<void> deletePath(String path) async {
    final id = projectId;
    if (id == null) {
      return;
    }
    await _catalog.deleteProjectFile(id, path);
    final toClose = [
      for (final g in groups)
        for (final t in g.tabs)
          if (t.path == path) t.viewId,
    ];
    for (final viewId in toClose) {
      await closeView(viewId);
    }
    documents.remove(path);
    _sessions.remove(path)?.dispose();
    await refreshTree();
  }

  TextEditorSession sessionFor(FileDocument doc) {
    return _sessions.putIfAbsent(doc.path, () => _DocSession(this, doc));
  }

  FileDocument? documentFor(String path) => documents[path];

  Future<void> previewSite() async {
    final root = children['.'] ?? const <FsEntry>[];
    for (final e in root) {
      if (!e.isDir && (e.name == 'index.html' || e.name == 'index.htm')) {
        await openWith(e.name, WorkspaceAppId.webPreview, toSide: true);
        return;
      }
    }
  }

  Future<void> loadCommits() async {
    final id = projectId;
    if (id == null) {
      return;
    }
    try {
      commits = await _catalog.listProjectCommits(id);
      error = null;
    } catch (e) {
      error = e.toString();
    }
    notifyListeners();
  }

  Future<Checkpoint?> createCheckpoint(String label) async {
    final id = projectId;
    if (id == null || label.trim().isEmpty) {
      return null;
    }
    try {
      final created = await _catalog.createCheckpoint(id, label: label.trim());
      await loadCommits();
      return created;
    } catch (e) {
      error = e.toString();
      notifyListeners();
      return null;
    }
  }

  Future<void> restoreCommit(String sha) async {
    final id = projectId;
    if (id == null || sha.trim().isEmpty) {
      return;
    }
    await _catalog.restoreProject(id, sha: sha);
    await refreshTree();
    for (final doc in documents.values.toList()) {
      await _reloadDocument(doc, force: true);
    }
    notifyListeners();
  }

  Future<String> diffCommits({required String from, String to = ''}) async {
    final id = projectId;
    if (id == null) {
      return '';
    }
    final result = await _catalog.projectDiff(id, from: from, to: to);
    lastDiff = result.diff;
    notifyListeners();
    return result.diff;
  }

  Future<FileDocument> _ensureDocument(String path) async {
    final existing = documents[path];
    if (existing != null) {
      return existing;
    }
    final id = projectId;
    if (id == null) {
      throw StateError('no project');
    }
    final bytes = await _catalog.getProjectFile(id, path);
    final doc = FileDocument(projectId: id, path: path, bytes: bytes);
    documents[path] = doc;
    return doc;
  }

  Future<void> _reloadDocument(FileDocument doc, {required bool force}) async {
    final id = projectId;
    if (id == null) {
      return;
    }
    if (doc.isDirty && !force) {
      doc.markDiskChanged();
      return;
    }
    final bytes = await _catalog.getProjectFile(id, doc.path);
    doc.replaceBytes(bytes, markDirty: false);
  }

  OpenView? _findView(String path, WorkspaceAppId app) {
    for (final g in groups) {
      for (final t in g.tabs) {
        if (t.path == path && t.appId == app) {
          return t;
        }
      }
    }
    return null;
  }

  void _focusView(String viewId) {
    for (final g in groups) {
      for (final t in g.tabs) {
        if (t.viewId == viewId) {
          g.activeViewId = viewId;
          focusedGroupId = g.groupId;
          return;
        }
      }
    }
  }

  void _placeView(OpenView view, {required bool toSide}) {
    if (groups.isEmpty) {
      final g = EditorGroup(
        groupId: 'g1',
        tabs: [view],
        activeViewId: view.viewId,
      );
      groups.add(g);
      focusedGroupId = g.groupId;
      return;
    }
    if (toSide) {
      if (groups.length == 1) {
        final g = EditorGroup(
          groupId: 'g2',
          tabs: [view],
          activeViewId: view.viewId,
        );
        groups.add(g);
        focusedGroupId = g.groupId;
        return;
      }
      final other = groups.firstWhere(
        (g) => g.groupId != focusedGroupId,
        orElse: () => groups.last,
      );
      other.tabs.add(view);
      other.activeViewId = view.viewId;
      focusedGroupId = other.groupId;
      return;
    }
    final g = focusedGroup ?? groups.first;
    g.tabs.add(view);
    g.activeViewId = view.viewId;
    focusedGroupId = g.groupId;
  }

  void _maybeCloseDocument(String path) {
    for (final g in groups) {
      for (final t in g.tabs) {
        if (t.path == path) {
          return;
        }
      }
    }
    documents.remove(path);
    _sessions.remove(path)?.dispose();
  }

  String _join(String dir, String name) {
    if (dir.isEmpty || dir == '/' || dir == '.') {
      return name;
    }
    return '$dir/$name';
  }
}

class _DocSession extends ChangeNotifier implements TextEditorSession {
  _DocSession(this._workspace, this._doc) {
    _doc.addListener(_onDoc);
  }

  final WorkspaceController _workspace;
  final FileDocument _doc;

  void _onDoc() => notifyListeners();

  @override
  String get path => _doc.path;

  @override
  String get languageId => _doc.languageId;

  @override
  bool get isDirty => _doc.isDirty;

  @override
  String get text => _doc.isUtf8 ? _doc.text : '';

  @override
  void handleTextChanged(String text) {
    if (!_doc.isUtf8 && text.isEmpty) {
      return;
    }
    _doc.replaceText(text);
  }

  @override
  Future<void> save() => _workspace.savePath(_doc.path);

  @override
  Future<void> reload({bool force = false}) =>
      _workspace._reloadDocument(_doc, force: force);

  @override
  void dispose() {
    _doc.removeListener(_onDoc);
    super.dispose();
  }
}
