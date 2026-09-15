import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

enum VisibilityMode { collapsed, expanded, hidden }

class ChatDisplaySettings extends ChangeNotifier {
  ChatDisplaySettings._(this._prefs, this.thinking, this.contentWidth);

  static const _thinkingKey = 'chat.thinkingVisibility';
  static const _widthKey = 'chat.contentWidth';
  static const minContentWidth = 560;
  static const maxContentWidth = 1200;
  static const defaultContentWidth = 720;

  final SharedPreferences _prefs;
  VisibilityMode thinking;
  int contentWidth;

  static Future<ChatDisplaySettings> load() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getInt(_widthKey);
    final width = (raw ?? defaultContentWidth)
        .clamp(minContentWidth, maxContentWidth);
    return ChatDisplaySettings._(
      prefs,
      _parse(prefs.getString(_thinkingKey)),
      width,
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

  Future<void> setContentWidth(int width) async {
    contentWidth = width.clamp(minContentWidth, maxContentWidth);
    await _prefs.setInt(_widthKey, contentWidth);
    notifyListeners();
  }
}
