import 'dart:convert';

import 'package:material_ui/material_ui.dart';

import '../acp/agent_connection.dart';
import 'chat_bubble.dart';

class StatFieldDef {
  const StatFieldDef({
    required this.key,
    required this.label,
    required this.description,
  });

  final String key;
  final String label;
  final String description;
}

/// Display catalog for known usage fields. Order defines Normalized tab order.
const List<StatFieldDef> kKnownStatFields = [
  StatFieldDef(
    key: 'promptTokens',
    label: 'Prompt tokens',
    description: 'Tokens in the prompt sent to the model for this turn.',
  ),
  StatFieldDef(
    key: 'completionTokens',
    label: 'Completion tokens',
    description: 'Tokens generated in the model’s reply.',
  ),
  StatFieldDef(
    key: 'totalTokens',
    label: 'Total tokens',
    description: 'Prompt plus completion tokens for this turn.',
  ),
  StatFieldDef(
    key: 'ttftMs',
    label: 'Time to first token',
    description:
        'Milliseconds from request start until the first output token.',
  ),
  StatFieldDef(
    key: 'elapsedMs',
    label: 'Elapsed time',
    description: 'Total wall-clock time for the turn, in milliseconds.',
  ),
  StatFieldDef(
    key: 'promptMs',
    label: 'Prompt eval time',
    description: 'Time spent evaluating / prefilling the prompt.',
  ),
  StatFieldDef(
    key: 'predictedMs',
    label: 'Generation time',
    description: 'Time spent generating completion tokens.',
  ),
  StatFieldDef(
    key: 'promptPerSecond',
    label: 'Prompt tokens / sec',
    description: 'Prompt evaluation throughput in tokens per second.',
  ),
  StatFieldDef(
    key: 'predictedPerSecond',
    label: 'Generation tokens / sec',
    description: 'Completion generation throughput in tokens per second.',
  ),
  StatFieldDef(
    key: 'deltas',
    label: 'Stream deltas',
    description: 'Number of streamed chunks received for this turn.',
  ),
  StatFieldDef(
    key: 'stopReason',
    label: 'Stop reason',
    description: 'Why generation stopped (for example end_turn or max_tokens).',
  ),
];

class StatRow {
  const StatRow({
    required this.key,
    required this.label,
    required this.value,
    required this.description,
    this.known = true,
  });

  final String key;
  final String label;
  final Object value;
  final String description;
  final bool known;
}

Object? _valueForKey(TurnUsage? usage, String? stopReason, String key) {
  if (key == 'stopReason') {
    final stop = stopReason ?? usage?.stopReason;
    if (stop == null || stop.isEmpty) return null;
    return stop;
  }
  if (usage == null) return null;
  return switch (key) {
    'promptTokens' => usage.promptTokens,
    'completionTokens' => usage.completionTokens,
    'totalTokens' => usage.totalTokens,
    'ttftMs' => usage.ttftMs,
    'elapsedMs' => usage.elapsedMs,
    'promptMs' => usage.promptMs,
    'predictedMs' => usage.predictedMs,
    'promptPerSecond' => usage.promptPerSecond,
    'predictedPerSecond' => usage.predictedPerSecond,
    'deltas' => usage.deltas,
    _ => null,
  };
}

String humanizeStatKey(String key) {
  final parts = key
      .replaceAllMapped(RegExp(r'([a-z0-9])([A-Z])'), (m) => '${m[1]} ${m[2]}')
      .replaceAll('_', ' ')
      .split(RegExp(r'\s+'))
      .where((part) => part.isNotEmpty)
      .map((part) => part.toLowerCase())
      .toList();
  if (parts.isEmpty) return key;
  final first = parts.first;
  parts[0] = first[0].toUpperCase() + first.substring(1);
  return parts.join(' ');
}

List<StatRow> normalizedStatRows(ChatBubble bubble) {
  final usage = bubble.usage;
  final stop = bubble.stopReason ?? usage?.stopReason;
  final rows = <StatRow>[
    for (final def in kKnownStatFields)
      if (_valueForKey(usage, stop, def.key) case final value?)
        StatRow(
          key: def.key,
          label: def.label,
          value: value,
          description: def.description,
        ),
  ];
  final extras = usage?.extras ?? const {};
  for (final entry in extras.entries) {
    if (entry.value == null) continue;
    rows.add(
      StatRow(
        key: entry.key,
        label: humanizeStatKey(entry.key),
        value: entry.value!,
        description:
            'Unrecognized field from the inference API. See the Raw tab for the original key.',
        known: false,
      ),
    );
  }
  return rows;
}

Map<String, Object?> rawStatsMap(ChatBubble bubble) {
  final usage = bubble.usage;
  final stop = bubble.stopReason ?? usage?.stopReason;
  final map = <String, Object?>{};
  void put(String key, Object? value) {
    if (value != null) map[key] = value;
  }

  put('promptTokens', usage?.promptTokens);
  put('completionTokens', usage?.completionTokens);
  put('totalTokens', usage?.totalTokens);
  put('ttftMs', usage?.ttftMs);
  put('elapsedMs', usage?.elapsedMs);
  put('promptMs', usage?.promptMs);
  put('predictedMs', usage?.predictedMs);
  put('promptPerSecond', usage?.promptPerSecond);
  put('predictedPerSecond', usage?.predictedPerSecond);
  put('deltas', usage?.deltas);
  if (stop != null && stop.isNotEmpty) put('stopReason', stop);
  if (usage != null) {
    for (final entry in usage.extras.entries) {
      put(entry.key, entry.value);
    }
  }
  return map;
}

String rawStatsJson(ChatBubble bubble) {
  return const JsonEncoder.withIndent('  ').convert(rawStatsMap(bubble));
}

/// Legacy one-line dump used by older tests/callers.
List<String> statsLines(ChatBubble bubble) {
  return [
    for (final row in normalizedStatRows(bubble)) '${row.key}: ${row.value}',
  ];
}

Future<void> showStatsDialog(BuildContext context, ChatBubble stats) {
  return showDialog<void>(
    context: context,
    builder: (context) => StatsDialog(stats: stats),
  );
}

class StatsDialog extends StatelessWidget {
  const StatsDialog({super.key, required this.stats});

  final ChatBubble stats;

  @override
  Widget build(BuildContext context) {
    final rows = normalizedStatRows(stats);
    final raw = rawStatsJson(stats);
    return AlertDialog(
      title: const Text('Stats'),
      content: SizedBox(
        width: 320,
        child: DefaultTabController(
          length: 2,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const TabBar(
                tabs: [
                  Tab(key: Key('stats-tab-normalized'), text: 'Normalized'),
                  Tab(key: Key('stats-tab-raw'), text: 'Raw'),
                ],
              ),
              SizedBox(
                height: 600,
                child: TabBarView(
                  children: [
                    ListView.separated(
                      key: const Key('stats-normalized-list'),
                      itemCount: rows.length,
                      separatorBuilder: (_, _) => const Divider(height: 1),
                      itemBuilder: (context, index) {
                        final row = rows[index];
                        return Tooltip(
                          message: row.description,
                          preferBelow: false,
                          waitDuration: const Duration(milliseconds: 300),
                          child: ListTile(
                            dense: true,
                            contentPadding: const EdgeInsets.only(right: 8),
                            title: Text(
                              row.label,
                              style: const TextStyle(fontWeight: FontWeight.w600),
                            ),
                            subtitle: SelectableText('${row.value}'),
                          ),
                        );
                      },
                    ),
                    SingleChildScrollView(
                      key: const Key('stats-raw-json'),
                      child: SelectableText(
                        raw,
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          fontFamily: 'monospace',
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Close'),
        ),
      ],
    );
  }
}
