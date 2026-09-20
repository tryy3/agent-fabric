import 'dart:convert';

import 'package:http/http.dart' as http;

import 'models.dart';

export 'models.dart' show defaultCatalogBase, CatalogException;

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
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/agents/$id',
      json: {
        if (name != null) 'name': name,
        if (description != null) 'description': description,
        if (providerId != null) 'providerId': providerId,
        if (defaultModel != null) 'defaultModel': defaultModel,
      },
    );
    return Agent.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteAgent(String id) async {
    await _send('DELETE', '/v1/agents/$id');
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
  }) async {
    final body = await _send(
      'PATCH',
      '/v1/projects/$id',
      json: {
        if (name != null) 'name': name,
        if (description != null) 'description': description,
      },
    );
    return Project.fromJson(jsonDecode(body) as Map<String, dynamic>);
  }

  Future<void> deleteProject(String id) async {
    await _send('DELETE', '/v1/projects/$id');
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

  Future<String> _send(
    String method,
    String path, {
    Map<String, dynamic>? json,
    Map<String, String>? query,
  }) async {
    var url = _baseUri.resolve(path);
    if (query != null && query.isNotEmpty) {
      url = url.replace(
        queryParameters: <String, String>{...url.queryParameters, ...query},
      );
    }
    final headers = <String, String>{
      if (json != null) 'content-type': 'application/json',
    };
    final body = json == null ? null : jsonEncode(json);
    late http.Response response;
    switch (method) {
      case 'GET':
        response = await _http.get(url, headers: headers);
      case 'POST':
        response = await _http.post(url, headers: headers, body: body);
      case 'PATCH':
        response = await _http.patch(url, headers: headers, body: body);
      case 'DELETE':
        response = await _http.delete(url, headers: headers);
      default:
        throw ArgumentError.value(method, 'method');
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw CatalogException(
        statusCode: response.statusCode,
        message: _errorMessage(response.body),
      );
    }
    return response.body;
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
}
