import 'dart:typed_data';

typedef SaveExportBytes = Future<void> Function(
  String filename,
  Uint8List bytes,
);

Future<void> saveExportBytes(String filename, Uint8List bytes) async {}
