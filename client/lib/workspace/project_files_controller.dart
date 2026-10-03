import 'dart:async';

import 'package:flutter/foundation.dart';

import '../catalog/catalog_client.dart';
import '../core/app_log.dart';
import '../core/operator_failure.dart';
import '../shell/project_document_ref.dart';
import 'file_document.dart';
import 'open_with.dart';
import 'project_paths.dart';
import 'text_editor_session.dart';

class ProjectFilesController extends ChangeNotifier {
  ProjectFilesController({required CatalogClient catalog}) : _catalog = catalog;

  final CatalogClient _catalog;

  String? projectId;
  String? _projectName;
  String? error;
  bool loading = false;
  List<GitCommit> commits = const [];
  String? lastDiff;

  final Map<String, List<FsEntry>> children = {};
  final Set<String> expanded = {'.'};

  final Map<String, FileDocument> documents = {};
  final Map<String, _DocSession> _sessions = {};
  final List<OpenView> openViews = [];
  final Map<String, EditorViewMode> _viewModes = {};
  String? focusedViewId;

  /// Tree row last picked in the explorer. Not persisted.
  String? selectedPath;

  void Function(OpenView view, {required bool toSide})? onViewOpened;
  void Function(OpenView view)? onViewClosed;

  /// Fired after a rename or move re-pointed [oldView] at a new path (same
  /// [OpenView.viewId]), so the dock can rekey its tab.
  void Function(OpenView oldView, OpenView newView)? onViewMoved;
  VoidCallback? onDocumentsCleared;

  int _viewSeq = 0;

  CatalogClient get catalog => _catalog;

  /// Display name for the tree root. Falls back to "Project files" in the explorer.
  String? get projectName => _projectName;

  set projectName(String? value) {
    if (value == _projectName) return;
    _projectName = value;
    notifyListeners();
  }

  OpenView? get focusedView {
    final id = focusedViewId;
    if (id == null) {
      return null;
    }
    for (final view in openViews) {
      if (view.viewId == id) {
        return view;
      }
    }
    return null;
  }

  /// Editor/preview arrangement for a document view. Defaults to code only.
  EditorViewMode viewModeFor(String viewId) {
    return _viewModes[viewId] ?? EditorViewMode.code;
  }

  void setViewMode(String viewId, EditorViewMode mode) {
    if (viewModeFor(viewId) == mode) {
      return;
    }
    if (mode == EditorViewMode.code) {
      _viewModes.remove(viewId);
    } else {
      _viewModes[viewId] = mode;
    }
    notifyListeners();
  }

  Uri previewUriFor(String path) {
    final id = projectId;
    if (id == null) {
      return Uri.parse('about:blank');
    }
    return _catalog.previewUri(id, path);
  }

  List<String> expansionSnapshot() {
    return expanded.where((path) => path != '.').toList(growable: false);
  }

  void collapseAll() {
    expanded
      ..clear()
      ..add('.');
    notifyListeners();
  }

  Future<void> setProjectId(
    String? id, {
    List<String> restoreExpanded = const [],
    bool notifyDocumentsCleared = true,
  }) async {
    if (id == projectId) {
      return;
    }
    projectId = id;
    children.clear();
    expanded
      ..clear()
      ..add('.')
      ..addAll(restoreExpanded.where((path) => path.isNotEmpty && path != '.'));
    documents.clear();
    for (final s in _sessions.values) {
      s.dispose();
    }
    _sessions.clear();
    openViews.clear();
    _viewModes.clear();
    focusedViewId = null;
    error = null;
    if (notifyDocumentsCleared) {
      onDocumentsCleared?.call();
    }
    notifyListeners();
    if (id != null) {
      await refreshTree();
    }
  }

  /// Reopens document views after a project switch or cold start.
  ///
  /// When [notifyDock] is false, dock items are assumed to already exist (or
  /// will be built from a saved layout) and [onViewOpened] is not called.
  Future<void> restoreViews(
    List<ProjectDocumentRef> refs, {
    Map<String, String> dirtyTextByPath = const {},
    bool notifyDock = true,
  }) async {
    if (refs.isEmpty || projectId == null) {
      return;
    }
    final savedOpened = onViewOpened;
    if (!notifyDock) {
      onViewOpened = null;
    }
    try {
      for (final ref in refs) {
        try {
          await openWith(ref.path, ref.appId);
        } on Object catch (_) {
          // File may have been deleted while the project was inactive.
          continue;
        }
        final view = _findView(ref.path, ref.appId);
        if (view == null) {
          continue;
        }
        if (ref.viewMode != EditorViewMode.code) {
          _viewModes[view.viewId] = ref.viewMode;
        }
        final dirty = dirtyTextByPath[ref.path];
        if (dirty != null) {
          final doc = documents[ref.path];
          if (doc != null && doc.isUtf8) {
            doc.replaceText(dirty);
          }
        }
      }
      ProjectDocumentRef? focus;
      for (final ref in refs) {
        if (ref.focused) {
          focus = ref;
        }
      }
      if (focus != null) {
        final view = _findView(focus.path, focus.appId);
        if (view != null) {
          focusedViewId = view.viewId;
        }
      }
    } finally {
      onViewOpened = savedOpened;
    }
    notifyListeners();
  }

  OpenView? findView(String path, ProjectFileAppId app) => _findView(path, app);

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
        } on Object catch (_) {
          // Folder may have been deleted by the agent.
          children.remove(path);
          expanded.remove(path);
        }
      }
    } on Object catch (e, s) {
      AppLog.record('workspace refreshTree: $e', s);
      error = operatorMessageFor(const WorkspaceIoFailure());
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
    } on Object catch (e, s) {
      AppLog.record('workspace expand: $e', s);
      error = operatorMessageFor(const WorkspaceIoFailure());
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
    ProjectFileAppId app, {
    bool toSide = false,
  }) async {
    final existing = _findView(path, app);
    if (existing != null) {
      focusedViewId = existing.viewId;
      selectedPath = path;
      onViewOpened?.call(existing, toSide: false);
      notifyListeners();
      return;
    }
    if (app == ProjectFileAppId.textEditor ||
        app == ProjectFileAppId.imagePreview) {
      await _ensureDocument(path);
    }
    var split = toSide;
    if (!split &&
        app == ProjectFileAppId.webPreview &&
        _findView(path, ProjectFileAppId.textEditor) != null) {
      split = true;
    }
    final view = OpenView(viewId: 'view-${++_viewSeq}', path: path, appId: app);
    openViews.add(view);
    focusedViewId = view.viewId;
    selectedPath = path;
    onViewOpened?.call(view, toSide: split);
    notifyListeners();
  }

  Future<void> closeView(String viewId) async {
    final i = openViews.indexWhere((t) => t.viewId == viewId);
    if (i < 0) {
      return;
    }
    final removed = openViews[i];
    onViewClosed?.call(removed);
    openViews.removeAt(i);
    _viewModes.remove(viewId);
    if (focusedViewId == viewId) {
      if (openViews.isEmpty) {
        focusedViewId = null;
      } else {
        focusedViewId = openViews[i.clamp(0, openViews.length - 1)].viewId;
      }
    }
    _maybeCloseDocument(removed.path);
    notifyListeners();
  }

  void focusView(String viewId) {
    for (final view in openViews) {
      if (view.viewId == viewId) {
        focusedViewId = viewId;
        selectedPath = view.path;
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

  final Map<String, Future<void>> _saving = {};

  Future<void> savePath(String path) async {
    final id = projectId;
    final doc = documents[path];
    if (id == null || doc == null) {
      return;
    }
    // movePath waits on this so a rename cannot land mid-save and leave the
    // write on the old path.
    final save = _catalog.putProjectFile(id, path, doc.bytes);
    _saving[path] = save;
    try {
      await save;
    } finally {
      if (identical(_saving[path], save)) {
        unawaited(_saving.remove(path));
      }
    }
    doc.markClean();
    notifyListeners();
  }

  Future<void> createFile(String dir, String name) async {
    final id = projectId;
    if (id == null || name.trim().isEmpty) {
      return;
    }
    final path = joinProjectPath(dir, name.trim());
    _ensureFree(path);
    await _catalog.putProjectFile(id, path, Uint8List(0));
    selectedPath = path;
    await refreshTree();
    // The file exists now, so a failed open must not read as a failed create.
    try {
      await openDefault(path);
    } on Object catch (e, s) {
      AppLog.record('createFile open: $e', s);
    }
  }

  Future<void> createDir(String dir, String name) async {
    final id = projectId;
    if (id == null || name.trim().isEmpty) {
      return;
    }
    final path = joinProjectPath(dir, name.trim());
    _ensureFree(path);
    await _catalog.createProjectDir(id, path);
    selectedPath = path;
    await refreshTree();
  }

  /// Renames [path] within its folder. Returns the new path.
  Future<String> renamePath(String path, String newName) async {
    final problem = validateEntryName(newName);
    if (problem != null) {
      throw CatalogException(statusCode: 400, message: problem);
    }
    return movePath(path, joinProjectPath(parentOfPath(path), newName.trim()));
  }

  /// Moves or renames [from] to [to] on the server, then remaps every piece of
  /// path-keyed state: open documents (dirty buffers included), views, the
  /// selection, expansion and cached listings. Throws, leaving local state
  /// untouched, when the server refuses (collision, invalid path, ...).
  Future<String> movePath(String from, String to) async {
    final id = projectId;
    if (id == null) {
      throw StateError('no project');
    }
    if (from == to) {
      return to;
    }
    if (isSameOrDescendant(to, from)) {
      throw CatalogException(
        statusCode: 400,
        message: 'Cannot move a folder into itself.',
      );
    }
    _ensureFree(to);
    final pending = [
      for (final entry in _saving.entries)
        if (isSameOrDescendant(entry.key, from)) entry.value,
    ];
    for (final save in pending) {
      await save.catchError((Object _) {});
    }
    final dest = await _catalog.moveProjectPath(id, from: from, to: to);
    if (projectId == id) {
      _applyMove(from, dest);
      await refreshTree();
    }
    return dest;
  }

  /// Copies [path] beside itself as `name copy.ext`. Returns the new path.
  Future<String> duplicatePath(String path) async {
    final id = projectId;
    if (id == null) {
      throw StateError('no project');
    }
    final dest = await _catalog.copyProjectPath(id, from: path);
    if (projectId != id) {
      return dest;
    }
    selectedPath = dest;
    await refreshTree();
    final entry = entryAt(dest);
    if (entry != null && !entry.isDir) {
      await openDefault(dest);
    }
    return dest;
  }

  /// The cached tree entry at [path], or null when its folder is not loaded.
  FsEntry? entryAt(String path) {
    final siblings = children[parentOfPath(path)];
    if (siblings == null) {
      return null;
    }
    final name = baseNameOfPath(path);
    for (final entry in siblings) {
      if (entry.name == name) {
        return entry;
      }
    }
    return null;
  }

  void select(String? path) {
    if (selectedPath == path) {
      return;
    }
    selectedPath = path;
    notifyListeners();
  }

  Future<void> deletePath(String path) async {
    final id = projectId;
    if (id == null) {
      return;
    }
    await _catalog.deleteProjectFile(id, path);
    final toClose = [
      for (final view in openViews)
        if (isSameOrDescendant(view.path, path)) view.viewId,
    ];
    for (final viewId in toClose) {
      await closeView(viewId);
    }
    for (final doomed
        in documents.keys
            .where((key) => isSameOrDescendant(key, path))
            .toList()) {
      documents.remove(doomed);
      _sessions.remove(doomed)?.dispose();
    }
    expanded.removeWhere((key) => key != '.' && isSameOrDescendant(key, path));
    children.removeWhere((key, _) => isSameOrDescendant(key, path));
    if (selectedPath != null && isSameOrDescendant(selectedPath!, path)) {
      selectedPath = null;
    }
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
        await openWith(e.name, ProjectFileAppId.webPreview, toSide: true);
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
    } on Object catch (e, s) {
      AppLog.record('workspace loadCommits: $e', s);
      error = operatorMessageFor(const WorkspaceIoFailure());
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
    } on Object catch (e, s) {
      AppLog.record('workspace createCheckpoint: $e', s);
      error = operatorMessageFor(const WorkspaceIoFailure());
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

  OpenView? _findView(String path, ProjectFileAppId app) {
    for (final view in openViews) {
      if (view.path == path && view.appId == app) {
        return view;
      }
    }
    return null;
  }

  void _maybeCloseDocument(String path) {
    for (final view in openViews) {
      if (view.path == path) {
        return;
      }
    }
    documents.remove(path);
    _sessions.remove(path)?.dispose();
  }

  /// Fails fast when [path] is already listed. The server has the final say
  /// (it refuses moves onto existing paths) but creating a file would
  /// otherwise silently overwrite.
  void _ensureFree(String path) {
    if (entryAt(path) != null) {
      throw CatalogException(
        statusCode: 409,
        message: '"${baseNameOfPath(path)}" already exists here.',
      );
    }
  }

  void _applyMove(String from, String to) {
    for (final key in documents.keys.toList()) {
      if (!isSameOrDescendant(key, from)) {
        continue;
      }
      final next = remapPath(key, from, to);
      final doc = documents.remove(key)!;
      doc.rebindPath(next);
      documents[next] = doc;
      final session = _sessions.remove(key);
      if (session != null) {
        _sessions[next] = session;
      }
    }
    for (var i = 0; i < openViews.length; i++) {
      final view = openViews[i];
      if (!isSameOrDescendant(view.path, from)) {
        continue;
      }
      final moved = OpenView(
        viewId: view.viewId,
        path: remapPath(view.path, from, to),
        appId: view.appId,
      );
      openViews[i] = moved;
      onViewMoved?.call(view, moved);
    }
    if (selectedPath != null) {
      selectedPath = remapPath(selectedPath!, from, to);
    }
    final nextExpanded = {for (final key in expanded) remapPath(key, from, to)};
    expanded
      ..clear()
      ..addAll(nextExpanded);
    final nextChildren = {
      for (final entry in children.entries)
        remapPath(entry.key, from, to): entry.value,
    };
    children
      ..clear()
      ..addAll(nextChildren);
  }
}

class _DocSession extends ChangeNotifier implements TextEditorSession {
  _DocSession(this._projectFiles, this._doc) {
    _doc.addListener(_onDoc);
  }

  final ProjectFilesController _projectFiles;
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
    // Controller listeners also fire for selection/composing; ignore no-ops.
    if (_doc.isUtf8 && text == _doc.text) {
      return;
    }
    _doc.replaceText(text);
  }

  @override
  Future<void> save() => _projectFiles.savePath(_doc.path);

  @override
  Future<void> reload({bool force = false}) =>
      _projectFiles._reloadDocument(_doc, force: force);

  @override
  void dispose() {
    _doc.removeListener(_onDoc);
    super.dispose();
  }
}
