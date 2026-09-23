import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import 'open_with.dart';
import 'workspace_controller.dart';

class FileExplorer extends StatelessWidget {
  const FileExplorer({super.key, required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) {
        final entries = controller.children['.'] ?? const <FsEntry>[];
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 8, 4),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Text(
                    'Files',
                    style: TextStyle(fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 4),
                  Wrap(
                    spacing: 0,
                    runSpacing: 0,
                    children: [
                      IconButton(
                        key: const Key('explorer-refresh'),
                        tooltip: 'Refresh',
                        visualDensity: VisualDensity.compact,
                        icon: const Icon(Icons.refresh, size: 18),
                        onPressed: controller.projectId == null
                            ? null
                            : controller.refreshTree,
                      ),
                      IconButton(
                        key: const Key('explorer-new-file'),
                        tooltip: 'New file',
                        visualDensity: VisualDensity.compact,
                        icon: const Icon(Icons.note_add_outlined, size: 18),
                        onPressed: controller.projectId == null
                            ? null
                            : () => _promptCreate(
                                context,
                                dir: '.',
                                folder: false,
                              ),
                      ),
                      IconButton(
                        key: const Key('explorer-new-folder'),
                        tooltip: 'New folder',
                        visualDensity: VisualDensity.compact,
                        icon: const Icon(
                          Icons.create_new_folder_outlined,
                          size: 18,
                        ),
                        onPressed: controller.projectId == null
                            ? null
                            : () => _promptCreate(
                                context,
                                dir: '.',
                                folder: true,
                              ),
                      ),
                      IconButton(
                        tooltip: 'Preview site',
                        visualDensity: VisualDensity.compact,
                        icon: const Icon(Icons.language, size: 18),
                        onPressed: controller.projectId == null
                            ? null
                            : controller.previewSite,
                      ),
                    ],
                  ),
                ],
              ),
            ),
            if (controller.error != null)
              Padding(
                padding: const EdgeInsets.all(8),
                child: Text(
                  controller.error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
            Expanded(
              child: ListView(
                key: const Key('file-explorer'),
                children: [
                  for (final entry in _sorted(entries))
                    ..._rows(context, dir: '.', entry: entry, depth: 0),
                ],
              ),
            ),
          ],
        );
      },
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
  }) {
    final path = dir == '.' ? entry.name : '$dir/${entry.name}';
    final expanded = controller.expanded.contains(path);
    final children = <Widget>[
      InkWell(
        key: Key('file-row-${entry.name}'),
        onTap: () {
          if (entry.isDir) {
            controller.expand(path);
          } else {
            controller.openDefault(path);
          }
        },
        onSecondaryTapDown: (details) {
          _showMenu(context, path: path, entry: entry, dir: dir);
        },
        child: Padding(
          padding: EdgeInsets.fromLTRB(8.0 + depth * 14, 6, 4, 6),
          child: Row(
            children: [
              Icon(
                entry.isDir
                    ? (expanded ? Icons.folder_open : Icons.folder)
                    : Icons.insert_drive_file_outlined,
                size: 18,
              ),
              const SizedBox(width: 6),
              Expanded(
                child: Text(entry.name, overflow: TextOverflow.ellipsis),
              ),
              IconButton(
                key: Key('file-menu-${entry.name}'),
                icon: const Icon(Icons.more_vert, size: 16),
                onPressed: () =>
                    _showMenu(context, path: path, entry: entry, dir: dir),
              ),
            ],
          ),
        ),
      ),
    ];
    if (entry.isDir && expanded) {
      final nested = controller.children[path] ?? const <FsEntry>[];
      for (final child in _sorted(nested)) {
        children.addAll(
          _rows(context, dir: path, entry: child, depth: depth + 1),
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
        const PopupMenuItem(value: 'open-with', child: Text('Open with…')),
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
