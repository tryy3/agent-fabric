import 'package:agent_fabric_client/acp/permission_modes.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/chat_controller.dart';
import 'package:agent_fabric_client/chat/permission_mode_picker.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'model_picker_test.dart' show FakeCatalog, FakeConn;

class ModeConn extends FakeConn implements PermissionModeApi {
  final List<(String, bool)> sets = [];

  @override
  List<PermissionModeOption> permissionModes = const [];

  @override
  String? currentPermissionMode;

  @override
  Future<void> startSession(String agentId, {String? threadId}) async {
    await super.startSession(agentId, threadId: threadId);
    permissionModes = const [
      PermissionModeOption(
        id: 'ask',
        name: 'Ask for approval',
        description: 'Asks from risk 3, cancels from 9.',
      ),
      PermissionModeOption(id: 'auto', name: 'Run automatically'),
      PermissionModeOption(id: 'full', name: 'Full access'),
    ];
    currentPermissionMode = 'ask';
  }

  @override
  Future<void> setPermissionMode(
    String modeId, {
    bool confirmFullAccess = false,
  }) async {
    sets.add((modeId, confirmFullAccess));
    currentPermissionMode = modeId;
  }
}

Assistant _assistant() {
  final now = DateTime.utc(2026, 9, 12, 9);
  return Assistant(
    id: 'ag-1',
    name: 'Alpha',
    version: 1,
    inferenceConnectionId: 'prov-1',
    defaultModel: 'm1',
    createdAt: now,
    updatedAt: now,
  );
}

Future<(ChatController, ModeConn)> _pump(WidgetTester tester) async {
  final conn = ModeConn();
  final c = ChatController(session: conn, catalog: FakeCatalog([_assistant()]));
  addTearDown(c.dispose);
  await c.connect();
  await c.createThread();
  await c.selectAssistant('ag-1');
  await tester.pumpWidget(
    MaterialApp(
      theme: AppTheme.dark(),
      home: Scaffold(
        body: Center(
          child: AnimatedBuilder(
            animation: c,
            builder: (context, _) => PermissionModePicker(controller: c),
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
  return (c, conn);
}

void main() {
  testWidgets('switches to a mode without confirmation', (tester) async {
    final (_, conn) = await _pump(tester);
    expect(find.text('Ask for approval'), findsOneWidget);

    await tester.tap(find.byKey(const Key('permission-mode-picker')));
    await tester.pumpAndSettle();
    expect(find.text('Asks from risk 3, cancels from 9.'), findsOneWidget);
    await tester.tap(find.byKey(const Key('permission-mode-auto')));
    await tester.pumpAndSettle();

    expect(conn.sets, [('auto', false)]);
    expect(find.byKey(const Key('full-access-confirm')), findsNothing);
    expect(find.text('Run automatically'), findsOneWidget);
  });

  testWidgets('Full access asks for confirmation every time', (tester) async {
    final (_, conn) = await _pump(tester);

    await tester.tap(find.byKey(const Key('permission-mode-picker')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('permission-mode-full')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('full-access-confirm')), findsOneWidget);
    await tester.tap(find.byKey(const Key('full-access-cancel')));
    await tester.pumpAndSettle();
    expect(conn.sets, isEmpty);
    expect(find.text('Ask for approval'), findsOneWidget);

    await tester.tap(find.byKey(const Key('permission-mode-picker')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('permission-mode-full')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('full-access-confirm-button')));
    await tester.pumpAndSettle();
    expect(conn.sets, [('full', true)]);
    expect(find.text('Full access'), findsOneWidget);
  });
}
