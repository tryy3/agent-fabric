import 'dart:js_interop';
import 'dart:typed_data';

import 'package:web/web.dart' as web;

typedef SaveExportBytes = Future<void> Function(
  String filename,
  Uint8List bytes,
);

Future<void> saveExportBytes(String filename, Uint8List bytes) async {
  final blob = web.Blob(
    [bytes.toJS].toJS,
    web.BlobPropertyBag(type: 'application/zip'),
  );
  final url = web.URL.createObjectURL(blob);
  final anchor = web.HTMLAnchorElement()
    ..href = url
    ..download = filename
    ..style.display = 'none';
  web.document.body?.appendChild(anchor);
  anchor.click();
  anchor.remove();
  web.URL.revokeObjectURL(url);
}
