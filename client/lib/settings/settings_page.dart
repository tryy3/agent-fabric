import 'package:flutter/material.dart';

import '../catalog/catalog_client.dart';
import '../chat/display_settings.dart';
import 'agents_tab.dart';
import 'chat_tab.dart';
import 'providers_tab.dart';

class SettingsPage extends StatelessWidget {
  const SettingsPage({
    super.key,
    required this.catalog,
    required this.displaySettings,
  });

  final CatalogClient catalog;
  final ChatDisplaySettings displaySettings;

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 3,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Settings'),
          bottom: const TabBar(
            tabs: [
              Tab(text: 'Providers'),
              Tab(text: 'Agents'),
              Tab(text: 'Chat'),
            ],
          ),
        ),
        body: TabBarView(
          children: [
            ProvidersTab(catalog: catalog),
            AgentsTab(catalog: catalog),
            ChatTab(settings: displaySettings),
          ],
        ),
      ),
    );
  }
}
