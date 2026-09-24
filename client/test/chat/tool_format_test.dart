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
}
