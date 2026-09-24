import 'package:material_ui/material_ui.dart';

import 'operator_failure.dart';

/// Single load state for Settings lists — never a loading+error+data triple.
sealed class SettingsLoadState<T> {
  const SettingsLoadState();
}

final class SettingsLoading<T> extends SettingsLoadState<T> {
  const SettingsLoading();
}

final class SettingsFailed<T> extends SettingsLoadState<T> {
  const SettingsFailed(this.failure);

  final OperatorFailure failure;
}

final class SettingsReady<T> extends SettingsLoadState<T> {
  const SettingsReady(this.items);

  final List<T> items;
}

/// Renders loading / empty / error / content from one sealed state.
class SettingsLoadBody<T> extends StatelessWidget {
  const SettingsLoadBody({
    super.key,
    required this.state,
    required this.itemBuilder,
    required this.emptyLabel,
    required this.onRetry,
  });

  final SettingsLoadState<T> state;
  final Widget Function(BuildContext context, T item) itemBuilder;
  final String emptyLabel;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return switch (state) {
      SettingsLoading() => const Center(child: CircularProgressIndicator()),
      SettingsFailed(:final failure) => Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(operatorMessageFor(failure), textAlign: TextAlign.center),
              const SizedBox(height: 16),
              Semantics(
                button: true,
                label: 'Retry',
                child: FilledButton(
                  onPressed: onRetry,
                  child: const Text('Retry'),
                ),
              ),
            ],
          ),
        ),
      ),
      SettingsReady(:final items) when items.isEmpty => Center(
        child: Text(emptyLabel),
      ),
      SettingsReady(:final items) => ListView.builder(
        itemCount: items.length,
        itemBuilder: (context, index) => itemBuilder(context, items[index]),
      ),
    };
  }
}
