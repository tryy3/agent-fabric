/// Parses markdown link targets and collects soft security warnings.
///
/// Warnings never block Open/Copy — they only surface in the confirm dialog.
class InspectedLink {
  const InspectedLink({
    required this.href,
    required this.displayUrl,
    this.uri,
    this.warnings = const [],
  });

  /// Raw href from markdown (may be empty).
  final String href;

  /// Full URL string shown to the user (prefer absolute [uri]).
  final String displayUrl;

  /// Parsed URI when [href] could be interpreted as one.
  final Uri? uri;

  /// Human-readable warnings; empty means nothing unusual detected.
  final List<String> warnings;

  bool get canLaunch => uri != null;
}

/// Inspect [href] (and optional visible [linkText]) for the confirm dialog.
InspectedLink inspectMarkdownLink({
  required String? href,
  String linkText = '',
}) {
  final raw = (href ?? '').trim();
  if (raw.isEmpty) {
    return const InspectedLink(
      href: '',
      displayUrl: '',
      warnings: ['This link has no destination URL.'],
    );
  }

  final uri = _parseHref(raw);
  final displayUrl = uri?.toString() ?? raw;
  final warnings = <String>[];

  if (uri == null) {
    warnings.add(
      'Could not parse this as a normal URL. Look carefully before opening.',
    );
    return InspectedLink(href: raw, displayUrl: displayUrl, warnings: warnings);
  }

  final scheme = uri.scheme.toLowerCase();
  if (scheme != 'http' && scheme != 'https') {
    warnings.add(
      'Uses the “$scheme” scheme instead of a normal http/https web address.',
    );
  }

  if (uri.userInfo.isNotEmpty) {
    warnings.add('Contains embedded credentials in the URL.');
  }

  final host = uri.host;
  if (host.isNotEmpty) {
    if (_looksLikeIpHost(host)) {
      warnings.add('Points at a raw IP address, not a normal domain name.');
    }
    if (host.toLowerCase().contains('xn--')) {
      warnings.add(
        'Uses an internationalized (punycode) domain — verify the real host.',
      );
    }

    final textHost = _hostishFromLinkText(linkText);
    if (textHost != null &&
        textHost.isNotEmpty &&
        textHost != host.toLowerCase()) {
      warnings.add(
        'Link text looks like “$textHost” but opens “${host.toLowerCase()}”.',
      );
    }
  } else if (scheme == 'http' || scheme == 'https') {
    warnings.add('Web URL is missing a host.');
  }

  return InspectedLink(
    href: raw,
    displayUrl: displayUrl,
    uri: uri,
    warnings: warnings,
  );
}

Uri? _parseHref(String raw) {
  final direct = Uri.tryParse(raw);
  if (direct != null && direct.hasScheme && direct.host.isNotEmpty) {
    return direct;
  }
  // Common markdown omission: www.example.com
  if (!raw.contains('://') && raw.startsWith('www.')) {
    final withScheme = Uri.tryParse('https://$raw');
    if (withScheme != null && withScheme.host.isNotEmpty) {
      return withScheme;
    }
  }
  if (direct != null && direct.hasScheme) {
    // e.g. mailto:user@x — valid URI, no http host
    return direct;
  }
  return direct;
}

bool _looksLikeIpHost(String host) {
  final v4 = RegExp(r'^\d{1,3}(\.\d{1,3}){3}$');
  if (v4.hasMatch(host)) {
    return true;
  }
  // Rough IPv6 (bracketed hosts already stripped by Uri.host)
  return host.contains(':');
}

/// If [text] looks like a URL or bare host, return its host (lowercase).
String? _hostishFromLinkText(String text) {
  final t = text.trim();
  if (t.isEmpty || !t.contains('.')) {
    return null;
  }
  final asUri = Uri.tryParse(t.contains('://') ? t : 'https://$t');
  if (asUri == null || asUri.host.isEmpty) {
    return null;
  }
  // Avoid treating normal sentences with periods as hosts.
  if (t.contains(' ') && !t.contains('://')) {
    return null;
  }
  return asUri.host.toLowerCase();
}
