import 'package:material_ui/material_ui.dart';

import 'chat_controller.dart';
import 'model_picker_grouping.dart';

String _currentLabel(ChatController controller) {
  final id = controller.currentModel;
  if (id == null) return 'Model';
  for (final m in controller.modelOptions) {
    if (m.id == id) return m.name;
  }
  return id;
}

class ModelPicker extends StatefulWidget {
  const ModelPicker({super.key, required this.controller});

  final ChatController controller;

  @override
  State<ModelPicker> createState() => _ModelPickerState();
}

class _ModelPickerState extends State<ModelPicker> {
  final _layerLink = LayerLink();
  final _portalController = OverlayPortalController();

  void _toggle({required bool enabled}) {
    if (!enabled) return;
    setState(() {
      if (_portalController.isShowing) {
        _portalController.hide();
      } else {
        _portalController.show();
      }
    });
  }

  void _close() {
    if (!_portalController.isShowing) return;
    setState(_portalController.hide);
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final enabled =
            widget.controller.canSelectModel &&
            widget.controller.modelOptions.isNotEmpty;
        final label = _currentLabel(widget.controller);

        return OverlayPortal(
          controller: _portalController,
          overlayChildBuilder: (context) {
            return Stack(
              children: [
                Positioned.fill(
                  child: GestureDetector(
                    behavior: HitTestBehavior.translucent,
                    onTap: _close,
                  ),
                ),
                CompositedTransformFollower(
                  link: _layerLink,
                  // Composer sits at the bottom of the chat; open upward.
                  targetAnchor: Alignment.topLeft,
                  followerAnchor: Alignment.bottomLeft,
                  offset: const Offset(0, -4),
                  child: Builder(
                    builder: (context) {
                      final scheme = Theme.of(context).colorScheme;
                      return Material(
                        elevation: 8,
                        shadowColor: Colors.black.withValues(alpha: 0.55),
                        surfaceTintColor: Colors.transparent,
                        color: scheme.surfaceContainerHigh,
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(8),
                          side: BorderSide(color: scheme.outlineVariant),
                        ),
                        clipBehavior: Clip.antiAlias,
                        child: _ModelPickerPanel(
                          chatController: widget.controller,
                          onClose: _close,
                        ),
                      );
                    },
                  ),
                ),
              ],
            );
          },
          child: CompositedTransformTarget(
            link: _layerLink,
            child: InkWell(
              key: const Key('model-picker'),
              onTap: enabled ? () => _toggle(enabled: enabled) : null,
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      label,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodyMedium,
                    ),
                  ),
                  Icon(
                    _portalController.isShowing
                        ? Icons.expand_less
                        : Icons.expand_more,
                    size: 18,
                  ),
                ],
              ),
            ),
          ),
        );
      },
    );
  }
}

class _ModelPickerPanel extends StatefulWidget {
  const _ModelPickerPanel({
    required this.chatController,
    required this.onClose,
  });

  final ChatController chatController;
  final VoidCallback onClose;

  @override
  State<_ModelPickerPanel> createState() => _ModelPickerPanelState();
}

class _ModelPickerPanelState extends State<_ModelPickerPanel> {
  String _query = '';
  final Set<String> _collapsedGroupKeys = {};

  String _groupKey(ModelProviderGroup g) => g.providerId ?? g.providerName;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.chatController,
      builder: (context, _) {
        final groups = filterModelGroups(
          groups: groupModelsByProvider(
            models: widget.chatController.modelOptions,
            providers: widget.chatController.providers,
          ),
          query: _query,
        );
        final current = widget.chatController.currentModel;
        final scheme = Theme.of(context).colorScheme;
        final groupStyle = Theme.of(context).textTheme.labelLarge?.copyWith(
          color: scheme.onSurfaceVariant,
          fontWeight: FontWeight.w600,
        );

        return SizedBox(
          width: 320,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              // Reserved for future favorites.
              const SizedBox.shrink(),
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 12, 12, 10),
                child: TextField(
                  key: const Key('model-picker-search'),
                  style: Theme.of(context).textTheme.bodyMedium,
                  decoration: InputDecoration(
                    hintText: 'Search models…',
                    isDense: true,
                    filled: true,
                    fillColor: scheme.surfaceContainerHighest,
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 10,
                    ),
                    prefixIcon: Icon(
                      Icons.search,
                      size: 18,
                      color: scheme.onSurfaceVariant,
                    ),
                    prefixIconConstraints: const BoxConstraints(
                      minWidth: 40,
                      minHeight: 36,
                    ),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: BorderSide(color: scheme.outlineVariant),
                    ),
                    enabledBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: BorderSide(color: scheme.outlineVariant),
                    ),
                    focusedBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: BorderSide(color: scheme.primary),
                    ),
                    suffixIcon: _query.isEmpty
                        ? null
                        : IconButton(
                            icon: const Icon(Icons.clear, size: 18),
                            onPressed: () => setState(() => _query = ''),
                          ),
                  ),
                  onChanged: (value) => setState(() => _query = value),
                ),
              ),
              ConstrainedBox(
                constraints: const BoxConstraints(maxHeight: 340),
                child: ListView(
                  shrinkWrap: true,
                  padding: const EdgeInsets.only(bottom: 8),
                  children: [
                    for (final g in groups) ...[
                      Material(
                        color: scheme.surfaceContainerHighest,
                        child: InkWell(
                          onTap: () {
                            setState(() {
                              final key = _groupKey(g);
                              if (_collapsedGroupKeys.contains(key)) {
                                _collapsedGroupKeys.remove(key);
                              } else {
                                _collapsedGroupKeys.add(key);
                              }
                            });
                          },
                          child: Padding(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 12,
                              vertical: 8,
                            ),
                            child: Row(
                              children: [
                                Icon(
                                  _collapsedGroupKeys.contains(_groupKey(g))
                                      ? Icons.arrow_right
                                      : Icons.arrow_drop_down,
                                  size: 20,
                                  color: scheme.onSurfaceVariant,
                                ),
                                Text(g.providerName, style: groupStyle),
                                Text(
                                  ' (${g.models.length})',
                                  style: groupStyle,
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                      if (!_collapsedGroupKeys.contains(_groupKey(g)))
                        for (final model in g.models)
                          InkWell(
                            onTap: () async {
                              await widget.chatController.selectModel(model.id);
                              widget.onClose();
                            },
                            child: Padding(
                              padding: const EdgeInsets.only(
                                left: 36,
                                right: 12,
                                top: 6,
                                bottom: 6,
                              ),
                              child: Row(
                                children: [
                                  Expanded(
                                    child: Text(
                                      model.name,
                                      style: Theme.of(
                                        context,
                                      ).textTheme.bodyMedium,
                                    ),
                                  ),
                                  if (model.id == current)
                                    Icon(
                                      Icons.check,
                                      size: 18,
                                      color: scheme.primary,
                                    ),
                                ],
                              ),
                            ),
                          ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
