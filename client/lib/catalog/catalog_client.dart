import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

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
        ExportArchive;

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

  Future<List<Provider>> listProviders() async {
    final body = await _send('GET', '/v1/providers');
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(Provider.fromJson)
        .toList();
  }

  Future<Provider> createProvider({
    required String name,
    required String type,
    required String baseUrl,
    required String apiKey,
  }) async {
    final body = await _send(
      'POST',
      '/v1/providers',
      json: {'name': name, 'type': type, 'baseUrl': baseUrl, 'apiKey': apiKey},
    );
    return Provider.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<Provider> updateProvider(
    String id, {
    String? name,
    String? baseUrl,
    String? apiKey,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/providers/$id',
      json: {
        if (name != null) 'name': name,
        if (baseUrl != null) 'baseUrl': baseUrl,
        if (apiKey != null) 'apiKey': apiKey,
      },
    );
    return Provider.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteProvider(String id) async {
    await _send('DELETE', '/v1/providers/$id');
  }

  Future<Provider> refreshModels(String id) async {
    final body = await _send('POST', '/v1/providers/$id/models/refresh');
    return Provider.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<List<Agent>> listAgents() async {
    final body = await _send('GET', '/v1/agents');
    return (jsonDecode(body) as List)
        .cast<Map<String, dynamic>>()
        .map(Agent.fromJson)
        .toList();
  }

  Future<Agent> createAgent({
    required String name,
    String description = '',
    required String providerId,
    required String defaultModel,
  }) async {
    final body = await _send(
      'POST',
      '/v1/agents',
      json: {
        'name': name,
        'description': description,
        'providerId': providerId,
        'defaultModel': defaultModel,
      },
    );
    return Agent.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<Agent> updateAgent(
    String id, {
    String? name,
    String? description,
    String? providerId,
    String? defaultModel,
    Map<String, dynamic>? settings,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/agents/$id',
      json: {
        if (name != null) 'name': name,
        if (description != null) 'description': description,
        if (providerId != null) 'providerId': providerId,
        if (defaultModel != null) 'defaultModel': defaultModel,
        if (settings != null) 'settings': settings,
      },
    );
    return Agent.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteAgent(String id) async {
    await _send('DELETE', '/v1/agents/$id');
  }

  Future<PlaneSettings> getSettings() async {
    final body = await _send('GET', '/v1/settings');
    return PlaneSettings.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<PlaneSettings> patchSettings({
    required Map<String, dynamic> sandbox,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/settings',
      json: {'sandbox': sandbox},
    );
    return PlaneSettings.fromJson(jsonDecode(body) as Map<String, dynamic>);
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
    String? isolation,
    Map<String, dynamic>? settings,
    List<dynamic>? remotes,
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/projects/$id',
      json: {
        if (name != null) 'name': name,
        if (description != null) 'description': description,
        if (isolation != null) 'isolation': isolation,
        if (settings != null) 'settings': settings,
        if (remotes != null) 'remotes': remotes,
      },
    );
    return Project.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteProject(String id) async {
    await _send('DELETE', '/v1/projects/$id');
  }

  Future<Map<String, dynamic>> resolvedProjectSandbox(String projectId) async {
    final body = await _send('GET', '/v1/projects/$projectId/sandbox/resolved');
    final decoded = jsonDecode(body);
    if (decoded is Map<String, dynamic>) {
      return decoded;
    }
    return const {};
  }

  Future<Map<String, dynamic>> resolvedAgentSandbox(
    String agentId, {
    required String projectId,
  }) async {
    final body = await _send(
      'GET',
      '/v1/agents/$agentId/sandbox/resolved',
      query: {'projectId': projectId},
    );
    final decoded = jsonDecode(body);
    if (decoded is Map<String, dynamic>) {
      return decoded;
    }
    return const {};
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

  Future<ExportArchive> exportProject(
    String projectId, {
    String method = 'download',
  }) async {
    final response = await _request(
      'POST',
      '/v1/projects/$projectId/export',
      json: {'method': method},
    );
    return ExportArchive(
      filename: _filenameFromDisposition(
        response.headers['content-disposition'],
        fallback: '$projectId.zip',
      ),
      bytes: response.bodyBytes,
      mediaType: response.headers['content-type'] ?? 'application/zip',
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
