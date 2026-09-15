import 'package:material_ui/material_ui.dart';

import '../ui/theme/chat_colors.dart';
import '../ui/theme/color_presets.dart';
import 'appearance_settings.dart';

class AppearanceTab extends StatelessWidget {
  const AppearanceTab({super.key, required this.settings});

  final AppearanceSettings settings;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: settings,
      builder: (context, _) {
        final brightness = _editBrightness(context);
        final colors = settings.colorsFor(brightness);

        return Material(
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              DropdownButtonFormField<ThemeMode>(
                key: const Key('theme-mode'),
                decoration: const InputDecoration(labelText: 'Theme mode'),
                initialValue: settings.themeMode,
                items: _themeModeItems(),
                onChanged: (mode) {
                  if (mode != null) {
                    settings.setThemeMode(mode);
                  }
                },
              ),
              const SizedBox(height: 16),
              Text('Preview', style: Theme.of(context).textTheme.titleSmall),
              const SizedBox(height: 8),
              Row(
                children: [
                  for (final role in ChatColorRole.values)
                    Expanded(
                      child: _PreviewChip(
                        fill: colors.forRole(role).fill,
                        bar: colors.forRole(role).bar,
                      ),
                    ),
                ],
              ),
              const SizedBox(height: 24),
              for (final role in ChatColorRole.values)
                _RoleSection(
                  role: role,
                  brightness: brightness,
                  settings: settings,
                  colors: colors.forRole(role),
                ),
            ],
          ),
        );
      },
    );
  }

  Brightness _editBrightness(BuildContext context) {
    return switch (settings.themeMode) {
      ThemeMode.light => Brightness.light,
      ThemeMode.dark => Brightness.dark,
      ThemeMode.system => MediaQuery.platformBrightnessOf(context),
    };
  }

  static List<DropdownMenuItem<ThemeMode>> _themeModeItems() {
    return [
      for (final mode in ThemeMode.values)
        DropdownMenuItem(value: mode, child: Text(_themeModeLabel(mode))),
    ];
  }

  static String _themeModeLabel(ThemeMode mode) {
    return switch (mode) {
      ThemeMode.system => 'System',
      ThemeMode.light => 'Light',
      ThemeMode.dark => 'Dark',
    };
  }
}

class _PreviewChip extends StatelessWidget {
  const _PreviewChip({required this.fill, required this.bar});

  final Color fill;
  final Color bar;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4),
      child: Container(
        height: 32,
        decoration: BoxDecoration(
          color: fill,
          borderRadius: BorderRadius.circular(4),
          border: Border(bottom: BorderSide(color: bar, width: 3)),
        ),
      ),
    );
  }
}

class _RoleSection extends StatelessWidget {
  const _RoleSection({
    required this.role,
    required this.brightness,
    required this.settings,
    required this.colors,
  });

  final ChatColorRole role;
  final Brightness brightness;
  final AppearanceSettings settings;
  final RoleColors colors;

  @override
  Widget build(BuildContext context) {
    final hasOverride = settings.hasOverride(brightness, role);

    return Padding(
      padding: const EdgeInsets.only(bottom: 24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Text(
                _roleLabel(role),
                style: Theme.of(context).textTheme.titleSmall,
              ),
              if (hasOverride) ...[
                const Spacer(),
                TextButton(
                  key: Key('reset-${role.name}'),
                  onPressed: () => settings.resetRole(brightness, role),
                  child: const Text('Reset'),
                ),
              ],
            ],
          ),
          const SizedBox(height: 8),
          Text('Fill', style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 4),
          _SwatchRow(
            presets: kFillPresets,
            selected: colors.fill,
            keyPrefix: 'swatch-${role.name}-fill',
            onSelected: (color) => settings.setRoleColor(
              brightness,
              role,
              fill: color,
              explicitSelection: true,
            ),
          ),
          const SizedBox(height: 8),
          Text('Bar', style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 4),
          _SwatchRow(
            presets: kBarPresets,
            selected: colors.bar,
            keyPrefix: 'swatch-${role.name}-bar',
            onSelected: (color) => settings.setRoleColor(
              brightness,
              role,
              bar: color,
              explicitSelection: true,
            ),
          ),
        ],
      ),
    );
  }

  static String _roleLabel(ChatColorRole role) {
    return switch (role) {
      ChatColorRole.thinking => 'Thinking',
      ChatColorRole.answer => 'Answer',
      ChatColorRole.stats => 'Stats',
      ChatColorRole.user => 'User',
    };
  }
}

class _SwatchRow extends StatelessWidget {
  const _SwatchRow({
    required this.presets,
    required this.selected,
    required this.keyPrefix,
    required this.onSelected,
  });

  final List<Color> presets;
  final Color selected;
  final String keyPrefix;
  final ValueChanged<Color> onSelected;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (var i = 0; i < presets.length; i++)
          InkWell(
            key: Key('$keyPrefix-$i'),
            onTap: () => onSelected(presets[i]),
            borderRadius: BorderRadius.circular(4),
            child: Container(
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                color: presets[i],
                borderRadius: BorderRadius.circular(4),
                border: Border.all(
                  color: presets[i] == selected
                      ? Theme.of(context).colorScheme.primary
                      : Theme.of(context).dividerColor,
                  width: presets[i] == selected ? 2 : 1,
                ),
              ),
            ),
          ),
      ],
    );
  }
}
