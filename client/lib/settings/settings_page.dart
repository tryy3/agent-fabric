import 'package:flutter/material.dart';

import '../catalog/catalog_client.dart';
import 'providers_tab.dart';

class SettingsPage extends StatelessWidget {
  const SettingsPage({super.key, required this.catalog});

  final CatalogClient catalog;

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 2,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Settings'),
          bottom: const TabBar(
            tabs: [
              Tab(text: 'Providers'),
              Tab(text: 'Agents'),
            ],
          ),
        ),
        body: TabBarView(
          children: [
            ProvidersTab(catalog: catalog),
            const Center(child: Text('Agents')),
          ],
        ),
      ),
    );
  }
}
