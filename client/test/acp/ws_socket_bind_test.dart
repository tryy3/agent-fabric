import 'dart:async';

import 'package:agent_fabric_client/acp/ws_socket_bind.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

class _FakeChannel implements WebSocketChannel {
  _FakeChannel(this._stream, this.sink, {Future<void>? ready})
    : _ready = ready ?? (Completer<void>()..complete()).future;

  final Stream<dynamic> _stream;
  @override
  final WebSocketSink sink;
  final Future<void> _ready;

  @override
  Stream get stream => _stream;

  @override
  Future<void> get ready => _ready;

  @override
  int? get closeCode => null;

  @override
  String? get closeReason => null;

  @override
  String? get protocol => null;

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeSink implements WebSocketSink {
  final sent = <dynamic>[];
  int? closeCode;
  String? closeReason;
  bool closed = false;

  @override
  void add(dynamic data) => sent.add(data);

  @override
  void addError(Object error, [StackTrace? stackTrace]) {}

  @override
  Future addStream(Stream stream) => throw UnimplementedError();

  @override
  Future close([int? closeCode, String? closeReason]) async {
    closed = true;
    this.closeCode = closeCode;
    this.closeReason = closeReason;
  }

  @override
  Future get done => Future.value();
}

void main() {
  test('bindWsChannel forwards text and rejects binary', () async {
    final controller = StreamController<dynamic>();
    final sink = _FakeSink();
    final socket = await bindWsChannel(_FakeChannel(controller.stream, sink));

    final events = <Object>[];
    final sub = socket.inbound.listen(
      events.add,
      onError: events.add,
    );
    controller.add('hello');
    await Future<void>.delayed(Duration.zero);
    expect(events.single, 'hello');

    controller.add(<int>[1, 2, 3]);
    await Future<void>.delayed(Duration.zero);
    expect(events.last, isA<FormatException>());
    await sub.cancel();

    socket.outbound('out');
    expect(sink.sent, ['out']);
    await socket.close();
    expect(sink.closeCode, isNotNull);
    await controller.close();
  });

  test('bindWsChannel closes sink when ready fails', () async {
    final controller = StreamController<dynamic>();
    final sink = _FakeSink();
    final ready = Completer<void>();
    final bound = bindWsChannel(
      _FakeChannel(controller.stream, sink, ready: ready.future),
    );
    ready.completeError(StateError('ready failed'));
    await expectLater(bound, throwsA(isA<StateError>()));
    expect(sink.closed, isTrue);
    // Nothing subscribed, so close() would wait forever for a done event.
  });
}
