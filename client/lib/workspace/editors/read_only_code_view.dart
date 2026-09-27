import 'package:re_editor/re_editor.dart';
import 'package:re_highlight/languages/json.dart';
import 'package:re_highlight/languages/plaintext.dart';
import 'package:re_highlight/re_highlight.dart';
import 'package:re_highlight/styles/atom-one-dark.dart';
import 'package:material_ui/material_ui.dart';

import '../../ui/theme/app_theme.dart';

/// Read-only [CodeEditor] with line numbers and fold markers (JSON braces/brackets).
///
/// Kept next to [ReEditorTextView] so `package:re_editor` stays confined to
/// `workspace/editors/`.
class ReadOnlyCodeView extends StatefulWidget {
  const ReadOnlyCodeView({
    super.key,
    required this.text,
    this.languageId = 'json',
    this.wordWrap = true,
  });

  final String text;
  final String languageId;
  final bool wordWrap;

  @override
  State<ReadOnlyCodeView> createState() => _ReadOnlyCodeViewState();
}

class _ReadOnlyCodeViewState extends State<ReadOnlyCodeView> {
  late CodeLineEditingController _controller;

  @override
  void initState() {
    super.initState();
    _controller = CodeLineEditingController.fromText(widget.text);
  }

  @override
  void didUpdateWidget(covariant ReadOnlyCodeView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.text != widget.text) {
      _controller.text = widget.text;
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Mode _modeFor(String languageId) {
    switch (languageId) {
      case 'json':
        return langJson;
      default:
        return langPlaintext;
    }
  }

  @override
  Widget build(BuildContext context) {
    final language = widget.languageId;
    return CodeEditor(
      controller: _controller,
      readOnly: true,
      showCursorWhenReadOnly: false,
      wordWrap: widget.wordWrap,
      chunkAnalyzer: const DefaultCodeChunkAnalyzer(),
      style: CodeEditorStyle(
        fontSize: 13,
        fontFamily: AppTheme.monoFontFamily,
        fontFamilyFallback: const [AppTheme.fontFamily],
        codeTheme: CodeHighlightTheme(
          languages: {
            language: CodeHighlightThemeMode(mode: _modeFor(language)),
          },
          theme: atomOneDarkTheme,
        ),
      ),
      indicatorBuilder:
          (context, editingController, chunkController, notifier) {
            return Row(
              children: [
                DefaultCodeLineNumber(
                  controller: editingController,
                  notifier: notifier,
                ),
                DefaultCodeChunkIndicator(
                  width: 20,
                  controller: chunkController,
                  notifier: notifier,
                ),
              ],
            );
          },
    );
  }
}
