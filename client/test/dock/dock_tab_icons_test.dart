import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tabbed_view/tabbed_view.dart';
import 'package:agent_fabric_client/dock/dock_ids.dart';
import 'package:agent_fabric_client/dock/dock_chat_tab_status.dart';
import 'package:agent_fabric_client/dock/dock_tab_icons.dart';
import 'package:agent_fabric_client/workspace/open_with.dart';

void main() {
  testWidgets('core ids get distinct leading widgets', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (context) {
            final threads = dockTabLeadingForId(DockIds.threads)!(
              context,
              TabStatus.normal,
            );
            final files = dockTabLeadingForId(DockIds.files)!(
              context,
              TabStatus.normal,
            );
            final chat = dockTabLeadingForId(DockIds.chat)!(
              context,
              TabStatus.selected,
            );
            return Row(children: [threads!, files!, chat!]);
          },
        ),
      ),
    );
    expect(find.byKey(const Key('dock-tab-leading-threads')), findsOneWidget);
    expect(find.byKey(const Key('dock-tab-leading-files')), findsOneWidget);
    expect(find.byKey(const Key('dock-tab-leading-chat')), findsOneWidget);
  });

  testWidgets('chat loading and unread leadings', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (context) {
            final loading = dockTabLeadingForId(
              DockIds.chat,
              chatLead: DockChatTabLead.loading,
            )!(context, TabStatus.normal);
            final unread = dockTabLeadingForId(
              DockIds.chat,
              chatLead: DockChatTabLead.unread,
            )!(context, TabStatus.normal);
            return Row(children: [loading!, unread!]);
          },
        ),
      ),
    );
    expect(
      find.byKey(const Key('dock-tab-leading-chat-loading')),
      findsOneWidget,
    );
    expect(
      find.byKey(const Key('dock-tab-leading-chat-unread')),
      findsOneWidget,
    );
  });

  test('dirty close button uses key and invokes onClose', () {
    var closed = false;
    final button = dirtyCloseTabButton(onClose: () => closed = true);
    expect(button.toolTip, isNotNull);
    button.onPressed!();
    expect(closed, isTrue);
  });

  testWidgets('text editor app leading exists', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Builder(
          builder: (context) {
            final w = dockTabLeadingForApp(WorkspaceAppId.textEditor)!(
              context,
              TabStatus.normal,
            );
            return w!;
          },
        ),
      ),
    );
    expect(find.byIcon(Icons.description_outlined), findsOneWidget);
  });
}
