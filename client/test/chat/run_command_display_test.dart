import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:agent_fabric_client/chat/pending_interaction.dart';
import 'package:agent_fabric_client/chat/permission_prompt.dart';
import 'package:agent_fabric_client/chat/tool_format.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';

PendingPermission _commandPermission({Object? cwd = 'web'}) {
  return PendingPermission(
    threadId: 'th_1',
    completer: Completer<RequestPermissionResponse>(),
    request: RequestPermissionRequest(
      sessionId: 's1',
      toolCall: ToolCallUpdate(
        toolCallId: 'c1',
        title: 'Run command',
        rawInput: {
          'reason': 'run npm test',
          'command': ['npm', 'test'],
          'cwd': cwd,
          'grantKey': 'npm test',
        },
      ),
      options: const [
        PermissionOption(
          optionId: 'allow_once',
          name: 'Allow once',
          kind: PermissionOptionKind.allowOnce,
        ),
        PermissionOption(
          optionId: 'allow_session',
          name: 'Allow for this session',
          kind: PermissionOptionKind.allowAlways,
        ),
        PermissionOption(
          optionId: 'reject_once',
          name: 'Reject',
          kind: PermissionOptionKind.rejectOnce,
        ),
      ],
    ),
  );
}

void main() {
  test('command permission reason shows the command line and cwd', () {
    expect(
      _commandPermission().reason,
      r'$ npm test'
      '\nin web',
    );
    expect(_commandPermission(cwd: '.').reason, r'$ npm test');
    expect(_commandPermission().commandLine, 'npm test');
  });

  test('non-command permission keeps the reason text', () {
    final pending = PendingPermission(
      threadId: 'th_1',
      completer: Completer<RequestPermissionResponse>(),
      request: const RequestPermissionRequest(
        sessionId: 's1',
        toolCall: ToolCallUpdate(
          toolCallId: 'c1',
          rawInput: {'reason': 'delete requires confirmation: a.txt'},
        ),
        options: [],
      ),
    );
    expect(pending.commandLine, isNull);
    expect(pending.reason, 'delete requires confirmation: a.txt');
  });

  test('tool description renders argv arrays as a command line', () {
    expect(
      toolCallDescription('{"command":["go","test","./..."],"cwd":"api"}'),
      'go test ./...',
    );
  });

  testWidgets('permission dock renders the command', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(
          body: PermissionDock(pending: _commandPermission(), onSelect: (_) {}),
        ),
      ),
    );
    expect(find.text('Run command'), findsOneWidget);
    expect(find.textContaining('npm test'), findsOneWidget);
    expect(find.text('Allow for this session'), findsOneWidget);
  });
}
