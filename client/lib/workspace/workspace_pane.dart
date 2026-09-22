import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../dock/dock_view_body.dart';
import '../dock/files_dock_panel.dart';
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
              return LayoutBuilder(
                builder: (context, constraints) {
                  final files = FilesDockPanel(controller: controller);
                  final views = _OpenViews(controller: controller);
                  if (constraints.maxWidth < 720) {
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
                  }
                  return Row(
                    children: [
                      SizedBox(width: 240, child: files),
                      const VerticalDivider(width: 1, thickness: 1),
                      Expanded(child: views),
                    ],
                  );
                },
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
    if (views.isEmpty) {
      return const Center(child: Text('Open a file from the tree'));
    }
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
                    onDeleted: () => controller.closeView(tab.viewId),
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
