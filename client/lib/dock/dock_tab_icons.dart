import 'package:flutter/material.dart';
import 'package:tabbed_view/tabbed_view.dart';

import '../workspace/open_with.dart';
import 'dock_chat_tab_status.dart';
import 'dock_ids.dart';

const _leadingIconSize = 16.0;
const _loadingSize = 16.0;
const _unreadDotSize = 9.0;
const _leadingTitleGap = 6.0;

Path _filledCircle(Size size) {
  return Path()..addOval(
    Rect.fromCircle(
      center: Offset(size.width / 2, size.height / 2),
      radius: size.shortestSide * 0.28,
    ),
  );
}

Color _leadingIconColor(BuildContext context, TabStatus status) {
  final base =
      DefaultTextStyle.of(context).style.color ??
      IconTheme.of(context).color ??
      Theme.of(context).colorScheme.onSurface;
  final opacity = status == TabStatus.selected ? 1.0 : 0.45;
  return base.withValues(alpha: base.a * opacity);
}

/// Space between the leading glyph and the tab title.
Widget _padLeading(Widget child) {
  return Padding(
    padding: const EdgeInsets.only(right: _leadingTitleGap),
    child: child,
  );
}

Widget _leadingIcon(
  BuildContext context,
  TabStatus status,
  IconData icon, {
  required Key key,
}) {
  return _padLeading(
    Icon(
      key: key,
      icon,
      size: _leadingIconSize,
      color: _leadingIconColor(context, status),
    ),
  );
}

IconData _iconForApp(WorkspaceAppId app) {
  switch (app) {
    case WorkspaceAppId.textEditor:
      return Icons.description_outlined;
    case WorkspaceAppId.webPreview:
      return Icons.language;
    case WorkspaceAppId.imagePreview:
      return Icons.image_outlined;
    case WorkspaceAppId.audioPreview:
      return Icons.audiotrack;
    case WorkspaceAppId.download:
      return Icons.download_outlined;
  }
}

TabLeadingBuilder dockTabLeadingForApp(WorkspaceAppId app) {
  final icon = _iconForApp(app);
  return (context, status) {
    return _leadingIcon(
      context,
      status,
      icon,
      key: Key('dock-tab-leading-app-${app.name}'),
    );
  };
}

TabLeadingBuilder dockTabLeadingForId(
  dynamic id, {
  DockChatTabLead chatLead = DockChatTabLead.plain,
}) {
  if (DockIds.isDoc(id)) {
    final docId = id as String;
    final appName = docId.split(':').last;
    try {
      final app = WorkspaceAppId.values.byName(appName);
      return dockTabLeadingForApp(app);
    } catch (_) {
      return (context, status) => _leadingIcon(
        context,
        status,
        Icons.description_outlined,
        key: const Key('dock-tab-leading-doc-fallback'),
      );
    }
  }

  return (context, status) {
    switch (id) {
      case DockIds.threads:
        return _leadingIcon(
          context,
          status,
          Icons.forum_outlined,
          key: const Key('dock-tab-leading-threads'),
        );
      case DockIds.files:
        return _leadingIcon(
          context,
          status,
          Icons.folder_outlined,
          key: const Key('dock-tab-leading-files'),
        );
      case DockIds.chat:
        switch (chatLead) {
          case DockChatTabLead.loading:
            return _padLeading(
              SizedBox(
                key: const Key('dock-tab-leading-chat-loading'),
                width: _loadingSize,
                height: _loadingSize,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  color: _leadingIconColor(context, status),
                ),
              ),
            );
          case DockChatTabLead.unread:
            return _padLeading(
              Container(
                key: const Key('dock-tab-leading-chat-unread'),
                width: _unreadDotSize,
                height: _unreadDotSize,
                decoration: BoxDecoration(
                  color: Theme.of(context).colorScheme.primary,
                  shape: BoxShape.circle,
                ),
              ),
            );
          case DockChatTabLead.plain:
            return _leadingIcon(
              context,
              status,
              Icons.chat_bubble_outline,
              key: const Key('dock-tab-leading-chat'),
            );
        }
      default:
        return null;
    }
  };
}

/// Dirty-document close control when the native tab `×` is hidden.
///
/// `tabbed_view` cannot swap the button icon on hover (only hover colors).
/// The filled circle stays until click; this matches the spec fallback of
/// replacing `×` without a hover glyph change.
TabButton dirtyCloseTabButton({required VoidCallback onClose}) {
  return TabButton(
    icon: IconProvider.path(_filledCircle),
    onPressed: onClose,
    toolTip: 'Close unsaved',
  );
}
