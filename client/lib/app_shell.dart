import 'package:flutter/material.dart';

import 'catalog/catalog_client.dart';
import 'catalog/models.dart';
import 'chat/chat_controller.dart';
import 'chat/chat_screen.dart';
import 'chat/thread_pane.dart';
import 'settings/settings_page.dart';

class AppShell extends StatefulWidget {
  const AppShell({super.key, required this.controller, this.catalog});

  final ChatController controller;
  final CatalogClient? catalog;

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  int _selectedIndex = 0;
  late final CatalogClient _catalog;
  late final bool _ownsCatalog;

  @override
  void initState() {
    super.initState();
    _ownsCatalog = widget.catalog == null;
    _catalog = widget.catalog ?? CatalogClient(baseUri: defaultCatalogBase);
  }

  @override
  void dispose() {
    if (_ownsCatalog) {
      _catalog.close();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Row(
        children: [
          NavigationRail(
            selectedIndex: _selectedIndex,
            onDestinationSelected: (index) {
              setState(() => _selectedIndex = index);
            },
            labelType: NavigationRailLabelType.all,
            destinations: const [
              NavigationRailDestination(
                icon: Icon(Icons.chat),
                label: Text('Chat'),
              ),
              NavigationRailDestination(
                icon: Icon(Icons.settings),
                label: Text('Settings'),
              ),
            ],
          ),
          const VerticalDivider(thickness: 1, width: 1),
          if (_selectedIndex == 0) ...[
            ThreadPane(controller: widget.controller),
            const VerticalDivider(thickness: 1, width: 1),
          ],
          Expanded(
            child: IndexedStack(
              index: _selectedIndex,
              children: [
                ChatScreen(controller: widget.controller),
                SettingsPage(catalog: _catalog),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
