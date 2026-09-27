import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../acp/agent_connection.dart';
import '../catalog/models.dart';

/// Catalog HTTP and ACP WebSocket endpoints resolved at startup.
@immutable
final class RuntimeConfig {
  /// Creates resolved endpoint URIs.
  const RuntimeConfig({required this.catalogBase, required this.acpUri});

  /// Base URL for catalog `/v1` (no trailing path required).
  final Uri catalogBase;

  /// Full WebSocket URL for ACP (`…/acp`).
  final Uri acpUri;
}

/// Optional fields from `/config.json` written by the client-web container.
@immutable
final class RuntimeConfigFile {
  /// Creates a parsed config file payload.
  const RuntimeConfigFile({this.catalogBase, this.acpUri});

  /// Explicit catalog origin, or null/empty to leave unresolved.
  final String? catalogBase;

  /// Explicit ACP WebSocket URI, or null/empty to leave unresolved.
  final String? acpUri;

  /// Parses JSON object keys `catalogBase` and `acpUri`.
  factory RuntimeConfigFile.fromJson(Map<dynamic, dynamic> json) {
    return RuntimeConfigFile(
      catalogBase: _stringField(json['catalogBase']),
      acpUri: _stringField(json['acpUri']),
    );
  }
}

String? _stringField(Object? value) {
  if (value is! String) {
    return null;
  }
  final trimmed = value.trim();
  return trimmed.isEmpty ? null : trimmed;
}

/// ACP WebSocket URI for the same host as [pageOrigin].
Uri sameOriginAcpUri(Uri pageOrigin) {
  final wsScheme = pageOrigin.scheme == 'https' ? 'wss' : 'ws';
  return Uri(
    scheme: wsScheme,
    host: pageOrigin.host,
    port: pageOrigin.hasPort ? pageOrigin.port : null,
    path: '/acp',
  );
}

/// Resolves endpoints from an optional config file.
///
/// - Explicit non-empty fields win.
/// - When the file is present but a field is empty and [isWeb] is true, that
///   field uses same-origin ([pageOrigin] / [sameOriginAcpUri]).
/// - When the file is missing/invalid, falls back to localhost defaults.
@visibleForTesting
RuntimeConfig resolveRuntimeConfig({
  required RuntimeConfigFile? file,
  required bool isWeb,
  required Uri pageOrigin,
  Uri? localhostCatalog,
  Uri? localhostAcp,
}) {
  final fallbackCatalog = localhostCatalog ?? defaultCatalogBase;
  final fallbackAcp = localhostAcp ?? defaultAcpUri;
  final sameCatalog = Uri.parse(pageOrigin.origin);
  final sameAcp = sameOriginAcpUri(sameCatalog);

  return RuntimeConfig(
    catalogBase: _resolveField(
      explicit: file?.catalogBase,
      filePresent: file != null,
      isWeb: isWeb,
      sameOrigin: sameCatalog,
      fallback: fallbackCatalog,
    ),
    acpUri: _resolveField(
      explicit: file?.acpUri,
      filePresent: file != null,
      isWeb: isWeb,
      sameOrigin: sameAcp,
      fallback: fallbackAcp,
    ),
  );
}

Uri _resolveField({
  required String? explicit,
  required bool filePresent,
  required bool isWeb,
  required Uri sameOrigin,
  required Uri fallback,
}) {
  if (explicit != null) {
    return Uri.parse(explicit);
  }
  if (filePresent && isWeb) {
    return sameOrigin;
  }
  return fallback;
}

/// Loads `/config.json` and resolves [RuntimeConfig].
///
/// Missing or invalid JSON yields localhost defaults (local `flutter run`).
/// An empty `{}` on web yields same-origin endpoints.
Future<RuntimeConfig> loadRuntimeConfig({
  http.Client? httpClient,
  Uri? configUri,
  bool? isWeb,
  Uri? pageOrigin,
}) async {
  final web = isWeb ?? kIsWeb;
  final origin = pageOrigin ?? Uri.base;
  final uri = configUri ?? Uri.parse('${origin.origin}/config.json');
  final client = httpClient ?? http.Client();
  final ownsClient = httpClient == null;

  try {
    final response = await client.get(uri);
    if (response.statusCode != 200) {
      return resolveRuntimeConfig(file: null, isWeb: web, pageOrigin: origin);
    }
    final decoded = jsonDecode(response.body);
    if (decoded is! Map) {
      return resolveRuntimeConfig(file: null, isWeb: web, pageOrigin: origin);
    }
    return resolveRuntimeConfig(
      file: RuntimeConfigFile.fromJson(decoded),
      isWeb: web,
      pageOrigin: origin,
    );
  } on Object {
    return resolveRuntimeConfig(file: null, isWeb: web, pageOrigin: origin);
  } finally {
    if (ownsClient) {
      client.close();
    }
  }
}
