import 'package:agent_fabric_client/catalog/models.dart';
import 'package:agent_fabric_client/chat/inspector_http_view.dart';
import 'package:agent_fabric_client/ui/theme/app_theme.dart';
import 'package:agent_fabric_client/workspace/editors/read_only_code_view.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

HopCapture _capture() {
  return HopCapture(
    id: 'cap_1',
    threadId: 'th_1',
    roundIndex: 1,
    hopKind: 'llm',
    direction: 'exchange',
    method: 'POST',
    url: 'https://example.test/v1/chat/completions',
    statusCode: 200,
    headers: const {
      'request': {
        'Authorization': '[REDACTED]',
        'Content-Type': 'application/json',
      },
      'response': {'X-Zen-Model': 'deepseek-v4.1-flash'},
    },
    bodyText: '{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hi"}]}',
    meta: const {
      'response_body': '{"content":"hello","thought":"brief"}',
      'model': 'deepseek-v4.1-flash',
      'scrubber': 'prompt-scrub+headers',
      'deltas': 3,
    },
    createdAt: DateTime.utc(2026, 1, 1),
  );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('inspectorContextText pretty-prints request JSON', () {
    final text = inspectorContextText(_capture());
    expect(text, contains('"model": "deepseek-v4.1-flash"'));
    expect(text, contains('\n'));
  });

  testWidgets('InspectorHttpView shows request/response sections', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(900, 1200);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      MaterialApp(
        theme: AppTheme.dark(),
        home: Scaffold(body: InspectorHttpView(capture: _capture())),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Request'), findsOneWidget);
    expect(find.text('Response'), findsOneWidget);
    expect(
      find.text('POST https://example.test/v1/chat/completions'),
      findsOneWidget,
    );
    expect(find.text('HTTP 200'), findsOneWidget);
    expect(find.textContaining('Authorization: [REDACTED]'), findsOneWidget);
    expect(
      find.textContaining('Content-Type: application/json'),
      findsOneWidget,
    );
    expect(
      find.textContaining('X-Zen-Model: deepseek-v4.1-flash'),
      findsOneWidget,
    );
    expect(find.text('Meta'), findsOneWidget);
    expect(find.byType(ReadOnlyCodeView), findsWidgets);
  });
}
