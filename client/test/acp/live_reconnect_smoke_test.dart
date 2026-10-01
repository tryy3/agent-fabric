import 'dart:async';

import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/acp/ws_transport.dart';
import 'package:agent_fabric_client/catalog/models.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;

/// Live smoke against a running control plane on localhost:8080.
///
/// Skipped when the plane is not reachable so CI stays green.
void main() {
  test('live plane: catalog probe + ACP connect', () async {
    final catalog = defaultCatalogBase;
    final client = http.Client();
    addTearDown(client.close);

    late http.Response settings;
    try {
      settings = await client
          .get(catalog.resolve('/v1/settings'))
          .timeout(const Duration(seconds: 2));
    } on Object {
      markTestSkipped('control plane not reachable on localhost:8080');
      return;
    }
    expect(settings.statusCode, lessThan(500));

    final probe = catalogHttpProbe(catalog, httpClient: client);
    expect(await probe(), isTrue);

    final conn = AgentConnection(
      acpUri: defaultAcpUri,
      transportFactory: WsTransport.connect,
      reachabilityProbe: probe,
      initializeTimeout: const Duration(seconds: 10),
    );
    final states = <AcpConnectionState>[];
    final sub = conn.connectionState.listen(states.add);
    addTearDown(() async {
      await conn.close();
      await sub.cancel();
    });

    await conn.connect().timeout(const Duration(seconds: 10));
    expect(states.last, AcpConnectionState.connected);
  });
}
