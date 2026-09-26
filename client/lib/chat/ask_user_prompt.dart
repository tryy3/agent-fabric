import 'package:material_ui/material_ui.dart';

import '../ui/theme/design_tokens.dart';

/// One multiple-choice question from an ask_user elicitation.
class AskUserQuestion {
  const AskUserQuestion({
    required this.id,
    required this.question,
    required this.options,
  });

  final String id;
  final String question;
  final List<String> options;
}

/// Calm clarification prompt for ask_user (ACP elicitation/create).
Future<Map<String, Object?>?> showAskUserPrompt(
  BuildContext context, {
  required String message,
  required List<AskUserQuestion> questions,
}) async {
  final tokens = designTokensOf(context);
  final selected = <String, String>{
    for (final q in questions)
      q.id: q.options.isNotEmpty ? q.options.first : '',
  };
  final otherText = <String, TextEditingController>{
    for (final q in questions) q.id: TextEditingController(),
  };

  try {
    return await showDialog<Map<String, Object?>>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) {
        return StatefulBuilder(
          builder: (ctx, setState) {
            return AlertDialog(
              backgroundColor: tokens.surfaceRaised,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(DesignTokens.radiusMd),
                side: BorderSide(color: tokens.border),
              ),
              title: Text(
                message.isEmpty ? 'Quick question' : message,
                style: TextStyle(
                  color: tokens.textPrimary,
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              content: Semantics(
                container: true,
                label: 'Clarification questions from the agent',
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 440),
                  child: SingleChildScrollView(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        for (final q in questions) ...[
                          Text(
                            q.question,
                            style: TextStyle(
                              color: tokens.textPrimary,
                              fontSize: 14,
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
                                  selected: selected[q.id] == opt,
                                  onSelected: (_) {
                                    setState(() => selected[q.id] = opt);
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
                          if (selected[q.id] == 'Other') ...[
                            const SizedBox(height: 8),
                            TextField(
                              controller: otherText[q.id],
                              style: TextStyle(
                                color: tokens.textPrimary,
                                fontSize: 13,
                              ),
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
                          const SizedBox(height: 16),
                        ],
                      ],
                    ),
                  ),
                ),
              ),
              actions: [
                TextButton(
                  onPressed: () => Navigator.of(ctx).pop(),
                  child: Text(
                    'Skip',
                    style: TextStyle(color: tokens.textSecondary),
                  ),
                ),
                FilledButton(
                  style: FilledButton.styleFrom(
                    backgroundColor: tokens.primary,
                    foregroundColor: tokens.background,
                  ),
                  onPressed: () {
                    final content = <String, Object?>{};
                    for (final q in questions) {
                      final choice = selected[q.id] ?? '';
                      content[q.id] = choice;
                      if (choice == 'Other') {
                        content['${q.id}_other'] = otherText[q.id]!.text.trim();
                      }
                    }
                    Navigator.of(ctx).pop(content);
                  },
                  child: const Text('Submit'),
                ),
              ],
            );
          },
        );
      },
    );
  } finally {
    for (final c in otherText.values) {
      c.dispose();
    }
  }
}

/// Parses ask_user questions from elicitation meta or schema enums.
List<AskUserQuestion> parseAskUserQuestions(Map<String, Object?> params) {
  final meta = params['_meta'];
  if (meta is Map) {
    final raw = meta['questions'];
    if (raw is List) {
      final out = <AskUserQuestion>[];
      for (final item in raw) {
        if (item is! Map) continue;
        final id = '${item['id'] ?? ''}'.trim();
        final question = '${item['question'] ?? ''}'.trim();
        final optionsRaw = item['options'];
        final options = <String>[];
        if (optionsRaw is List) {
          for (final opt in optionsRaw) {
            if (opt is Map) {
              final label = '${opt['label'] ?? ''}'.trim();
              if (label.isNotEmpty) options.add(label);
            } else {
              final label = '$opt'.trim();
              if (label.isNotEmpty) options.add(label);
            }
          }
        }
        if (id.isNotEmpty && question.isNotEmpty && options.length >= 2) {
          out.add(
            AskUserQuestion(id: id, question: question, options: options),
          );
        }
      }
      if (out.isNotEmpty) return out;
    }
  }

  final schema = params['requestedSchema'];
  if (schema is! Map) return const [];
  final props = schema['properties'];
  if (props is! Map) return const [];
  final out = <AskUserQuestion>[];
  for (final entry in props.entries) {
    final key = '${entry.key}';
    if (key.endsWith('_other')) continue;
    final value = entry.value;
    if (value is! Map) continue;
    final enums = value['enum'];
    if (enums is! List) continue;
    final options = [
      for (final e in enums)
        if ('$e' != 'Other' && '$e'.trim().isNotEmpty) '$e'.trim(),
    ];
    final title = '${value['title'] ?? value['description'] ?? key}'.trim();
    if (options.length >= 2) {
      out.add(AskUserQuestion(id: key, question: title, options: options));
    }
  }
  return out;
}
