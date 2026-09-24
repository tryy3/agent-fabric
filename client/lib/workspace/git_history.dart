import 'package:material_ui/material_ui.dart';

import 'workspace_controller.dart';

Future<void> showCheckpointDialog(
  BuildContext context,
  WorkspaceController controller,
) async {
  final label = await showDialog<String>(
    context: context,
    builder: (context) => const _CheckpointDialog(),
  );
  if (label != null && label.trim().isNotEmpty) {
    await controller.createCheckpoint(label);
  }
}

class _CheckpointDialog extends StatefulWidget {
  const _CheckpointDialog();

  @override
  State<_CheckpointDialog> createState() => _CheckpointDialogState();
}

class _CheckpointDialogState extends State<_CheckpointDialog> {
  final TextEditingController _label = TextEditingController();

  @override
  void dispose() {
    _label.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Checkpoint'),
      content: TextField(
        key: const Key('checkpoint-label-field'),
        controller: _label,
        decoration: const InputDecoration(
          labelText: 'Label',
          hintText: 'before rewrite',
        ),
        autofocus: true,
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        FilledButton(
          key: const Key('checkpoint-confirm'),
          onPressed: () => Navigator.pop(context, _label.text),
          child: const Text('Checkpoint'),
        ),
      ],
    );
  }
}

Future<void> showHistoryDialog(
  BuildContext context,
  WorkspaceController controller,
) async {
  await controller.loadCommits();
  if (!context.mounted) {
    return;
  }
  await showDialog<void>(
    context: context,
    builder: (context) {
      return AlertDialog(
        title: const Text('History'),
        content: SizedBox(
          width: 480,
          height: 360,
          child: ListenableBuilder(
            listenable: controller,
            builder: (context, _) {
              if (controller.commits.isEmpty) {
                return const Text('No commits yet');
              }
              return ListView.builder(
                itemCount: controller.commits.length,
                itemBuilder: (context, index) {
                  final commit = controller.commits[index];
                  final title = commit.label?.isNotEmpty == true
                      ? commit.label!
                      : commit.message;
                  final short = commit.sha.length > 7
                      ? commit.sha.substring(0, 7)
                      : commit.sha;
                  return ListTile(
                    title: Text(title),
                    subtitle: Text(short),
                    trailing: Wrap(
                      children: [
                        TextButton(
                          key: Key('diff-${commit.sha}'),
                          onPressed: () =>
                              _showDiff(context, controller, commit.sha),
                          child: const Text('Diff'),
                        ),
                        FilledButton(
                          key: Key('restore-${commit.sha}'),
                          onPressed: () =>
                              _confirmRestore(context, controller, commit.sha),
                          child: const Text('Restore'),
                        ),
                      ],
                    ),
                  );
                },
              );
            },
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Close'),
          ),
        ],
      );
    },
  );
}

Future<void> _confirmRestore(
  BuildContext context,
  WorkspaceController controller,
  String sha,
) async {
  final confirmed = await showDialog<bool>(
    context: context,
    builder: (context) {
      return AlertDialog(
        title: const Text('Restore files?'),
        content: const Text(
          'Restore the workspace files to this commit. Chat is unchanged.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            key: const Key('restore-confirm'),
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Restore'),
          ),
        ],
      );
    },
  );
  if (confirmed == true) {
    await controller.restoreCommit(sha);
    if (context.mounted) {
      Navigator.pop(context);
    }
  }
}

Future<void> _showDiff(
  BuildContext context,
  WorkspaceController controller,
  String sha,
) async {
  final from = sha;
  final to =
      controller.commits.isNotEmpty && controller.commits.first.sha != sha
      ? controller.commits.first.sha
      : '';
  final diff = await controller.diffCommits(from: from, to: to);
  if (!context.mounted) {
    return;
  }
  await showDialog<void>(
    context: context,
    builder: (context) {
      return AlertDialog(
        title: const Text('Diff'),
        content: SizedBox(
          width: 520,
          height: 320,
          child: SingleChildScrollView(
            child: SelectableText(
              key: const Key('diff-body'),
              diff.isEmpty ? '(no changes)' : diff,
              style: const TextStyle(fontFamily: 'monospace', fontSize: 12),
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Close'),
          ),
        ],
      );
    },
  );
}
