import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../workspace/file_explorer.dart';
import '../workspace/workspace_controller.dart';

class FilesDockPanel extends StatelessWidget {
  const FilesDockPanel({super.key, required this.controller, this.onExport});

  final WorkspaceController controller;

  /// Optional export/publish hook for the explorer overflow menu.
  final Future<ExportPublishResult?> Function(String method)? onExport;

  @override
  Widget build(BuildContext context) {
    // Own Material under dock content-area DecoratedBox so ink paints.
    return Material(
      type: MaterialType.transparency,
      child: FileExplorer(controller: controller, onExport: onExport),
    );
  }
}
