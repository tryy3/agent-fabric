import 'dart:convert';

import 'package:material_ui/material_ui.dart';

import '../acp/agent_connection.dart';
import '../ui/theme/design_tokens.dart';
import 'chat_bubble.dart';
import 'cost_format.dart';

/// Where a stat comes from: the inference provider's response, or the plane
/// itself (measured timings, prices applied to the reported token counts).
enum StatSource {
  provider('Provider'),
  plane('Plane');

  StatSource(this.label);

  final String label;
}

class StatFieldDef {
  const StatFieldDef({
    required this.key,
    required this.label,
    required this.description,
    this.source = StatSource.provider,
  });

  final String key;
  final String label;
  final String description;
  final StatSource source;
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
    description: 'Tokens generated in the model\'s reply.',
  ),
  StatFieldDef(
    key: 'totalTokens',
    label: 'Total tokens',
    description: 'Prompt plus completion tokens for this turn.',
  ),
  StatFieldDef(
    key: 'cachedTokens',
    label: 'Cached tokens',
    description: 'Part of the prompt served from the provider\'s prompt cache, usually at a lower price.',
  ),
  StatFieldDef(
    key: 'cacheWriteTokens',
    label: 'Cache write tokens',
    description: 'Part of the prompt written to the provider\'s prompt cache.',
  ),
  StatFieldDef(
    key: 'reasoningTokens',
    label: 'Reasoning tokens',
    description: 'Part of the completion spent on reasoning.',
  ),
  StatFieldDef(
    key: 'cost',
    label: 'Estimated cost',
    description: 'Estimated from the model\'s published prices and the reported token counts; not exact billing.',
    source: StatSource.plane,
  ),
  StatFieldDef(
    key: 'reportedCostUsd',
    label: 'Provider-reported cost',
    description: 'Cost the inference provider itself reported for this turn.',
  ),
  StatFieldDef(
    key: 'ttftMs',
    label: 'Time to first token',
    description:
        'Milliseconds from request start until the first output token.',
    source: StatSource.plane,
  ),
  StatFieldDef(
    key: 'elapsedMs',
    label: 'Elapsed time',
    description: 'Total wall-clock time for the turn, in milliseconds.',
    source: StatSource.plane,
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
    key: 'co2Grams',
    label: 'CO₂',
    description: 'Estimated carbon dioxide emissions for this turn, in grams.',
  ),
  StatFieldDef(
    key: 'gpuEnergyJoules',
    label: 'GPU energy',
    description: 'Estimated GPU energy used for this turn, in joules.',
  ),
  StatFieldDef(
    key: 'deltas',
    label: 'Stream deltas',
    description: 'Number of streamed chunks received for this turn.',
    source: StatSource.plane,
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
    this.source = StatSource.provider,
  });

  final String key;
  final String label;
  final Object value;
  final String description;
  final bool known;
  final StatSource source;
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
    'co2Grams' => usage.co2Grams,
    'gpuEnergyJoules' => usage.gpuEnergyJoules,
    'deltas' => usage.deltas,
    'cachedTokens' => usage.cachedTokens,
    'cacheWriteTokens' => usage.cacheWriteTokens,
    'reasoningTokens' => usage.reasoningTokens,
    'cost' => usage.cost == null ? null : _costRowText(usage.cost!),
    'reportedCostUsd' =>
      usage.reportedCostUsd == null ? null : formatUsd(usage.reportedCostUsd!),
    _ => null,
  };
}

String _costRowText(TurnCost cost) {
  final breakdown = costBreakdownText(cost);
  final base = formatCost(cost);
  return breakdown.isEmpty ? base : '$base ($breakdown)';
}

/// One line per LLM round of a tool-using turn: tokens and estimated cost.
List<StatRow> roundStatRows(TurnUsage? usage) {
  final rounds = usage?.rounds ?? const [];
  return [
    for (var i = 0; i < rounds.length; i++)
      StatRow(
        key: 'round$i',
        label: 'Round ${i + 1}',
        value: _roundText(rounds[i]),
        description: 'Usage and estimated cost of one LLM call in this turn (a tool call ends a round).',
        source: StatSource.plane,
      ),
  ];
}

String _roundText(Map<String, Object?> round) {
  String? tokens(String key, String suffix) {
    final v = round[key];
    return v is num ? '${v.toInt()} $suffix' : null;
  }

  final cost = TurnCost.tryParse(round['cost']);
  final parts = <String>[
    ?tokens('promptTokens', 'in'),
    ?tokens('completionTokens', 'out'),
    ?tokens('cachedTokens', 'cached'),
    if (cost != null) formatCost(cost),
  ];
  return parts.isEmpty ? 'no usage reported' : parts.join(' · ');
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
          source: def.source,
        ),
  ];
  rows.addAll(roundStatRows(usage));
  final extras = usage?.extras ?? const {};
  for (final entry in extras.entries) {
    if (entry.value == null) continue;
    rows.add(
      StatRow(
        key: entry.key,
        label: humanizeStatKey(entry.key),
        value: entry.value!,
        description: 'Unrecognized field from the inference API. See the Raw tab for the original key.',
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
  put('co2Grams', usage?.co2Grams);
  put('gpuEnergyJoules', usage?.gpuEnergyJoules);
  put('deltas', usage?.deltas);
  put('cachedTokens', usage?.cachedTokens);
  put('cacheWriteTokens', usage?.cacheWriteTokens);
  put('reasoningTokens', usage?.reasoningTokens);
  put('cost', usage?.cost?.toJson());
  put('reportedCostUsd', usage?.reportedCostUsd);
  if (usage != null && usage.rounds.isNotEmpty) put('rounds', usage.rounds);
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

class StatsDialog extends StatefulWidget {
  const StatsDialog({super.key, required this.stats});

  final ChatBubble stats;

  @override
  State<StatsDialog> createState() => _StatsDialogState();
}

class _StatsDialogState extends State<StatsDialog>
    with SingleTickerProviderStateMixin {
  late final TabController _tabs = TabController(length: 2, vsync: this);

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final rows = normalizedStatRows(widget.stats);
    final raw = rawStatsJson(widget.stats);
    return AlertDialog(
      titlePadding: const EdgeInsets.fromLTRB(20, 16, 20, 0),
      contentPadding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
      actionsPadding: const EdgeInsets.fromLTRB(12, 4, 12, 8),
      title: const Text('Stats'),
      content: SizedBox(
        width: 320,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TabBar(
              controller: _tabs,
              tabs: const [
                Tab(key: Key('stats-tab-normalized'), text: 'Normalized'),
                Tab(key: Key('stats-tab-raw'), text: 'Raw'),
              ],
            ),
            ConstrainedBox(
              constraints: const BoxConstraints(maxHeight: 600),
              child: AnimatedBuilder(
                animation: _tabs,
                builder: (context, _) {
                  if (_tabs.index == 0) {
                    return ListView.separated(
                      key: const Key('stats-normalized-list'),
                      shrinkWrap: true,
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
                            visualDensity: VisualDensity.compact,
                            contentPadding: const EdgeInsets.only(right: 8),
                            title: Row(
                              children: [
                                Expanded(
                                  child: Text(
                                    row.label,
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                    ),
                                  ),
                                ),
                                Text(
                                  row.source.label,
                                  key: Key('stat-source-${row.key}'),
                                  style: TextStyle(
                                    fontSize: 11,
                                    color: designTokensOf(context).textMuted,
                                  ),
                                ),
                              ],
                            ),
                            subtitle: SelectableText('${row.value}'),
                          ),
                        );
                      },
                    );
                  }
                  return SingleChildScrollView(
                    key: const Key('stats-raw-json'),
                    child: SelectableText(
                      raw,
                      style: Theme.of(context).textTheme.bodySmall
                          ?.copyWith(fontFamily: 'monospace'),
                    ),
                  );
                },
              ),
            ),
          ],
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
