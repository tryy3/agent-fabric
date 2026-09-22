import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../dock/dock_view_body.dart';
import 'file_explorer.dart';
import 'git_history.dart';
import 'open_with.dart';
import 'workspace_controller.dart';

class SaveFileIntent extends Intent {
  const SaveFileIntent();
}

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
              return Column(
                children: [
                  Material(
                    elevation: 1,
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 8),
                      child: Row(
                        children: [
                          const Expanded(child: Text('Workspace')),
                          IconButton(
                            key: const Key('save-file'),
                            tooltip: 'Save',
                            onPressed: controller.focusedView == null
                                ? null
                                : controller.saveFocused,
                            icon: const Icon(Icons.save_outlined),
                          ),
                          IconButton(
                            key: const Key('checkpoint-button'),
                            tooltip: 'Checkpoint',
                            onPressed: controller.projectId == null
                                ? null
                                : () =>
                                      showCheckpointDialog(context, controller),
                            icon: const Icon(Icons.bookmark_add_outlined),
                          ),
                          IconButton(
                            key: const Key('history-button'),
                            tooltip: 'History',
                            onPressed: controller.projectId == null
                                ? null
                                : () => showHistoryDialog(context, controller),
                            icon: const Icon(Icons.history),
                          ),
                        ],
                      ),
                    ),
                  ),
                  Expanded(
                    child: Row(
                      children: [
                        SizedBox(
                          width: 168,
                          child: FileExplorer(controller: controller),
                        ),
                        const VerticalDivider(width: 1, thickness: 1),
                        Expanded(child: _Groups(controller: controller)),
                      ],
                    ),
                  ),
                ],
              );
            },
          ),
        ),
      ),
    );
  }
}

class _Groups extends StatelessWidget {
  const _Groups({required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    final groups = controller.groups;
    if (groups.isEmpty) {
      return const Center(child: Text('Open a file from the tree'));
    }
    return Row(
      children: [
        for (var i = 0; i < groups.length; i++) ...[
          if (i > 0) const VerticalDivider(width: 1, thickness: 1),
          Expanded(
            child: _GroupPane(
              controller: controller,
              group: groups[i],
              index: i,
            ),
          ),
        ],
      ],
    );
  }
}

class _GroupPane extends StatelessWidget {
  const _GroupPane({
    required this.controller,
    required this.group,
    required this.index,
  });

  final WorkspaceController controller;
  final EditorGroup group;
  final int index;

  @override
  Widget build(BuildContext context) {
    final active = group.active;
    return Column(
      children: [
        SizedBox(
          height: 36,
          child: ListView(
            scrollDirection: Axis.horizontal,
            children: [
              for (final tab in group.tabs)
                Padding(
                  padding: const EdgeInsets.only(right: 4),
                  child: InputChip(
                    key: Key('tab-${tab.viewId}'),
                    selected: tab.viewId == group.activeViewId,
                    label: Text(tab.tabLabel),
                    onPressed: () =>
                        controller.focusTab(group.groupId, tab.viewId),
                    onDeleted: () => controller.closeView(tab.viewId),
                  ),
                ),
            ],
          ),
        ),
        Expanded(
          child: GestureDetector(
            onTap: () => controller.focusGroup(group.groupId),
            child: _ViewBody(
              key: Key('editor-group-$index'),
              controller: controller,
              view: active,
            ),
          ),
        ),
      ],
    );
  }
}

class _ViewBody extends StatelessWidget {
  const _ViewBody({super.key, required this.controller, required this.view});

  final WorkspaceController controller;
  final OpenView? view;

  @override
  Widget build(BuildContext context) {
    return DockViewBody(controller: controller, view: view);
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
