import 'package:flutter/material.dart';

import 'app_shell.dart';
import 'catalog/catalog_client.dart';
import 'catalog/models.dart';
import 'chat/chat_controller.dart';

void main() {
  runApp(const AgentFabricApp());
}

class AgentFabricApp extends StatefulWidget {
  const AgentFabricApp({super.key, this.controller, this.catalog});

  /// Optional override for tests. Production leaves this null and owns the
  /// controller lifecycle.
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
    _controller =
        widget.controller ?? ChatController(catalog: _catalog);
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
    return MaterialApp(
      title: 'Agent Fabric',
      home: AppShell(controller: _controller, catalog: _catalog),
    );
  }
}
