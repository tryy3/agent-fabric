import 'dart:async';
import 'dart:io';

import 'package:acpd/acpd.dart' hide AgentConnection;
import 'package:agent_fabric_client/acp/agent_connection.dart';
import 'package:agent_fabric_client/acp/ws_transport.dart';
import 'package:flutter_test/flutter_test.dart';

/// Serves ACP over a real local WebSocket for reconnect integration tests.
class _AcpWsServer {
  _AcpWsServer(this._server);

  final HttpServer _server;
  final List<WebSocket> _sockets = [];
  final List<Future<void> Function()> _agentClosers = [];

  Uri get acpUri => Uri(
    scheme: 'ws',
    host: _server.address.host,
    port: _server.port,
    path: '/acp',
  );

  static Future<_AcpWsServer> start() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final wrapper = _AcpWsServer(server);
    server.listen(wrapper._handle);
    return wrapper;
  }

  Future<void> _handle(HttpRequest request) async {
    if (request.uri.path != '/acp') {
      request.response.statusCode = HttpStatus.notFound;
      await request.response.close();
      return;
    }
    final ws = await WebSocketTransformer.upgrade(request);
    _sockets.add(ws);
    final inbound = StreamController<String>();
    final agentTransport = WsTransport.loopback(
      inbound: inbound,
      outbound: ws.add,
    );
    ws.listen(
      (event) {
        if (event is String) {
          inbound.add(event);
        }
      },
      onDone: () {
        if (!inbound.isClosed) {
          unawaited(inbound.close());
        }
      },
      onError: (_, _) {
        if (!inbound.isClosed) {
          unawaited(inbound.close());
        }
      },
    );
    final agentConn = AgentRole()
        .onInitialize((ctx, request, cancellation) async {
          return const InitializeResponse(
            protocolVersion: ProtocolVersion.v1,
            agentInfo: Implementation(name: 'ws-test', version: '0.0.1'),
          );
        })
        .onNewSession((ctx, request, cancellation) async {
          return const NewSessionResponse(sessionId: 'sess-ws');
        })
        .connect(agentTransport);
    _agentClosers.add(agentConn.close);
  }

  Future<void> dropClients() async {
    final sockets = List<WebSocket>.from(_sockets);
    _sockets.clear();
    for (final ws in sockets) {
      await ws.close();
    }
  }

  Future<void> close() async {
    await dropClients();
    for (final closer in _agentClosers) {
      try {
        await closer();
      } on Object {
        // best-effort
      }
    }
    _agentClosers.clear();
    await _server.close(force: true);
  }
}

void main() {
  test('real WebSocket reconnect recovers after server drop and retryNow', () async {
    final server = await _AcpWsServer.start();
    addTearDown(server.close);

    // Gate dials so the first reconnect attempt fails immediately into backoff
    // instead of hanging on a refused/half-open WebSocket upgrade.
    var planeUp = true;
    final conn = AgentConnection(
      acpUri: server.acpUri,
      transportFactory: (uri) async {
        if (!planeUp) {
          throw StateError('plane down');
        }
        return WsTransport.connect(uri);
      },
      backoffForAttempt: (_) => const Duration(hours: 1),
    );
    final states = <AcpConnectionState>[];
    final sub = conn.connectionState.listen(states.add);
    addTearDown(() async {
      await conn.close();
      await sub.cancel();
    });

    await conn.connect();
    expect(states.last, AcpConnectionState.connected);

    planeUp = false;
    await server.dropClients();

    for (
      var i = 0;
      i < 100 && !states.contains(AcpConnectionState.reconnecting);
      i++
    ) {
      await Future<void>.delayed(const Duration(milliseconds: 20));
    }
    expect(states, contains(AcpConnectionState.reconnecting));

    // Still in long backoff until retryNow.
    await Future<void>.delayed(const Duration(milliseconds: 50));
    expect(states.last, isNot(AcpConnectionState.connected));

    planeUp = true;
    conn.retryNow();

    for (
      var i = 0;
      i < 100 && states.last != AcpConnectionState.connected;
      i++
    ) {
      await Future<void>.delayed(const Duration(milliseconds: 20));
    }
    expect(states.last, AcpConnectionState.connected);
  });

  test('real WebSocket reconnect recovers via reachability probe', () async {
    final server = await _AcpWsServer.start();
    addTearDown(server.close);

    var planeUp = true;
    var reachable = false;
    final conn = AgentConnection(
      acpUri: server.acpUri,
      transportFactory: (uri) async {
        if (!planeUp) {
          throw StateError('plane down');
        }
        return WsTransport.connect(uri);
      },
      backoffForAttempt: (_) => const Duration(hours: 1),
      reachabilityProbe: () async => reachable,
    );
    final states = <AcpConnectionState>[];
    final sub = conn.connectionState.listen(states.add);
    addTearDown(() async {
      await conn.close();
      await sub.cancel();
    });

    await conn.connect();
    planeUp = false;
    await server.dropClients();

    for (
      var i = 0;
      i < 100 && !states.contains(AcpConnectionState.reconnecting);
      i++
    ) {
      await Future<void>.delayed(const Duration(milliseconds: 20));
    }
    expect(states, contains(AcpConnectionState.reconnecting));

    planeUp = true;
    reachable = true;

    for (
      var i = 0;
      i < 200 && states.last != AcpConnectionState.connected;
      i++
    ) {
      await Future<void>.delayed(const Duration(milliseconds: 20));
    }
    expect(states.last, AcpConnectionState.connected);
  });
}
