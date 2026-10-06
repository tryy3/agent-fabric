import 'dart:async';
import 'dart:convert';

import 'package:material_ui/material_ui.dart';

import '../acp/agent_connection.dart';
import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';
import 'chat_bubble.dart';
import 'cost_format.dart';
import 'inspector_http_view.dart';

/// Where a stat comes from: the inference provider's response, or the plane
/// itself (measured timings, prices applied to the reported token counts).
enum StatSource {
  provider('Provider'),
  plane('Computed');

  StatSource(this.label);

  final String label;
}

/// Small pill saying where a stat comes from. Provider is neutral; computed
/// (measured or calculated by the plane) is tinted so the two read apart at a
/// glance without relying on color alone: the text says it too.
class StatSourceBadge extends StatelessWidget {
  const StatSourceBadge({super.key, required this.source});

  final StatSource source;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final computed = source == StatSource.plane;
    final color = computed
        ? Theme.of(context).colorScheme.primary
        : tokens.textSecondary;
    return Tooltip(
      message: computed
          ? 'Measured or calculated by Agent Fabric, not reported by the provider.'
          : 'Reported by the inference provider.',
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
        decoration: BoxDecoration(
          color: computed ? color.withValues(alpha: 0.14) : null,
          border: Border.all(
            color: color.withValues(alpha: computed ? 0.5 : 0.4),
          ),
          borderRadius: BorderRadius.circular(DesignTokens.radiusXs),
        ),
        child: Text(
          source.label,
          style: TextStyle(
            fontSize: 10.5,
            fontWeight: FontWeight.w600,
            color: color,
          ),
        ),
      ),
    );
  }
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
    key: 'model',
    label: 'Model',
    description: 'The model that served this request.',
    source: StatSource.plane,
  ),
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
    'model' => usage.model,
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

  put('model', usage?.model);
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

/// Opens the stats dialog for a turn (a stats bubble) or for one LLM round (a
/// round divider's bubble). Both use the same dialog: the same rows, and a Raw
/// tab with the request inspector when the plane has the captures.
Future<void> showStatsDialog(
  BuildContext context,
  ChatBubble bubble, {
  CatalogClient? catalog,
  String? threadId,
}) {
  return showDialog<void>(
    context: context,
    builder: (context) =>
        StatsDialog(stats: bubble, catalog: catalog, threadId: threadId),
  );
}

class StatsDialog extends StatefulWidget {
  const StatsDialog({
    super.key,
    required this.stats,
    this.catalog,
    this.threadId,
  });

  /// A stats bubble (the whole turn) or a round-cost bubble (one round).
  final ChatBubble stats;

  /// Used to load the request captures behind the Raw tab.
  final CatalogClient? catalog;
  final String? threadId;

  @override
  State<StatsDialog> createState() => _StatsDialogState();
}

class _StatsDialogState extends State<StatsDialog>
    with SingleTickerProviderStateMixin {
  late final TabController _tabs = TabController(length: 2, vsync: this);
  List<HopCapture> _captures = const [];
  String? _selectedCaptureId;

  bool get _isRound => widget.stats.kind == ChatBubbleKind.roundCost;
  int? get _round => widget.stats.usage?.round;

  @override
  void initState() {
    super.initState();
    unawaited(_loadCaptures());
  }

  Future<void> _loadCaptures() async {
    final catalog = widget.catalog;
    final threadId = widget.threadId;
    final messageId = widget.stats.catalogMessageId;
    if (catalog == null || threadId == null || messageId == null) return;
    try {
      final all = await catalog.listMessageCaptures(threadId, messageId);
      final llm = [
        for (final c in all)
          if (c.hopKind == 'llm' && (!_isRound || c.roundIndex == _round)) c,
      ]..sort((a, b) => a.roundIndex.compareTo(b.roundIndex));
      if (mounted && llm.isNotEmpty) {
        setState(() {
          _captures = llm;
          _selectedCaptureId = llm.first.id;
        });
      }
    } on Object catch (_) {
      // The dialog still shows what the bubble knows.
    }
  }

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  /// The bubble to show rows for. A round starts from its divider's tokens and
  /// cost; the stored capture's normalized usage replaces the rest.
  ChatBubble _displayBubble() {
    final bubble = widget.stats;
    if (!_isRound) return bubble;
    final lean = bubble.usage!;
    var usage = lean.asRound();
    final stored = _captures.isEmpty ? null : _captures.first.meta['usage'];
    if (stored is Map) {
      final model = _captures.first.meta['model'];
      usage = turnUsageFromMeta({
        ...stored.cast<String, Object?>(),
        'model': ?(model is String ? model : lean.model),
        'round': lean.round,
      }).withCost(cost: lean.roundCost);
    }
    return ChatBubble(
      kind: ChatBubbleKind.stats,
      usage: usage,
      stopReason: usage.stopReason,
    );
  }

  Widget _rawTab(BuildContext context, ChatBubble shown) {
    if (_captures.isEmpty) {
      return SingleChildScrollView(
        key: const Key('stats-raw-json'),
        child: SelectableText(
          rawStatsJson(shown),
          style: Theme.of(context).textTheme.bodySmall
              ?.copyWith(fontFamily: 'monospace'),
        ),
      );
    }
    final selected = _captures.firstWhere(
      (c) => c.id == _selectedCaptureId,
      orElse: () => _captures.first,
    );
    return SizedBox(
      key: const Key('stats-raw-view'),
      height: (MediaQuery.sizeOf(context).height - 230).clamp(200.0, 700.0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (_captures.length > 1)
            Padding(
              padding: const EdgeInsets.only(top: 8, bottom: 4),
              child: Wrap(
                spacing: 6,
                children: [
                  for (final c in _captures)
                    ChoiceChip(
                      key: Key('stats-raw-round-${c.roundIndex}'),
                      label: Text('Round ${c.roundIndex + 1}'),
                      selected: c.id == selected.id,
                      onSelected: (_) =>
                          setState(() => _selectedCaptureId = c.id),
                    ),
                ],
              ),
            ),
          Expanded(
            child: InspectorHttpView(
              key: ValueKey(selected.id),
              capture: selected,
            ),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final shown = _displayBubble();
    final rows = normalizedStatRows(shown);
    final title = _isRound ? 'Round ${(_round ?? 0) + 1}' : 'Stats';
    final screen = MediaQuery.sizeOf(context);
    return AlertDialog(
      titlePadding: const EdgeInsets.fromLTRB(20, 16, 20, 0),
      contentPadding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
      actionsPadding: const EdgeInsets.fromLTRB(12, 4, 12, 8),
      insetPadding: const EdgeInsets.symmetric(horizontal: 24, vertical: 24),
      title: Text(title),
      content: AnimatedBuilder(
        animation: _tabs,
        builder: (context, _) {
          // The request inspector needs room, so the dialog widens on Raw.
          final wide = _tabs.index == 1 && _captures.isNotEmpty;
          return SizedBox(
            width: wide ? (screen.width - 48).clamp(320.0, 960.0) : 320,
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
                  constraints: BoxConstraints(
                    maxHeight: wide
                        ? (screen.height - 180).clamp(260.0, 780.0)
                        : 600,
                  ),
                  child: _tabs.index == 0
                      ? ListView.separated(
                          key: const Key('stats-normalized-list'),
                          shrinkWrap: true,
                          itemCount: rows.length,
                          separatorBuilder: (_, _) => const Divider(height: 1),
                          itemBuilder: (context, index) =>
                              _StatRowTile(row: rows[index]),
                        )
                      : _rawTab(context, shown),
                ),
              ],
            ),
          );
        },
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

class _StatRowTile extends StatelessWidget {
  const _StatRowTile({required this.row});

  final StatRow row;

  @override
  Widget build(BuildContext context) {
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
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ),
            StatSourceBadge(
              key: Key('stat-source-${row.key}'),
              source: row.source,
            ),
          ],
        ),
        subtitle: SelectableText('${row.value}'),
      ),
    );
  }
}
