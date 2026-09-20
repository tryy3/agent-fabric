import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import 'editors/re_editor_text_view.dart';
import 'file_explorer.dart';
import 'open_with.dart';
import 'web_preview_host.dart';
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
    final open = view;
    if (open == null) {
      return const SizedBox.expand();
    }
    switch (open.appId) {
      case WorkspaceAppId.textEditor:
        final doc = controller.documentFor(open.path);
        if (doc == null) {
          return const Center(child: CircularProgressIndicator());
        }
        if (!doc.isUtf8) {
          return const Center(child: Text('not valid text'));
        }
        return ReEditorTextView(session: controller.sessionFor(doc));
      case WorkspaceAppId.webPreview:
        final uri = controller.previewUriFor(open.path);
        final prefix = uri.replace(query: '', fragment: '').toString();
        final cut = prefix.lastIndexOf('/preview/');
        final originPrefix = cut >= 0
            ? prefix.substring(0, cut + '/preview/'.length)
            : prefix;
        return WebPreviewHost(uri: uri, prefix: originPrefix);
      case WorkspaceAppId.imagePreview:
        final doc = controller.documentFor(open.path);
        if (doc == null) {
          return const Center(child: CircularProgressIndicator());
        }
        return Center(child: Image.memory(doc.bytes));
      case WorkspaceAppId.audioPreview:
        return Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text('Audio preview is not available yet'),
              TextButton(
                onPressed: () => launchUrl(controller.previewUriFor(open.path)),
                child: const Text('Download'),
              ),
            ],
          ),
        );
      case WorkspaceAppId.download:
        return Center(
          child: FilledButton(
            onPressed: () => launchUrl(controller.previewUriFor(open.path)),
            child: const Text('Download'),
          ),
        );
    }
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
