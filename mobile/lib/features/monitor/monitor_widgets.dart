// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// MonitorPage 的展示层扩展（无 setState 段；从 monitor_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/monitor/monitor_page.dart';

extension on _MonitorPageState {
  Widget _errorView() => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Icon(Icons.error_outline, size: 40, color: Colors.redAccent),
        const SizedBox(height: 8),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 32),
          child: Text(
            _error ?? '加载失败',
            textAlign: TextAlign.center,
            style: const TextStyle(color: Colors.redAccent, fontSize: 14),
          ),
        ),
        const SizedBox(height: 12),
        FilledButton.tonalIcon(
          icon: const Icon(Icons.refresh, size: 18),
          label: const Text('重试'),
          onPressed: _load,
        ),
      ],
    ),
  );

  Widget _typeBadge(String type) {
    final color = switch (type) {
      'host' => const Color(0xFF4285F4),
      'nas' => PTColors.primary,
      'camera' => const Color(0xFFFF5F6D),
      _ => context.dimText,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(6),
        color: color.withValues(alpha: 0.18),
      ),
      child: Text(
        type.toUpperCase(),
        style: TextStyle(
          color: color,
          fontSize: 12,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  Widget _row(MonitorTarget t) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 12, 12, 12),
      onTap: () => _open(t),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              _typeBadge(t.type),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      t.name,
                      style: const TextStyle(
                        fontWeight: FontWeight.w600,
                        fontSize: 16,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      t.host.isEmpty ? '(本机)' : t.host,
                      style: TextStyle(color: context.dimText, fontSize: 12),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (t.lastError.isNotEmpty)
                      Text(
                        t.lastError,
                        style: const TextStyle(
                          color: Colors.redAccent,
                          fontSize: 12,
                        ),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                  ],
                ),
              ),
              Icon(
                t.lastOnline == true
                    ? Icons.circle
                    : t.lastOnline == false
                    ? Icons.circle_outlined
                    : Icons.help_outline,
                size: 14,
                color: t.lastOnline == true
                    ? PTColors.success
                    : t.lastOnline == false
                    ? PTColors.danger
                    : context.dimText,
              ),
            ],
          ),
          // 内联指标条（CPU/内存/磁盘）——对齐 Web DeviceCard 的 MetricBar。摄像头不显示。
          if (t.type != 'camera')
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: _metricBars(_latest[t.id]),
            ),
        ],
      ),
    ),
  );

  // 内联指标：离线显「离线」；在线显 CPU/内存/磁盘 三条迷你进度条。
  Widget _metricBars(Metrics? m) {
    if (m == null) return const SizedBox.shrink();
    if (!m.online) {
      return Text(
        '离线',
        style: TextStyle(color: context.dimText, fontSize: 12),
      );
    }
    final cpu = m.cpuUsage ?? 0;
    final mem = m.memUsage ?? 0;
    final disk = m.disks.fold<double>(0, (a, d) => d.usage > a ? d.usage : a);
    return Wrap(
      spacing: 14,
      runSpacing: 4,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [_gauge('CPU', cpu), _gauge('内存', mem), _gauge('磁盘', disk)],
    );
  }

  Widget _gauge(String label, double pct) {
    final p = pct.clamp(0, 100).toDouble();
    final color = p > 90
        ? PTColors.danger
        : (p > 70 ? PTColors.warn : PTColors.success);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          label,
          style: TextStyle(color: context.dimText, fontSize: 12),
        ),
        const SizedBox(width: 4),
        SizedBox(
          width: 44,
          height: 4,
          child: ClipRRect(
            borderRadius: BorderRadius.circular(2),
            child: LinearProgressIndicator(
              value: p / 100,
              backgroundColor: context.dimText.withValues(alpha: 0.2),
              valueColor: AlwaysStoppedAnimation<Color>(color),
            ),
          ),
        ),
        const SizedBox(width: 4),
        Text(
          '${p.toStringAsFixed(0)}%',
          style: TextStyle(
            fontSize: 12,
            color: color,
            fontWeight: FontWeight.w600,
          ),
        ),
      ],
    );
  }

  Widget _listFor(String type, String emptyText) {
    if (_loading && _targets.isEmpty) {
      return const PTLoadingView();
    }
    final items = _targets.where((t) => t.type == type).toList();
    if (items.isEmpty) {
      return Center(
        child: Text(emptyText, style: TextStyle(color: context.dimText)),
      );
    }
    return ListView(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      children: items.map(_row).toList(),
    );
  }

}
