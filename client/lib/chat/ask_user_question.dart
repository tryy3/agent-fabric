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
