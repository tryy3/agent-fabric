import 'package:acpd/acpd.dart' hide AgentConnection;
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/acp/gate_info.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/agent_bubble.dart';
import 'package:agent_fabric_client/chat/chat_bubble.dart';
import 'package:agent_fabric_client/chat/display_settings.dart';
import 'package:agent_fabric_client/chat/view_modes.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

const _gateMeta = {
  'mode': 'auto_approve',
  'outcome': 'approved',
  'verdict': 'ask',
  'ruleId': 'rules.command_destructive_ask',
  'risk': 6,
  'band': 'elevated',
  'reason': 'command can destroy or change data',
  'scores': [
    {'source': 'rules', 'risk': 6, 'ruleId': 'rules.command_destructive_ask'},
    {'source': 'llm:m', 'risk': 4, 'rationale': 'user asked for it'},
  ],
};

void main() {
  test('GateInfo.tryParse reads the control plane meta shape', () {
    final gate = GateInfo.tryParse(_gateMeta)!;
    expect(gate.risk, 6);
    expect(gate.band, 'elevated');
    expect(gate.badgeLabel, 'risk 6 · elevated');
    expect(gate.outcomeLabel, 'approved');
    expect(gate.modeLabel, 'Approve for me');
    expect(gate.scores.map((s) => s.source), ['rules', 'llm:m']);
    expect(GateInfo.tryParse(null), isNull);
    expect(GateInfo.tryParse('nope'), isNull);
  });

  test('tool call update carries _meta.gate onto the turn event', () {
    final event = agentToolCallEventFromUpdate(
      ToolCallStatusUpdate(
        update: const ToolCallUpdate(
          toolCallId: 'call_1',
          status: ToolCallStatus.completed,
          meta: {'gate': _gateMeta},
        ),
      ),
    );
    expect(event!.gate!.risk, 6);
    final none = agentToolCallEventFromUpdate(
      ToolCallStatusUpdate(
        update: const ToolCallUpdate(
          toolCallId: 'call_2',
          status: ToolCallStatus.completed,
        ),
      ),
    );
    expect(none!.gate, isNull);
  });

  test('persisted tool_call part restores the gate', () {
    final call = ThreadToolCall.fromJson({
      'toolCallId': 'call_1',
      'name': 'run_command',
      'status': 'completed',
      'gate': _gateMeta,
    });
    expect(call.gate!.outcome, 'approved');
  });

  testWidgets('tool call shows a risk badge and gate details when expanded', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.light(),
        home: Scaffold(
          body: AgentBubble(
            viewMode: const ViewMode(
              id: 'v',
              label: 'v',
              description: '',
              markdownRender: false,
              thinkingVisibility: VisibilityMode.collapsed,
              toolVisibility: VisibilityMode.collapsed,
              toolIO: ToolIOMode.both,
            ),
            bubble: ChatBubble(
              kind: ChatBubbleKind.toolCall,
              toolCallId: 'call_1',
              toolTitle: 'Run command',
              toolStatus: 'completed',
              toolInput: const {
                'command': ['rm', 'old.txt'],
              },
              toolOutput: 'ok',
              toolGate: GateInfo.tryParse(_gateMeta),
            ),
          ),
        ),
      ),
    );
    expect(find.text('risk 6 · elevated'), findsOneWidget);
    expect(find.byKey(const Key('gate-details')), findsNothing);
    await tester.tap(find.byKey(const Key('activity-tool-call_1')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('gate-details')), findsOneWidget);
    expect(find.text('Approve for me'), findsOneWidget);
    expect(find.textContaining('llm:m'), findsOneWidget);
  });
}
