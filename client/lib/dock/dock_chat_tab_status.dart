import 'dock_ids.dart';
import 'dock_layout_controller.dart';

enum DockChatTabLead { plain, loading, unread }

DockChatTabLead resolveDockChatTabLead({
  required bool chatFocused,
  required bool sending,
  required bool unread,
}) {
  if (chatFocused) return DockChatTabLead.plain;
  if (sending) return DockChatTabLead.loading;
  if (unread) return DockChatTabLead.unread;
  return DockChatTabLead.plain;
}

class DockChatTabUnread {
  bool value = false;

  void clear() => value = false;

  void markIfUnfocused({required bool chatFocused}) {
    if (!chatFocused) value = true;
  }
}

/// Applies chat leading from focus and the sending falling edge.
///
/// The caller owns [wasSending] and updates it after this returns.
void syncChatDockTabStatus({
  required DockLayoutController dock,
  required DockChatTabUnread unread,
  required bool sending,
  required bool wasSending,
}) {
  final chatFocused = dock.focusedItemId == DockIds.chat;
  if (chatFocused) unread.clear();
  if (wasSending && !sending) {
    unread.markIfUnfocused(chatFocused: chatFocused);
  }
  dock.setChatTabLead(
    resolveDockChatTabLead(
      chatFocused: chatFocused,
      sending: sending,
      unread: unread.value,
    ),
  );
}
