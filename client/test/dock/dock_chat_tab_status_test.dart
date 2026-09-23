import 'package:flutter_test/flutter_test.dart';
import 'package:agent_fabric_client/dock/dock_chat_tab_status.dart';

void main() {
  group('resolveDockChatTabLead', () {
    test('focused always plain even when sending or unread', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: true,
          sending: true,
          unread: true,
        ),
        DockChatTabLead.plain,
      );
    });

    test('unfocused sending is loading', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: false,
          sending: true,
          unread: false,
        ),
        DockChatTabLead.loading,
      );
    });

    test('unfocused not sending with unread is unread', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: false,
          sending: false,
          unread: true,
        ),
        DockChatTabLead.unread,
      );
    });

    test('unfocused idle is plain', () {
      expect(
        resolveDockChatTabLead(
          chatFocused: false,
          sending: false,
          unread: false,
        ),
        DockChatTabLead.plain,
      );
    });
  });

  group('DockChatTabUnread', () {
    test('markIfUnfocused sets only when unfocused', () {
      final u = DockChatTabUnread();
      u.markIfUnfocused(chatFocused: true);
      expect(u.value, isFalse);
      u.markIfUnfocused(chatFocused: false);
      expect(u.value, isTrue);
    });

    test('clear resets', () {
      final u = DockChatTabUnread()..markIfUnfocused(chatFocused: false);
      u.clear();
      expect(u.value, isFalse);
    });
  });
}
