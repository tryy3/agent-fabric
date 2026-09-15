import 'package:flutter/material.dart';

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
              Text('Content width (${settings.contentWidth}px)'),
              Slider(
                key: const Key('content-width'),
                value: settings.contentWidth.toDouble(),
                min: ChatDisplaySettings.minContentWidth.toDouble(),
                max: ChatDisplaySettings.maxContentWidth.toDouble(),
                divisions:
                    (ChatDisplaySettings.maxContentWidth -
                        ChatDisplaySettings.minContentWidth) ~/
                    20,
                label: '${settings.contentWidth}',
                onChanged: (v) => settings.setContentWidth(v.round()),
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
