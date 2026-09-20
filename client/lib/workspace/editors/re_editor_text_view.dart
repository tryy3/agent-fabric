import 'package:re_editor/re_editor.dart';
import 'package:re_highlight/languages/css.dart';
import 'package:re_highlight/languages/javascript.dart';
import 'package:re_highlight/languages/json.dart';
import 'package:re_highlight/languages/plaintext.dart';
import 'package:re_highlight/languages/xml.dart';
import 'package:re_highlight/re_highlight.dart';
import 'package:re_highlight/styles/atom-one-dark.dart';
import 'package:material_ui/material_ui.dart';

import '../text_editor_session.dart';

/// The only file allowed to import package:re_editor / re_highlight.
class ReEditorTextView extends StatefulWidget {
  const ReEditorTextView({super.key, required this.session});

  final TextEditorSession session;

  @override
  State<ReEditorTextView> createState() => _ReEditorTextViewState();
}

class _ReEditorTextViewState extends State<ReEditorTextView> {
  late final CodeLineEditingController _controller;
  bool _applying = false;

  @override
  void initState() {
    super.initState();
    _controller = CodeLineEditingController.fromText(widget.session.text);
    _controller.addListener(_onEditor);
    widget.session.addListener(_onSession);
  }

  @override
  void didUpdateWidget(ReEditorTextView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.session != widget.session) {
      oldWidget.session.removeListener(_onSession);
      widget.session.addListener(_onSession);
      _syncFromSession();
    }
  }

  @override
  void dispose() {
    widget.session.removeListener(_onSession);
    _controller.removeListener(_onEditor);
    _controller.dispose();
    super.dispose();
  }

  void _onEditor() {
    if (_applying) {
      return;
    }
    widget.session.handleTextChanged(_controller.text);
  }

  void _onSession() {
    if (_controller.text == widget.session.text) {
      return;
    }
    _syncFromSession();
  }

  void _syncFromSession() {
    _applying = true;
    _controller.text = widget.session.text;
    _applying = false;
  }

  Mode _modeFor(String languageId) {
    switch (languageId) {
      case 'html':
        return langXml;
      case 'javascript':
        return langJavascript;
      case 'css':
        return langCss;
      case 'json':
        return langJson;
      default:
        return langPlaintext;
    }
  }

  @override
  Widget build(BuildContext context) {
    try {
      widget.session.text;
    } on FormatException {
      return const Center(child: Text('not valid text'));
    }
    final language = widget.session.languageId;
    return CodeEditor(
      controller: _controller,
      wordWrap: true,
      style: CodeEditorStyle(
        fontSize: 13,
        codeTheme: CodeHighlightTheme(
          languages: {
            language: CodeHighlightThemeMode(mode: _modeFor(language)),
          },
          theme: atomOneDarkTheme,
        ),
      ),
      indicatorBuilder:
          (context, editingController, chunkController, notifier) {
            return DefaultCodeLineNumber(
              controller: editingController,
              notifier: notifier,
            );
          },
    );
  }
}
