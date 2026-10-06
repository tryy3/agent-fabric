import 'dart:convert';

import 'package:agent_fabric_client/catalog/catalog_client.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  test('ModelInfo parses joined specs and tolerates absence', () {
    final m = ModelInfo.fromJson({
      'id': 'big-1',
      'name': 'Big 1',
      'specs': {
        'tool_call': true,
        'reasoning': true,
        'status': 'beta',
        'modalities': {
          'input': ['text', 'image'],
          'output': ['text'],
        },
        'limit': {'context': 200000, 'output': 8000},
        'cost': {'input': 3, 'output': 15.5, 'cache_read': 0.3},
      },
    });
    final s = m.specs!;
    expect(s.toolCall, isTrue);
    expect(s.attachment, isFalse);
    expect(s.inputModalities, ['text', 'image']);
    expect(s.contextLimit, 200000);
    expect(s.costInput, 3);
    expect(s.costOutput, 15.5);
    expect(s.costCacheRead, 0.3);
    expect(s.costCacheWrite, isNull);

    expect(ModelInfo.fromJson({'id': 'x'}).specs, isNull);
  });

  test('absent price stays null, zero price stays zero', () {
    final s = ModelSpecs.fromJson({
      'cost': {'input': 0},
      'limit': {'context': 0},
    });
    expect(s.costInput, 0);
    expect(s.costOutput, isNull);
    expect(s.contextLimit, isNull);
    expect(s.hasPrice, isTrue);
  });

  test('InferenceConnection carries the matched specs provider', () {
    final c = InferenceConnection.fromJson({
      'id': 'c1',
      'name': 'Acme',
      'type': 'openai_compatible',
      'baseUrl': 'https://api.acme.test/v1',
      'models': <Object>[],
      'specsProvider': {
        'id': 'acme',
        'name': 'Acme',
        'logoUrl': '/v1/model-specs/providers/acme/logo',
      },
      'createdAt': '2026-10-01T00:00:00Z',
      'updatedAt': '2026-10-01T00:00:00Z',
    });
    expect(c.specsProvider!.id, 'acme');
  });

  test('catalog client reads status, patches settings, caches logos', () async {
    var logoHits = 0;
    final requests = <String>[];
    final client = CatalogClient(
      baseUri: Uri.parse('http://catalog.test'),
      httpClient: MockClient((req) async {
        requests.add('${req.method} ${req.url.path}');
        switch (req.url.path) {
          case '/v1/model-specs/status':
            return http.Response(
              jsonEncode({
                'sourceUrl': '',
                'effectiveSourceUrl': 'https://models.dev/api.json',
                'syncIntervalHours': 24,
                'enabled': true,
                'providerCount': 2,
                'modelCount': 10,
                'lastSyncedAt': '2026-10-05T10:00:00Z',
                'lastError': '',
              }),
              200,
            );
          case '/v1/model-specs/settings':
            expect(jsonDecode(req.body), {'enabled': false});
            return http.Response('{}', 200);
          case '/v1/model-specs/providers/acme/logo':
            logoHits++;
            return http.Response(
              '<svg/>',
              200,
              headers: {'content-type': 'image/svg+xml'},
            );
        }
        return http.Response('{"error":"nope"}', 404);
      }),
    );
    final status = await client.getModelSpecsStatus();
    expect(status.modelCount, 10);
    expect(status.lastSyncedAt, DateTime.utc(2026, 10, 5, 10));
    await client.updateModelSpecsSettings(enabled: false);

    const logo = '/v1/model-specs/providers/acme/logo';
    expect((await client.providerLogo(logo))!.isSvg, isTrue);
    await client.providerLogo(logo);
    expect(logoHits, 1);
    expect(
      await client.providerLogo('/v1/model-specs/providers/x/logo'),
      isNull,
    );
  });
}
