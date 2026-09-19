# Markdown Selection Copy (Visual + Newlines) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When copying a multi-block selection from rendered markdown in chat, put visual plain text on the clipboard with `\n\n` between selected pieces (not one run-on line; not raw `#` markdown).

**Architecture:** Keep `SelectionArea` + `MarkdownBody(selectable: false)`. Insert a local `SelectionTransformer.separated` (`SelectionContainer` that overrides `getSelectedContent`) between them so copy joins each selected selectable’s `plainText` with `\n\n`. Plain/Detailed `MessageText` stays unchanged.

**Tech Stack:** Flutter (Nix). Work from `/home/tryy3/src/agent-fabric`. Flutter is not on default PATH: prefix with `nix develop /home/tryy3/src/agent-fabric -c` and run from `client/`. Example: `nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/message_text_test.dart'`.

## Global Constraints

- Spec: [`docs/superpowers/specs/2026-09-19-markdown-selection-copy-design.md`](../specs/2026-09-19-markdown-selection-copy-design.md).
- Clipboard = **visual** text only; separator between selected pieces = `\n\n`.
- Markdown path of `MessageText` only; do not change plain mode.
- Never set `MarkdownBody.selectable: true` (breaks cross-block selection).
- No new pub dependencies; local helper under `client/lib/chat/`.
- Do not commit `controlplane/data/` or `controlplane/sandbox.json`.
- Follow TDD: failing test → implement → pass → commit per task.
- Existing link dialog / checkbox tests must keep passing.

## File Structure

| Path | Responsibility |
| --- | --- |
| `client/lib/chat/selection_transformer.dart` | Pure join helper + `SelectionTransformer` widget / delegate |
| `client/test/chat/selection_transformer_test.dart` | Unit tests for join; light widget smoke for transformer |
| `client/lib/chat/message_text.dart` | Wrap markdown body with `SelectionTransformer.separated` |
| `client/test/chat/message_text_test.dart` | Assert transformer present on markdown path; plain path unchanged |

**Interfaces this plan locks:**

```dart
/// Joins selected plain-text fragments for clipboard (spec separator).
String joinSelectedPlainTexts(
  Iterable<String> plainTexts, {
  String separator = '\n\n',
}) {
  return plainTexts.join(separator);
}

class SelectionTransformer extends StatefulWidget {
  const SelectionTransformer({
    super.key,
    required this.transform,
    required this.child,
  });

  /// Joins with [separator] (default `\n\n`).
  SelectionTransformer.separated({
    super.key,
    this.separator = '\n\n',
    required this.child,
  }) : transform = null; // implement via factory body that sets transform — see Task 1

  final String Function(Iterable<String> plainTexts) transform;
  final Widget child;
}
```

Prefer a clean constructor form in code:

```dart
SelectionTransformer.separated({
  super.key,
  String separator = '\n\n',
  required Widget child,
}) : this(
        key: key,
        transform: (texts) => joinSelectedPlainTexts(texts, separator: separator),
        child: child,
      );
```

(Use a redirecting constructor or initialize `transform` in the initializer list — pick whichever analyzes cleanly.)

---

### Task 1: Selection join helper + SelectionTransformer

**Files:**
- Create: `client/lib/chat/selection_transformer.dart`
- Create: `client/test/chat/selection_transformer_test.dart`
- Test: `client/test/chat/selection_transformer_test.dart`

**Interfaces:**
- Consumes: Flutter `SelectionContainer`, `MultiSelectableSelectionContainerDelegate`, `SelectedContent`
- Produces: `joinSelectedPlainTexts`, `SelectionTransformer`, `SelectionTransformer.separated`

- [ ] **Step 1: Write the failing unit tests for join**

Create `client/test/chat/selection_transformer_test.dart`:

```dart
import 'package:agent_fabric_client/chat/selection_transformer.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  group('joinSelectedPlainTexts', () {
    test('joins with double newline by default', () {
      expect(
        joinSelectedPlainTexts(['H1 test', 'H2 Test']),
        'H1 test\n\nH2 Test',
      );
    });

    test('single fragment unchanged', () {
      expect(joinSelectedPlainTexts(['only']), 'only');
    });

    test('empty iterable yields empty string', () {
      expect(joinSelectedPlainTexts(const <String>[]), '');
    });

    test('custom separator', () {
      expect(
        joinSelectedPlainTexts(['a', 'b'], separator: '\n'),
        'a\nb',
      );
    });
  });

  testWidgets('SelectionTransformer.separated builds SelectionContainer', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: SelectionArea(
          child: SelectionTransformer.separated(
            child: const Text('hello'),
          ),
        ),
      ),
    );
    expect(find.byType(SelectionTransformer), findsOneWidget);
    expect(find.byType(SelectionContainer), findsWidgets);
    expect(find.text('hello'), findsOneWidget);
  });
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/selection_transformer_test.dart'
```

Expected: FAIL (library / `joinSelectedPlainTexts` / `SelectionTransformer` not found).

- [ ] **Step 3: Implement join + SelectionTransformer**

Create `client/lib/chat/selection_transformer.dart` with the full file below (adapted from the community SelectionTransformer gist; no `package:collection`; no tabular/markdownWidget factories).

```dart
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
      transform: (texts) =>
          joinSelectedPlainTexts(texts, separator: separator),
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
    return SelectionContainer(
      delegate: _delegate,
      child: widget.child,
    );
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
    if (selectable is State<StatefulWidget>) {
      final Widget maybeContainer = selectable.widget;
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
```

If analysis fails on `selectable is State<StatefulWidget>` / `SelectionContainer.delegate` access, adjust `_allSelectables` to match the gist’s cast style until `dart analyze` is clean — behavior must still flatten nested multi-selectable containers.
- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/selection_transformer_test.dart'
```

Expected: All tests PASS. If analyze complains about private types or `Selectable` casting, fix types until clean.

- [ ] **Step 5: Commit**

```bash
git add client/lib/chat/selection_transformer.dart client/test/chat/selection_transformer_test.dart
git commit -m "$(cat <<'EOF'
Add SelectionTransformer to join copied selection with newlines.

EOF
)"
```

---

### Task 2: Wire MessageText markdown path

**Files:**
- Modify: `client/lib/chat/message_text.dart`
- Modify: `client/test/chat/message_text_test.dart`
- Test: `client/test/chat/message_text_test.dart`

**Interfaces:**
- Consumes: `SelectionTransformer.separated` from Task 1
- Produces: Markdown `MessageText` tree `SelectionArea` → `SelectionTransformer.separated` → `MarkdownBody(selectable: false)`

- [ ] **Step 1: Write the failing MessageText wiring tests**

In `client/test/chat/message_text_test.dart`:

1. Add import:

```dart
import 'package:agent_fabric_client/chat/selection_transformer.dart';
```

2. Update `markdown mode uses SelectionArea with non-selectable body` to also require the transformer:

```dart
  testWidgets('markdown mode uses SelectionArea with non-selectable body', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: MessageText(
          text: '# Hello\n\nhi\n\nhow are you',
          markdown: true,
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byType(SelectionArea), findsOneWidget);
    expect(find.byType(SelectionTransformer), findsOneWidget);
    final body = tester.widget<MarkdownBody>(find.byType(MarkdownBody));
    expect(body.selectable, isFalse);
    expect(find.byType(SelectableText), findsNothing);
    expect(find.textContaining('Hello'), findsOneWidget);
    expect(find.textContaining('hi'), findsOneWidget);
    expect(find.textContaining('how are you'), findsOneWidget);
  });
```

3. Update plain-mode test so it does **not** introduce `SelectionTransformer`:

```dart
  testWidgets('plain mode uses selectable Text under SelectionArea', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(home: MessageText(text: '**x**', markdown: false)),
    );
    expect(find.byType(SelectionArea), findsOneWidget);
    expect(find.byType(SelectionTransformer), findsNothing);
    expect(find.byType(Text), findsWidgets);
    expect(find.byType(MarkdownBody), findsNothing);
    expect(find.byType(SelectableText), findsNothing);
  });
```

- [ ] **Step 2: Run MessageText tests to verify the new assertion fails**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/message_text_test.dart'
```

Expected: FAIL on markdown test — `SelectionTransformer` not found (0 widgets).

- [ ] **Step 3: Wrap MarkdownBody with SelectionTransformer.separated**

In `client/lib/chat/message_text.dart`:

1. Add:

```dart
import 'selection_transformer.dart';
```

2. In the markdown branch only, wrap the existing `MarkdownBody(...)` with `SelectionTransformer.separated` — do not change `checkboxBuilder`, `styleSheet`, `onTapLink`, or `selectable: false`. Diff shape:

```dart
    return SelectionArea(
      child: SelectionTransformer.separated(
        child: MarkdownBody(
          // existing MarkdownBody arguments unchanged
        ),
      ),
    );
```

Leave the plain branch as `SelectionArea(child: Text(text))`.
- [ ] **Step 4: Run MessageText tests to verify they pass**

Run:

```bash
nix develop /home/tryy3/src/agent-fabric -c bash -c 'cd client && flutter test test/chat/message_text_test.dart'
```

Expected: All tests PASS (including link + checkbox cases).

- [ ] **Step 5: Manual check (optional but recommended)**

Run the client, Pretty mode, select a heading and the next paragraph, copy, paste into an editor. Expect a blank line between the two visual strings.

- [ ] **Step 6: Commit**

```bash
git add client/lib/chat/message_text.dart client/test/chat/message_text_test.dart
git commit -m "$(cat <<'EOF'
Preserve newlines when copying rendered markdown selections.

EOF
)"
```

---

## Spec coverage (self-review)

| Spec item | Task |
| --- | --- |
| Visual clipboard text | Task 1 transform / Task 2 wiring |
| `\n\n` between selected pieces | Task 1 `joinSelectedPlainTexts` default |
| `SelectionArea` → transform → `MarkdownBody(selectable: false)` | Task 2 |
| Plain path unchanged | Task 2 assertion `SelectionTransformer` absent |
| No raw markdown | Non-goal; not implemented |
| Unit-test join | Task 1 |
| Widget assert helper present | Task 2 |
| Links still work | Task 2 reuses existing link tests |

No TBD/TODO left in steps. Types: `joinSelectedPlainTexts` / `SelectionTransformer.separated` names match across tasks.
