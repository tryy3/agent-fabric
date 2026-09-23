import 'dart:async';

import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../dock/dock_view_body.dart';
import '../dock/files_dock_panel.dart';
import 'open_with.dart';
import 'workspace_controller.dart';

class SaveFileIntent extends Intent {
  const SaveFileIntent();
}

/// Files explorer and a single column of open views for narrow screens.
class WorkspacePane extends StatelessWidget {
  const WorkspacePane({super.key, required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    return Shortcuts(
      shortcuts: const {
        SingleActivator(LogicalKeyboardKey.keyS, meta: true): SaveFileIntent(),
        SingleActivator(LogicalKeyboardKey.keyS, control: true):
            SaveFileIntent(),
      },
      child: Actions(
        actions: {
          SaveFileIntent: CallbackAction<SaveFileIntent>(
            onInvoke: (_) {
              controller.saveFocused();
              return null;
            },
          ),
        },
        child: Focus(
          autofocus: true,
          child: ListenableBuilder(
            listenable: controller,
            builder: (context, _) {
              final files = FilesDockPanel(controller: controller);
              final views = _OpenViews(controller: controller);
              return Column(
                children: [
                  Expanded(
                    flex: controller.openViews.isEmpty ? 1 : 2,
                    child: files,
                  ),
                  if (controller.openViews.isNotEmpty)
                    Expanded(flex: 3, child: views),
                ],
              );
            },
          ),
        ),
      ),
    );
  }
}

class _OpenViews extends StatelessWidget {
  const _OpenViews({required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    final views = controller.openViews;
    final focusedId = controller.focusedView?.viewId;
    return Column(
      children: [
        SizedBox(
          height: 36,
          child: ListView(
            scrollDirection: Axis.horizontal,
            children: [
              for (final tab in views)
                Padding(
                  padding: const EdgeInsets.only(right: 4),
                  child: InputChip(
                    key: Key('tab-${tab.viewId}'),
                    selected: tab.viewId == focusedId,
                    label: Text(tab.tabLabel),
                    onPressed: () => controller.focusView(tab.viewId),
                    onDeleted: () {
                      unawaited(
                        confirmDirtyViewClose(context, controller, tab),
                      );
                    },
                  ),
                ),
            ],
          ),
        ),
        Expanded(
          child: DockViewBody(
            controller: controller,
            view: controller.focusedView,
          ),
        ),
      ],
    );
  }
}

class WorkspacePage extends StatelessWidget {
  const WorkspacePage({super.key, required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Files')),
      body: WorkspacePane(controller: controller),
    );
  }
}

enum _DirtyCloseAction { save, discard }

/// Closes [view]. A dirty [FileDocument] for that path asks Save, Discard,
/// or Cancel first. Cancel leaves the view open. Save writes, then closes.
Future<bool> confirmDirtyViewClose(
  BuildContext context,
  WorkspaceController controller,
  OpenView view,
) async {
  final doc = controller.documentFor(view.path);
  if (doc != null && doc.isDirty) {
    final action = await showDialog<_DirtyCloseAction>(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          title: const Text('Save changes?'),
          content: Text('${view.path} has unsaved changes.'),
          actions: [
            TextButton(
              key: const Key('dirty-close-cancel'),
              onPressed: () => Navigator.pop(dialogContext),
              child: const Text('Cancel'),
            ),
            TextButton(
              key: const Key('dirty-close-discard'),
              onPressed: () =>
                  Navigator.pop(dialogContext, _DirtyCloseAction.discard),
              child: const Text('Discard'),
            ),
            FilledButton(
              key: const Key('dirty-close-save'),
              onPressed: () =>
                  Navigator.pop(dialogContext, _DirtyCloseAction.save),
              child: const Text('Save'),
            ),
          ],
        );
      },
    );
    if (!context.mounted || action == null) {
      return false;
    }
    if (action == _DirtyCloseAction.save) {
      await controller.savePath(view.path);
    }
  }
  await controller.closeView(view.viewId);
  return true;
}
