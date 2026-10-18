// SubscriptionsPage 的展示层扩展（无 setState 段；从 subscriptions_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/subscriptions/subscriptions_page.dart';

extension on _SubscriptionsPageState {
  String _statusText(String s) =>
      const {'subscribed': '订阅中', 'completed': '已完成', 'paused': '已暂停'}[s] ?? s;

  Color _statusColor(String s) => switch (s) {
    'subscribed' => PTColors.success,
    'completed' => PTColors.primary,
    _ => PTColors.warn,
  };

  String _fmtTime(String? t) => (t == null || t.isEmpty)
      ? '—'
      : t.replaceAll('T', ' ').replaceAll(RegExp(r'\.\d+Z?$'), '');

  // 空状态 CTA：切到主菜单的「搜索」Tab（经 HomeShell 控制器按 label 定位索引，避免硬编码）。
  void _goSearch() {
    final idx = navItems.indexWhere((n) => n.label == '搜索');
    if (idx >= 0) HomeShellScope.of(context).select(idx);
  }

  Widget _buildBody() {
    if (_loading && _subs.isEmpty) {
      return const PTLoadingView();
    }
    if (_error != null) {
      return PTEmptyState(text: _error!, onRetry: _load);
    }
    if (_subs.isEmpty) {
      return PTEmptyState(
        icon: Icons.subscriptions_outlined,
        text: '暂无订阅',
        description: '去搜索/推荐找到影视 → 进详情页 → 订阅',
        actionLabel: '去搜索影视',
        onAction: _goSearch,
      );
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      itemCount: _subs.length,
      separatorBuilder: (_, _) => const SizedBox(height: 12),
      itemBuilder: (_, i) => PTListItemEntrance(index: i, child: _card(_subs[i])),
    );
  }

  Widget _card(Subscription s) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 72,
            height: 104,
            child: ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: MediaPoster(
                url: s.posterUrl,
                kind: s.type == 'music' ? 'music' : 'movie',
              ),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Flexible(
                      child: Text(
                        s.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w700,
                          fontSize: 15,
                        ),
                      ),
                    ),
                    if (s.year > 0)
                      Padding(
                        padding: const EdgeInsets.only(left: 4),
                        child: Text(
                          '(${s.year})',
                          style: TextStyle(
                            color: context.dimText,
                            fontSize: 13,
                          ),
                        ),
                      ),
                  ],
                ),
                const SizedBox(height: 4),
                Wrap(
                  spacing: 6,
                  runSpacing: 4,
                  children: [
                    _chip(_statusText(s.status), _statusColor(s.status)),
                    if (s.resolution.isNotEmpty)
                      _chip(s.resolution, PTColors.primary2),
                    if (s.bestVersion ?? _global.bestVersion)
                      _chip(
                        s.currentPriority > 0 ? '洗版 Pri${s.currentPriority}' : '洗版中',
                        PTColors.success,
                      ),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  '上次检查：${_fmtTime(s.lastCheckedAt)} · 类型 ${s.type == 'tv' ? '剧集' : '电影'} · 源 ${s.source}',
                  style: TextStyle(color: context.dimText, fontSize: 12),
                ),
                // 「检查过」不等于「查得动」：本轮有站读不出来时把原因留在卡片上，
                // 否则 1/2 个站失效与「确实没有新命中」是同一句文案（#262）。
                if (s.lastCheckError.isNotEmpty) ...[
                  const SizedBox(height: 4),
                  Text(
                    '上次检查有问题：${s.lastCheckError}',
                    style: const TextStyle(color: PTColors.warn, fontSize: 12),
                  ),
                ],
                const SizedBox(height: 8),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    FilledButton.tonalIcon(
                      onPressed: _checking.contains(s.id)
                          ? null
                          : () => _check(s),
                      icon: _checking.contains(s.id)
                          ? const SizedBox(
                              width: 14,
                              height: 14,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : const Icon(Icons.refresh, size: 16),
                      label: const Text('立即检查', style: TextStyle(fontSize: 12)),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _editResolution(s),
                      icon: const Icon(Icons.high_quality_outlined, size: 16),
                      label: Text(
                        s.resolution.isEmpty ? '设分辨率' : '改分辨率',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    GestureDetector(
                      onLongPress: () => _resetRatingGuard(s),
                      child: FilledButton.tonalIcon(
                        onPressed: () => _toggleRatingGuard(s),
                        style: FilledButton.styleFrom(
                          foregroundColor: (s.enableRatingGuard ?? _global.enableRatingGuard) ? PTColors.warn : null,
                        ),
                        icon: Icon(
                          (s.enableRatingGuard ?? _global.enableRatingGuard) ? Icons.shield : Icons.shield_outlined,
                          size: 16,
                        ),
                        label: Text(
                          (s.enableRatingGuard ?? _global.enableRatingGuard)
                              ? (s.enableRatingGuard == null ? '保护(全局)' : '保护中')
                              : (s.enableRatingGuard == null ? '保护关(全局)' : '保护'),
                          style: const TextStyle(fontSize: 12),
                        ),
                      ),
                    ),
                    GestureDetector(
                      onLongPress: () => _resetBestVersion(s),
                      child: FilledButton.tonalIcon(
                        onPressed: () => _toggleBestVersion(s),
                        style: FilledButton.styleFrom(
                          foregroundColor: (s.bestVersion ?? _global.bestVersion) ? PTColors.success : null,
                        ),
                        icon: Icon(
                          (s.bestVersion ?? _global.bestVersion) ? Icons.auto_awesome : Icons.auto_awesome_outlined,
                          size: 16,
                        ),
                        label: Text(
                          (s.bestVersion ?? _global.bestVersion)
                              ? (s.bestVersion == null ? '洗版(全局)' : '洗版中')
                              : (s.bestVersion == null ? '洗版关(全局)' : '洗版'),
                          style: const TextStyle(fontSize: 12),
                        ),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _editPromoTypes(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: s.promoTypes.isNotEmpty ? PTColors.success : null,
                      ),
                      icon: const Icon(Icons.savings_outlined, size: 16),
                      label: Text(
                        s.promoTypes.isEmpty ? '促销' : '${s.promoTypes.length}种促销',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _editSites(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: s.siteIds.isNotEmpty ? PTColors.primary : null,
                      ),
                      icon: const Icon(Icons.public, size: 16),
                      label: Text(
                        s.siteIds.isEmpty ? '全部站' : '${s.siteIds.length}站',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _toggle(s),
                      icon: Icon(
                        s.status == 'paused' ? Icons.play_arrow : Icons.pause,
                        size: 16,
                      ),
                      label: Text(
                        s.status == 'paused' ? '恢复' : '暂停',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _remove(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: PTColors.danger,
                      ),
                      icon: const Icon(Icons.delete_outline, size: 16),
                      label: const Text('删除', style: TextStyle(fontSize: 12)),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _chip(String label, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

}
