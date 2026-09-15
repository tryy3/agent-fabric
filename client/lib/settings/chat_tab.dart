import 'package:material_ui/material_ui.dart';

import '../chat/display_settings.dart';
import '../ui/theme/chat_colors.dart';

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
              const SizedBox(height: 24),
              Text(
                'Content width',
                style: Theme.of(context).textTheme.titleSmall,
              ),
              const SizedBox(height: 4),
              Text(
                'How wide the chat column is inside the window.',
                style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 12),
              _ContentWidthPreview(settings: settings),
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

/// Mini window whose centered content column (and the slider inside it)
/// scale with [ChatDisplaySettings.contentWidth].
class _ContentWidthPreview extends StatelessWidget {
  const _ContentWidthPreview({required this.settings});

  final ChatDisplaySettings settings;

  /// Logical desktop width the preview pane represents.
  static const double _desktopLogical = 1400;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final chat = theme.extension<ChatColors>();
    final userFill = chat?.user.fill ?? scheme.primaryContainer;
    final answerHint = chat?.answer.bar ?? scheme.primary;

    return LayoutBuilder(
      builder: (context, constraints) {
        final pane = constraints.maxWidth;
        final fraction =
            (settings.contentWidth / _desktopLogical).clamp(0.35, 0.92);
        final contentW = pane * fraction;

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
                    height: 132,
                    child: Stack(
                      alignment: Alignment.center,
                      children: [
                        // Side gutters hint at unused window space.
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
                          width: contentW,
                          child: Material(
                            color: scheme.surface,
                            elevation: 0,
                            child: Padding(
                              padding: const EdgeInsets.fromLTRB(10, 10, 10, 4),
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.stretch,
                                children: [
                                  Align(
                                    alignment: Alignment.centerRight,
                                    child: Container(
                                      width: contentW * 0.42,
                                      height: 14,
                                      decoration: BoxDecoration(
                                        color: userFill,
                                        borderRadius: BorderRadius.circular(6),
                                      ),
                                    ),
                                  ),
                                  const SizedBox(height: 8),
                                  Container(
                                    height: 8,
                                    decoration: BoxDecoration(
                                      color: answerHint.withValues(alpha: 0.35),
                                      borderRadius: BorderRadius.circular(4),
                                    ),
                                  ),
                                  const SizedBox(height: 4),
                                  Container(
                                    height: 8,
                                    width: contentW * 0.7,
                                    decoration: BoxDecoration(
                                      color: answerHint.withValues(alpha: 0.22),
                                      borderRadius: BorderRadius.circular(4),
                                    ),
                                  ),
                                  const Spacer(),
                                  SliderTheme(
                                    data: SliderTheme.of(context).copyWith(
                                      trackHeight: 3,
                                      overlayShape:
                                          const RoundSliderOverlayShape(
                                        overlayRadius: 12,
                                      ),
                                      thumbShape: const RoundSliderThumbShape(
                                        enabledThumbRadius: 7,
                                      ),
                                    ),
                                    child: Slider(
                                      key: const Key('content-width'),
                                      value: settings.contentWidth.toDouble(),
                                      min: ChatDisplaySettings
                                          .minContentWidth
                                          .toDouble(),
                                      max: ChatDisplaySettings
                                          .maxContentWidth
                                          .toDouble(),
                                      divisions:
                                          (ChatDisplaySettings
                                                      .maxContentWidth -
                                                  ChatDisplaySettings
                                                      .minContentWidth) ~/
                                              20,
                                      label: '${settings.contentWidth}px',
                                      onChanged: (v) =>
                                          settings.setContentWidth(v.round()),
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
                ],
              ),
            ),
            const SizedBox(height: 8),
            Text(
              '${settings.contentWidth}px · slider matches column width',
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
      canvas.drawLine(Offset(x, 0), Offset(x + size.height, size.height), paint);
    }
  }

  @override
  bool shouldRepaint(covariant _GutterHatchPainter oldDelegate) {
    return oldDelegate.color != color;
  }
}
