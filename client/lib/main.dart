import 'dart:ui';

import 'package:flutter/foundation.dart';
import 'package:material_ui/material_ui.dart';

import 'app_shell.dart';
import 'catalog/catalog_client.dart';
import 'chat/chat_controller.dart';
import 'chat/display_settings.dart';
import 'core/app_log.dart';
import 'settings/appearance_settings.dart';
import 'ui/theme/app_theme.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  _installErrorHandlers();

  await AppTheme.preloadFonts();
  final displaySettings = await ChatDisplaySettings.load();
  final appearanceSettings = await AppearanceSettings.load();
  runApp(
    AgentFabricApp(
      displaySettings: displaySettings,
      appearanceSettings: appearanceSettings,
    ),
  );
}

/// Install before anything that can throw. Exactly two handlers — no zone.
void _installErrorHandlers() {
  FlutterError.onError = (FlutterErrorDetails details) {
    try {
      FlutterError.presentError(details);
      AppLog.record(details.exceptionAsString(), details.stack);
    } on Object catch (_) {
      // Never let the error handler throw — do not "fix" this into recursion.
    }
  };

  PlatformDispatcher.instance.onError = (Object error, StackTrace stack) {
    try {
      AppLog.record('$error', stack);
      if (kDebugMode) {
        debugPrint('$error\n$stack');
      }
    } on Object catch (_) {
      // Never let the error handler throw — do not "fix" this into recursion.
    }
    return true;
  };
}

class AgentFabricApp extends StatefulWidget {
  const AgentFabricApp({
    super.key,
    required this.displaySettings,
    required this.appearanceSettings,
    this.controller,
    this.catalog,
  });

  /// Optional override for tests. Production leaves this null and owns the
  /// controller lifecycle.
  final ChatDisplaySettings displaySettings;
  final AppearanceSettings appearanceSettings;
  final ChatController? controller;
  final CatalogClient? catalog;

  @override
  State<AgentFabricApp> createState() => _AgentFabricAppState();
}

class _AgentFabricAppState extends State<AgentFabricApp> {
  late final CatalogClient _catalog;
  late final ChatController _controller;
  late final bool _ownsCatalog;
  late final bool _ownsController;

  @override
  void initState() {
    super.initState();
    _ownsCatalog = widget.catalog == null;
    _catalog = widget.catalog ?? CatalogClient(baseUri: defaultCatalogBase);
    _ownsController = widget.controller == null;
    _controller = widget.controller ?? ChatController(catalog: _catalog);
  }

  @override
  void dispose() {
    if (_ownsController) {
      _controller.dispose();
    }
    if (_ownsCatalog) {
      _catalog.close();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.appearanceSettings,
      builder: (context, _) {
        final appearance = widget.appearanceSettings;
        return MaterialApp(
          title: 'Agent Fabric',
          theme: appearance.lightTheme,
          darkTheme: appearance.darkTheme,
          themeMode: appearance.themeMode,
          builder: (context, child) {
            // Bridge until dock/legacy flutter/material subtrees migrate.
            // ignore: deprecated_member_use
            return MaterialUiCompatibilityBridge(child: child!);
          },
          home: AppShell(
            controller: _controller,
            catalog: _catalog,
            displaySettings: widget.displaySettings,
            appearanceSettings: appearance,
          ),
        );
      },
    );
  }
}
