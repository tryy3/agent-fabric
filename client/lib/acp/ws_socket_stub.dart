/// Browser WebSocket session opened by [openWsSocket].
class WsSocket {
  const WsSocket({
    required this.inbound,
    required this.outbound,
    required this.close,
  });

  final Stream<String> inbound;
  final void Function(String data) outbound;
  final Future<void> Function() close;
}

Future<WsSocket> openWsSocket(Uri uri) {
  throw UnsupportedError(
    'WsTransport.connect is only supported on web platforms.',
  );
}
