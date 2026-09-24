import 'package:material_ui/material_ui.dart';

import '../chat/display_settings.dart';
import '../ui/theme/app_theme.dart';
import '../ui/theme/chat_colors.dart';
import '../ui/theme/color_presets.dart';
import 'appearance_settings.dart';

/// Combined Chat + Appearance settings with a live window preview on top.
class DisplayTab extends StatelessWidget {
  const DisplayTab({
    super.key,
    required this.displaySettings,
    required this.appearanceSettings,
  });

  final ChatDisplaySettings displaySettings;
  final AppearanceSettings appearanceSettings;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: Listenable.merge([displaySettings, appearanceSettings]),
      builder: (context, _) {
        final brightness = _editBrightness(context, appearanceSettings);
        final colors = appearanceSettings.colorsFor(brightness);
        final previewTheme = brightness == Brightness.dark
            ? AppTheme.dark(chatColors: colors)
            : AppTheme.light(chatColors: colors);

        return Material(
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              Text(
                'Preview',
                style: Theme.of(context).textTheme.titleSmall,
              ),
              const SizedBox(height: 4),
              Text(
                'Width and colors update this window live.',
                style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 12),
              Theme(
                data: previewTheme,
                child: ChatSettingsPreview(displaySettings: displaySettings),
              ),
              const SizedBox(height: 28),
              Text(
                'Appearance',
                style: Theme.of(context).textTheme.titleMedium,
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<ThemeMode>(
                key: const Key('theme-mode'),
                decoration: const InputDecoration(labelText: 'Theme mode'),
                initialValue: appearanceSettings.themeMode,
                items: [
                  for (final mode in ThemeMode.values)
                    DropdownMenuItem(
                      value: mode,
                      child: Text(_themeModeLabel(mode)),
                    ),
                ],
                onChanged: (mode) {
                  if (mode != null) {
                    appearanceSettings.setThemeMode(mode);
                  }
                },
              ),
              const SizedBox(height: 24),
              for (final role in ChatColorRole.values)
                _RoleSection(
                  role: role,
                  brightness: brightness,
                  settings: appearanceSettings,
                  colors: colors.forRole(role),
                ),
            ],
          ),
        );
      },
    );
  }

  static Brightness _editBrightness(
    BuildContext context,
    AppearanceSettings settings,
  ) {
    return switch (settings.themeMode) {
      ThemeMode.light => Brightness.light,
      ThemeMode.dark => Brightness.dark,
      ThemeMode.system => MediaQuery.platformBrightnessOf(context),
    };
  }

  static String _themeModeLabel(ThemeMode mode) {
    return switch (mode) {
      ThemeMode.system => 'System',
      ThemeMode.light => 'Light',
      ThemeMode.dark => 'Dark',
    };
  }
}

/// Mini chat window: content column width + bubble colors from theme.
class ChatSettingsPreview extends StatelessWidget {
  const ChatSettingsPreview({super.key, required this.displaySettings});

  final ChatDisplaySettings displaySettings;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final chat = theme.extension<ChatColors>()!;
    final bandWidth = displaySettings.contentWidth.toDouble();

    return LayoutBuilder(
      builder: (context, constraints) {
        final pane = constraints.maxWidth;
        final scrollExtent = pane > bandWidth ? pane : bandWidth;

        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Container(
              key: const Key('content-width-preview'),
              decoration: BoxDecoration(
                color: scheme.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: scheme.outlineVariant),
              ),
              clipBehavior: Clip.antiAlias,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Container(
                    height: 28,
                    padding: const EdgeInsets.symmetric(horizontal: 10),
                    color: scheme.surfaceContainer,
                    child: Row(
                      children: [
                        _TrafficDot(scheme.error),
                        const SizedBox(width: 6),
                        _TrafficDot(scheme.tertiary),
                        const SizedBox(width: 6),
                        _TrafficDot(scheme.primary),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Text(
                            'Chat',
                            style: theme.textTheme.labelSmall?.copyWith(
                              color: scheme.onSurfaceVariant,
                            ),
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                      ],
                    ),
                  ),
                  SizedBox(
                    height: 140,
                    child: SingleChildScrollView(
                      scrollDirection: Axis.horizontal,
                      child: SizedBox(
                        width: scrollExtent,
                        height: 140,
                        child: Stack(
                          alignment: Alignment.center,
                          children: [
                            Positioned.fill(
                              child: CustomPaint(
                                painter: _GutterHatchPainter(
                                  color: scheme.outlineVariant.withValues(
                                    alpha: 0.35,
                                  ),
                                ),
                              ),
                            ),
                            SizedBox(
                              key: const Key('content-width-column'),
                              width: bandWidth,
                              child: Material(
                                color: scheme.surface,
                                child: Padding(
                                  padding: const EdgeInsets.all(10),
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.stretch,
                                    children: [
                                      Align(
                                        alignment: Alignment.centerRight,
                                        child: Container(
                                          width: bandWidth * 0.42,
                                          height: 14,
                                          decoration: BoxDecoration(
                                            color: chat.user.fill,
                                            borderRadius:
                                                BorderRadius.circular(6),
                                          ),
                                        ),
                                      ),
                                      const SizedBox(height: 8),
                                      Container(
                                        height: 22,
                                        padding: const EdgeInsets.symmetric(
                                          horizontal: 8,
                                        ),
                                        decoration: BoxDecoration(
                                          color: chat.thinking.fill,
                                          borderRadius:
                                              BorderRadius.circular(6),
                                          border: Border(
                                            left: BorderSide(
                                              color: chat.thinking.bar,
                                              width: 3,
                                            ),
                                          ),
                                        ),
                                        alignment: Alignment.centerLeft,
                                        child: Text(
                                          'Thinking',
                                          style: theme.textTheme.labelSmall
                                              ?.copyWith(
                                            fontWeight: FontWeight.w600,
                                          ),
                                        ),
                                      ),
                                      const SizedBox(height: 8),
                                      Container(
                                        height: 8,
                                        decoration: BoxDecoration(
                                          color: chat.answer.fill,
                                          borderRadius:
                                              BorderRadius.circular(4),
                                        ),
                                      ),
                                      const SizedBox(height: 4),
                                      Align(
                                        alignment: Alignment.centerLeft,
                                        child: Container(
                                          width: bandWidth * 0.55,
                                          height: 8,
                                          decoration: BoxDecoration(
                                            color: chat.answer.fill
                                                .withValues(alpha: 0.7),
                                            borderRadius:
                                                BorderRadius.circular(4),
                                          ),
                                        ),
                                      ),
                                      const SizedBox(height: 8),
                                      Align(
                                        alignment: Alignment.centerLeft,
                                        child: Container(
                                          padding: const EdgeInsets.symmetric(
                                            horizontal: 8,
                                            vertical: 4,
                                          ),
                                          decoration: BoxDecoration(
                                            color: chat.stats.fill,
                                            borderRadius:
                                                BorderRadius.circular(6),
                                          ),
                                          child: Text(
                                            'Stats',
                                            style: theme
                                                .textTheme.labelSmall
                                                ?.copyWith(
                                              color: chat.stats.bar,
                                              fontWeight: FontWeight.w600,
                                            ),
                                          ),
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Text('Content width', style: theme.textTheme.titleSmall),
            const SizedBox(height: 2),
            Text(
              'The band is ${displaySettings.contentWidth}px wide - drag either edge.',
              style: theme.textTheme.bodySmall?.copyWith(
                color: scheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 8),
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: SizedBox(
                width: scrollExtent,
                child: Center(
                  child: _TrueWidthBand(
                    key: const Key('content-width'),
                    width: bandWidth,
                    onWidthDelta: (dxFromLeftEdge, dxFromRightEdge) {
                      // Left: drag left (negative) grows; right: drag right grows.
                      final delta = dxFromRightEdge - dxFromLeftEdge;
                      displaySettings.setContentWidth(
                        (displaySettings.contentWidth + delta).round(),
                      );
                    },
                  ),
                ),
              ),
            ),
            const SizedBox(height: 6),
            Text(
              '${displaySettings.contentWidth}px',
              textAlign: TextAlign.center,
              style: theme.textTheme.bodySmall?.copyWith(
                color: scheme.onSurfaceVariant,
              ),
            ),
          ],
        );
      },
    );
  }
}

/// A band whose own width equals [width] CSS/logical pixels.
class _TrueWidthBand extends StatelessWidget {
  const _TrueWidthBand({
    super.key,
    required this.width,
    required this.onWidthDelta,
  });

  final double width;
  final void Function(double dxFromLeftEdge, double dxFromRightEdge)
      onWidthDelta;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      width: width,
      height: 36,
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: scheme.primary.withValues(alpha: 0.25),
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: scheme.primary),
        ),
        child: Row(
          children: [
            _EdgeHandle(
              key: const Key('content-width-left'),
              onDrag: (dx) => onWidthDelta(dx, 0),
            ),
            Expanded(
              child: Center(
                child: Text(
                  '${width.round()}px',
                  style: Theme.of(context).textTheme.labelMedium?.copyWith(
                    color: scheme.primary,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
            ),
            _EdgeHandle(
              key: const Key('content-width-right'),
              onDrag: (dx) => onWidthDelta(0, dx),
            ),
          ],
        ),
      ),
    );
  }
}

class _EdgeHandle extends StatelessWidget {
  const _EdgeHandle({super.key, required this.onDrag});

  final ValueChanged<double> onDrag;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onHorizontalDragUpdate: (details) => onDrag(details.delta.dx),
      child: SizedBox(
        width: 20,
        child: Center(
          child: Container(
            width: 4,
            height: 20,
            decoration: BoxDecoration(
              color: scheme.primary,
              borderRadius: BorderRadius.circular(2),
            ),
          ),
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

class _TrafficDot extends StatelessWidget {
  const _TrafficDot(this.color);

  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 8,
      height: 8,
      decoration: BoxDecoration(color: color, shape: BoxShape.circle),
    );
  }
}

class _GutterHatchPainter extends CustomPainter {
  _GutterHatchPainter({required this.color});

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1;
    const step = 8.0;
    for (var x = -size.height; x < size.width + size.height; x += step) {
      canvas.drawLine(
        Offset(x, 0),
        Offset(x + size.height, size.height),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(covariant _GutterHatchPainter oldDelegate) {
    return oldDelegate.color != color;
  }
}
