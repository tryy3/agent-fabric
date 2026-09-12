import 'dart:convert';

import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  final baseUri = Uri.parse('http://catalog.test');

  test('defaultCatalogBase points at local controlplane', () {
    expect(defaultCatalogBase, Uri.parse('http://localhost:8080'));
  });

  test('listProviders GET /v1/providers and parses camelCase JSON', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url, Uri.parse('http://catalog.test/v1/providers'));
        return http.Response(
          jsonEncode([
            {
              'id': 'prov-1',
              'name': 'Local',
              'type': 'openai_compatible',
              'baseUrl': 'http://127.0.0.1:8888/v1',
              'apiKey': 'sk-test',
              'models': [
                {'id': 'm1', 'name': 'Model 1'},
              ],
              'modelsUpdatedAt': '2026-09-12T10:00:00Z',
              'createdAt': '2026-09-12T09:00:00Z',
              'updatedAt': '2026-09-12T10:00:00Z',
            },
          ]),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final providers = await client.listProviders();
    expect(providers, hasLength(1));
    expect(providers.single.id, 'prov-1');
    expect(providers.single.name, 'Local');
    expect(providers.single.type, 'openai_compatible');
    expect(providers.single.baseUrl, 'http://127.0.0.1:8888/v1');
    expect(providers.single.apiKey, 'sk-test');
    expect(providers.single.models.single.id, 'm1');
    expect(providers.single.models.single.name, 'Model 1');
    expect(providers.single.modelsUpdatedAt, DateTime.utc(2026, 9, 12, 10));
  });

  test('createProvider POST /v1/providers returns created provider', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/providers');
        expect(request.headers['content-type'], contains('application/json'));
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['name'], 'Local');
        expect(body['type'], 'openai_compatible');
        expect(body['baseUrl'], 'http://x/v1');
        expect(body['apiKey'], 'sk');
        return http.Response(
          jsonEncode({
            'id': 'prov-2',
            'name': 'Local',
            'type': 'openai_compatible',
            'baseUrl': 'http://x/v1',
            'apiKey': 'sk',
            'models': <Object>[],
            'createdAt': '2026-09-12T09:00:00Z',
            'updatedAt': '2026-09-12T09:00:00Z',
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final provider = await client.createProvider(
      name: 'Local',
      type: 'openai_compatible',
      baseUrl: 'http://x/v1',
      apiKey: 'sk',
    );
    expect(provider.id, 'prov-2');
    expect(provider.models, isEmpty);
    expect(provider.modelsUpdatedAt, isNull);
  });

  test('updateProvider PATCH /v1/providers/{id}', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(request.url.path, '/v1/providers/prov-1');
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['name'], 'Renamed');
        expect(body.containsKey('baseUrl'), isFalse);
        return http.Response(
          jsonEncode({
            'id': 'prov-1',
            'name': 'Renamed',
            'type': 'openai_compatible',
            'baseUrl': 'http://x/v1',
            'apiKey': 'sk',
            'models': <Object>[],
            'createdAt': '2026-09-12T09:00:00Z',
            'updatedAt': '2026-09-12T11:00:00Z',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final provider = await client.updateProvider('prov-1', name: 'Renamed');
    expect(provider.name, 'Renamed');
  });

  test('deleteProvider DELETE /v1/providers/{id}', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'DELETE');
        expect(request.url.path, '/v1/providers/prov-1');
        return http.Response('', 204);
      }),
    );

    await client.deleteProvider('prov-1');
  });

  test('refreshModels POST /v1/providers/{id}/models/refresh', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/providers/prov-1/models/refresh');
        return http.Response(
          jsonEncode({
            'id': 'prov-1',
            'name': 'Local',
            'type': 'openai_compatible',
            'baseUrl': 'http://x/v1',
            'apiKey': 'sk',
            'models': [
              {'id': 'm2', 'name': 'm2'},
            ],
            'modelsUpdatedAt': '2026-09-12T12:00:00Z',
            'createdAt': '2026-09-12T09:00:00Z',
            'updatedAt': '2026-09-12T12:00:00Z',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final provider = await client.refreshModels('prov-1');
    expect(provider.models.single.id, 'm2');
  });

  test('listAgents GET /v1/agents and parses providerId and defaultModel', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/v1/agents');
        return http.Response(
          jsonEncode([
            {
              'id': 'ag-1',
              'name': 'Work',
              'description': 'desc',
              'version': 2,
              'providerId': 'prov-1',
              'defaultModel': 'm1',
              'createdAt': '2026-09-12T09:00:00Z',
              'updatedAt': '2026-09-12T10:00:00Z',
            },
          ]),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final agents = await client.listAgents();
    expect(agents.single.id, 'ag-1');
    expect(agents.single.providerId, 'prov-1');
    expect(agents.single.defaultModel, 'm1');
    expect(agents.single.version, 2);
    expect(agents.single.description, 'desc');
  });

  test('createAgent POST /v1/agents', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/agents');
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['name'], 'Work');
        expect(body['description'], 'desc');
        expect(body['providerId'], 'prov-1');
        expect(body['defaultModel'], 'm1');
        return http.Response(
          jsonEncode({
            'id': 'ag-2',
            'name': 'Work',
            'description': 'desc',
            'version': 1,
            'providerId': 'prov-1',
            'defaultModel': 'm1',
            'createdAt': '2026-09-12T09:00:00Z',
            'updatedAt': '2026-09-12T09:00:00Z',
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final agent = await client.createAgent(
      name: 'Work',
      description: 'desc',
      providerId: 'prov-1',
      defaultModel: 'm1',
    );
    expect(agent.id, 'ag-2');
  });

  test('updateAgent PATCH /v1/agents/{id}', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(request.url.path, '/v1/agents/ag-1');
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['defaultModel'], 'm2');
        return http.Response(
          jsonEncode({
            'id': 'ag-1',
            'name': 'Work',
            'description': 'desc',
            'version': 3,
            'providerId': 'prov-1',
            'defaultModel': 'm2',
            'createdAt': '2026-09-12T09:00:00Z',
            'updatedAt': '2026-09-12T11:00:00Z',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    final agent = await client.updateAgent('ag-1', defaultModel: 'm2');
    expect(agent.defaultModel, 'm2');
    expect(agent.version, 3);
  });

  test('deleteAgent DELETE /v1/agents/{id}', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'DELETE');
        expect(request.url.path, '/v1/agents/ag-1');
        return http.Response('', 204);
      }),
    );

    await client.deleteAgent('ag-1');
  });

  test('non-success responses throw CatalogException with error body', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        return http.Response(
          jsonEncode({'error': 'provider "missing" not found'}),
          404,
          headers: {'content-type': 'application/json'},
        );
      }),
    );

    expect(
      () => client.deleteProvider('missing'),
      throwsA(
        isA<CatalogException>()
            .having((e) => e.statusCode, 'statusCode', 404)
            .having((e) => e.message, 'message', 'provider "missing" not found'),
      ),
    );
  });
}
