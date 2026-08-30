// _SitesPageState 的行渲染扩展：列表主体/连通性胶囊。
// （从 sites_widgets.dart 拆出；行为不变。）
part of 'package:ptpatronus/features/sites/sites_page.dart';

extension on _SitesPageState {
  Widget _listBody() {
    if (_sites.isEmpty && !_loading) {
      return RefreshIndicator(
        onRefresh: _load,
        child: _error != null
            ? PTEmptyState(
                icon: Icons.error_outline,
                text: '加载失败',
                description: _error,
                onRetry: _load,
              )
            : PTEmptyState(
                icon: Icons.language,
                text: '暂无站点',
                description: '添加你的第一个 PT 站点，开始跨站统计与签到',
                actionLabel: '添加站点',
                onAction: () => _showForm(),
              ),
      );
    }
    return RefreshIndicator(
      onRefresh: _load,
      child: PTAdaptiveTable<Site>(
        items: _sites,
        columns: [
          PTColumn(
            label: '名称',
            flex: 2,
            cell: (_, s) => Text(
              s.name,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
          ),
          PTColumn(
            label: '定义',
            flex: 2,
            cell: (_, s) => Text(
              s.definition,
              style: TextStyle(fontSize: 13, color: context.dimText),
            ),
          ),
          PTColumn(
            label: '平台',
            flex: 1,
            cell: (_, s) => Text(
              s.platform.isNotEmpty ? s.platform : '-',
              style: const TextStyle(fontSize: 13),
            ),
          ),
          PTColumn(
            label: '地址',
            flex: 3,
            cell: (_, s) => Text(
              s.baseUrl,
              style: const TextStyle(fontSize: 12),
              overflow: TextOverflow.ellipsis,
            ),
          ),
          PTColumn(
            label: '联通状态',
            flex: 1,
            cell: (_, s) => _connectivityPill(s),
          ),
          PTColumn(
            label: '状态',
            flex: 1,
            cell: (_, s) => _statusPill(s.isActive),
          ),
          PTColumn(label: '操作', flex: 3, cell: (ctx, s) => _rowActions(ctx, s)),
        ],
        emptyText: '暂无站点',
        narrowCard: (ctx, s) => GlassCard(
          padding: const EdgeInsets.fromLTRB(14, 12, 8, 12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      s.name,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 16,
                      ),
                    ),
                  ),
                  _connectivityPill(s),
                  const SizedBox(width: 6),
                  _statusPill(s.isActive),
                ],
              ),
              const SizedBox(height: 2),
              Text(
                s.baseUrl,
                style: TextStyle(color: context.dimText, fontSize: 12),
                overflow: TextOverflow.ellipsis,
              ),
              const SizedBox(height: 4),
              Text(
                '${s.definition} · ${s.platform.isNotEmpty ? s.platform : '-'}',
                style: TextStyle(color: context.dimText, fontSize: 12),
              ),
              const SizedBox(height: 8),
              _rowActions(ctx, s),
            ],
          ),
        ),
      ),
    );
  }

  Widget _connectivityPill(Site s) {
    final status = _connectivity[s.id];
    final pending = _connectivityLoading;
    final state = status?.state;
    final label = pending
        ? '检测中'
        : status == null
        ? '未检测'
        : state == SiteConnectivityState.connected
        ? '已联通'
        : state == SiteConnectivityState.disabled
        ? '已停用'
        : '未联通';
    final color = pending || status == null
        ? context.dimText
        : state == SiteConnectivityState.connected
        ? PTColors.success
        : state == SiteConnectivityState.disabled
        ? context.dimText
        : PTColors.danger;
    final child = Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: pending || status == null ? 0.1 : 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(color: color, shape: BoxShape.circle),
          ),
          const SizedBox(width: 5),
          Text(
            label,
            style: TextStyle(
              fontSize: 11,
              color: color,
              fontWeight: FontWeight.w600,
            ),
          ),
        ],
      ),
    );
    if (pending ||
        status == null ||
        status.state == SiteConnectivityState.connected) {
      return child;
    }
    return GestureDetector(
      onTap: () => _showConnectivityReason(s),
      child: child,
    );
  }
}
