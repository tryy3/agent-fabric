import 'dart:convert';

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
      'nested': '{"usage":{"prompt_tokens":1}}',
    },
    createdAt: DateTime.utc(2026, 1, 1),
  );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('prettyInspectorJson', () {
    test('indents compact object blobs', () {
      final text = prettyInspectorJson(
        '{"model":"x","messages":[{"role":"user","content":"hi"}]}',
      );
      expect(text, contains('\n'));
      expect(text, contains('"model": "x"'));
      expect(text, contains('"role": "user"'));
    });

    test('indents already-decoded maps', () {
      expect(
        prettyInspectorJson({
          'content': 'hello',
          'usage': {'prompt_tokens': 1},
        }),
        '{\n'
        '  "content": "hello",\n'
        '  "usage": {\n'
        '    "prompt_tokens": 1\n'
        '  }\n'
        '}',
      );
    });

    test('unwraps JSON-string-encoded objects', () {
      final text = prettyInspectorJson('"{\\"a\\":1}"');
      expect(text, '{\n  "a": 1\n}');
    });

    test('leaves non-JSON plaintext alone', () {
      expect(prettyInspectorJson('not json'), 'not json');
      expect(prettyInspectorJson(''), '');
      expect(prettyInspectorJson(null), '');
    });

    test('expandNestedStrings decodes stringified meta values', () {
      final text = prettyInspectorJson({
        'model': 'x',
        'nested': '{"usage":{"prompt_tokens":1}}',
      }, expandNestedStrings: true);
      expect(text, contains('"prompt_tokens": 1'));
      expect(text, isNot(contains('\\"')));
    });

    test('indents scrubbed invalid JSON blobs', () {
      // prompt-scrub can splice «Path_N» across a string boundary and leave
      // jsonDecode unable to parse — Raw tab should still indent structure.
      const broken =
          '{"model":"x","messages":[{"role":"user","content":"tex«Path_1»"hi"}]}';
      final text = prettyInspectorJson(broken);
      expect(() => jsonDecode(broken), throwsFormatException);
      expect(text, contains('\n'));
      expect(text, contains('"model": "x"'));
      expect(text, contains('"messages": ['));
      expect(text.split('\n').length, greaterThan(3));
    });

    test('keeps empty containers compact', () {
      final text = prettyInspectorJson('{"a":{},"b":[]}');
      expect(text, contains('"a": {}'));
      expect(text, contains('"b": []'));
    });
  });

  test('inspectorContextText pretty-prints request JSON', () {
    final text = inspectorContextText(_capture());
    expect(text, contains('"model": "deepseek-v4.1-flash"'));
    expect(text, contains('\n'));
  });

  testWidgets('InspectorHttpView shows pretty request/response/meta JSON', (
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
    expect(find.byType(ReadOnlyCodeView), findsNWidgets(3));
  });
}
