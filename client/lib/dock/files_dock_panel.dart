import 'package:material_ui/material_ui.dart';

import '../workspace/file_explorer.dart';
import '../workspace/workspace_controller.dart';

class FilesDockPanel extends StatelessWidget {
  const FilesDockPanel({super.key, required this.controller});

  final WorkspaceController controller;

  @override
  Widget build(BuildContext context) {
    // Own Material under dock content-area DecoratedBox so ink paints.
    return Material(
      type: MaterialType.transparency,
      child: FileExplorer(controller: controller),
    );
  }
}
