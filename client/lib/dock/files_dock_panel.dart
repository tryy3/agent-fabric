import 'package:material_ui/material_ui.dart';

import '../ui/pane_header.dart';
import '../workspace/file_explorer.dart';
import '../workspace/git_history.dart';
import '../workspace/workspace_controller.dart';

class FilesDockPanel extends StatelessWidget {
  const FilesDockPanel({super.key, required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    // Own Material under dock content-area DecoratedBox so ink paints.
    return Material(
      type: MaterialType.transparency,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          ListenableBuilder(
            listenable: controller,
            builder: (context, _) {
              return Padding(
                padding: const EdgeInsets.fromLTRB(12, 10, 8, 6),
                child: PaneHeader(
                  title: const Text(
                    'Workspace',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                  actions: [
                    IconButton(
                      key: const Key('save-file'),
                      tooltip: 'Save',
                      style: PaneHeader.actionStyle,
                      onPressed: controller.focusedView == null
                          ? null
                          : controller.saveFocused,
                      icon: const Icon(Icons.save_outlined, size: 20),
                    ),
                    IconButton(
                      key: const Key('checkpoint-button'),
                      tooltip: 'Checkpoint',
                      style: PaneHeader.actionStyle,
                      onPressed: controller.projectId == null
                          ? null
                          : () => showCheckpointDialog(context, controller),
                      icon: const Icon(Icons.bookmark_add_outlined, size: 20),
                    ),
                    IconButton(
                      key: const Key('history-button'),
                      tooltip: 'History',
                      style: PaneHeader.actionStyle,
                      onPressed: controller.projectId == null
                          ? null
                          : () => showHistoryDialog(context, controller),
                      icon: const Icon(Icons.history, size: 20),
                    ),
                  ],
                ),
              );
            },
          ),
          Expanded(child: FileExplorer(controller: controller)),
        ],
      ),
    );
  }
}
