import 'package:flutter/foundation.dart';

/// Local diagnostic sink for the cockpit. Never throws; safe from error handlers.
abstract final class AppLog {
  static void record(String message, [StackTrace? stack]) {
    try {
      if (kDebugMode) {
        debugPrint(message);
        if (stack != null) {
          debugPrint('$stack');
        }
      }
    } on Object catch (_) {
      // Never let the logger throw — do not call AppLog.record here.
      return;
    }
  }
}
