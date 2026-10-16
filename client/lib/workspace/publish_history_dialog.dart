import 'dart:async';

import 'package:agent_fabric_client/core/app_log.dart';
import 'package:material_ui/material_ui.dart';

import '../catalog/catalog_client.dart';
import '../catalog/models.dart';
import '../ui/theme/design_tokens.dart';
import 'link_actions.dart';
import 'publish_history.dart';

/// Shows the project's publish history: each publish provider's latest link
/// and its deploys, with sync and delete.
Future<void> showPublishHistoryDialog(
  BuildContext context, {
  required CatalogClient catalog,
  required String projectId,
  required String projectName,
}) {
  return showDialog<void>(
    context: context,
    builder: (context) => PublishHistoryDialog(
      catalog: catalog,
      projectId: projectId,
      projectName: projectName,
    ),
  );
}

class PublishHistoryDialog extends StatefulWidget {
  const PublishHistoryDialog({
    super.key,
    required this.catalog,
    required this.projectId,
    required this.projectName,
  });

  final CatalogClient catalog;
  final String projectId;
  final String projectName;

  @override
  State<PublishHistoryDialog> createState() => _PublishHistoryDialogState();
}

/// Outcome line shown under the dialog title after a sync or delete.
typedef _Notice = ({String text, bool isError});

class _PublishHistoryDialogState extends State<PublishHistoryDialog> {
  List<PublishRecord>? _publishes;
  Object? _loadError;
  bool _syncing = false;
  String? _deletingId;
  List<_Notice> _notices = const [];

  /// Selected provider filter; null shows every provider.
  String? _provider;

  @override
  void initState() {
    super.initState();
    unawaited(_reload());
  }

  Future<void> _reload() async {
    setState(() {
      _loadError = null;
      _publishes = null;
    });
    try {
      final publishes = await widget.catalog.listPublishes(widget.projectId);
      if (!mounted) {
        return;
      }
      setState(() => _publishes = publishes);
    } on Object catch (e, s) {
      AppLog.record('publish history load: $e', s);
      if (!mounted) {
        return;
      }
      setState(() => _loadError = e);
    }
  }

  Future<void> _sync() async {
    setState(() {
      _syncing = true;
      _notices = const [];
    });
    try {
      final result = await widget.catalog.syncPublishes(widget.projectId);
      if (!mounted) {
        return;
      }
      setState(() {
        _publishes = result.publishes;
        _loadError = null;
        _notices = syncNotices(result.providers);
      });
    } on Object catch (e, s) {
      AppLog.record('publish sync: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _notices = [(text: 'Sync failed: ${_message(e)}', isError: true)];
      });
    } finally {
      if (mounted) {
        setState(() => _syncing = false);
      }
    }
  }

  Future<void> _delete(PublishRecord publish) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => _ConfirmDeleteDialog(publish: publish),
    );
    if (confirmed != true || !mounted) {
      return;
    }
    setState(() {
      _deletingId = publish.id;
      _notices = const [];
    });
    try {
      await widget.catalog.deletePublish(widget.projectId, publish.id);
      if (!mounted) {
        return;
      }
      setState(() {
        _publishes = [
          for (final p in _publishes ?? const <PublishRecord>[])
            if (p.id != publish.id) p,
        ];
        _notices = [
          (
            text:
                'Deleted the ${formatPublishTime(publish.createdAt)} deploy '
                'from ${publish.providerLabel}.',
            isError: false,
          ),
        ];
      });
    } on Object catch (e, s) {
      AppLog.record('publish delete: $e', s);
      if (!mounted) {
        return;
      }
      setState(() {
        _notices = [(text: 'Delete failed: ${_message(e)}', isError: true)];
      });
    } finally {
      if (mounted) {
        setState(() => _deletingId = null);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    final busy = _syncing || _deletingId != null;
    return AlertDialog(
      key: const Key('publish-history-dialog'),
      title: Row(
        children: [
          Expanded(child: Text('Published · ${widget.projectName}')),
          Tooltip(
            message:
                'Fetch deploys from connected providers, including ones made '
                'or deleted outside Agent Fabric',
            child: TextButton.icon(
              key: const Key('publish-history-sync'),
              onPressed: busy ? null : () => unawaited(_sync()),
              icon: _syncing
                  ? const SizedBox.square(
                      dimension: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.sync, size: 18),
              label: Text(_syncing ? 'Syncing…' : 'Sync'),
            ),
          ),
        ],
      ),
      content: SizedBox(
        width: 560,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (final notice in _notices)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Icon(
                      notice.isError
                          ? Icons.error_outline
                          : Icons.check_circle_outline,
                      size: 16,
                      color: notice.isError ? tokens.error : tokens.success,
                    ),
                    const SizedBox(width: 6),
                    Expanded(
                      child: Text(
                        notice.text,
                        style: tokens.bodySm().copyWith(
                          color: notice.isError
                              ? tokens.error
                              : tokens.textSecondary,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            Flexible(child: _body(tokens)),
          ],
        ),
      ),
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
    final error = _loadError;
    if (error != null) {
      return Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Could not load publish history: ${_message(error)}',
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
        'shareable link, or Sync to import deploys made elsewhere.',
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
              _GroupSection(
                group: group,
                tokens: tokens,
                deletingId: _deletingId,
                onDelete: _syncing || _deletingId != null
                    ? null
                    : (publish) => unawaited(_delete(publish)),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// Summarizes a sync per provider, e.g. "Netlify: 2 imported, 1 removed".
List<({String text, bool isError})> syncNotices(
  List<PublishSyncProvider> providers,
) {
  if (providers.isEmpty) {
    return const [
      (
        text:
            'No publish provider to sync yet. Publish to a provider that '
            'supports sync, such as Netlify, first.',
        isError: false,
      ),
    ];
  }
  return [
    for (final p in providers)
      if (p.error != null)
        (text: '${p.providerLabel}: ${p.error}', isError: true)
      else
        (text: '${p.providerLabel}: ${_syncSummary(p)}', isError: false),
  ];
}

String _syncSummary(PublishSyncProvider p) {
  final parts = [
    if (p.added > 0) '${p.added} imported',
    if (p.removed > 0) '${p.removed} removed',
    if (p.updated > 0) '${p.updated} updated',
  ];
  return parts.isEmpty ? 'up to date' : parts.join(', ');
}

String _message(Object error) =>
    error is CatalogException ? error.message : '$error';

class _ConfirmDeleteDialog extends StatelessWidget {
  const _ConfirmDeleteDialog({required this.publish});

  final PublishRecord publish;

  @override
  Widget build(BuildContext context) {
    final tokens = designTokensOf(context);
    return AlertDialog(
      key: const Key('publish-delete-confirm'),
      title: const Text('Delete this deploy?'),
      content: SizedBox(
        width: 420,
        child: Text(
          'The ${formatPublishTime(publish.createdAt)} deploy will be deleted '
          'from ${publish.providerLabel} and its link will stop working. The '
          'live site is not affected. This can\'t be undone.',
          style: tokens.bodySm(),
        ),
      ),
      actions: [
        TextButton(
          key: const Key('publish-delete-cancel'),
          onPressed: () => Navigator.of(context).pop(false),
          child: const Text('Cancel'),
        ),
        TextButton(
          key: const Key('publish-delete-confirm-button'),
          style: TextButton.styleFrom(foregroundColor: tokens.error),
          onPressed: () => Navigator.of(context).pop(true),
          child: const Text('Delete deploy'),
        ),
      ],
    );
  }
}

class _GroupSection extends StatelessWidget {
  const _GroupSection({
    required this.group,
    required this.tokens,
    required this.deletingId,
    required this.onDelete,
  });

  final PublishGroup group;
  final DesignTokens tokens;
  final String? deletingId;

  /// Null while a sync or delete is running.
  final ValueChanged<PublishRecord>? onDelete;

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
        for (final publish in group.publishes)
          _DeployRow(
            publish: publish,
            tokens: tokens,
            deleting: publish.id == deletingId,
            onDelete: onDelete,
          ),
      ],
    );
  }
}

class _DeployRow extends StatelessWidget {
  const _DeployRow({
    required this.publish,
    required this.tokens,
    required this.deleting,
    required this.onDelete,
  });

  final PublishRecord publish;
  final DesignTokens tokens;
  final bool deleting;
  final ValueChanged<PublishRecord>? onDelete;

  @override
  Widget build(BuildContext context) {
    final url = publish.deployUrl.isNotEmpty
        ? publish.deployUrl
        : publish.siteUrl;
    final onDelete = this.onDelete;
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
              if (publish.live) ...[
                const SizedBox(width: 8),
                DecoratedBox(
                  key: Key('publish-live-${publish.id}'),
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
                      'Live',
                      style: tokens.caption().copyWith(
                        color: tokens.textPrimary,
                      ),
                    ),
                  ),
                ),
              ],
              const Spacer(),
              if (deleting)
                const SizedBox.square(
                  dimension: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              else if (publish.canDelete)
                IconButton(
                  key: Key('publish-delete-${publish.id}'),
                  tooltip: 'Delete deploy',
                  onPressed: onDelete == null ? null : () => onDelete(publish),
                  icon: Icon(
                    Icons.delete_outline,
                    size: 18,
                    color: tokens.textSecondary,
                  ),
                ),
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
