import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

import '../workspace/editor_preview_pane.dart';
import '../workspace/editors/re_editor_text_view.dart';
import '../workspace/open_with.dart';
import '../workspace/web_preview_host.dart';
import '../workspace/workspace_controller.dart';

class DockViewBody extends StatelessWidget {
  const DockViewBody({super.key, required this.controller, required this.view});

  final WorkspaceController controller;
  final OpenView? view;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) => _buildView(),
    );
  }

  Widget _buildView() {
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
        final editor = ReEditorTextView(session: controller.sessionFor(doc));
        if (!appCanOpen(WorkspaceAppId.webPreview, open.path)) {
          return editor;
        }
        return EditorPreviewPane(
          controller: controller,
          view: open,
          editor: editor,
        );
      case WorkspaceAppId.webPreview:
        return webPreviewHostFor(controller, open.path);
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
