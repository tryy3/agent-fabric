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

  test('listAgents parses null providerId and defaultModel', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        return http.Response(
          jsonEncode([
            {
              'id': 'ag-1',
              'name': 'Work',
              'version': 2,
              'providerId': null,
              'defaultModel': null,
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
    expect(agents.single.providerId, isNull);
    expect(agents.single.defaultModel, isNull);
    expect(agents.single.isComplete, isFalse);
  });

  test(
    'listAgents GET /v1/agents and parses providerId and defaultModel',
    () async {
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
                'providerName': 'Local',
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
      expect(agents.single.providerName, 'Local');
      expect(agents.single.defaultModel, 'm1');
      expect(agents.single.version, 2);
      expect(agents.single.description, 'desc');
    },
  );

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

  test('listProjects GET /v1/projects', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/v1/projects');
        return http.Response(
          jsonEncode([
            {
              'id': 'proj_1',
              'name': 'Personal',
              'isolation': 'isolated',
              'settings': <String, dynamic>{},
              'remotes': <Object>[],
              'createdAt': '2026-09-20T10:00:00Z',
              'updatedAt': '2026-09-20T10:00:00Z',
            },
          ]),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final projects = await client.listProjects();
    expect(projects.single.id, 'proj_1');
    expect(projects.single.name, 'Personal');
    expect(projects.single.isolation, 'isolated');
  });

  test('createProject POST /v1/projects', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/projects');
        expect(jsonDecode(request.body)['name'], 'Landing');
        return http.Response(
          jsonEncode({
            'id': 'proj_2',
            'name': 'Landing',
            'isolation': 'isolated',
            'settings': <String, dynamic>{},
            'remotes': <Object>[],
            'createdAt': '2026-09-20T10:00:00Z',
            'updatedAt': '2026-09-20T10:00:00Z',
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final project = await client.createProject(name: 'Landing');
    expect(project.id, 'proj_2');
    expect(project.name, 'Landing');
  });

  test(
    'non-success responses throw CatalogException with error body',
    () async {
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
              .having(
                (e) => e.message,
                'message',
                'provider "missing" not found',
              ),
        ),
      );
    },
  );

  test('listThreads GET /v1/threads', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/v1/threads');
        return http.Response(
          jsonEncode([
            {
              'id': 'th_1',
              'title': 'Untitled',
              'titleSource': 'auto',
              'agentId': null,
              'currentModel': null,
              'messageCount': 0,
              'viewModeId': 'compact',
              'createdAt': '2026-09-13T10:00:00Z',
              'updatedAt': '2026-09-13T10:00:00Z',
            },
          ]),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final threads = await client.listThreads();
    expect(threads, hasLength(1));
    expect(threads.single.id, 'th_1');
    expect(threads.single.title, 'Untitled');
    expect(threads.single.titleSource, 'auto');
    expect(threads.single.agentId, isNull);
    expect(threads.single.messageCount, 0);
    expect(threads.single.viewModeId, 'compact');
  });

  test('listThreads GET /v1/threads?projectId=', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/v1/threads');
        expect(request.url.queryParameters['projectId'], 'proj_1');
        return http.Response(
          jsonEncode([
            {
              'id': 'th_1',
              'title': 'Landing',
              'titleSource': 'auto',
              'projectId': 'proj_1',
              'messageCount': 0,
              'createdAt': '2026-09-13T10:00:00Z',
              'updatedAt': '2026-09-13T10:00:00Z',
            },
          ]),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final threads = await client.listThreads(projectId: 'proj_1');
    expect(threads.single.projectId, 'proj_1');
  });

  test('createThread POST /v1/threads', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/threads');
        return http.Response(
          jsonEncode({
            'id': 'th_2',
            'title': 'Untitled',
            'titleSource': 'auto',
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T10:00:00Z',
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final t = await client.createThread();
    expect(t.id, 'th_2');
    expect(t.title, 'Untitled');
  });

  test('createThread POST /v1/threads with projectId', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/threads');
        expect(jsonDecode(request.body)['projectId'], 'proj_1');
        return http.Response(
          jsonEncode({
            'id': 'th_3',
            'title': 'Untitled',
            'titleSource': 'auto',
            'projectId': 'proj_1',
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T10:00:00Z',
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final t = await client.createThread(projectId: 'proj_1');
    expect(t.projectId, 'proj_1');
  });

  test('getThread parses messages', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.url.path, '/v1/threads/th_1');
        return http.Response(
          jsonEncode({
            'id': 'th_1',
            'title': 'Hi',
            'titleSource': 'auto',
            'agentId': 'ag-1',
            'currentModel': 'm1',
            'messageCount': 1,
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T11:00:00Z',
            'messages': [
              {
                'id': 'msg_1',
                'role': 'user',
                'content': 'Hi',
                'position': 0,
                'createdAt': '2026-09-13T11:00:00Z',
              },
            ],
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final detail = await client.getThread('th_1');
    expect(detail.agentId, 'ag-1');
    expect(detail.thread.messageCount, 1);
    expect(detail.messages.single.content, 'Hi');
    expect(detail.messages.single.role, 'user');
  });

  test('getThread parses assistant parts and skips unknown types', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        return http.Response(
          jsonEncode({
            'id': 'th_1',
            'title': 'Hi',
            'titleSource': 'auto',
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T11:00:00Z',
            'messages': [
              {
                'id': 'msg_2',
                'role': 'assistant',
                'content': 'hello',
                'position': 1,
                'createdAt': '2026-09-13T11:00:01Z',
                'model': 'm1',
                'providerName': 'Local',
                'stopReason': 'end_turn',
                'parts': [
                  {'type': 'thought', 'text': 'hmm'},
                  {
                    'type': 'sent',
                    'blocks': ['ignored'],
                  },
                  {'type': 'message', 'text': 'hello'},
                  {'type': 'usage', 'predictedPerSecond': 35.5, 'deltas': 1},
                ],
              },
            ],
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final detail = await client.getThread('th_1');
    expect(detail.messages.single.thought, 'hmm');
    expect(detail.messages.single.content, 'hello');
    expect(detail.messages.single.model, 'm1');
    expect(detail.messages.single.providerName, 'Local');
    expect(detail.messages.single.stopReason, 'end_turn');
    expect(detail.messages.single.usage?.predictedPerSecond, 35.5);
    expect(detail.messages.single.usage?.deltas, 1);
  });

  test('getThread infers messageCount from messages when omitted', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        return http.Response(
          jsonEncode({
            'id': 'th_1',
            'title': 'Hi',
            'titleSource': 'auto',
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T11:00:00Z',
            'messages': [
              {
                'id': 'msg_1',
                'role': 'user',
                'content': 'Hi',
                'position': 0,
                'createdAt': '2026-09-13T11:00:00Z',
              },
              {
                'id': 'msg_2',
                'role': 'assistant',
                'content': 'Yo',
                'position': 1,
                'createdAt': '2026-09-13T11:00:01Z',
              },
            ],
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final detail = await client.getThread('th_1');
    expect(detail.thread.messageCount, 2);
  });

  test('renameThread PATCH title', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(jsonDecode(request.body)['title'], 'Renamed');
        return http.Response(
          jsonEncode({
            'id': 'th_1',
            'title': 'Renamed',
            'titleSource': 'user',
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T12:00:00Z',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final t = await client.renameThread('th_1', 'Renamed');
    expect(t.title, 'Renamed');
    expect(t.titleSource, 'user');
  });

  test('patchThreadViewMode PATCH viewModeId', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(request.url.path, '/v1/threads/th_1');
        expect(jsonDecode(request.body)['viewModeId'], 'detailed');
        return http.Response(
          jsonEncode({
            'id': 'th_1',
            'title': 'Hi',
            'titleSource': 'auto',
            'viewModeId': 'detailed',
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T12:00:00Z',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final t = await client.patchThreadViewMode('th_1', 'detailed');
    expect(t.viewModeId, 'detailed');
  });

  test('patchThreadViewMode can clear with null', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(jsonDecode(request.body)['viewModeId'], isNull);
        return http.Response(
          jsonEncode({
            'id': 'th_1',
            'title': 'Hi',
            'titleSource': 'auto',
            'createdAt': '2026-09-13T10:00:00Z',
            'updatedAt': '2026-09-13T12:00:00Z',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final t = await client.patchThreadViewMode('th_1', null);
    expect(t.viewModeId, isNull);
  });

  test('getSettings GET /v1/settings parses sandbox overlay', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/v1/settings');
        return http.Response(
          jsonEncode({
            'sandbox': {
              'kind': 'docker',
              'workspaceRoot': '/workspace',
              'image': 'alpine:3.20',
              'idleTTLSeconds': 3600,
            },
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final settings = await client.getSettings();
    expect(settings.sandbox['kind'], 'docker');
    expect(settings.sandbox['image'], 'alpine:3.20');
    expect(settings.sandbox['idleTTLSeconds'], 3600);
  });

  test('patchSettings PATCH /v1/settings sends sandbox object', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(request.url.path, '/v1/settings');
        expect(request.headers['content-type'], contains('application/json'));
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['sandbox'], {'image': 'golang:1.23'});
        return http.Response(
          jsonEncode({
            'sandbox': {'kind': 'docker', 'image': 'golang:1.23'},
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final settings = await client.patchSettings(
      sandbox: {'image': 'golang:1.23'},
    );
    expect(settings.sandbox['image'], 'golang:1.23');
    expect(settings.sandbox['kind'], 'docker');
  });

  test('listProjectFs GET catalog fs not ACP', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/v1/projects/proj_1/fs');
        expect(request.url.path, isNot(contains('/acp')));
        return http.Response(
          jsonEncode({
            'path': '/',
            'entries': [
              {
                'name': 'index.html',
                'isDir': false,
                'size': 3,
                'modTime': '2026-09-20T00:00:00Z',
              },
            ],
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final listing = await client.listProjectFs('proj_1');
    expect(listing.entries.single.name, 'index.html');
  });

  test('putProjectFile PUT /v1/projects/{id}/files', () async {
    final client = CatalogClient(
      baseUri: baseUri,
      httpClient: MockClient((request) async {
        expect(request.method, 'PUT');
        expect(request.url.path, '/v1/projects/proj_1/files');
        expect(request.url.queryParameters['path'], 'index.html');
        return http.Response('', 204);
      }),
    );
    await client.putProjectFile('proj_1', 'index.html', utf8.encode('hi'));
  });
}
