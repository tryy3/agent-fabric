import 'package:material_ui/material_ui.dart';

/// Optional numeric inference param: slider + editable field + tooltip.
///
/// Empty [controller] text means "provider default" (omit from PATCH). Moving
/// the slider or typing a value commits an override; clear resets to default.
class InferenceParamRow extends StatelessWidget {
  const InferenceParamRow({
    super.key,
    required this.fieldKey,
    required this.label,
    required this.tooltip,
    required this.controller,
    required this.min,
    required this.max,
    required this.unsetDisplay,
    required this.onChanged,
    this.divisions,
    this.integer = false,
    this.displaySuffix,
  });

  final Key fieldKey;
  final String label;
  final String tooltip;
  final TextEditingController controller;
  final double min;
  final double max;
  final double unsetDisplay;
  final VoidCallback onChanged;
  final int? divisions;
  final bool integer;
  final String? displaySuffix;

  bool get _isSet => controller.text.trim().isNotEmpty;

  double get _sliderValue {
    final raw = controller.text.trim();
    if (raw.isEmpty) {
      return unsetDisplay.clamp(min, max);
    }
    final parsed = integer
        ? int.tryParse(raw)?.toDouble()
        : double.tryParse(raw);
    if (parsed == null) {
      return unsetDisplay.clamp(min, max);
    }
    return parsed.clamp(min, max);
  }

  String _format(double value) {
    if (integer) {
      return value.round().toString();
    }
    return value.toStringAsFixed(2).replaceAll(RegExp(r'\.?0+$'), '');
  }

  void _commit(double value) {
    final next = _format(value);
    if (controller.text == next) {
      onChanged();
      return;
    }
    controller.text = next;
    onChanged();
  }

  void _clear() {
    if (controller.text.isEmpty) {
      return;
    }
    controller.clear();
    onChanged();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.only(top: 8, bottom: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Expanded(
                child: Row(
                  children: [
                    Flexible(
                      child: Text(label, style: theme.textTheme.labelLarge),
                    ),
                    const SizedBox(width: 4),
                    Tooltip(
                      message: tooltip,
                      child: Icon(
                        Icons.info_outline,
                        size: 16,
                        color: theme.colorScheme.onSurfaceVariant,
                      ),
                    ),
                  ],
                ),
              ),
              SizedBox(
                width: 88,
                child: TextField(
                  key: fieldKey,
                  controller: controller,
                  textAlign: TextAlign.end,
                  decoration: InputDecoration(
                    isDense: true,
                    hintText: 'Default',
                    suffixText: displaySuffix,
                    border: const OutlineInputBorder(),
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 8,
                    ),
                  ),
                  keyboardType: TextInputType.numberWithOptions(
                    decimal: !integer,
                  ),
                  onChanged: (_) => onChanged(),
                ),
              ),
              if (_isSet)
                IconButton(
                  tooltip: 'Use provider default',
                  icon: const Icon(Icons.clear, size: 18),
                  visualDensity: VisualDensity.compact,
                  onPressed: _clear,
                ),
            ],
          ),
          Slider(
            value: _sliderValue,
            min: min,
            max: max,
            divisions: divisions,
            label: _isSet ? _format(_sliderValue) : 'Default',
            onChanged: _commit,
          ),
        ],
      ),
    );
  }
}

/// Label row with an info tooltip (for non-slider controls).
class InferenceLabeledControl extends StatelessWidget {
  const InferenceLabeledControl({
    super.key,
    required this.label,
    required this.tooltip,
    required this.child,
  });

  final String label;
  final String tooltip;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.only(top: 8, bottom: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Text(label, style: theme.textTheme.labelLarge),
              const SizedBox(width: 4),
              Tooltip(
                message: tooltip,
                child: Icon(
                  Icons.info_outline,
                  size: 16,
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
          child,
        ],
      ),
    );
  }
}
