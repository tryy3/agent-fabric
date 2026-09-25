import 'package:agent_fabric_client/core/app_log.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import 'export_result_dialog.dart';

/// Runs an export/publish action with optional progress UI and result dialog.
Future<void> runProjectExport(
  BuildContext context, {
  required String method,
  required Future<ExportPublishResult?> Function(String method) export,
}) async {
  final isPublish = method != 'download';
  if (isPublish) {
    showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (context) => const AlertDialog(
        key: Key('export-progress-dialog'),
        content: Row(
          children: [
            CircularProgressIndicator(),
            SizedBox(width: 16),
            Expanded(child: Text('Publishing…')),
          ],
        ),
      ),
    );
  }
  try {
    final result = await export(method);
    if (isPublish && context.mounted) {
      Navigator.of(context, rootNavigator: true).pop();
    }
    if (result != null && context.mounted) {
      await showExportResultDialog(context, result);
    }
  } on Object catch (e, s) {
    AppLog.record('runProjectExport($method): $e', s);
    if (isPublish && context.mounted) {
      Navigator.of(context, rootNavigator: true).pop();
    }
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    }
  }
}
