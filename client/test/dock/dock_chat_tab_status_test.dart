import 'package:agent_fabric_client/dock/dock_chat_tab_status.dart';
import 'package:agent_fabric_client/dock/dock_ids.dart';
import 'package:agent_fabric_client/dock/dock_layout_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:tabbed_view/tabbed_view.dart';

void main() {
  group('resolveDockChatTabLead', () {
    test('focused always plain even when sending or unread', () {
      expect(
        resolveDockChatTabLead(chatFocused: true, sending: true, unread: true),
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

  group('syncChatDockTabStatus', () {
    late DockLayoutController dock;

    setUp(() {
      SharedPreferences.setMockInitialValues({});
      dock = DockLayoutController()
        ..resetToDefault(
          widgets: const DockItemWidgets(
            threads: SizedBox(),
            files: SizedBox(),
            chat: SizedBox(),
          ),
        );
    });

    tearDown(() => dock.dispose());

    testWidgets('focused clears unread and stays plain while sending', (
      tester,
    ) async {
      dock.focusedItemId = DockIds.chat;
      final unread = DockChatTabUnread()..markIfUnfocused(chatFocused: false);

      syncChatDockTabStatus(
        dock: dock,
        unread: unread,
        sending: true,
        wasSending: true,
      );

      expect(unread.value, isFalse);
      await _expectChatLead(tester, dock, 'dock-tab-leading-chat');
    });

    testWidgets('unfocused sending shows loading and does not mark unread', (
      tester,
    ) async {
      dock.focusedItemId = DockIds.files;
      final unread = DockChatTabUnread();

      syncChatDockTabStatus(
        dock: dock,
        unread: unread,
        sending: true,
        wasSending: false,
      );

      expect(unread.value, isFalse);
      await _expectChatLead(tester, dock, 'dock-tab-leading-chat-loading');
    });

    testWidgets('sending falling while unfocused marks unread', (tester) async {
      dock.focusedItemId = DockIds.files;
      final unread = DockChatTabUnread();

      syncChatDockTabStatus(
        dock: dock,
        unread: unread,
        sending: false,
        wasSending: true,
      );

      expect(unread.value, isTrue);
      await _expectChatLead(tester, dock, 'dock-tab-leading-chat-unread');
    });

    testWidgets('sending falling while focused stays plain', (tester) async {
      dock.focusedItemId = DockIds.chat;
      final unread = DockChatTabUnread();

      syncChatDockTabStatus(
        dock: dock,
        unread: unread,
        sending: false,
        wasSending: true,
      );

      expect(unread.value, isFalse);
      await _expectChatLead(tester, dock, 'dock-tab-leading-chat');
    });

    testWidgets('unfocused idle keeps an existing unread mark', (tester) async {
      dock.focusedItemId = DockIds.threads;
      final unread = DockChatTabUnread()..markIfUnfocused(chatFocused: false);

      syncChatDockTabStatus(
        dock: dock,
        unread: unread,
        sending: false,
        wasSending: false,
      );

      expect(unread.value, isTrue);
      await _expectChatLead(tester, dock, 'dock-tab-leading-chat-unread');
    });
  });
}

Future<void> _expectChatLead(
  WidgetTester tester,
  DockLayoutController dock,
  String key,
) async {
  final leading = dock.layout.findDockingItem(DockIds.chat)!.leading!;
  await tester.pumpWidget(
    MaterialApp(
      home: Builder(
        builder: (context) =>
            leading(context, TabStatus.normal) ?? const SizedBox.shrink(),
      ),
    ),
  );
  expect(find.byKey(Key(key)), findsOneWidget);
  // setChatTabLead rebuilds the layout, which schedules a persist timer.
  await tester.pump(const Duration(milliseconds: 300));
}
