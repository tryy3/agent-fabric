import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

enum VisibilityMode { collapsed, expanded, hidden }

class ChatDisplaySettings extends ChangeNotifier {
  ChatDisplaySettings._(this._prefs, this.thinking, this.stats);

  static const _thinkingKey = 'chat.thinkingVisibility';
  static const _statsKey = 'chat.statsVisibility';

  final SharedPreferences _prefs;
  VisibilityMode thinking;
  VisibilityMode stats;

  static Future<ChatDisplaySettings> load() async {
    final prefs = await SharedPreferences.getInstance();
    return ChatDisplaySettings._(
      prefs,
      _parse(prefs.getString(_thinkingKey)),
      _parse(prefs.getString(_statsKey)),
    );
  }

  static VisibilityMode _parse(String? raw) {
    for (final mode in VisibilityMode.values) {
      if (mode.name == raw) {
        return mode;
      }
    }
    return VisibilityMode.collapsed;
  }

  Future<void> setThinking(VisibilityMode mode) async {
    thinking = mode;
    await _prefs.setString(_thinkingKey, mode.name);
    notifyListeners();
  }

  Future<void> setStats(VisibilityMode mode) async {
    stats = mode;
    await _prefs.setString(_statsKey, mode.name);
    notifyListeners();
  }
}
