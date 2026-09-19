import 'dart:convert';

String formatToolValue(Object? value) {
  if (value == null) return '';
  Object? decoded = value;
  if (value is String) {
    try {
      decoded = jsonDecode(value);
    } on FormatException {
      return value;
    }
  }
  if (decoded is Map || decoded is List) {
    return const JsonEncoder.withIndent('  ').convert(decoded);
  }
  return decoded.toString();
}

String formatToolCopyText({
  required String title,
  Object? input,
  Object? output,
}) {
  String section(Object? value) {
    final formatted = formatToolValue(value);
    return formatted.isEmpty ? '—' : formatted;
  }

  return 'tool: $title\n'
      'args: ${section(input)}\n'
      'output:\n'
      '${section(output)}';
}
