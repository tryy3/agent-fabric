import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'permission_tiers.dart';
import 'models.dart';

export 'models.dart'
    show
        defaultCatalogBase,
        CatalogException,
        FsEntry,
        FsListing,
        GitCommit,
        Checkpoint,
        DiffResult,
        ExportMethod,
        ExportArchive,
        ExportLink,
        ExportPublishResult,
        ExportOutcome,
        ExportArchiveOutcome,
        ExportPublishOutcome,
        PlaneSettings,
        ToolDefinition,
        ToolIntegration;

/// Raw logo bytes plus whether they are SVG (otherwise PNG).
class ProviderLogoImage {
  const ProviderLogoImage({required this.bytes, required this.isSvg});

  final Uint8List bytes;
  final bool isSvg;
}

class CatalogClient {
  CatalogClient({required Uri baseUri, http.Client? httpClient})
    : _baseUri = baseUri,
      _http = httpClient ?? http.Client(),
      _ownsClient = httpClient == null;

  final Uri _baseUri;
  final http.Client _http;
  final bool _ownsClient;

  void close() {
    if (_ownsClient) {
      _http.close();
    }
  }

  Future<List<InferenceConnection>> listInferenceConnections() async {
    final body = await _send('GET', '/v1/inference/connections');
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(InferenceConnection.fromJson)
        .toList();
  }

  Future<InferenceConnection> createInferenceConnection({
    required String name,
    required String type,
    required String baseUrl,
    required String apiKey,
  }) async {
    final body = await _send(
      'POST',
      '/v1/inference/connections',
      json: {'name': name, 'type': type, 'baseUrl': baseUrl, 'apiKey': apiKey},
    );
    return InferenceConnection.fromJson(
      jsonDecode(body) as Map<String, dynamic>,
    );
  }

  Future<InferenceConnection> updateInferenceConnection(
    String id, {
    String? name,
    String? baseUrl,
    String? apiKey,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/inference/connections/$id',
      json: {
        if (name != null) 'name': name,
        if (baseUrl != null) 'baseUrl': baseUrl,
        if (apiKey != null) 'apiKey': apiKey,
      },
    );
    return InferenceConnection.fromJson(
      jsonDecode(body) as Map<String, dynamic>,
    );
  }

  Future<void> deleteInferenceConnection(String id) async {
    await _send('DELETE', '/v1/inference/connections/$id');
  }

  Future<InferenceConnection> refreshModels(String id) async {
    final body = await _send(
      'POST',
      '/v1/inference/connections/$id/models/refresh',
    );
    return InferenceConnection.fromJson(
      jsonDecode(body) as Map<String, dynamic>,
    );
  }

  Future<ModelSpecsStatus> getModelSpecsStatus() async {
    final body = await _send('GET', '/v1/model-specs/status');
    return ModelSpecsStatus.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  /// Syncs now. The plane answers 502 with the error when the source fails.
  Future<ModelSpecsStatus> syncModelSpecs() async {
    final body = await _send('POST', '/v1/model-specs/sync');
    return ModelSpecsStatus.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  /// Patches the specs source settings; omitted fields stay unchanged.
  Future<void> updateModelSpecsSettings({
    String? sourceUrl,
    int? syncIntervalHours,
    bool? enabled,
  }) async {
    await _send(
      'PATCH',
      '/v1/model-specs/settings',
      json: {
        'sourceUrl': ?sourceUrl,
        'syncIntervalHours': ?syncIntervalHours,
        'enabled': ?enabled,
      },
    );
  }

  Future<List<SpecsProviderSummary>> listSpecsProviders() async {
    final body = await _send('GET', '/v1/model-specs/providers');
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(SpecsProviderSummary.fromJson)
        .toList();
  }

  Future<SpecsProviderDetail> getSpecsProvider(String id) async {
    final body = await _send('GET', '/v1/model-specs/providers/$id');
    return SpecsProviderDetail.fromJson(
      jsonDecode(body) as Map<String, dynamic>,
    );
  }

  final Map<String, Future<ProviderLogoImage?>> _logos = {};

  /// Downloads a provider logo through the plane (cached per client). Null
  /// when the plane has none for it.
  Future<ProviderLogoImage?> providerLogo(String logoUrl) {
    return _logos.putIfAbsent(logoUrl, () async {
      try {
        final response = await _request('GET', logoUrl);
        final isSvg = (response.headers['content-type'] ?? '').contains('svg');
        return ProviderLogoImage(bytes: response.bodyBytes, isSvg: isSvg);
      } on CatalogException {
        return null;
      }
    });
  }

  Future<List<Assistant>> listAssistants() async {
    final body = await _send('GET', '/v1/assistants');
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(Assistant.fromJson)
        .toList();
  }

  Future<Assistant> createAssistant({
    required String name,
    String description = '',
    String instructions = '',
    required String inferenceConnectionId,
    required String defaultModel,
  }) async {
    final body = await _send(
      'POST',
      '/v1/assistants',
      json: {
        'name': name,
        'description': description,
        'instructions': instructions,
        'inferenceConnectionId': inferenceConnectionId,
        'defaultModel': defaultModel,
      },
    );
    return Assistant.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<Assistant> updateAssistant(
    String id, {
    String? name,
    String? description,
    String? instructions,
    String? inferenceConnectionId,
    String? defaultModel,
    Map<String, dynamic>? settings,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/assistants/$id',
      json: {
        if (name != null) 'name': name,
        if (description != null) 'description': description,
        if (instructions != null) 'instructions': instructions,
        if (inferenceConnectionId != null)
          'inferenceConnectionId': inferenceConnectionId,
        if (defaultModel != null) 'defaultModel': defaultModel,
        if (settings != null) 'settings': settings,
      },
    );
    return Assistant.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteAssistant(String id) async {
    await _send('DELETE', '/v1/assistants/$id');
  }

  Future<PlaneSettings> getSettings() async {
    final body = await _send('GET', '/v1/settings');
    return PlaneSettings.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  /// Sentinel for optional nullable PATCH fields (distinguish omit vs null).
  static const Object fieldUnset = Object();

  Future<PlaneSettings> patchSettings({
    Map<String, dynamic>? sandbox,
    Map<String, dynamic>? environment,
    Map<String, dynamic>? integrations,
    Object? webSearchIntegrationId = fieldUnset,
    Object? fetchPageIntegrationId = fieldUnset,
    String? platformInstructions,
    String? runtimeContext,
    Map<String, dynamic>? permissions,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/settings',
      json: {
        if (sandbox != null) 'sandbox': sandbox,
        if (environment != null) 'environment': environment,
        if (integrations != null) 'integrations': integrations,
        if (!identical(webSearchIntegrationId, fieldUnset))
          'webSearchIntegrationId': webSearchIntegrationId,
        if (!identical(fetchPageIntegrationId, fieldUnset))
          'fetchPageIntegrationId': fetchPageIntegrationId,
        if (platformInstructions != null)
          'platformInstructions': platformInstructions,
        if (runtimeContext != null) 'runtimeContext': runtimeContext,
        if (permissions != null) 'permissions': permissions,
      },
    );
    return PlaneSettings.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  /// The gate's built-in rule tiers with their defaults.
  Future<List<PermissionTier>> listPermissionBuiltins() async {
    final body = await _send('GET', '/v1/permissions/builtins');
    final decoded = jsonDecode(body) as Map<String, dynamic>;
    final list = decoded['tiers'] as List? ?? const [];
    return [
      for (final item in list)
        if (item is Map)
          PermissionTier.fromJson(Map<String, Object?>.from(item)),
    ];
  }

  Future<List<ToolIntegration>> listToolIntegrations() async {
    final body = await _send('GET', '/v1/tool/integrations');
    final decoded = jsonDecode(body) as Map<String, dynamic>;
    final list = decoded['toolIntegrations'] as List? ?? const [];
    return list
        .cast<Map<String, dynamic>>()
        .map(ToolIntegration.fromJson)
        .toList();
  }

  Future<ToolIntegration> createToolIntegration({
    required String name,
    required String kind,
    required String mode,
    String endpoint = '',
    Map<String, String?>? secrets,
    bool enabled = true,
  }) async {
    final body = await _send(
      'POST',
      '/v1/tool/integrations',
      json: {
        'name': name,
        'kind': kind,
        'mode': mode,
        'endpoint': endpoint,
        'enabled': enabled,
        if (secrets != null) 'secrets': secrets,
      },
    );
    return ToolIntegration.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<ToolIntegration> updateToolIntegration(
    String id, {
    String? name,
    bool? enabled,
    String? endpoint,
    String? mode,
    Map<String, String?>? secrets,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/tool/integrations/$id',
      json: {
        if (name != null) 'name': name,
        if (enabled != null) 'enabled': enabled,
        if (endpoint != null) 'endpoint': endpoint,
        if (mode != null) 'mode': mode,
        if (secrets != null) 'secrets': secrets,
      },
    );
    return ToolIntegration.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteToolIntegration(String id) async {
    await _send('DELETE', '/v1/tool/integrations/$id');
  }

  Future<({bool ok, String message, ToolIntegration integration})>
  testToolIntegration(String id) async {
    final body = await _send('POST', '/v1/tool/integrations/$id/test');
    final decoded = jsonDecode(body) as Map<String, dynamic>;
    return (
      ok: decoded['ok'] == true,
      message: '${decoded['message'] ?? ''}',
      integration: ToolIntegration.fromJson(decoded),
    );
  }

  Future<List<Resource>> listResources() async {
    final body = await _send('GET', '/v1/resources');
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(Resource.fromJson)
        .toList();
  }

  Future<Resource> createResource({
    required String name,
    required String kind,
    required Map<String, dynamic> spec,
  }) async {
    final body = await _send(
      'POST',
      '/v1/resources',
      json: {'name': name, 'kind': kind, 'spec': spec},
    );
    return Resource.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<Resource> updateResource(
    String id, {
    String? name,
    Map<String, dynamic>? spec,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/resources/$id',
      json: {if (name != null) 'name': name, if (spec != null) 'spec': spec},
    );
    return Resource.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteResource(String id) async {
    await _send('DELETE', '/v1/resources/$id');
  }

  Future<List<Project>> listProjects() async {
    final body = await _send('GET', '/v1/projects');
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(Project.fromJson)
        .toList();
  }

  Future<Project> createProject({
    required String name,
    String description = '',
  }) async {
    final body = await _send(
      'POST',
      '/v1/projects',
      json: {
        'name': name,
        if (description.isNotEmpty) 'description': description,
      },
    );
    return Project.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<Project> getProject(String id) async {
    final body = await _send('GET', '/v1/projects/$id');
    return Project.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<Project> updateProject(
    String id, {
    String? name,
    String? description,
    Map<String, dynamic>? settings,
    List<dynamic>? remotes,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/projects/$id',
      json: {
        if (name != null) 'name': name,
        if (description != null) 'description': description,
        if (settings != null) 'settings': settings,
        if (remotes != null) 'remotes': remotes,
      },
    );
    return Project.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteProject(String id) async {
    await _send('DELETE', '/v1/projects/$id');
  }

  Future<Map<String, dynamic>> resolvedEnvironment(String projectId) async {
    final body = await _send(
      'GET',
      '/v1/projects/$projectId/environment/resolved',
    );
    final decoded = jsonDecode(body);
    if (decoded is Map<String, dynamic>) {
      return decoded;
    }
    return const {};
  }

  Future<List<ToolDefinition>> listTools() async {
    final body = await _send('GET', '/v1/tools');
    final decoded = jsonDecode(body);
    if (decoded is! Map<String, dynamic>) {
      return const [];
    }
    final raw = decoded['tools'];
    if (raw is! List) {
      return const [];
    }
    return [
      for (final item in raw)
        if (item is Map<String, dynamic>) ToolDefinition.fromJson(item),
    ];
  }

  Future<List<ThreadSummary>> listThreads({String? projectId}) async {
    final body = await _send(
      'GET',
      '/v1/threads',
      query: {
        if (projectId != null && projectId.isNotEmpty) 'projectId': projectId,
      },
    );
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(ThreadSummary.fromJson)
        .toList();
  }

  Future<ThreadSummary> createThread({String? projectId}) async {
    final body = await _send(
      'POST',
      '/v1/threads',
      json: {
        if (projectId != null && projectId.isNotEmpty) 'projectId': projectId,
      },
    );
    return ThreadSummary.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<ThreadDetail> getThread(String id) async {
    final body = await _send('GET', '/v1/threads/$id');
    return ThreadDetail.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<ThreadSummary> renameThread(String id, String title) async {
    final body = await _send(
      'PATCH',
      '/v1/threads/$id',
      json: {'title': title},
    );
    return ThreadSummary.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<ThreadSummary> patchThreadViewMode(
    String id,
    String? viewModeId,
  ) async {
    final body = await _send(
      'PATCH',
      '/v1/threads/$id',
      json: {'viewModeId': viewModeId},
    );
    return ThreadSummary.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<List<HopCapture>> listThreadCaptures(String threadId) async {
    final body = await _send('GET', '/v1/threads/$threadId/captures');
    final list = jsonDecode(body) as List<dynamic>;
    return [
      for (final raw in list)
        HopCapture.fromJson(Map<String, dynamic>.from(raw as Map)),
    ];
  }

  Future<List<HopCapture>> listMessageCaptures(
    String threadId,
    String messageId,
  ) async {
    final body = await _send(
      'GET',
      '/v1/threads/$threadId/messages/$messageId/captures',
    );
    final list = jsonDecode(body) as List<dynamic>;
    return [
      for (final raw in list)
        HopCapture.fromJson(Map<String, dynamic>.from(raw as Map)),
    ];
  }

  Future<FsListing> listProjectFs(String projectId, {String path = '/'}) async {
    final body = await _send(
      'GET',
      '/v1/projects/$projectId/fs',
      query: {'path': path},
    );
    return FsListing.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<Uint8List> getProjectFile(String projectId, String path) async {
    final response = await _request(
      'GET',
      '/v1/projects/$projectId/files',
      query: {'path': path},
    );
    return response.bodyBytes;
  }

  Future<void> putProjectFile(
    String projectId,
    String path,
    List<int> bytes, {
    String contentType = 'application/octet-stream',
  }) async {
    await _request(
      'PUT',
      '/v1/projects/$projectId/files',
      query: {'path': path},
      bytes: bytes,
      headers: {'content-type': contentType},
    );
  }

  Future<void> deleteProjectFile(String projectId, String path) async {
    await _send(
      'DELETE',
      '/v1/projects/$projectId/files',
      query: {'path': path},
    );
  }

  Future<void> createProjectDir(String projectId, String path) async {
    await _request(
      'PUT',
      '/v1/projects/$projectId/dirs',
      query: {'path': path},
    );
  }

  /// Renames or moves [from] to [to]. Fails with 409 when [to] exists. Returns
  /// the destination path as normalized by the server.
  Future<String> moveProjectPath(
    String projectId, {
    required String from,
    required String to,
  }) async {
    final body = await _send(
      'POST',
      '/v1/projects/$projectId/fs/move',
      json: {'from': from, 'to': to},
    );
    return _pathFromBody(body, fallback: to);
  }

  /// Copies [from] to [to], or beside itself as `name copy.ext` when [to] is
  /// omitted. Returns the new path.
  Future<String> copyProjectPath(
    String projectId, {
    required String from,
    String? to,
  }) async {
    final body = await _send(
      'POST',
      '/v1/projects/$projectId/fs/copy',
      json: {'from': from, if (to != null) 'to': to},
    );
    return _pathFromBody(body, fallback: to ?? from);
  }

  String _pathFromBody(String body, {required String fallback}) {
    final decoded = jsonDecode(body);
    if (decoded is Map<String, dynamic> && decoded['path'] is String) {
      return decoded['path'] as String;
    }
    return fallback;
  }

  Uri previewUri(String projectId, String path) {
    var cleaned = path.trim();
    if (cleaned.startsWith('/')) {
      cleaned = cleaned.substring(1);
    }
    return _baseUri.resolve('/v1/projects/$projectId/preview/$cleaned');
  }

  Future<List<GitCommit>> listProjectCommits(String projectId) async {
    final body = await _send('GET', '/v1/projects/$projectId/commits');
    final decoded = jsonDecode(body);
    if (decoded is! Map<String, dynamic>) {
      return const [];
    }
    final commits = decoded['commits'];
    if (commits is! List) {
      return const [];
    }
    return [
      for (final item in commits)
        if (item is Map<String, dynamic>) GitCommit.fromJson(item),
    ];
  }

  Future<Checkpoint> createCheckpoint(
    String projectId, {
    required String label,
    String? threadId,
  }) async {
    final body = await _send(
      'POST',
      '/v1/projects/$projectId/checkpoints',
      json: {
        'label': label,
        if (threadId != null && threadId.isNotEmpty) 'threadId': threadId,
      },
    );
    return Checkpoint.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> restoreProject(String projectId, {required String sha}) async {
    await _send('POST', '/v1/projects/$projectId/restore', json: {'sha': sha});
  }

  Future<DiffResult> projectDiff(
    String projectId, {
    required String from,
    String to = '',
  }) async {
    final body = await _send(
      'GET',
      '/v1/projects/$projectId/diff',
      query: {'from': from, if (to.isNotEmpty) 'to': to},
    );
    return DiffResult.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<List<ExportMethod>> listExporters(String projectId) async {
    final body = await _send('GET', '/v1/projects/$projectId/exporters');
    final decoded = jsonDecode(body);
    if (decoded is! Map<String, dynamic>) {
      return const [];
    }
    final exporters = decoded['exporters'];
    if (exporters is! List) {
      return const [];
    }
    return [
      for (final item in exporters)
        if (item is Map<String, dynamic>) ExportMethod.fromJson(item),
    ];
  }

  Future<ExportOutcome> exportProject(
    String projectId, {
    String method = 'download',
  }) async {
    final response = await _request(
      'POST',
      '/v1/projects/$projectId/export',
      json: {'method': method},
    );
    final contentType = response.headers['content-type'] ?? '';
    if (contentType.contains('application/json')) {
      final decoded = jsonDecode(response.body);
      if (decoded is Map<String, dynamic>) {
        return ExportPublishOutcome(ExportPublishResult.fromJson(decoded));
      }
      throw CatalogException(
        statusCode: 200,
        message: 'unexpected publish response',
      );
    }
    return ExportArchiveOutcome(
      ExportArchive(
        filename: _filenameFromDisposition(
          response.headers['content-disposition'],
          fallback: '$projectId.zip',
        ),
        bytes: response.bodyBytes,
        mediaType: contentType.isEmpty ? 'application/zip' : contentType,
      ),
    );
  }

  Future<String> _send(
    String method,
    String path, {
    Map<String, dynamic>? json,
    Map<String, String>? query,
  }) async {
    final response = await _request(method, path, json: json, query: query);
    return response.body;
  }

  Future<http.Response> _request(
    String method,
    String path, {
    Map<String, dynamic>? json,
    Map<String, String>? query,
    List<int>? bytes,
    Map<String, String>? headers,
  }) async {
    var url = _baseUri.resolve(path);
    if (query != null && query.isNotEmpty) {
      url = url.replace(
        queryParameters: <String, String>{...url.queryParameters, ...query},
      );
    }
    final requestHeaders = <String, String>{
      if (json != null) 'content-type': 'application/json',
      ...?headers,
    };
    final body = bytes ?? (json == null ? null : jsonEncode(json));
    late http.Response response;
    switch (method) {
      case 'GET':
        response = await _http.get(url, headers: requestHeaders);
      case 'POST':
        response = await _http.post(url, headers: requestHeaders, body: body);
      case 'PUT':
        response = await _http.put(url, headers: requestHeaders, body: body);
      case 'PATCH':
        response = await _http.patch(url, headers: requestHeaders, body: body);
      case 'DELETE':
        response = await _http.delete(url, headers: requestHeaders);
      default:
        throw ArgumentError.value(method, 'method');
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw CatalogException(
        statusCode: response.statusCode,
        message: _errorMessage(response.body),
      );
    }
    return response;
  }

  String _errorMessage(String body) {
    if (body.isEmpty) {
      return 'catalog request failed';
    }
    try {
      final decoded = jsonDecode(body);
      if (decoded is Map<String, dynamic> && decoded['error'] is String) {
        return decoded['error'] as String;
      }
    } on FormatException {
      // Fall through to raw body.
    }
    return body;
  }

  String _filenameFromDisposition(String? header, {required String fallback}) {
    if (header == null || header.isEmpty) {
      return fallback;
    }
    const marker = 'filename=';
    final lower = header.toLowerCase();
    final at = lower.indexOf(marker);
    if (at < 0) {
      return fallback;
    }
    var name = header.substring(at + marker.length).trim();
    if (name.startsWith('"')) {
      final end = name.indexOf('"', 1);
      name = end > 0 ? name.substring(1, end) : name.substring(1);
    } else {
      name = name.split(';').first.trim();
    }
    if (name.isEmpty) {
      return fallback;
    }
    return name;
  }
}
