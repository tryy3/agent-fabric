import 'dart:async';

import 'package:web_socket_channel/status.dart' as status;
import 'package:web_socket_channel/web_socket_channel.dart';

import 'ws_socket_stub.dart';

import 'package:agent_fabric_client/core/app_log.dart';

const kWsConnectTimeout = Duration(seconds: 30);
const kWsPingInterval = Duration(seconds: 30);

Future<WsSocket> bindWsChannel(
  WebSocketChannel channel, {
  Duration? readyTimeout,
}) async {
  try {
    final ready = channel.ready;
    if (readyTimeout != null) {
      await ready.timeout(readyTimeout);
    } else {
      await ready;
    }
  } on Object catch (_) {
    try {
      await channel.sink.close();
    } on Object catch (e, s) {
      AppLog.record('teardown: $e', s);
    }
    rethrow;
  }

  final inbound = StreamController<String>();
  final sub = channel.stream.listen(
    (event) {
      if (event is String) {
        inbound.add(event);
      } else {
        inbound.addError(
          const FormatException('ACP WebSocket messages must use text frames.'),
        );
      }
    },
    onError: inbound.addError,
    onDone: () {
      if (!inbound.isClosed) {
        unawaited(
          inbound.close().catchError((Object e, StackTrace s) {
            AppLog.record('inbound close: $e', s);
          }),
        );
      }
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
