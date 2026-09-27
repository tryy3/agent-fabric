import 'package:agent_fabric_client/core/runtime_config.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  final page = Uri.parse('https://agents.example.ts.net/app');

  group('resolveRuntimeConfig', () {
    test('missing file uses localhost defaults', () {
      final cfg = resolveRuntimeConfig(
        file: null,
        isWeb: true,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('http://localhost:8080'));
      expect(cfg.acpUri, Uri.parse('ws://localhost:8080/acp'));
    });

    test('empty file on web uses same-origin', () {
      final cfg = resolveRuntimeConfig(
        file: const RuntimeConfigFile(),
        isWeb: true,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('https://agents.example.ts.net'));
      expect(cfg.acpUri, Uri.parse('wss://agents.example.ts.net/acp'));
    });

    test('empty file off web uses localhost', () {
      final cfg = resolveRuntimeConfig(
        file: const RuntimeConfigFile(),
        isWeb: false,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('http://localhost:8080'));
      expect(cfg.acpUri, Uri.parse('ws://localhost:8080/acp'));
    });

    test('explicit fields win', () {
      final cfg = resolveRuntimeConfig(
        file: const RuntimeConfigFile(
          catalogBase: 'https://catalog.example/v1-host',
          acpUri: 'wss://acp.example/acp',
        ),
        isWeb: true,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('https://catalog.example/v1-host'));
      expect(cfg.acpUri, Uri.parse('wss://acp.example/acp'));
    });

    test('partial explicit mixes with same-origin on web', () {
      final cfg = resolveRuntimeConfig(
        file: const RuntimeConfigFile(
          catalogBase: 'https://only-catalog.example',
        ),
        isWeb: true,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('https://only-catalog.example'));
      expect(cfg.acpUri, Uri.parse('wss://agents.example.ts.net/acp'));
    });
  });

  group('loadRuntimeConfig', () {
    test('404 yields localhost defaults', () async {
      final client = MockClient((_) async => http.Response('missing', 404));
      final cfg = await loadRuntimeConfig(
        httpClient: client,
        configUri: Uri.parse('http://test/config.json'),
        isWeb: true,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('http://localhost:8080'));
    });

    test('empty JSON on web yields same-origin', () async {
      final client = MockClient((_) async => http.Response('{}', 200));
      final cfg = await loadRuntimeConfig(
        httpClient: client,
        configUri: Uri.parse('http://test/config.json'),
        isWeb: true,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('https://agents.example.ts.net'));
      expect(cfg.acpUri, Uri.parse('wss://agents.example.ts.net/acp'));
    });

    test('parses explicit JSON fields', () async {
      final client = MockClient(
        (_) async => http.Response(
          '{"catalogBase":"https://c.example","acpUri":"wss://a.example/acp"}',
          200,
        ),
      );
      final cfg = await loadRuntimeConfig(
        httpClient: client,
        configUri: Uri.parse('http://test/config.json'),
        isWeb: true,
        pageOrigin: page,
      );
      expect(cfg.catalogBase, Uri.parse('https://c.example'));
      expect(cfg.acpUri, Uri.parse('wss://a.example/acp'));
    });
  });

  test('sameOriginAcpUri maps http to ws and https to wss', () {
    expect(
      sameOriginAcpUri(Uri.parse('http://localhost')),
      Uri.parse('ws://localhost/acp'),
    );
    expect(
      sameOriginAcpUri(Uri.parse('https://host.example')),
      Uri.parse('wss://host.example/acp'),
    );
  });
}
