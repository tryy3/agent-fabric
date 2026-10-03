import 'dart:async';

import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../core/operator_failure.dart';
import '../ui/theme/design_tokens.dart';
import 'project_files_controller.dart';
import 'project_paths.dart';

/// Asks where [sourcePath] should move. Returns the chosen destination
/// folder (`.` for the project root), or null when cancelled.
///
/// Browses folders one level at a time through the catalog so folders that
/// are not expanded in the tree can still be targets. The source itself is
/// hidden, which also keeps a folder from being moved into its own subtree.
Future<String?> showFolderPickerDialog(
  BuildContext context, {
  required ProjectFilesController controller,
  required String sourcePath,
}) {
  return showDialog<String>(
    context: context,
    builder: (context) =>
        _FolderPickerDialog(controller: controller, sourcePath: sourcePath),
  );
}

class _FolderPickerDialog extends StatefulWidget {
  const _FolderPickerDialog({
    required this.controller,
    required this.sourcePath,
  });

  final ProjectFilesController controller;
  final String sourcePath;

  @override
  State<_FolderPickerDialog> createState() => _FolderPickerDialogState();
}

class _FolderPickerDialogState extends State<_FolderPickerDialog> {
  String _current = projectRootPath;
  List<FsEntry> _folders = const [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    unawaited(_load(_current));
  }

  Future<void> _load(String dir) async {
    final id = widget.controller.projectId;
    if (id == null) {
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final listing = await widget.controller.catalog.listProjectFs(
        id,
        path: isProjectRoot(dir) ? '/' : dir,
      );
      if (!mounted) {
        return;
      }
      final folders = [
        for (final entry in listing.entries)
          if (entry.isDir &&
              joinProjectPath(dir, entry.name) != widget.sourcePath)
            entry,
      ]..sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
      setState(() {
        _current = dir;
        _folders = folders;
        _loading = false;
      });
    } on Object catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _error = operatorMessageFromError(e);
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final atRoot = isProjectRoot(_current);
    final alreadyHere = parentOfPath(widget.sourcePath) == _current;
    return AlertDialog(
      title: Text('Move ${baseNameOfPath(widget.sourcePath)}'),
      content: SizedBox(
        width: 360,
        height: 320,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                IconButton(
                  key: const Key('folder-picker-up'),
                  tooltip: 'Up one folder',
                  onPressed: atRoot
                      ? null
                      : () => _load(parentOfPath(_current)),
                  icon: const Icon(Icons.arrow_upward, size: 16),
                ),
                Expanded(
                  child: Text(
                    atRoot ? 'Project root' : _current,
                    key: const Key('folder-picker-current'),
                    overflow: TextOverflow.ellipsis,
                    style: tokens.labelSm().copyWith(color: tokens.textMuted),
                  ),
                ),
              ],
            ),
            if (_error != null)
              Text(
                _error!,
                style: tokens.caption().copyWith(color: tokens.error),
              ),
            Expanded(
              child: _loading
                  ? const Center(child: CircularProgressIndicator())
                  : _folders.isEmpty
                  ? Center(
                      child: Text(
                        'No subfolders',
                        style: tokens.bodySm().copyWith(
                          color: tokens.textMuted,
                        ),
                      ),
                    )
                  : ListView(
                      children: [
                        for (final folder in _folders)
                          ListTile(
                            key: Key('folder-picker-${folder.name}'),
                            dense: true,
                            leading: Icon(
                              Icons.folder,
                              size: 16,
                              color: tokens.secondary,
                            ),
                            title: Text(folder.name),
                            onTap: () =>
                                _load(joinProjectPath(_current, folder.name)),
                          ),
                      ],
                    ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          key: const Key('folder-picker-confirm'),
          onPressed: alreadyHere || _loading
              ? null
              : () => Navigator.pop(context, _current),
          child: const Text('Move here'),
        ),
      ],
    );
  }
}
