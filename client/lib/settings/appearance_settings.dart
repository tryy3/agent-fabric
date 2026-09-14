import 'package:flutter/foundation.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../ui/theme/app_theme.dart';
import '../ui/theme/chat_colors.dart';

class AppearanceSettings extends ChangeNotifier {
  AppearanceSettings._(
    this._prefs,
    this.themeMode,
    this._lightColors,
    this._darkColors,
  );

  static const _themeModeKey = 'appearance.themeMode';

  final SharedPreferences _prefs;
  ThemeMode themeMode;
  ChatColors _lightColors;
  ChatColors _darkColors;

  ThemeData get lightTheme => AppTheme.light(chatColors: _lightColors);

  ThemeData get darkTheme => AppTheme.dark(chatColors: _darkColors);

  static Future<AppearanceSettings> load() async {
    final prefs = await SharedPreferences.getInstance();
    return AppearanceSettings._(
      prefs,
      _parseThemeMode(prefs.getString(_themeModeKey)),
      _resolveColors(prefs, Brightness.light),
      _resolveColors(prefs, Brightness.dark),
    );
  }

  static ThemeMode _parseThemeMode(String? raw) {
    return switch (raw) {
      'light' => ThemeMode.light,
      'dark' => ThemeMode.dark,
      _ => ThemeMode.system,
    };
  }

  static String _brightnessName(Brightness brightness) {
    return brightness == Brightness.light ? 'light' : 'dark';
  }

  static String _colorKey(
    Brightness brightness,
    ChatColorRole role,
    String property,
  ) {
    return 'appearance.${_brightnessName(brightness)}.${role.name}.$property';
  }

  static ChatColors _defaultsFor(Brightness brightness) {
    return brightness == Brightness.light
        ? ChatColors.light()
        : ChatColors.dark();
  }

  static ChatColors _resolveColors(
    SharedPreferences prefs,
    Brightness brightness,
  ) {
    var colors = _defaultsFor(brightness);
    for (final role in ChatColorRole.values) {
      Color? fill;
      Color? bar;
      final fillRaw = prefs.getInt(_colorKey(brightness, role, 'fill'));
      if (fillRaw != null) {
        fill = Color(fillRaw);
      }
      final barRaw = prefs.getInt(_colorKey(brightness, role, 'bar'));
      if (barRaw != null) {
        bar = Color(barRaw);
      }
      if (fill != null || bar != null) {
        colors = colors.withOverride(role, fill: fill, bar: bar);
      }
    }
    return colors;
  }

  void _rebuildColors() {
    _lightColors = _resolveColors(_prefs, Brightness.light);
    _darkColors = _resolveColors(_prefs, Brightness.dark);
  }

  ChatColors colorsFor(Brightness brightness) {
    return brightness == Brightness.light ? _lightColors : _darkColors;
  }

  bool hasOverride(Brightness brightness, ChatColorRole role) {
    return _prefs.containsKey(_colorKey(brightness, role, 'fill')) ||
        _prefs.containsKey(_colorKey(brightness, role, 'bar'));
  }

  Future<void> setThemeMode(ThemeMode mode) async {
    themeMode = mode;
    if (mode == ThemeMode.system) {
      await _prefs.remove(_themeModeKey);
    } else {
      await _prefs.setString(_themeModeKey, mode.name);
    }
    notifyListeners();
  }

  Future<void> setRoleColor(
    Brightness brightness,
    ChatColorRole role, {
    Color? fill,
    Color? bar,
    bool explicitSelection = false,
  }) async {
    final defaults = _defaultsFor(brightness).forRole(role);

    if (fill != null) {
      final key = _colorKey(brightness, role, 'fill');
      if (fill == defaults.fill && !explicitSelection) {
        await _prefs.remove(key);
      } else {
        await _prefs.setInt(key, fill.toARGB32());
      }
    }

    if (bar != null) {
      final key = _colorKey(brightness, role, 'bar');
      if (bar == defaults.bar && !explicitSelection) {
        await _prefs.remove(key);
      } else {
        await _prefs.setInt(key, bar.toARGB32());
      }
    }

    _rebuildColors();
    notifyListeners();
  }

  Future<void> resetRole(Brightness brightness, ChatColorRole role) async {
    await _prefs.remove(_colorKey(brightness, role, 'fill'));
    await _prefs.remove(_colorKey(brightness, role, 'bar'));
    _rebuildColors();
    notifyListeners();
  }

  Future<void> resetAll(Brightness brightness) async {
    for (final role in ChatColorRole.values) {
      await _prefs.remove(_colorKey(brightness, role, 'fill'));
      await _prefs.remove(_colorKey(brightness, role, 'bar'));
    }
    _rebuildColors();
    notifyListeners();
  }
}
