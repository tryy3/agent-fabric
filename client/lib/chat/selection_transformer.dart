import 'package:flutter/rendering.dart';
import 'package:material_ui/material_ui.dart';

/// Joins selected plain-text fragments for the clipboard.
String joinSelectedPlainTexts(
  Iterable<String> plainTexts, {
  String separator = '\n\n',
}) {
  return plainTexts.join(separator);
}

typedef SelectionTransform = String Function(Iterable<String> plainTexts);

/// Transforms [SelectedContent.plainText] when copying under a [SelectionArea].
class SelectionTransformer extends StatefulWidget {
  const SelectionTransformer({
    super.key,
    required this.transform,
    required this.child,
  });

  factory SelectionTransformer.separated({
    Key? key,
    String separator = '\n\n',
    required Widget child,
  }) {
    return SelectionTransformer(
      key: key,
      transform: (texts) => joinSelectedPlainTexts(texts, separator: separator),
      child: child,
    );
  }

  final SelectionTransform transform;
  final Widget child;

  @override
  State<SelectionTransformer> createState() => _SelectionTransformerState();
}

class _SelectionTransformerState extends State<SelectionTransformer> {
  late final _SeparatedSelectionContainerDelegate _delegate =
      _SeparatedSelectionContainerDelegate(widget.transform);

  @override
  void didUpdateWidget(covariant SelectionTransformer oldWidget) {
    super.didUpdateWidget(oldWidget);
    _delegate.transform = widget.transform;
  }

  @override
  void dispose() {
    _delegate.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SelectionContainer(delegate: _delegate, child: widget.child);
  }
}

class _SeparatedSelectionContainerDelegate
    extends MultiSelectableSelectionContainerDelegate {
  _SeparatedSelectionContainerDelegate(this.transform);

  SelectionTransform transform;

  @override
  SelectedContent? getSelectedContent() {
    final List<SelectedContent> selections = <SelectedContent>[];
    for (final Selectable selectable in selectables) {
      selections.addAll(
        _allSelectables(selectable)
            .map((Selectable s) => s.getSelectedContent())
            .nonNulls,
      );
    }
    if (selections.isEmpty) {
      return null;
    }
    return SelectedContent(
      plainText: transform(selections.map((SelectedContent s) => s.plainText)),
    );
  }

  List<Selectable> _allSelectables(Selectable selectable) {
    final List<Selectable> result = <Selectable>[];
    if (selectable case final State<StatefulWidget> state) {
      final Widget maybeContainer = state.widget;
      if (maybeContainer is SelectionContainer) {
        final SelectionContainerDelegate? nested = maybeContainer.delegate;
        if (nested is MultiSelectableSelectionContainerDelegate) {
          for (final Selectable child in nested.selectables) {
            result.addAll(_allSelectables(child));
          }
          return result;
        }
      }
    }
    result.add(selectable);
    return result;
  }

  // Edge-update bookkeeping (from Flutter SelectableRegion / community gist).
  final Set<Selectable> _hasReceivedStartEvent = <Selectable>{};
  final Set<Selectable> _hasReceivedEndEvent = <Selectable>{};
  Offset? _lastStartEdgeUpdateGlobalPosition;
  Offset? _lastEndEdgeUpdateGlobalPosition;

  @override
  void remove(Selectable selectable) {
    _hasReceivedStartEvent.remove(selectable);
    _hasReceivedEndEvent.remove(selectable);
    super.remove(selectable);
  }

  void _updateLastEdgeEventsFromGeometries() {
    if (currentSelectionStartIndex != -1) {
      final Selectable start = selectables[currentSelectionStartIndex];
      final Offset localStartEdge =
          start.value.startSelectionPoint!.localPosition +
          Offset(0, -start.value.startSelectionPoint!.lineHeight / 2);
      _lastStartEdgeUpdateGlobalPosition = MatrixUtils.transformPoint(
        start.getTransformTo(null),
        localStartEdge,
      );
    }
    if (currentSelectionEndIndex != -1) {
      final Selectable end = selectables[currentSelectionEndIndex];
      final Offset localEndEdge =
          end.value.endSelectionPoint!.localPosition +
          Offset(0, -end.value.endSelectionPoint!.lineHeight / 2);
      _lastEndEdgeUpdateGlobalPosition = MatrixUtils.transformPoint(
        end.getTransformTo(null),
        localEndEdge,
      );
    }
  }

  @override
  SelectionResult handleSelectAll(SelectAllSelectionEvent event) {
    final SelectionResult result = super.handleSelectAll(event);
    for (final Selectable selectable in selectables) {
      _hasReceivedStartEvent.add(selectable);
      _hasReceivedEndEvent.add(selectable);
    }
    _updateLastEdgeEventsFromGeometries();
    return result;
  }

  @override
  SelectionResult handleSelectWord(SelectWordSelectionEvent event) {
    final SelectionResult result = super.handleSelectWord(event);
    if (currentSelectionStartIndex != -1) {
      _hasReceivedStartEvent.add(selectables[currentSelectionStartIndex]);
    }
    if (currentSelectionEndIndex != -1) {
      _hasReceivedEndEvent.add(selectables[currentSelectionEndIndex]);
    }
    _updateLastEdgeEventsFromGeometries();
    return result;
  }

  @override
  SelectionResult handleClearSelection(ClearSelectionEvent event) {
    final SelectionResult result = super.handleClearSelection(event);
    _hasReceivedStartEvent.clear();
    _hasReceivedEndEvent.clear();
    _lastStartEdgeUpdateGlobalPosition = null;
    _lastEndEdgeUpdateGlobalPosition = null;
    return result;
  }

  @override
  SelectionResult handleSelectionEdgeUpdate(SelectionEdgeUpdateEvent event) {
    if (event.type == SelectionEventType.endEdgeUpdate) {
      _lastEndEdgeUpdateGlobalPosition = event.globalPosition;
    } else {
      _lastStartEdgeUpdateGlobalPosition = event.globalPosition;
    }
    return super.handleSelectionEdgeUpdate(event);
  }

  @override
  void dispose() {
    _hasReceivedStartEvent.clear();
    _hasReceivedEndEvent.clear();
    super.dispose();
  }

  @override
  SelectionResult dispatchSelectionEventToChild(
    Selectable selectable,
    SelectionEvent event,
  ) {
    switch (event.type) {
      case SelectionEventType.startEdgeUpdate:
        _hasReceivedStartEvent.add(selectable);
        ensureChildUpdated(selectable);
      case SelectionEventType.endEdgeUpdate:
        _hasReceivedEndEvent.add(selectable);
        ensureChildUpdated(selectable);
      case SelectionEventType.clear:
        _hasReceivedStartEvent.remove(selectable);
        _hasReceivedEndEvent.remove(selectable);
      case SelectionEventType.selectAll:
      case SelectionEventType.selectWord:
      case SelectionEventType.selectParagraph:
        break;
      case SelectionEventType.granularlyExtendSelection:
      case SelectionEventType.directionallyExtendSelection:
        _hasReceivedStartEvent.add(selectable);
        _hasReceivedEndEvent.add(selectable);
        ensureChildUpdated(selectable);
    }
    return super.dispatchSelectionEventToChild(selectable, event);
  }

  @override
  void ensureChildUpdated(Selectable selectable) {
    if (_lastEndEdgeUpdateGlobalPosition != null &&
        _hasReceivedEndEvent.add(selectable)) {
      final SelectionEdgeUpdateEvent synthesizedEvent =
          SelectionEdgeUpdateEvent.forEnd(
            globalPosition: _lastEndEdgeUpdateGlobalPosition!,
          );
      if (currentSelectionEndIndex == -1) {
        handleSelectionEdgeUpdate(synthesizedEvent);
      }
      selectable.dispatchSelectionEvent(synthesizedEvent);
    }
    if (_lastStartEdgeUpdateGlobalPosition != null &&
        _hasReceivedStartEvent.add(selectable)) {
      final SelectionEdgeUpdateEvent synthesizedEvent =
          SelectionEdgeUpdateEvent.forStart(
            globalPosition: _lastStartEdgeUpdateGlobalPosition!,
          );
      if (currentSelectionStartIndex == -1) {
        handleSelectionEdgeUpdate(synthesizedEvent);
      }
      selectable.dispatchSelectionEvent(synthesizedEvent);
    }
  }

  @override
  void didChangeSelectables() {
    if (_lastEndEdgeUpdateGlobalPosition != null) {
      handleSelectionEdgeUpdate(
        SelectionEdgeUpdateEvent.forEnd(
          globalPosition: _lastEndEdgeUpdateGlobalPosition!,
        ),
      );
    }
    if (_lastStartEdgeUpdateGlobalPosition != null) {
      handleSelectionEdgeUpdate(
        SelectionEdgeUpdateEvent.forStart(
          globalPosition: _lastStartEdgeUpdateGlobalPosition!,
        ),
      );
    }
    final Set<Selectable> selectableSet = selectables.toSet();
    _hasReceivedEndEvent.removeWhere(
      (Selectable selectable) => !selectableSet.contains(selectable),
    );
    _hasReceivedStartEvent.removeWhere(
      (Selectable selectable) => !selectableSet.contains(selectable),
    );
    super.didChangeSelectables();
  }
}
