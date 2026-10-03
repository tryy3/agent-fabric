import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../core/app_log.dart';
import '../core/operator_failure.dart';
import '../ui/theme/design_tokens.dart';
import 'export_actions.dart';
import 'folder_picker_dialog.dart';
import 'git_history.dart';
import 'open_with.dart';
import 'project_files_controller.dart';
import 'project_paths.dart';

/// Compact IDE-style tree for the active project's files.
///
/// The root row names the project and carries the pane actions; less frequent
/// actions sit in its overflow menu. Rows can be renamed or created inline,
/// moved by drag-and-drop or the Move... dialog, and driven from the keyboard
/// (arrows, Enter, F2, Delete).
class FileExplorer extends StatefulWidget {
  const FileExplorer({super.key, required this.controller, this.onExport});

  final ProjectFilesController controller;

  /// Runs a catalog export/publish for the active project.
  final Future<ExportPublishResult?> Function(String method)? onExport;

  @override
  State<FileExplorer> createState() => _FileExplorerState();
}

enum _EditKind { rename, newFile, newFolder }

/// The one inline name editor that can be open at a time.
class _InlineEdit {
  const _InlineEdit({required this.kind, required this.path});

  final _EditKind kind;

  /// Rename: the entry being renamed. Create: the folder it is created in.
  final String path;
}

/// One visible tree row.
class _Node {
  const _Node({
    required this.path,
    required this.entry,
    required this.dir,
    required this.depth,
  });

  final String path;
  final FsEntry entry;
  final String dir;
  final int depth;
}

class _FileExplorerState extends State<FileExplorer> {
  final FocusNode _treeFocus = FocusNode(debugLabel: 'file-tree');
  final ScrollController _scroll = ScrollController();

  _InlineEdit? _edit;
  String? _editError;
  bool _busy = false;

  ProjectFilesController get controller => widget.controller;

  @override
  void initState() {
    super.initState();
    _treeFocus.addListener(_onFocusChanged);
  }

  @override
  void dispose() {
    _treeFocus
      ..removeListener(_onFocusChanged)
      ..dispose();
    _scroll.dispose();
    super.dispose();
  }

  void _onFocusChanged() {
    if (mounted) {
      setState(() {});
    }
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) {
        // Dirty markers follow each open document, not just the controller.
        return ListenableBuilder(
          listenable: Listenable.merge(controller.documents.values.toList()),
          builder: (context, _) => _buildTree(context),
        );
      },
    );
  }

  // ---------------------------------------------------------------- tree

  List<_Node> _visible() {
    final nodes = <_Node>[];
    void walk(String dir, int depth) {
      for (final entry in _sorted(
        controller.children[dir] ?? const <FsEntry>[],
      )) {
        final path = joinProjectPath(dir, entry.name);
        nodes.add(_Node(path: path, entry: entry, dir: dir, depth: depth));
        if (entry.isDir && controller.expanded.contains(path)) {
          walk(path, depth + 1);
        }
      }
    }

    if (controller.expanded.contains('.')) {
      walk('.', 1);
    }
    return nodes;
  }

  List<FsEntry> _sorted(List<FsEntry> entries) {
    final copy = [...entries];
    copy.sort((a, b) {
      if (a.isDir != b.isDir) {
        return a.isDir ? -1 : 1;
      }
      return a.name.toLowerCase().compareTo(b.name.toLowerCase());
    });
    return copy;
  }

  Widget _buildTree(BuildContext context) {
    final tokens = designTokensOf(context);
    final hasProject = controller.projectId != null;
    final rootOpen = controller.expanded.contains('.');
    final nodes = _visible();
    final edit = _edit;
    final creating = edit != null && edit.kind != _EditKind.rename;
    final rows = <Widget>[];
    if (creating && edit.path == '.') {
      rows.add(_pendingRow(edit, depth: 1));
    }
    for (final node in nodes) {
      if (edit != null &&
          edit.kind == _EditKind.rename &&
          edit.path == node.path) {
        rows.add(
          _InlineNameRow(
            key: Key('inline-edit-${node.path}'),
            depth: node.depth,
            isDir: node.entry.isDir,
            initial: node.entry.name,
            error: _editError,
            onSubmit: _commitEdit,
            onCancel: _cancelEdit,
          ),
        );
      } else {
        rows.add(_row(context, node));
      }
      if (creating && edit.path == node.path && node.entry.isDir) {
        rows.add(_pendingRow(edit, depth: node.depth + 1));
      }
    }
    return Focus(
      focusNode: _treeFocus,
      onKeyEvent: (_, event) => _onKey(event),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _header(context, tokens, hasProject: hasProject, rootOpen: rootOpen),
          if (controller.error != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 4, 12, 8),
              child: Text(
                controller.error!,
                style: tokens.caption().copyWith(color: tokens.error),
              ),
            ),
          Expanded(
            // Empty space below the rows clears the selection, as in most
            // editors, and is a drop target for the project root; row taps
            // and folder drop targets win first.
            child: _DropZone(
              key: const Key('explorer-empty-drop'),
              dir: '.',
              canAccept: _canDropOn,
              onDrop: _dropOn,
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: hasProject ? () => controller.select(null) : null,
                child: ListView(
                  key: const Key('file-explorer'),
                  controller: _scroll,
                  padding: const EdgeInsets.fromLTRB(4, 0, 4, 8),
                  children: rootOpen ? rows : const [],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _header(
    BuildContext context,
    DesignTokens tokens, {
    required bool hasProject,
    required bool rootOpen,
  }) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 6, 4, 2),
      child: SizedBox(
        height: DesignTokens.treeRowHeight,
        child: Row(
          children: [
            Expanded(
              child: _DropZone(
                key: const Key('explorer-root-drop'),
                dir: '.',
                canAccept: _canDropOn,
                onDrop: _dropOn,
                child: _RowSurface(
                  key: const Key('explorer-root'),
                  selected: controller.selectedPath == '.',
                  onTap: hasProject ? _activateRoot : null,
                  child: Row(
                    children: [
                      _Chevron(open: rootOpen, tokens: tokens),
                      Expanded(
                        child: Text(
                          controller.projectName ?? 'Project files',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: tokens.labelSm().copyWith(
                            color: tokens.textSecondary,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
            _HeaderAction(
              key: const Key('explorer-new-file'),
              tooltip: 'New file',
              icon: Icons.note_add_outlined,
              onPressed: hasProject
                  ? () => _beginCreate(_createTargetDir(), folder: false)
                  : null,
            ),
            _HeaderAction(
              key: const Key('explorer-new-folder'),
              tooltip: 'New folder',
              icon: Icons.create_new_folder_outlined,
              onPressed: hasProject
                  ? () => _beginCreate(_createTargetDir(), folder: true)
                  : null,
            ),
            _HeaderAction(
              key: const Key('explorer-refresh'),
              tooltip: 'Refresh',
              icon: Icons.refresh,
              onPressed: hasProject ? controller.refreshTree : null,
            ),
            _HeaderAction(
              key: const Key('explorer-collapse'),
              tooltip: 'Collapse all',
              icon: Icons.unfold_less,
              onPressed: hasProject ? controller.collapseAll : null,
            ),
            _overflowMenu(context, tokens, hasProject: hasProject),
          ],
        ),
      ),
    );
  }

  Widget _row(BuildContext context, _Node node) {
    final path = node.path;
    final entry = node.entry;
    final expanded = controller.expanded.contains(path);
    final dirty = controller.documents[path]?.isDirty ?? false;
    final row = _TreeRow(
      key: Key('file-row-$path'),
      name: entry.name,
      isDir: entry.isDir,
      expanded: expanded,
      depth: node.depth,
      selected: path == controller.selectedPath,
      focused: _treeFocus.hasFocus,
      modified: dirty,
      menuKey: Key('file-menu-$path'),
      onTap: () => _activate(node),
      onMenu: (position) => _showMenu(context, node, position),
    );
    final draggable = _draggable(path, entry.name, row);
    if (!entry.isDir) {
      // A file row is not a destination, but it must still swallow drops so
      // they do not fall through to the root zone behind the list. Only a
      // valid move into the file's folder acts on the drop.
      return _DropZone(
        key: Key('file-drop-$path'),
        dir: parentOfPath(path),
        canAccept: (_, _) => true,
        highlightWhen: _canDropOn,
        onDrop: (dir, dragged) async {
          if (_canDropOn(dir, dragged)) await _dropOn(dir, dragged);
        },
        child: draggable,
      );
    }
    return _DropZone(
      key: Key('file-drop-$path'),
      dir: path,
      canAccept: _canDropOn,
      onDrop: _dropOn,
      child: draggable,
    );
  }

  Widget _draggable(String path, String name, Widget child) {
    final feedback = _DragChip(label: name);
    // Touch scrolls the list on drag, so it starts a move on long press.
    final touch = switch (defaultTargetPlatform) {
      TargetPlatform.android ||
      TargetPlatform.iOS ||
      TargetPlatform.fuchsia => true,
      _ => false,
    };
    if (touch) {
      return LongPressDraggable<String>(
        data: path,
        feedback: feedback,
        child: child,
      );
    }
    return Draggable<String>(data: path, feedback: feedback, child: child);
  }

  Widget _pendingRow(_InlineEdit edit, {required int depth}) {
    return _InlineNameRow(
      key: const Key('inline-edit-new'),
      depth: depth,
      isDir: edit.kind == _EditKind.newFolder,
      initial: '',
      error: _editError,
      onSubmit: _commitEdit,
      onCancel: _cancelEdit,
    );
  }

  // ------------------------------------------------------------ selection

  void _activate(_Node node) {
    _treeFocus.requestFocus();
    controller.select(node.path);
    if (node.entry.isDir) {
      unawaited(
        controller.expand(node.path).catchError((Object e, StackTrace s) {
          AppLog.record('expand: $e', s);
        }),
      );
    } else {
      unawaited(
        controller.openDefault(node.path).catchError((Object e, StackTrace s) {
          AppLog.record('openDefault: $e', s);
        }),
      );
    }
  }

  /// Selecting the root lets new items land at the top level again after a
  /// folder or file was selected.
  void _activateRoot() {
    _treeFocus.requestFocus();
    final wasSelected = controller.selectedPath == '.';
    controller.select('.');
    final open = controller.expanded.contains('.');
    // First click selects; a click on the selected root toggles it, and a
    // collapsed root always opens.
    if (wasSelected || !open) {
      unawaited(
        controller.expand('.').catchError((Object e, StackTrace s) {
          AppLog.record('expand: $e', s);
        }),
      );
    }
  }

  /// Folder new items land in: the selected folder, the selected file's
  /// folder, or the project root.
  String _createTargetDir() {
    final selected = controller.selectedPath;
    if (selected == null || selected == '.') {
      return '.';
    }
    final entry = controller.entryAt(selected);
    if (entry != null && entry.isDir) {
      return selected;
    }
    return parentOfPath(selected);
  }

  KeyEventResult _onKey(KeyEvent event) {
    if (event is! KeyDownEvent && event is! KeyRepeatEvent) {
      return KeyEventResult.ignored;
    }
    if (_edit != null || controller.projectId == null) {
      return KeyEventResult.ignored;
    }
    final keyboard = HardwareKeyboard.instance;
    if (keyboard.isControlPressed ||
        keyboard.isMetaPressed ||
        keyboard.isAltPressed) {
      return KeyEventResult.ignored;
    }
    final nodes = _visible();
    final selected = controller.selectedPath;
    final index = nodes.indexWhere((n) => n.path == selected);
    final key = event.logicalKey;
    if (key == LogicalKeyboardKey.arrowDown) {
      if (nodes.isEmpty) return KeyEventResult.ignored;
      _selectIndex(
        nodes,
        index < 0 ? 0 : (index + 1).clamp(0, nodes.length - 1),
      );
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.arrowUp) {
      if (nodes.isEmpty) return KeyEventResult.ignored;
      _selectIndex(nodes, index <= 0 ? 0 : index - 1);
      return KeyEventResult.handled;
    }
    if (index < 0) {
      return KeyEventResult.ignored;
    }
    final node = nodes[index];
    final open = controller.expanded.contains(node.path);
    if (key == LogicalKeyboardKey.arrowRight) {
      if (node.entry.isDir && !open) {
        unawaited(controller.expand(node.path));
      } else if (node.entry.isDir && index + 1 < nodes.length) {
        _selectIndex(nodes, index + 1);
      }
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.arrowLeft) {
      if (node.entry.isDir && open) {
        unawaited(controller.expand(node.path));
      } else {
        final parent = nodes.indexWhere((n) => n.path == node.dir);
        if (parent >= 0) _selectIndex(nodes, parent);
      }
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.enter ||
        key == LogicalKeyboardKey.numpadEnter) {
      _activate(node);
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.f2) {
      _beginRename(node.path);
      return KeyEventResult.handled;
    }
    if (key == LogicalKeyboardKey.delete) {
      unawaited(_confirmAndDelete(node.path, node.entry));
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  void _selectIndex(List<_Node> nodes, int index) {
    controller.select(nodes[index].path);
    // Rows share one height, so the offset is index * height.
    if (!_scroll.hasClients) {
      return;
    }
    final top = index * DesignTokens.treeRowHeight;
    final bottom = top + DesignTokens.treeRowHeight;
    final position = _scroll.position;
    if (top < position.pixels) {
      _scroll.jumpTo(top);
    } else if (bottom > position.pixels + position.viewportDimension) {
      _scroll.jumpTo(bottom - position.viewportDimension);
    }
  }

  // --------------------------------------------------------- inline edit

  void _beginRename(String path) {
    setState(() {
      _edit = _InlineEdit(kind: _EditKind.rename, path: path);
      _editError = null;
    });
  }

  Future<void> _beginCreate(String dir, {required bool folder}) async {
    if (!controller.expanded.contains(dir)) {
      await controller.expand(dir);
    }
    if (!mounted) {
      return;
    }
    setState(() {
      _edit = _InlineEdit(
        kind: folder ? _EditKind.newFolder : _EditKind.newFile,
        path: dir,
      );
      _editError = null;
    });
  }

  void _cancelEdit() {
    if (_edit == null) {
      return;
    }
    setState(() {
      _edit = null;
      _editError = null;
    });
    _treeFocus.requestFocus();
  }

  Future<void> _commitEdit(String value) async {
    final edit = _edit;
    if (edit == null || _busy) {
      return;
    }
    if (edit.kind == _EditKind.rename &&
        value.trim() == baseNameOfPath(edit.path)) {
      _cancelEdit();
      return;
    }
    final problem = validateEntryName(value);
    if (problem != null) {
      setState(() => _editError = problem);
      return;
    }
    _busy = true;
    try {
      switch (edit.kind) {
        case _EditKind.rename:
          await controller.renamePath(edit.path, value);
        case _EditKind.newFile:
          await controller.createFile(edit.path, value);
        case _EditKind.newFolder:
          await controller.createDir(edit.path, value);
      }
      if (mounted) {
        setState(() {
          _edit = null;
          _editError = null;
        });
        _treeFocus.requestFocus();
      }
    } on Object catch (e, s) {
      AppLog.record('file tree edit: $e', s);
      // Keep the editor open so the typed name is not lost.
      if (mounted) {
        setState(() => _editError = operatorMessageFromError(e));
      }
    } finally {
      _busy = false;
    }
  }

  // ------------------------------------------------------- drag and drop

  bool _canDropOn(String dir, String dragged) {
    if (dir == dragged || isSameOrDescendant(dir, dragged)) {
      return false;
    }
    return parentOfPath(dragged) != dir;
  }

  Future<void> _dropOn(String dir, String dragged) async {
    try {
      await controller.movePath(
        dragged,
        joinProjectPath(dir, baseNameOfPath(dragged)),
      );
      if (dir != '.' && !controller.expanded.contains(dir)) {
        await controller.expand(dir);
      }
    } on Object catch (e, s) {
      _reportFailure('Could not move', e, s);
    }
  }

  void _reportFailure(String action, Object error, StackTrace stack) {
    AppLog.record('$action: $error', stack);
    if (!mounted) {
      return;
    }
    ScaffoldMessenger.maybeOf(context)?.showSnackBar(
      SnackBar(content: Text('$action: ${operatorMessageFromError(error)}')),
    );
  }

  // ----------------------------------------------------------- menus

  Widget _overflowMenu(
    BuildContext context,
    DesignTokens tokens, {
    required bool hasProject,
  }) {
    return PopupMenuButton<String>(
      key: const Key('explorer-more'),
      tooltip: 'More actions',
      enabled: hasProject,
      padding: EdgeInsets.zero,
      constraints: const BoxConstraints(minWidth: 180),
      style: _HeaderAction.style,
      icon: Icon(Icons.more_horiz, size: 16, color: tokens.textMuted),
      onSelected: (value) {
        switch (value) {
          case 'save':
            unawaited(
              controller.saveFocused().catchError((Object e, StackTrace s) {
                AppLog.record('saveFocused: $e', s);
              }),
            );
          case 'checkpoint':
            unawaited(
              showCheckpointDialog(context, controller).catchError((
                Object e,
                StackTrace s,
              ) {
                AppLog.record('checkpoint dialog: $e', s);
              }),
            );
          case 'history':
            unawaited(
              showHistoryDialog(context, controller).catchError((
                Object e,
                StackTrace s,
              ) {
                AppLog.record('history dialog: $e', s);
              }),
            );
          case 'preview':
            unawaited(
              controller.previewSite().catchError((Object e, StackTrace s) {
                AppLog.record('previewSite: $e', s);
              }),
            );
          case 'export-download':
            unawaited(_export(context, 'download'));
          case 'export-netlify':
            unawaited(_export(context, 'netlify'));
        }
      },
      itemBuilder: (context) => [
        PopupMenuItem(
          key: const Key('save-file'),
          value: 'save',
          enabled: controller.focusedView != null,
          child: const _MenuRow(icon: Icons.save_outlined, label: 'Save'),
        ),
        const PopupMenuItem(
          key: Key('checkpoint-button'),
          value: 'checkpoint',
          child: _MenuRow(
            icon: Icons.bookmark_add_outlined,
            label: 'Checkpoint',
          ),
        ),
        const PopupMenuItem(
          key: Key('history-button'),
          value: 'history',
          child: _MenuRow(icon: Icons.history, label: 'History'),
        ),
        const PopupMenuItem(
          key: Key('explorer-preview-site'),
          value: 'preview',
          child: _MenuRow(icon: Icons.language, label: 'Preview site'),
        ),
        if (widget.onExport != null) ...[
          const PopupMenuDivider(),
          const PopupMenuItem(
            key: Key('explorer-export-download'),
            value: 'export-download',
            child: _MenuRow(
              icon: Icons.download_outlined,
              label: 'Download zip',
            ),
          ),
          const PopupMenuItem(
            key: Key('explorer-export-netlify'),
            value: 'export-netlify',
            child: _MenuRow(
              icon: Icons.cloud_upload_outlined,
              label: 'Publish to Netlify',
            ),
          ),
        ],
      ],
    );
  }

  Future<void> _export(BuildContext context, String method) async {
    final export = widget.onExport;
    if (export == null) {
      return;
    }
    await runProjectExport(context, method: method, export: export);
  }

  Future<void> _showMenu(
    BuildContext context,
    _Node node,
    Offset position,
  ) async {
    final path = node.path;
    final entry = node.entry;
    controller.select(path);
    final overlay =
        Overlay.of(context).context.findRenderObject()! as RenderBox;
    final selected = await showMenu<String>(
      context: context,
      position: RelativeRect.fromRect(
        position & const Size(1, 1),
        Offset.zero & overlay.size,
      ),
      items: [
        const PopupMenuItem(value: 'open', child: Text('Open')),
        const PopupMenuItem(
          value: 'open-side',
          child: Text('Open to the Side'),
        ),
        const PopupMenuItem(value: 'open-with', child: Text('Open with...')),
        if (entry.isDir) ...[
          const PopupMenuItem(value: 'new-file', child: Text('New file')),
          const PopupMenuItem(value: 'new-folder', child: Text('New folder')),
        ],
        const PopupMenuDivider(),
        const PopupMenuItem(
          key: Key('menu-rename'),
          value: 'rename',
          child: Text('Rename'),
        ),
        const PopupMenuItem(
          key: Key('menu-move'),
          value: 'move',
          child: Text('Move...'),
        ),
        const PopupMenuItem(
          key: Key('menu-duplicate'),
          value: 'duplicate',
          child: Text('Duplicate'),
        ),
        const PopupMenuDivider(),
        const PopupMenuItem(
          key: Key('menu-delete'),
          value: 'delete',
          child: Text('Delete'),
        ),
      ],
    );
    if (!mounted || selected == null) {
      return;
    }
    try {
      switch (selected) {
        case 'open':
          if (entry.isDir) {
            await controller.expand(path);
          } else {
            await controller.openDefault(path);
          }
        case 'open-side':
          if (!entry.isDir) {
            await controller.openDefault(path, toSide: true);
          }
        case 'open-with':
          if (!entry.isDir) {
            await _openWithPicker(path);
          }
        case 'new-file':
          await _beginCreate(path, folder: false);
        case 'new-folder':
          await _beginCreate(path, folder: true);
        case 'rename':
          _beginRename(path);
        case 'move':
          await _moveViaPicker(path);
        case 'duplicate':
          await controller.duplicatePath(path);
        case 'delete':
          await _confirmAndDelete(path, entry);
      }
    } on Object catch (e, s) {
      _reportFailure('Could not ${_verb(selected)}', e, s);
    }
  }

  String _verb(String action) => switch (action) {
    'duplicate' => 'duplicate',
    'move' => 'move',
    'rename' => 'rename',
    'delete' => 'delete',
    _ => 'complete that action',
  };

  Future<void> _moveViaPicker(String path) async {
    final dir = await showFolderPickerDialog(
      context,
      controller: controller,
      sourcePath: path,
    );
    if (dir == null || !mounted) {
      return;
    }
    await controller.movePath(path, joinProjectPath(dir, baseNameOfPath(path)));
    if (dir != '.' && !controller.expanded.contains(dir)) {
      await controller.expand(dir);
    }
  }

  Future<void> _confirmAndDelete(String path, FsEntry entry) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Delete ${entry.name}?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            key: const Key('delete-confirm'),
            autofocus: true,
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (ok != true || !mounted) {
      return;
    }
    try {
      await controller.deletePath(path);
    } on Object catch (e, s) {
      _reportFailure('Could not delete', e, s);
    }
    if (mounted) {
      _treeFocus.requestFocus();
    }
  }

  Future<void> _openWithPicker(String path) async {
    final selected = await showDialog<ProjectFileAppId>(
      context: context,
      builder: (context) => SimpleDialog(
        title: const Text('Open with'),
        children: [
          for (final app in ProjectFileAppId.values)
            SimpleDialogOption(
              key: Key('open-with-${app.name}'),
              onPressed: () => Navigator.pop(context, app),
              child: Text(
                '${appLabel(app)}${appCanOpen(app, path) ? '' : ' (any)'}',
              ),
            ),
        ],
      ),
    );
    if (selected != null) {
      await controller.openWith(path, selected);
    }
  }
}

/// Drop target for a folder row (or the root header). Highlights while a
/// droppable entry hovers over it.
class _DropZone extends StatelessWidget {
  const _DropZone({
    super.key,
    required this.dir,
    required this.canAccept,
    required this.onDrop,
    required this.child,
    this.highlightWhen,
  });

  final String dir;
  final bool Function(String dir, String dragged) canAccept;
  final Future<void> Function(String dir, String dragged) onDrop;
  final Widget child;

  /// Hover highlight predicate; defaults to [canAccept].
  final bool Function(String dir, String dragged)? highlightWhen;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final highlight = highlightWhen ?? canAccept;
    return DragTarget<String>(
      onWillAcceptWithDetails: (details) => canAccept(dir, details.data),
      onAcceptWithDetails: (details) => unawaited(onDrop(dir, details.data)),
      builder: (context, candidates, _) {
        final hovering = candidates.any(
          (dragged) => dragged != null && highlight(dir, dragged),
        );
        return DecoratedBox(
          decoration: BoxDecoration(
            color: hovering ? tokens.surfaceActive : null,
            border: hovering
                ? Border.all(color: tokens.focus)
                : Border.all(color: const Color(0x00000000)),
            borderRadius: BorderRadius.circular(DesignTokens.radiusXs),
          ),
          child: child,
        );
      },
    );
  }
}

class _DragChip extends StatelessWidget {
  const _DragChip({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Material(
      color: tokens.surfaceRaised,
      elevation: 2,
      borderRadius: BorderRadius.circular(DesignTokens.radiusXs),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        child: Text(label, style: tokens.bodySm()),
      ),
    );
  }
}

/// Row that swaps the name for a text field. Enter commits, Escape or losing
/// focus cancels, and a rejected name stays open with its error shown below.
class _InlineNameRow extends StatefulWidget {
  const _InlineNameRow({
    super.key,
    required this.depth,
    required this.isDir,
    required this.initial,
    required this.error,
    required this.onSubmit,
    required this.onCancel,
  });

  final int depth;
  final bool isDir;
  final String initial;
  final String? error;
  final Future<void> Function(String value) onSubmit;
  final VoidCallback onCancel;

  @override
  State<_InlineNameRow> createState() => _InlineNameRowState();
}

class _InlineNameRowState extends State<_InlineNameRow> {
  late final TextEditingController _text;
  late final FocusNode _focus = FocusNode(
    debugLabel: 'inline-name',
    // The field's own node sees Escape before the text-editing shortcuts do.
    onKeyEvent: (_, event) {
      if (event is KeyDownEvent &&
          event.logicalKey == LogicalKeyboardKey.escape) {
        widget.onCancel();
        return KeyEventResult.handled;
      }
      return KeyEventResult.ignored;
    },
  );

  @override
  void initState() {
    super.initState();
    _text = TextEditingController(text: widget.initial)
      ..selection = TextSelection(
        baseOffset: 0,
        extentOffset: widget.isDir
            ? widget.initial.length
            : renameSelectionLength(widget.initial),
      );
    _focus.addListener(_onFocus);
    // autofocus is ignored while the tree already holds focus, so ask for it.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        _focus.requestFocus();
      }
    });
  }

  void _onFocus() {
    // Clicking elsewhere abandons the edit, like Escape. The window itself
    // losing focus (alt-tab) keeps it so the typed name survives.
    final lifecycle = WidgetsBinding.instance.lifecycleState;
    final windowActive =
        lifecycle == null || lifecycle == AppLifecycleState.resumed;
    if (!_focus.hasFocus && windowActive) {
      widget.onCancel();
    }
  }

  @override
  void dispose() {
    _focus
      ..removeListener(_onFocus)
      ..dispose();
    _text.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final hasError = widget.error != null;
    final (icon, color) = widget.isDir
        ? (Icons.folder, tokens.secondary)
        : fileIconFor(_text.text, tokens);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          height: DesignTokens.treeRowHeight,
          child: Padding(
            padding: EdgeInsets.only(
              left: 6 + (widget.depth - 1) * DesignTokens.treeIndent,
              right: 6,
            ),
            child: Row(
              children: [
                const SizedBox(width: _chevronWidth),
                Icon(icon, size: 16, color: color),
                const SizedBox(width: 6),
                Expanded(
                  child: TextField(
                    key: const Key('create-name-field'),
                    controller: _text,
                    focusNode: _focus,
                    autofocus: true,
                    style: tokens.bodySm().copyWith(color: tokens.textPrimary),
                    decoration: InputDecoration(
                      isDense: true,
                      filled: true,
                      fillColor: tokens.surfaceRaised,
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 6,
                        vertical: 6,
                      ),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(
                          DesignTokens.radiusXs,
                        ),
                        borderSide: BorderSide(
                          color: hasError ? tokens.error : tokens.focus,
                        ),
                      ),
                      enabledBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(
                          DesignTokens.radiusXs,
                        ),
                        borderSide: BorderSide(
                          color: hasError ? tokens.error : tokens.focus,
                        ),
                      ),
                      focusedBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(
                          DesignTokens.radiusXs,
                        ),
                        borderSide: BorderSide(
                          color: hasError ? tokens.error : tokens.focus,
                        ),
                      ),
                    ),
                    // Skip the default unfocus, which would read as a cancel.
                    onEditingComplete: () {},
                    onSubmitted: (value) {
                      unawaited(widget.onSubmit(value));
                    },
                  ),
                ),
              ],
            ),
          ),
        ),
        if (hasError)
          Padding(
            padding: EdgeInsets.fromLTRB(
              6 +
                  _chevronWidth +
                  22 +
                  (widget.depth - 1) * DesignTokens.treeIndent,
              0,
              6,
              4,
            ),
            child: Text(
              widget.error!,
              key: const Key('inline-edit-error'),
              style: tokens.caption().copyWith(color: tokens.error),
            ),
          ),
      ],
    );
  }
}

const double _chevronWidth = 18;

/// Tonal row background: active fill when [selected], raised fill on hover.
class _RowSurface extends StatelessWidget {
  const _RowSurface({
    super.key,
    required this.child,
    this.onTap,
    this.onSecondaryTap,
    this.selected = false,
    this.focusRing = false,
  });

  final Widget child;
  final VoidCallback? onTap;
  final ValueChanged<Offset>? onSecondaryTap;
  final bool selected;
  final bool focusRing;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final radius = BorderRadius.circular(DesignTokens.radiusXs);
    return Material(
      color: selected ? tokens.surfaceActive : Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: radius,
        side: focusRing ? BorderSide(color: tokens.focus) : BorderSide.none,
      ),
      child: InkWell(
        borderRadius: radius,
        hoverColor: tokens.surfaceRaised,
        onTap: onTap,
        onSecondaryTapUp: onSecondaryTap == null
            ? null
            : (details) => onSecondaryTap!(details.globalPosition),
        child: SizedBox(
          height: DesignTokens.treeRowHeight,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 6),
            child: child,
          ),
        ),
      ),
    );
  }
}

class _Chevron extends StatelessWidget {
  const _Chevron({required this.open, required this.tokens});

  final bool open;
  final DesignTokens tokens;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: _chevronWidth,
      child: Icon(
        open ? Icons.expand_more : Icons.chevron_right,
        size: 16,
        color: tokens.textMuted,
      ),
    );
  }
}

class _HeaderAction extends StatelessWidget {
  const _HeaderAction({
    super.key,
    required this.tooltip,
    required this.icon,
    required this.onPressed,
  });

  final String tooltip;
  final IconData icon;
  final VoidCallback? onPressed;

  static final ButtonStyle style = IconButton.styleFrom(
    padding: EdgeInsets.zero,
    minimumSize: const Size(28, 28),
    fixedSize: const Size(28, 28),
    tapTargetSize: MaterialTapTargetSize.shrinkWrap,
    shape: RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
    ),
  );

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return IconButton(
      tooltip: tooltip,
      style: style,
      onPressed: onPressed,
      icon: Icon(icon, size: 16, color: tokens.textMuted),
    );
  }
}

class _MenuRow extends StatelessWidget {
  const _MenuRow({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return Row(
      children: [
        Icon(icon, size: 16, color: tokens.textSecondary),
        const SizedBox(width: 10),
        Text(label, style: tokens.bodySm()),
      ],
    );
  }
}

class _TreeRow extends StatefulWidget {
  const _TreeRow({
    super.key,
    required this.name,
    required this.isDir,
    required this.expanded,
    required this.depth,
    required this.selected,
    required this.focused,
    required this.modified,
    required this.menuKey,
    required this.onTap,
    required this.onMenu,
  });

  final String name;
  final bool isDir;
  final bool expanded;
  final int depth;
  final bool selected;
  final bool focused;
  final bool modified;
  final Key menuKey;
  final VoidCallback onTap;
  final ValueChanged<Offset> onMenu;

  @override
  State<_TreeRow> createState() => _TreeRowState();
}

class _TreeRowState extends State<_TreeRow> {
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final (icon, iconColor) = widget.isDir
        ? (widget.expanded ? Icons.folder_open : Icons.folder, tokens.secondary)
        : fileIconFor(widget.name, tokens);
    return MouseRegion(
      onEnter: (_) => setState(() => _hovered = true),
      onExit: (_) => setState(() => _hovered = false),
      child: _RowSurface(
        selected: widget.selected,
        focusRing: widget.selected && widget.focused,
        onTap: widget.onTap,
        onSecondaryTap: widget.onMenu,
        child: Padding(
          padding: EdgeInsets.only(
            left: (widget.depth - 1) * DesignTokens.treeIndent,
          ),
          child: Row(
            children: [
              if (widget.isDir)
                _Chevron(open: widget.expanded, tokens: tokens)
              else
                const SizedBox(width: _chevronWidth),
              Icon(icon, size: 16, color: iconColor),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  widget.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: tokens.bodySm().copyWith(
                    color: widget.selected
                        ? tokens.textPrimary
                        : tokens.textSecondary,
                  ),
                ),
              ),
              if (widget.modified)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 4),
                  child: Tooltip(
                    message: 'Unsaved changes',
                    child: Text(
                      'M',
                      key: Key('file-modified-${widget.name}'),
                      style: tokens.caption().copyWith(
                        color: tokens.warning,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                ),
              // Stays hittable while hidden so keyboard and tests reach it.
              Opacity(
                opacity: _hovered || widget.selected ? 1 : 0,
                child: Builder(
                  builder: (buttonContext) => IconButton(
                    key: widget.menuKey,
                    tooltip: 'More',
                    padding: EdgeInsets.zero,
                    constraints: const BoxConstraints.tightFor(
                      width: 22,
                      height: 22,
                    ),
                    icon: Icon(
                      Icons.more_horiz,
                      size: 14,
                      color: tokens.textMuted,
                    ),
                    onPressed: () {
                      final box =
                          buttonContext.findRenderObject()! as RenderBox;
                      widget.onMenu(
                        box.localToGlobal(box.size.bottomLeft(Offset.zero)),
                      );
                    },
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Leading glyph and tint for a file row, keyed by name and extension.
///
/// Code, markup, and data share the secondary blue; everything else stays
/// neutral so the tree does not turn into a color legend.
(IconData, Color) fileIconFor(String name, DesignTokens tokens) {
  final lower = name.toLowerCase();
  final dot = lower.lastIndexOf('.');
  final ext = dot <= 0 ? '' : lower.substring(dot + 1);
  if (lower.startsWith('.env') ||
      lower.startsWith('.git') ||
      const {
        'toml',
        'yaml',
        'yml',
        'ini',
        'cfg',
        'conf',
        'lock',
      }.contains(ext)) {
    return (Icons.settings_outlined, tokens.textMuted);
  }
  switch (ext) {
    case 'md' || 'markdown' || 'mdx' || 'txt' || 'rst':
      return (Icons.description_outlined, tokens.textSecondary);
    case 'html' || 'htm':
      return (Icons.html, tokens.secondary);
    case 'css' || 'scss' || 'sass' || 'less':
      return (Icons.css, tokens.secondary);
    case 'js' || 'mjs' || 'cjs' || 'jsx' || 'ts' || 'tsx':
      return (Icons.javascript, tokens.secondary);
    case 'json' || 'jsonl' || 'xml' || 'csv':
      return (Icons.data_object, tokens.secondary);
    case 'py' ||
        'dart' ||
        'go' ||
        'rs' ||
        'java' ||
        'kt' ||
        'c' ||
        'h' ||
        'cpp' ||
        'rb' ||
        'php' ||
        'sh' ||
        'sql':
      return (Icons.code, tokens.secondary);
    case 'png' || 'jpg' || 'jpeg' || 'gif' || 'webp' || 'svg' || 'ico':
      return (Icons.image_outlined, tokens.textSecondary);
    case 'mp3' || 'wav' || 'ogg' || 'flac' || 'm4a':
      return (Icons.audiotrack_outlined, tokens.textSecondary);
    case 'pdf':
      return (Icons.picture_as_pdf_outlined, tokens.textSecondary);
  }
  return (Icons.insert_drive_file_outlined, tokens.textMuted);
}
