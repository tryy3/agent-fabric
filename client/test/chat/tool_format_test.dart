import 'package:agent_fabric_client/chat/tool_format.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('formatToolValue', () {
    test('null is empty string', () {
      expect(formatToolValue(null), '');
    });

    test('map pretty-prints as indented JSON', () {
      expect(
        formatToolValue({'path': 'notes.txt'}),
        '{\n  "path": "notes.txt"\n}',
      );
    });

    test('JSON string decodes then pretty-prints', () {
      expect(
        formatToolValue('{"path":"notes.txt"}'),
        '{\n  "path": "notes.txt"\n}',
      );
    });

    test('plain string passes through', () {
      expect(formatToolValue('hello'), 'hello');
    });
  });

  group('formatToolCopyText', () {
    test('structured dump with args and output', () {
      expect(
        formatToolCopyText(
          title: 'skill_view',
          input: {'name': 'bike-maintenance'},
          output: {'success': true},
        ),
        'tool: skill_view\n'
        'args: {\n  "name": "bike-maintenance"\n}\n'
        'output:\n'
        '{\n  "success": true\n}',
      );
    });

    test('empty fields use em dash sentinel', () {
      expect(
        formatToolCopyText(title: 'Read file', input: null, output: null),
        'tool: Read file\n'
        'args: -\n'
        'output:\n'
        '-',
      );
    });

    test('empty formatted string uses sentinel', () {
      expect(
        formatToolCopyText(title: 'x', input: '', output: ''),
        'tool: x\n'
        'args: -\n'
        'output:\n'
        '-',
      );
    });
  });

  group('toolCallDescription', () {
    test('prefers query for search tools', () {
      expect(
        toolCallDescription({'query': 'agent fabric acp', 'max_results': 5}),
        'agent fabric acp',
      );
    });

    test('prefers url for fetch tools', () {
      expect(
        toolCallDescription({'url': 'https://example.com/docs'}),
        'https://example.com/docs',
      );
    });

    test('prefers path for file tools', () {
      expect(toolCallDescription({'path': 'notes.txt'}), 'notes.txt');
    });

    test('decodes JSON string args', () {
      expect(toolCallDescription('{"query":"hello world"}'), 'hello world');
    });

    test('empty or unknown shapes return empty', () {
      expect(toolCallDescription(null), '');
      expect(toolCallDescription(<String, Object?>{}), '');
      expect(toolCallDescription({'max_results': 5}), '');
    });

    test('appends web search result count from output', () {
      expect(
        toolCallDescription(
          {'query': 'agent fabric'},
          output: {
            'query': 'agent fabric',
            'results': [
              {'title': 'One'},
              {'title': 'Two'},
            ],
          },
        ),
        'agent fabric · 2 results',
      );
      expect(
        toolCallDescription(
          {'query': 'solo'},
          output: {
            'results': [
              {'title': 'Only'},
            ],
          },
        ),
        'solo · 1 result',
      );
      expect(
        toolCallDescription(
          {'query': 'empty'},
          output: {'results': <Object>[]},
        ),
        'empty · 0 results',
      );
    });

    test('ignores output without results list', () {
      expect(
        toolCallDescription(
          {'url': 'https://example.com'},
          output: {'title': 'Example', 'markdown': '# Hi'},
        ),
        'https://example.com',
      );
    });
  });
}
