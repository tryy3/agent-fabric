import 'dart:async';
import 'dart:js_interop';

import 'package:agent_fabric_client/acp/ws_socket_stub.dart';
import 'package:web/web.dart' as web;

Future<WsSocket> openWsSocket(Uri uri) async {
  if (uri.scheme != 'ws' && uri.scheme != 'wss') {
    throw ArgumentError.value(uri, 'uri', 'must be ws or wss');
  }
  final socket = web.WebSocket(uri.toString());
  final ready = Completer<void>();
  final inbound = StreamController<String>();

  socket.onOpen.listen((event) {
    if (!ready.isCompleted) ready.complete();
  });
  socket.onError.listen((event) {
    if (!ready.isCompleted) {
      ready.completeError(StateError('WebSocket connection failed'));
    }
    inbound.addError(StateError('WebSocket error'));
  });
  socket.onClose.listen((event) {
    if (!inbound.isClosed) {
      inbound.close();
    }
  });
  socket.onMessage.listen((event) {
    final data = event.data;
    if (data == null) {
      return;
    }
    final text = data.dartify();
    if (text is String) {
      inbound.add(text);
    } else {
      inbound.addError(
        const FormatException('ACP WebSocket messages must use text frames.'),
      );
    }
  });

  await ready.future.timeout(const Duration(seconds: 30));

  return WsSocket(
    inbound: inbound.stream,
    outbound: (data) => socket.send(data.toJS),
    close: () async {
      socket.close(1000, 'normal');
      if (!inbound.isClosed) await inbound.close();
    },
  );
}
