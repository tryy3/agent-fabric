import 'package:flutter_svg/flutter_svg.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import 'theme/design_tokens.dart';

/// Compact token count: 200000 -> "200K", 1048576 -> "1M".
String formatTokenCount(int n) {
  if (n >= 1000000) {
    final m = n / 1000000;
    return '${m == m.roundToDouble() ? m.toInt() : m.toStringAsFixed(1)}M';
  }
  if (n >= 1000) {
    final k = n / 1000;
    return '${k == k.roundToDouble() ? k.toInt() : k.toStringAsFixed(1)}K';
  }
  return '$n';
}

/// USD price per million tokens, e.g. "$3", "$0.30", "free".
String formatPrice(double perMillion) {
  if (perMillion == 0) {
    return 'free';
  }
  if (perMillion >= 1) {
    final whole = perMillion == perMillion.roundToDouble();
    return '\$${whole ? perMillion.toInt() : perMillion.toStringAsFixed(2)}';
  }
  return '\$${perMillion.toStringAsFixed(2)}';
}

/// Provider logo served through the plane, with an initials fallback while
/// loading or when the source has no logo.
class ProviderLogo extends StatelessWidget {
  const ProviderLogo({
    super.key,
    required this.catalog,
    required this.name,
    this.logoUrl,
    this.size = 28,
  });

  final CatalogClient catalog;
  final String name;
  final String? logoUrl;
  final double size;

  @override
  Widget build(BuildContext context) {
    final url = logoUrl;
    if (url == null || url.isEmpty) {
      return _fallback(context);
    }
    return FutureBuilder<ProviderLogoImage?>(
      future: catalog.providerLogo(url),
      builder: (context, snapshot) {
        final logo = snapshot.data;
        if (logo == null) {
          return _fallback(context);
        }
        final tokens = designTokensOf(context);
        return SizedBox(
          key: const Key('provider-logo-image'),
          width: size,
          height: size,
          child: logo.isSvg
              ? SvgPicture.memory(
                  logo.bytes,
                  width: size,
                  height: size,
                  // Logos are often single-color on a transparent background;
                  // tint so they stay visible in both themes.
                  colorFilter: ColorFilter.mode(
                    tokens.textSecondary,
                    BlendMode.srcIn,
                  ),
                  errorBuilder: (context, error, stack) => _fallback(context),
                )
              : Image.memory(
                  logo.bytes,
                  width: size,
                  height: size,
                  errorBuilder: (context, error, stack) => _fallback(context),
                ),
        );
      },
    );
  }

  Widget _fallback(BuildContext context) {
    final tokens = designTokensOf(context);
    final initial = name.trim().isEmpty ? '?' : name.trim()[0].toUpperCase();
    return Container(
      key: const Key('provider-logo-fallback'),
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: tokens.surfaceRaised,
        border: Border.all(color: tokens.border),
        borderRadius: BorderRadius.circular(DesignTokens.radiusSm),
      ),
      child: Text(
        initial,
        style: TextStyle(
          fontSize: size * 0.45,
          fontWeight: FontWeight.w600,
          color: tokens.textSecondary,
        ),
      ),
    );
  }
}

/// Capability chips, limits and prices for one model. Renders nothing when
/// [specs] is null.
class ModelSpecChips extends StatelessWidget {
  const ModelSpecChips({super.key, required this.specs});

  final ModelSpecs? specs;

  @override
  Widget build(BuildContext context) {
    final s = specs;
    if (s == null) {
      return const SizedBox.shrink();
    }
    final tokens = designTokensOf(context);
    final input = s.inputModalities;
    final labels = <(String, Color?)>[
      if (s.toolCall) ('Tools', null),
      if (s.reasoning) ('Reasoning', null),
      if (s.attachment || input.contains('image')) ('Images', null),
      if (input.contains('audio')) ('Audio', null),
      if (input.contains('video')) ('Video', null),
      if (input.contains('pdf')) ('PDF', null),
      if (s.structuredOutput) ('Structured output', null),
      if (s.openWeights) ('Open weights', null),
      if (s.status.isNotEmpty)
        (
          s.status,
          s.status == 'deprecated' ? tokens.warning : tokens.secondary,
        ),
    ];
    final facts = <String>[
      if (s.contextLimit != null)
        '${formatTokenCount(s.contextLimit!)} context',
      if (s.outputLimit != null) '${formatTokenCount(s.outputLimit!)} output',
      if (s.costInput != null) '${formatPrice(s.costInput!)} in',
      if (s.costOutput != null) '${formatPrice(s.costOutput!)} out',
      if (s.costCacheRead != null) '${formatPrice(s.costCacheRead!)} cached',
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (labels.isNotEmpty)
          Wrap(
            spacing: 4,
            runSpacing: 4,
            children: [
              for (final (label, color) in labels) _chip(tokens, label, color),
            ],
          ),
        if (facts.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(
              facts.join(' · '),
              style: TextStyle(fontSize: 11, color: tokens.textMuted),
            ),
          ),
      ],
    );
  }

  Widget _chip(DesignTokens tokens, String label, Color? color) {
    final fg = color ?? tokens.textSecondary;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: tokens.surfaceRaised,
        border: Border.all(color: color ?? tokens.border),
        borderRadius: BorderRadius.circular(DesignTokens.radiusXs),
      ),
      child: Text(label, style: TextStyle(fontSize: 11, color: fg)),
    );
  }
}
