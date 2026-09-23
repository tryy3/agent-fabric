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
