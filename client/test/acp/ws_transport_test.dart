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
}
