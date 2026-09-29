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
    return formatted.isEmpty ? '-' : formatted;
  }

  return 'tool: $title\n'
      'args: ${section(input)}\n'
      'output:\n'
      '${section(output)}';
}

/// One-line collapsed summary from tool args (query, URL, path, …).
///
/// When [output] includes a `results` list (web_search), appends the hit count.
String toolCallDescription(Object? input, {Object? output}) {
  final primary = _primaryArgSummary(input);
  final resultCount = _searchResultCount(output);
  if (primary.isEmpty) {
    if (resultCount == null) {
      return '';
    }
    return _resultCountLabel(resultCount);
  }
  if (resultCount == null) {
    return primary;
  }
  return '$primary · ${_resultCountLabel(resultCount)}';
}

String _primaryArgSummary(Object? input) {
  final map = decodeToolArgMap(input);
  if (map == null) {
    if (input is String) {
      return input.trim();
    }
    return '';
  }
  const preferredKeys = <String>[
    'query',
    'url',
    'path',
    'command',
    'cmd',
    'pattern',
    'name',
    'file',
    'target',
  ];
  for (final key in preferredKeys) {
    final value = map[key];
    if (value is String) {
      final trimmed = value.trim();
      if (trimmed.isNotEmpty) {
        return trimmed;
      }
    }
  }
  for (final value in map.values) {
    if (value is String) {
      final trimmed = value.trim();
      if (trimmed.isNotEmpty && trimmed.length <= 240) {
        return trimmed;
      }
    }
  }
  return '';
}

int? _searchResultCount(Object? output) {
  final map = decodeToolArgMap(output);
  if (map == null) {
    return null;
  }
  final results = map['results'];
  if (results is List) {
    return results.length;
  }
  return null;
}

String _resultCountLabel(int count) {
  return count == 1 ? '1 result' : '$count results';
}

/// Decodes tool args into a string-keyed map when possible.
Map<String, Object?>? decodeToolArgMap(Object? input) {
  Object? decoded = input;
  if (input is String) {
    final trimmed = input.trim();
    if (trimmed.isEmpty) {
      return null;
    }
    try {
      decoded = jsonDecode(trimmed);
    } on FormatException {
      return null;
    }
  }
  if (decoded is Map) {
    return <String, Object?>{
      for (final entry in decoded.entries) '${entry.key}': entry.value,
    };
  }
  return null;
}
