import 'dart:convert';

import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';
import '../workspace/editors/read_only_code_view.dart';

/// DevTools / httpie-style presentation of one hop capture exchange.
class InspectorHttpView extends StatelessWidget {
  const InspectorHttpView({super.key, required this.capture});

  final HopCapture capture;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final requestHeaders = _headersSide(capture.headers, 'request');
    final responseHeaders = _headersSide(capture.headers, 'response');
    final requestBody = prettyInspectorJson(capture.bodyText);
    final responseBody = prettyInspectorJson(capture.meta['response_body']);
    final metaText = prettyInspectorJson(
      _extraMeta(capture.meta),
      expandNestedStrings: true,
    );

    return ListView(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      children: [
        _Section(
          title: 'Request',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              SelectableText(
                _requestLine(capture),
                style: tokens.code().copyWith(
                  color: tokens.textPrimary,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                'Headers',
                style: tokens.labelSm().copyWith(color: tokens.textSecondary),
              ),
              const SizedBox(height: 4),
              SelectableText(
                _formatHeaders(requestHeaders),
                style: tokens.code().copyWith(color: tokens.textPrimary),
              ),
              const SizedBox(height: 12),
              Text(
                'Body',
                style: tokens.labelSm().copyWith(color: tokens.textSecondary),
              ),
              const SizedBox(height: 6),
              _BodyPane(
                text: requestBody,
                languageId: _bodyLanguage(requestBody),
              ),
            ],
          ),
        ),
        const SizedBox(height: 12),
        _Section(
          title: 'Response',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              SelectableText(
                _statusLine(capture),
                style: tokens.code().copyWith(
                  color: tokens.textPrimary,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                'Headers',
                style: tokens.labelSm().copyWith(color: tokens.textSecondary),
              ),
              const SizedBox(height: 4),
              SelectableText(
                _formatHeaders(responseHeaders),
                style: tokens.code().copyWith(color: tokens.textPrimary),
              ),
              const SizedBox(height: 12),
              Text(
                'Body',
                style: tokens.labelSm().copyWith(color: tokens.textSecondary),
              ),
              const SizedBox(height: 6),
              _BodyPane(
                text: responseBody,
                languageId: _bodyLanguage(responseBody),
              ),
            ],
          ),
        ),
        if (metaText.isNotEmpty) ...[
          const SizedBox(height: 12),
          _Section(
            title: 'Meta',
            child: SizedBox(
              height: 160,
              child: ReadOnlyCodeView(
                text: metaText,
                languageId: 'json',
                enableFolding: false,
              ),
            ),
          ),
        ],
      ],
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return DecoratedBox(
      decoration: BoxDecoration(
        color: tokens.surfaceRaised,
        borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
        border: Border.all(color: tokens.border),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              title,
              style: tokens.labelMd().copyWith(color: tokens.textPrimary),
            ),
            const SizedBox(height: 10),
            child,
          ],
        ),
      ),
    );
  }
}

class _BodyPane extends StatelessWidget {
  const _BodyPane({required this.text, required this.languageId});

  final String text;
  final String languageId;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    if (text.isEmpty) {
      return Text(
        '(empty)',
        style: tokens.code().copyWith(color: tokens.textMuted),
      );
    }
    // Fixed viewport; body scrolls inside. Avoid depending on MediaQuery so
    // nested ListView layout stays predictable in tests and Split mode.
    return SizedBox(
      height: 360,
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: tokens.surface,
          borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
          border: Border.all(color: tokens.border),
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
          child: ReadOnlyCodeView(
            text: text,
            languageId: languageId,
            // Folding hides the top of large request JSON (messages) and makes
            // the pane look truncated when scrolled into tools.
            enableFolding: false,
          ),
        ),
      ),
    );
  }
}

String _requestLine(HopCapture c) {
  final method = (c.method ?? 'GET').toUpperCase();
  final url = c.url ?? '';
  return '$method $url';
}

String _statusLine(HopCapture c) {
  final code = c.statusCode;
  if (code == null) {
    return 'Status (none)';
  }
  return 'HTTP $code';
}

Map<String, dynamic> _headersSide(Map<String, dynamic> headers, String side) {
  final raw = headers[side];
  if (raw is Map) {
    return raw.map((k, v) => MapEntry('$k', v));
  }
  return const {};
}

String _formatHeaders(Map<String, dynamic> headers) {
  if (headers.isEmpty) {
    return '(none)';
  }
  final keys = headers.keys.toList()..sort();
  final buf = StringBuffer();
  for (final key in keys) {
    final value = headers[key];
    if (value is List) {
      for (final item in value) {
        buf.writeln('$key: $item');
      }
    } else {
      buf.writeln('$key: $value');
    }
  }
  return buf.toString().trimRight();
}

Map<String, dynamic> _extraMeta(Map<String, dynamic> meta) {
  final out = <String, dynamic>{};
  for (final e in meta.entries) {
    if (e.key == 'response_body') {
      continue;
    }
    out[e.key] = e.value;
  }
  return out;
}

/// Indented JSON for Inspector Raw/Context panes.
///
/// Accepts a JSON text blob or an already-decoded [Map]/[List]. Compact and
/// double-encoded JSON strings are normalized.
///
/// When [expandNestedStrings] is true (Meta), string values that themselves
/// contain a JSON object/array are decoded so nested blobs become readable.
///
/// Scrubbed hop bodies are often *almost* JSON but no longer parseable
/// (`«Path_N»` placeholders can break string boundaries). Those still get a
/// brace-aware indent pass so the Raw tab is readable.
String prettyInspectorJson(Object? value, {bool expandNestedStrings = false}) {
  if (value == null) {
    return '';
  }
  if (value is String) {
    final trimmed = value.trim();
    if (trimmed.isEmpty) {
      return '';
    }
  }
  final decoded = _decodeJsonValue(
    value,
    expandNestedStrings: expandNestedStrings,
  );
  if (decoded is Map || decoded is List) {
    return const JsonEncoder.withIndent('  ').convert(decoded);
  }
  if (decoded is String) {
    final trimmed = decoded.trim();
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
      return _heuristicPrettyJson(trimmed);
    }
    return decoded;
  }
  return '$decoded';
}

/// Indent `{` / `[` / `,` / `:` structure without requiring valid JSON.
///
/// Used when prompt-scrub (or similar) leaves a JSON-shaped blob that
/// [jsonDecode] rejects. String contents are copied verbatim.
String _heuristicPrettyJson(String raw) {
  final buf = StringBuffer();
  var indent = 0;
  var inString = false;
  var escaped = false;
  String? lastSignificant;

  void newline() {
    buf.write('\n');
    buf.write('  ' * indent);
  }

  for (var i = 0; i < raw.length; i++) {
    final ch = raw[i];
    if (inString) {
      buf.write(ch);
      lastSignificant = ch;
      if (escaped) {
        escaped = false;
      } else if (ch == '\\') {
        escaped = true;
      } else if (ch == '"') {
        inString = false;
      }
      continue;
    }

    switch (ch) {
      case '"':
        inString = true;
        buf.write(ch);
        lastSignificant = ch;
      case '{':
      case '[':
        buf.write(ch);
        lastSignificant = ch;
        // Keep empty containers on one line: {} / []
        final closer = ch == '{' ? '}' : ']';
        if (_nextNonWs(raw, i + 1) == closer) {
          break;
        }
        indent++;
        newline();
      case '}':
      case ']':
        final opener = ch == '}' ? '{' : '[';
        if (lastSignificant == opener) {
          buf.write(ch);
          lastSignificant = ch;
          break;
        }
        indent = indent > 0 ? indent - 1 : 0;
        newline();
        buf.write(ch);
        lastSignificant = ch;
      case ',':
        buf.write(ch);
        lastSignificant = ch;
        newline();
      case ':':
        buf.write(': ');
        // Avoid doubling spaces when the source already had `: `.
        if (i + 1 < raw.length && raw[i + 1] == ' ') {
          i++;
        }
        lastSignificant = ch;
      case ' ':
      case '\t':
      case '\n':
      case '\r':
        // Preserve spaces so a scrub-broken string boundary does not glue
        // neighboring tokens; skip EOL so our inserted indents stay clean.
        if (ch == ' ' || ch == '\t') {
          buf.write(ch);
        }
      default:
        buf.write(ch);
        lastSignificant = ch;
    }
  }
  return buf.toString();
}

String? _nextNonWs(String raw, int from) {
  for (var i = from; i < raw.length; i++) {
    final ch = raw[i];
    if (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') {
      continue;
    }
    return ch;
  }
  return null;
}

Object? _decodeJsonValue(
  Object? value, {
  required bool expandNestedStrings,
  int depth = 0,
}) {
  if (depth > 4) {
    return value;
  }
  if (value is Map) {
    if (!expandNestedStrings) {
      return value;
    }
    return <String, dynamic>{
      for (final e in value.entries)
        '${e.key}': _decodeJsonValue(
          e.value,
          expandNestedStrings: true,
          depth: depth + 1,
        ),
    };
  }
  if (value is List) {
    if (!expandNestedStrings) {
      return value;
    }
    return [
      for (final item in value)
        _decodeJsonValue(item, expandNestedStrings: true, depth: depth + 1),
    ];
  }
  if (value is! String) {
    return value;
  }
  final trimmed = value.trim();
  if (trimmed.isEmpty) {
    return '';
  }
  final looksLikeObjectOrArray =
      trimmed.startsWith('{') || trimmed.startsWith('[');
  // Top-level only: peel a JSON string wrapper (`"{\"a\":1}"` → object).
  final looksLikeJsonString = depth == 0 && trimmed.startsWith('"');
  if (!looksLikeObjectOrArray && !looksLikeJsonString) {
    return value;
  }
  try {
    final decoded = jsonDecode(trimmed);
    if (expandNestedStrings || decoded is String) {
      return _decodeJsonValue(
        decoded,
        expandNestedStrings: expandNestedStrings,
        depth: depth + 1,
      );
    }
    return decoded;
  } on FormatException {
    return value;
  }
}

String _bodyLanguage(String text) {
  final t = text.trimLeft();
  if (t.startsWith('{') || t.startsWith('[')) {
    return 'json';
  }
  return 'plaintext';
}

/// Pretty-printed request body for the Context tab.
String inspectorContextText(HopCapture c) {
  final pretty = prettyInspectorJson(c.bodyText);
  return pretty.isEmpty ? c.bodyText : pretty;
}
