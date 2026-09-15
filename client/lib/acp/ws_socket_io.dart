import 'package:web_socket_channel/io.dart';

import 'ws_socket_bind.dart';
import 'ws_socket_stub.dart';

Future<WsSocket> openWsSocket(Uri uri) async {
  if (uri.scheme != 'ws' && uri.scheme != 'wss') {
    throw ArgumentError.value(uri, 'uri', 'must be ws or wss');
  }
  final channel = IOWebSocketChannel.connect(
    uri,
    pingInterval: kWsPingInterval,
    connectTimeout: kWsConnectTimeout,
  );
  return bindWsChannel(channel);
}
