import 'dart:async';

import 'package:acpd/acpd.dart';
import 'package:agent_fabric_client/acp/ws_socket.dart';
import 'package:meta/meta.dart';

/// WebSocket transport for acpd across browser and native platforms.
class WsTransport implements Transport {
  WsTransport._({
    required Stream<String> inbound,
    required void Function(String data) outbound,
    required Future<void> Function() onClose,
  }) : _outbound = outbound,
       _onClose = onClose {
    _subscription = inbound.listen(
      _onData,
      onError: _fail,
      onDone: _onDone,
      cancelOnError: true,
    );
  }

  /// Test / loopback constructor — no real socket.
  @visibleForTesting
  factory WsTransport.loopback({
    required StreamController<String> inbound,
    required void Function(String) outbound,
  }) {
    return WsTransport._(
      inbound: inbound.stream,
      outbound: outbound,
      onClose: () async {
        if (!inbound.isClosed) {
          await inbound.close();
        }
      },
    );
  }

  static Future<WsTransport> connect(Uri uri) async {
    final socket = await openWsSocket(uri);
    return WsTransport._(
      inbound: socket.inbound,
      outbound: socket.outbound,
      onClose: socket.close,
    );
  }

  final void Function(String data) _outbound;
  final Future<void> Function() _onClose;
  final StreamController<TransportFrame> _incoming =
      StreamController<TransportFrame>();
  late final StreamSubscription<String> _subscription;
  bool _closed = false;
  bool _incomingListened = false;

  @override
  Stream<TransportFrame> get incoming {
    _incomingListened = true;
    return _incoming.stream;
  }

  @override
  void send(TransportFrame frame) {
    if (_closed) {
      throw StateError('WebSocket ACP transport is closed.');
    }
    _outbound(frame.toWire());
  }

  void _onData(String data) {
    if (_closed) return;
    try {
      _incoming.add(decodeFrame(data));
    } catch (error, stackTrace) {
      _fail(error, stackTrace);
    }
  }

  void _onDone() {
    if (_closed) return;
    _fail(StateError('ACP WebSocket closed by the peer.'));
  }

  void _fail(Object error, [StackTrace? stackTrace]) {
    if (_closed) return;
    if (!_incoming.isClosed) {
      _incoming.addError(error, stackTrace ?? StackTrace.current);
    }
    unawaited(close());
  }

  @override
  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    await _subscription.cancel();
    await _onClose();
    if (!_incoming.isClosed) {
      await _closeIncoming();
    }
  }

  Future<void> _closeIncoming() async {
    StreamSubscription<TransportFrame>? placeholder;
    if (!_incoming.hasListener && !_incomingListened) {
      placeholder = _incoming.stream.listen((_) {});
    }
    await _incoming.close();
    await placeholder?.cancel();
  }
}
