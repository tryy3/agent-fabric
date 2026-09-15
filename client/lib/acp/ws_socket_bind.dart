import 'dart:async';

import 'package:web_socket_channel/status.dart' as status;
import 'package:web_socket_channel/web_socket_channel.dart';

import 'ws_socket_stub.dart';

const kWsConnectTimeout = Duration(seconds: 30);
const kWsPingInterval = Duration(seconds: 30);

Future<WsSocket> bindWsChannel(
  WebSocketChannel channel, {
  Duration? readyTimeout,
}) async {
  final ready = channel.ready;
  if (readyTimeout != null) {
    await ready.timeout(readyTimeout);
  } else {
    await ready;
  }

  final inbound = StreamController<String>();
  final sub = channel.stream.listen(
    (event) {
      if (event is String) {
        inbound.add(event);
      } else {
        inbound.addError(
          const FormatException(
            'ACP WebSocket messages must use text frames.',
          ),
        );
      }
    },
    onError: inbound.addError,
    onDone: () {
      if (!inbound.isClosed) inbound.close();
    },
    cancelOnError: false,
  );

  return WsSocket(
    inbound: inbound.stream,
    outbound: (data) => channel.sink.add(data),
    close: () async {
      await sub.cancel();
      await channel.sink.close(status.normalClosure);
      if (!inbound.isClosed) await inbound.close();
    },
  );
}
