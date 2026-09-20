import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../chat/display_settings.dart';
import 'agents_tab.dart';
import 'appearance_settings.dart';
import 'display_tab.dart';
import 'projects_tab.dart';
import 'providers_tab.dart';
import 'sandbox_tab.dart';

class SettingsPage extends StatelessWidget {
  const SettingsPage({
    super.key,
    required this.catalog,
    required this.displaySettings,
    required this.appearanceSettings,
  });

  final CatalogClient catalog;
  final ChatDisplaySettings displaySettings;
  final AppearanceSettings appearanceSettings;

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 5,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Settings'),
          bottom: const TabBar(
            tabs: [
              Tab(text: 'Providers'),
              Tab(text: 'Agents'),
              Tab(text: 'Projects'),
              Tab(text: 'Sandbox'),
              Tab(text: 'Display'),
            ],
          ),
        ),
        body: TabBarView(
          children: [
            ProvidersTab(catalog: catalog),
            AgentsTab(catalog: catalog),
            ProjectsTab(catalog: catalog),
            SandboxTab(catalog: catalog),
            DisplayTab(
              displaySettings: displaySettings,
              appearanceSettings: appearanceSettings,
            ),
          ],
        ),
      ),
    );
  }
}
