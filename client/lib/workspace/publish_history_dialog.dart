import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';
import 'link_actions.dart';
import 'publish_history.dart';

/// Shows the project's publish history: each publish provider's latest link
/// and its earlier deploys.
Future<void> showPublishHistoryDialog(
  BuildContext context, {
  required String projectName,
  required Future<List<PublishRecord>> Function() load,
}) {
  return showDialog<void>(
    context: context,
    builder: (context) =>
        PublishHistoryDialog(projectName: projectName, load: load),
  );
}

class PublishHistoryDialog extends StatefulWidget {
  const PublishHistoryDialog({
    super.key,
    required this.projectName,
    required this.load,
  });

  final String projectName;
  final Future<List<PublishRecord>> Function() load;

  @override
  State<PublishHistoryDialog> createState() => _PublishHistoryDialogState();
}

class _PublishHistoryDialogState extends State<PublishHistoryDialog> {
  List<PublishRecord>? _publishes;
  Object? _error;

  /// Selected provider filter; null shows every provider.
  String? _provider;

  @override
  void initState() {
    super.initState();
    unawaited(_reload());
  }

  Future<void> _reload() async {
    setState(() {
      _error = null;
      _publishes = null;
    });
    try {
      final publishes = await widget.load();
      if (!mounted) {
        return;
      }
      setState(() => _publishes = publishes);
    } on Object catch (e, s) {
      AppLog.record('publish history load: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _error = e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return AlertDialog(
      key: const Key('publish-history-dialog'),
      title: Text('Published · ${widget.projectName}'),
      content: SizedBox(width: 520, child: _body(tokens)),
      actions: [
        TextButton(
          key: const Key('publish-history-close'),
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Close'),
        ),
      ],
    );
  }

  Widget _body(DesignTokens tokens) {
    final error = _error;
    if (error != null) {
      return Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Could not load publish history: $error',
            style: tokens.bodySm().copyWith(color: tokens.error),
          ),
          const SizedBox(height: 8),
          TextButton(
            key: const Key('publish-history-retry'),
            onPressed: () => unawaited(_reload()),
            child: const Text('Retry'),
          ),
        ],
      );
    }
    final publishes = _publishes;
    if (publishes == null) {
      return const SizedBox(
        height: 96,
        child: Center(child: CircularProgressIndicator()),
      );
    }
    if (publishes.isEmpty) {
      return Text(
        'Nothing published yet. Publish from the project menu to get a '
        'shareable link.',
        key: const Key('publish-history-empty'),
        style: tokens.bodySm().copyWith(color: tokens.textSecondary),
      );
    }
    final providers = publishProviders(publishes);
    final provider = providers.any((p) => p.provider == _provider)
        ? _provider
        : null;
    final groups = groupPublishes([
      for (final publish in publishes)
        if (provider == null || publish.provider == provider) publish,
    ]);
    return ConstrainedBox(
      constraints: const BoxConstraints(maxHeight: 520),
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (providers.length > 1) ...[
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  ChoiceChip(
                    key: const Key('publish-filter-all'),
                    label: Text('All (${publishes.length})'),
                    selected: provider == null,
                    onSelected: (_) => setState(() => _provider = null),
                  ),
                  for (final p in providers)
                    ChoiceChip(
                      key: Key('publish-filter-${p.provider}'),
                      label: Text('${p.label} (${p.count})'),
                      selected: provider == p.provider,
                      onSelected: (_) => setState(() => _provider = p.provider),
                    ),
                ],
              ),
              const SizedBox(height: 16),
            ],
            for (final (index, group) in groups.indexed) ...[
              if (index > 0) Divider(height: 32, color: tokens.border),
              _GroupSection(group: group, tokens: tokens),
            ],
          ],
        ),
      ),
    );
  }
}

class _GroupSection extends StatelessWidget {
  const _GroupSection({required this.group, required this.tokens});

  final PublishGroup group;
  final DesignTokens tokens;

  String get _groupKey => group.target.isEmpty
      ? group.provider
      : '${group.provider}-${group.target}';

  @override
  Widget build(BuildContext context) {
    final latest = group.latestUrl;
    return Column(
      key: Key('publish-group-$_groupKey'),
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Icon(Icons.public, size: 18, color: tokens.textSecondary),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                group.showTarget
                    ? '${group.providerLabel} · ${group.target}'
                    : group.providerLabel,
                style: tokens.labelMd().copyWith(color: tokens.textPrimary),
              ),
            ),
            Text(
              'Last published ${formatPublishTime(group.lastPublishedAt)}',
              style: tokens.caption().copyWith(color: tokens.textMuted),
            ),
          ],
        ),
        if (latest.isNotEmpty) ...[
          const SizedBox(height: 12),
          Text('Latest', style: tokens.labelSm()),
          const SizedBox(height: 4),
          SelectableText(
            latest,
            style: tokens.caption().copyWith(color: tokens.textSecondary),
          ),
          LinkCopyOpenActions(
            url: latest,
            label: '${group.providerLabel} latest',
            copyKey: Key('publish-latest-copy-$_groupKey'),
            openKey: Key('publish-latest-open-$_groupKey'),
          ),
        ],
        const SizedBox(height: 12),
        Text('Deploys (${group.publishes.length})', style: tokens.labelSm()),
        const SizedBox(height: 4),
        for (final (index, publish) in group.publishes.indexed)
          _DeployRow(publish: publish, current: index == 0, tokens: tokens),
      ],
    );
  }
}

class _DeployRow extends StatelessWidget {
  const _DeployRow({
    required this.publish,
    required this.current,
    required this.tokens,
  });

  final PublishRecord publish;

  /// Whether this is the newest deploy, which the latest link serves.
  final bool current;
  final DesignTokens tokens;

  @override
  Widget build(BuildContext context) {
    final url = publish.deployUrl.isNotEmpty
        ? publish.deployUrl
        : publish.siteUrl;
    return Padding(
      key: Key('publish-deploy-${publish.id}'),
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Text(
                formatPublishTime(publish.createdAt),
                style: tokens.bodySm().copyWith(color: tokens.textPrimary),
              ),
              if (current) ...[
                const SizedBox(width: 8),
                DecoratedBox(
                  decoration: BoxDecoration(
                    color: tokens.surfaceActive,
                    borderRadius: BorderRadius.circular(DesignTokens.radiusXs),
                  ),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 6,
                      vertical: 2,
                    ),
                    child: Text(
                      'Current',
                      style: tokens.caption().copyWith(
                        color: tokens.textPrimary,
                      ),
                    ),
                  ),
                ),
              ],
            ],
          ),
          if (url.isNotEmpty) ...[
            const SizedBox(height: 2),
            SelectableText(
              url,
              style: tokens.caption().copyWith(color: tokens.textSecondary),
            ),
            LinkCopyOpenActions(
              url: url,
              label: 'deploy',
              copyKey: Key('publish-deploy-copy-${publish.id}'),
              openKey: Key('publish-deploy-open-${publish.id}'),
            ),
          ],
        ],
      ),
    );
  }
}

/// Formats a publish time in local time, e.g. "Oct 11, 2026 3:04 PM".
String formatPublishTime(DateTime when) {
  const months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  final local = when.toLocal();
  final hour12 = local.hour % 12 == 0 ? 12 : local.hour % 12;
  final minute = local.minute.toString().padLeft(2, '0');
  final suffix = local.hour >= 12 ? 'PM' : 'AM';
  return '${months[local.month - 1]} ${local.day}, ${local.year} '
      '$hour12:$minute $suffix';
}
