import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';
import 'ask_user_question.dart';
import 'pending_interaction.dart';

export 'ask_user_question.dart';

/// Calm clarification dock shown above the chat composer.
class AskUserDock extends StatefulWidget {
  const AskUserDock({
    super.key,
    required this.pending,
    required this.onSubmit,
    required this.onSkip,
  });

  final PendingAskUser pending;
  final ValueChanged<Map<String, Object?>> onSubmit;
  final VoidCallback onSkip;

  @override
  State<AskUserDock> createState() => _AskUserDockState();
}

class _AskUserDockState extends State<AskUserDock> {
  late final Map<String, String> _selected;
  late final Map<String, TextEditingController> _otherText;

  @override
  void initState() {
    super.initState();
    _selected = {
      for (final q in widget.pending.questions)
        q.id: q.options.isNotEmpty ? q.options.first : '',
    };
    _otherText = {
      for (final q in widget.pending.questions) q.id: TextEditingController(),
    };
  }

  @override
  void dispose() {
    for (final c in _otherText.values) {
      c.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final message = widget.pending.message;
    return Semantics(
      container: true,
      label: 'Clarification questions from the agent',
      child: Material(
        color: tokens.surfaceRaised,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
          side: BorderSide(color: tokens.border),
        ),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(12, 10, 12, 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                message.isEmpty ? 'Quick question' : message,
                style: TextStyle(
                  color: tokens.textPrimary,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 10),
              for (final q in widget.pending.questions) ...[
                Text(
                  q.question,
                  style: TextStyle(
                    color: tokens.textPrimary,
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    for (final opt in [...q.options, 'Other'])
                      ChoiceChip(
                        label: Text(opt),
                        selected: _selected[q.id] == opt,
                        onSelected: (_) {
                          setState(() => _selected[q.id] = opt);
                        },
                        selectedColor: tokens.primaryMuted,
                        labelStyle: TextStyle(
                          color: tokens.textPrimary,
                          fontSize: 12,
                        ),
                        side: BorderSide(color: tokens.border),
                        backgroundColor: tokens.surface,
                      ),
                  ],
                ),
                if (_selected[q.id] == 'Other') ...[
                  const SizedBox(height: 8),
                  TextField(
                    controller: _otherText[q.id],
                    style: TextStyle(color: tokens.textPrimary, fontSize: 13),
                    decoration: InputDecoration(
                      hintText: 'Type your answer',
                      hintStyle: TextStyle(color: tokens.textMuted),
                      filled: true,
                      fillColor: tokens.surface,
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(
                          DesignTokens.radiusSm,
                        ),
                      ),
                    ),
                  ),
                ],
                const SizedBox(height: 12),
              ],
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  TextButton(
                    onPressed: widget.onSkip,
                    child: Text(
                      'Skip',
                      style: TextStyle(color: tokens.textSecondary),
                    ),
                  ),
                  const SizedBox(width: 8),
                  FilledButton(
                    style: FilledButton.styleFrom(
                      backgroundColor: tokens.primary,
                      foregroundColor: tokens.background,
                    ),
                    onPressed: () {
                      final content = <String, Object?>{};
                      for (final q in widget.pending.questions) {
                        final choice = _selected[q.id] ?? '';
                        content[q.id] = choice;
                        if (choice == 'Other') {
                          content['${q.id}_other'] = _otherText[q.id]!.text
                              .trim();
                        }
                      }
                      widget.onSubmit(content);
                    },
                    child: const Text('Submit'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
