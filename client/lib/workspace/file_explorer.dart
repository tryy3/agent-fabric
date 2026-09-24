import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../ui/theme/design_tokens.dart';
import 'git_history.dart';
import 'open_with.dart';
import 'workspace_controller.dart';

/// Compact IDE-style tree for the active project's files.
///
/// The root row names the project and carries the pane actions; less frequent
/// actions sit in its overflow menu.
class FileExplorer extends StatelessWidget {
  const FileExplorer({super.key, required this.controller});

  final WorkspaceController controller;

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

  Widget _buildTree(BuildContext context) {
    final tokens = designTokensOf(context);
    final hasProject = controller.projectId != null;
    final rootOpen = controller.expanded.contains('.');
    final entries = controller.children['.'] ?? const <FsEntry>[];
    final selectedPath = controller.focusedView?.path;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(4, 6, 4, 2),
          child: SizedBox(
            height: DesignTokens.treeRowHeight,
            child: Row(
              children: [
                Expanded(
                  child: _RowSurface(
                    key: const Key('explorer-root'),
                    onTap: hasProject ? () => controller.expand('.') : null,
                    child: Row(
                      children: [
                        _Chevron(open: rootOpen, tokens: tokens),
                        Expanded(
                          child: Text(
                            controller.projectName ?? 'Workspace',
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
                _HeaderAction(
                  key: const Key('explorer-new-file'),
                  tooltip: 'New file',
                  icon: Icons.note_add_outlined,
                  onPressed: hasProject
                      ? () => _promptCreate(context, dir: '.', folder: false)
                      : null,
                ),
                _HeaderAction(
                  key: const Key('explorer-new-folder'),
                  tooltip: 'New folder',
                  icon: Icons.create_new_folder_outlined,
                  onPressed: hasProject
                      ? () => _promptCreate(context, dir: '.', folder: true)
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
        ),
        if (controller.error != null)
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 4, 12, 8),
            child: Text(
              controller.error!,
              style: tokens.caption().copyWith(color: tokens.error),
            ),
          ),
        Expanded(
          child: ListView(
            key: const Key('file-explorer'),
            padding: const EdgeInsets.fromLTRB(4, 0, 4, 8),
            children: [
              if (rootOpen)
                for (final entry in _sorted(entries))
                  ..._rows(
                    context,
                    dir: '.',
                    entry: entry,
                    depth: 1,
                    selectedPath: selectedPath,
                  ),
            ],
          ),
        ),
      ],
    );
  }

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
            controller.saveFocused();
          case 'checkpoint':
            showCheckpointDialog(context, controller);
          case 'history':
            showHistoryDialog(context, controller);
          case 'preview':
            controller.previewSite();
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
      ],
    );
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

  List<Widget> _rows(
    BuildContext context, {
    required String dir,
    required FsEntry entry,
    required int depth,
    required String? selectedPath,
  }) {
    final path = dir == '.' ? entry.name : '$dir/${entry.name}';
    final expanded = controller.expanded.contains(path);
    final dirty = controller.documents[path]?.isDirty ?? false;
    final children = <Widget>[
      _TreeRow(
        key: Key('file-row-${entry.name}'),
        name: entry.name,
        isDir: entry.isDir,
        expanded: expanded,
        depth: depth,
        selected: path == selectedPath,
        modified: dirty,
        menuKey: Key('file-menu-${entry.name}'),
        onTap: () {
          if (entry.isDir) {
            controller.expand(path);
          } else {
            controller.openDefault(path);
          }
        },
        onMenu: () => _showMenu(context, path: path, entry: entry, dir: dir),
      ),
    ];
    if (entry.isDir && expanded) {
      final nested = controller.children[path] ?? const <FsEntry>[];
      for (final child in _sorted(nested)) {
        children.addAll(
          _rows(
            context,
            dir: path,
            entry: child,
            depth: depth + 1,
            selectedPath: selectedPath,
          ),
        );
      }
    }
    return children;
  }

  Future<void> _showMenu(
    BuildContext context, {
    required String path,
    required FsEntry entry,
    required String dir,
  }) async {
    final selected = await showMenu<String>(
      context: context,
      position: const RelativeRect.fromLTRB(120, 80, 0, 0),
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
        const PopupMenuItem(value: 'delete', child: Text('Delete')),
      ],
    );
    if (!context.mounted || selected == null) {
      return;
    }
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
          await _openWithPicker(context, path);
        }
      case 'new-file':
        await _promptCreate(context, dir: path, folder: false);
      case 'new-folder':
        await _promptCreate(context, dir: path, folder: true);
      case 'delete':
        final ok = await showDialog<bool>(
          context: context,
          builder: (context) => AlertDialog(
            title: const Text('Delete?'),
            content: Text('Delete ${entry.name}?'),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(context, false),
                child: const Text('Cancel'),
              ),
              FilledButton(
                onPressed: () => Navigator.pop(context, true),
                child: const Text('Delete'),
              ),
            ],
          ),
        );
        if (ok == true) {
          await controller.deletePath(path);
        }
    }
  }

  Future<void> _openWithPicker(BuildContext context, String path) async {
    final selected = await showDialog<WorkspaceAppId>(
      context: context,
      builder: (context) => SimpleDialog(
        title: const Text('Open with'),
        children: [
          for (final app in WorkspaceAppId.values)
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

  Future<void> _promptCreate(
    BuildContext context, {
    required String dir,
    required bool folder,
  }) async {
    final name = await showDialog<String>(
      context: context,
      builder: (context) {
        final input = TextEditingController();
        return AlertDialog(
          title: Text(folder ? 'New folder' : 'New file'),
          content: TextField(
            key: const Key('create-name-field'),
            controller: input,
            autofocus: true,
            decoration: InputDecoration(
              hintText: folder ? 'src' : 'index.html',
            ),
            onSubmitted: (value) => Navigator.pop(context, value),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Cancel'),
            ),
            FilledButton(
              key: const Key('create-confirm'),
              onPressed: () => Navigator.pop(context, input.text),
              child: const Text('Create'),
            ),
          ],
        );
      },
    );
    if (name == null || name.trim().isEmpty) {
      return;
    }
    if (folder) {
      await controller.createDir(dir, name);
    } else {
      await controller.createFile(dir, name);
    }
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
  });

  final Widget child;
  final VoidCallback? onTap;
  final VoidCallback? onSecondaryTap;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final radius = BorderRadius.circular(DesignTokens.radiusXs);
    return Material(
      color: selected ? tokens.surfaceActive : Colors.transparent,
      borderRadius: radius,
      child: InkWell(
        borderRadius: radius,
        hoverColor: tokens.surfaceRaised,
        onTap: onTap,
        onSecondaryTap: onSecondaryTap,
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
  final bool modified;
  final Key menuKey;
  final VoidCallback onTap;
  final VoidCallback onMenu;

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
                child: IconButton(
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
                  onPressed: widget.onMenu,
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
