import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

enum VisibilityMode { collapsed, expanded, hidden }

class ChatDisplaySettings extends ChangeNotifier {
  ChatDisplaySettings._(this._prefs, this.contentWidth);

  static const _widthKey = 'chat.contentWidth';
  static const minContentWidth = 560;
  static const maxContentWidth = 1200;
  static const defaultContentWidth = 720;

  final SharedPreferences _prefs;
  int contentWidth;

  static Future<ChatDisplaySettings> load() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getInt(_widthKey);
    final width = (raw ?? defaultContentWidth).clamp(
      minContentWidth,
      maxContentWidth,
    );
    return ChatDisplaySettings._(prefs, width);
  }

  Future<void> setContentWidth(int width) async {
    contentWidth = width.clamp(minContentWidth, maxContentWidth);
    await _prefs.setInt(_widthKey, contentWidth);
    notifyListeners();
  }
}
