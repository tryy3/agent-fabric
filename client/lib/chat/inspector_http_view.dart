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
    final requestBody = _prettyBody(capture.bodyText);
    final responseBody = _prettyBody(
      _metaString(capture.meta, 'response_body'),
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
        if (_extraMeta(capture.meta).isNotEmpty) ...[
          const SizedBox(height: 12),
          _Section(
            title: 'Meta',
            child: SizedBox(
              height: 160,
              child: ReadOnlyCodeView(
                text: _prettyJsonObject(_extraMeta(capture.meta)),
                languageId: 'json',
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

String _metaString(Map<String, dynamic> meta, String key) {
  final v = meta[key];
  if (v == null) {
    return '';
  }
  return '$v';
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

String _prettyBody(String raw) {
  final trimmed = raw.trim();
  if (trimmed.isEmpty) {
    return '';
  }
  try {
    final decoded = jsonDecode(trimmed);
    return const JsonEncoder.withIndent('  ').convert(decoded);
  } on Object {
    return raw;
  }
}

String _prettyJsonObject(Map<String, dynamic> value) {
  return const JsonEncoder.withIndent('  ').convert(value);
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
  final pretty = _prettyBody(c.bodyText);
  return pretty.isEmpty ? c.bodyText : pretty;
}
