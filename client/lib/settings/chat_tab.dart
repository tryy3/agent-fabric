import 'package:material_ui/material_ui.dart';

import '../chat/display_settings.dart';

class ChatTab extends StatelessWidget {
  const ChatTab({super.key, required this.settings});

  final ChatDisplaySettings settings;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: settings,
      builder: (context, _) {
        return Material(
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              DropdownButtonFormField<VisibilityMode>(
                key: const Key('thinking-visibility'),
                decoration: const InputDecoration(labelText: 'Thinking'),
                initialValue: settings.thinking,
                items: _modeItems(),
                onChanged: (mode) {
                  if (mode != null) {
                    settings.setThinking(mode);
                  }
                },
              ),
              DropdownButtonFormField<VisibilityMode>(
                key: const Key('stats-visibility'),
                decoration: const InputDecoration(labelText: 'Stats'),
                initialValue: settings.stats,
                items: _modeItems(),
                onChanged: (mode) {
                  if (mode != null) {
                    settings.setStats(mode);
                  }
                },
              ),
            ],
          ),
        );
      },
    );
  }

  static List<DropdownMenuItem<VisibilityMode>> _modeItems() {
    return [
      for (final mode in VisibilityMode.values)
        DropdownMenuItem(value: mode, child: Text(_label(mode))),
    ];
  }

  static String _label(VisibilityMode mode) {
    return switch (mode) {
      VisibilityMode.collapsed => 'Collapsed',
      VisibilityMode.expanded => 'Expanded',
      VisibilityMode.hidden => 'Hidden',
    };
  }
}
