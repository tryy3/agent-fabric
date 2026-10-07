import 'package:agent_fabric_client/core/app_log.dart';
import 'package:flutter/material.dart' as flutter_material show ColorScheme;
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:markdown/markdown.dart' as md;
import 'package:material_ui/material_ui.dart';
import 'package:mermaid_core/mermaid_core.dart' as core;
import 'package:mermaid_flutter/mermaid_flutter.dart';

const String _closedAttribute = 'data-closed';
const String _mermaidClass = 'language-mermaid';

final RegExp _fenceLine = RegExp(r'^ {0,3}(`{3,}|~{3,})\s*$');
final RegExp _openingFence = RegExp(r'^ {0,3}(`{3,}|~{3,})');

/// [md.FencedCodeBlockSyntax] that marks the `code` element when the closing
/// fence was seen. The stock syntax emits the same node for an unterminated
/// fence at end of input, so streaming text is otherwise indistinguishable.
class ClosedAwareFencedCodeBlockSyntax extends md.FencedCodeBlockSyntax {
  const ClosedAwareFencedCodeBlockSyntax();

  @override
  md.Node parse(md.BlockParser parser) {
    final start = parser.lines.indexOf(parser.current);
    final opening = _openingFence.firstMatch(parser.current.content);
    final node = super.parse(parser);
    final next = parser.isDone
        ? parser.lines.length
        : parser.lines.indexOf(parser.current);
    final last = next - 1;
    final closing = last > start && opening != null
        ? _fenceLine.firstMatch(parser.lines[last].content)
        : null;
    final closed =
        closing != null && closing.group(1)!.startsWith(opening!.group(1)!);
    if (closed && node is md.Element) {
      final code = node.children?.whereType<md.Element>().firstOrNull;
      code?.attributes[_closedAttribute] = 'true';
    }
    return node;
  }
}

typedef _CacheKey = (String, core.MermaidTheme);

/// Layout results by source and theme, so token-by-token rebuilds of the
/// surrounding message do not re-parse a diagram that has not changed. A null
/// value records a failed render.
final Map<_CacheKey, core.RenderScene?> _scenes = {};
const int _maxCachedScenes = 32;

core.RenderScene? _sceneFor(String source, core.MermaidTheme theme) {
  final key = (source, theme);
  if (_scenes.containsKey(key)) return _scenes[key];
  core.RenderScene? scene;
  try {
    scene = core.Mermaid(
      measurer: const FlutterTextMeasurer(),
      theme: theme,
    ).render(source);
  } on Object catch (e, s) {
    AppLog.record('mermaid render: $e', s);
  }
  if (_scenes.length >= _maxCachedScenes) {
    _scenes.remove(_scenes.keys.first);
  }
  return _scenes[key] = scene;
}

// material_ui and the SDK's material library declare distinct ColorScheme types.
core.MermaidTheme _mermaidTheme(ColorScheme c) =>
    MaterialMermaidTheme.fromColorScheme(
      flutter_material.ColorScheme(
        brightness: c.brightness,
        primary: c.primary,
        onPrimary: c.onPrimary,
        primaryContainer: c.primaryContainer,
        onPrimaryContainer: c.onPrimaryContainer,
        secondary: c.secondary,
        onSecondary: c.onSecondary,
        secondaryContainer: c.secondaryContainer,
        onSecondaryContainer: c.onSecondaryContainer,
        tertiary: c.tertiary,
        onTertiary: c.onTertiary,
        tertiaryContainer: c.tertiaryContainer,
        onTertiaryContainer: c.onTertiaryContainer,
        error: c.error,
        onError: c.onError,
        errorContainer: c.errorContainer,
        onErrorContainer: c.onErrorContainer,
        surface: c.surface,
        onSurface: c.onSurface,
        onSurfaceVariant: c.onSurfaceVariant,
        outline: c.outline,
        outlineVariant: c.outlineVariant,
        surfaceContainerLowest: c.surfaceContainerLowest,
        surfaceContainerLow: c.surfaceContainerLow,
        surfaceContainer: c.surfaceContainer,
        surfaceContainerHigh: c.surfaceContainerHigh,
        surfaceContainerHighest: c.surfaceContainerHighest,
      ),
    );

/// Renders a closed ```mermaid fence as a diagram. Anything else, including
/// unterminated fences, returns null so the stock code block is used.
class MermaidBuilder extends MarkdownElementBuilder {
  MermaidBuilder({required this.codeStyle});

  final TextStyle? codeStyle;

  @override
  Widget? visitElementAfterWithContext(
    BuildContext context,
    md.Element element,
    TextStyle? preferredStyle,
    TextStyle? parentStyle,
  ) {
    if (element.attributes['class'] != _mermaidClass) return null;
    if (element.attributes[_closedAttribute] != 'true') return null;
    final source = element.textContent.trimRight();
    final scene = _sceneFor(
      source,
      _mermaidTheme(Theme.of(context).colorScheme),
    );
    if (scene == null) {
      return _MermaidFallback(source: source, style: codeStyle);
    }
    return MermaidBlock(source: source, scene: scene);
  }
}

class MermaidBlock extends StatelessWidget {
  const MermaidBlock({super.key, required this.source, required this.scene});

  final String source;
  final core.RenderScene scene;

  @override
  Widget build(BuildContext context) {
    final background = scene.background;
    return Semantics(
      container: true,
      label: 'Mermaid diagram',
      value: source,
      child: ExcludeSemantics(
        child: SizedBox(
          width: scene.size.width,
          height: scene.size.height,
          child: DecoratedBox(
            decoration: BoxDecoration(
              color: background != null ? Color(background.value) : null,
            ),
            child: CustomPaint(painter: ScenePainter(scene)),
          ),
        ),
      ),
    );
  }
}

class _MermaidFallback extends StatelessWidget {
  const _MermaidFallback({required this.source, required this.style});

  final String source;
  final TextStyle? style;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.error_outline,
              size: 14,
              color: theme.colorScheme.onSurfaceVariant,
            ),
            const SizedBox(width: 4),
            Text(
              'Diagram could not be rendered',
              style: theme.textTheme.labelSmall,
            ),
          ],
        ),
        const SizedBox(height: 4),
        Text(source, style: style),
      ],
    );
  }
}
