import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/ws_transport.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('send writes toWire payload to the socket', () async {
    final sent = <String>[];
    final inbound = StreamController<String>();
    final transport = WsTransport.loopback(
      inbound: inbound,
      outbound: sent.add,
    );

    final frame = TransportFrame.decode(
      '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}',
    );
    transport.send(frame);

    expect(sent, [frame.toWire()]);
    await transport.close();
    await inbound.close();
  });

  test('inbound text frames become TransportFrames on incoming', () async {
    final inbound = StreamController<String>();
    final transport = WsTransport.loopback(
      inbound: inbound,
      outbound: (_) {},
    );

    final future = transport.incoming.first;
    inbound.add('{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}');
    final frame = await future.timeout(const Duration(seconds: 1));

    expect(frame.messages, isNotEmpty);
    await transport.close();
    await inbound.close();
  });

  test('incoming preserves frames added before listener attaches', () async {
    final inbound = StreamController<String>();
    final transport = WsTransport.loopback(
      inbound: inbound,
      outbound: (_) {},
    );

    inbound.add('{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}');
    final frame = await transport.incoming.first;

    expect(frame.messages, isNotEmpty);
    await transport.close();
    await inbound.close();
  });

  test('invalid inbound json becomes MalformedTransportFrame', () async {
    final inbound = StreamController<String>();
    final transport = WsTransport.loopback(
      inbound: inbound,
      outbound: (_) {},
    );

    inbound.add('not json');
    final frame = await transport.incoming.first;

    expect(frame, isA<MalformedTransportFrame>());
    await transport.close();
    await inbound.close();
  });

  test('inbound stream errors fail transport', () async {
    final inbound = StreamController<String>();
    final transport = WsTransport.loopback(
      inbound: inbound,
      outbound: (_) {},
    );

    final expectDone = expectLater(
      transport.incoming,
      emitsError(isA<FormatException>()),
    );
    inbound.addError(
      const FormatException('ACP WebSocket messages must use text frames.'),
    );
    await expectDone;
    await transport.close();
    await inbound.close();
  });
}
