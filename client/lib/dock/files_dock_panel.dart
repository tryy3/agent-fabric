import 'package:material_ui/material_ui.dart';

import '../workspace/file_explorer.dart';
import '../workspace/git_history.dart';
import '../workspace/workspace_controller.dart';

class FilesDockPanel extends StatelessWidget {
  const FilesDockPanel({super.key, required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        ListenableBuilder(
          listenable: controller,
          builder: (context, _) {
            return Material(
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
                          : () => showCheckpointDialog(context, controller),
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
            );
          },
        ),
        Expanded(child: FileExplorer(controller: controller)),
      ],
    );
  }
}
