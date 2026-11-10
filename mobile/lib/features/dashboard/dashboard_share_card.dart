// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _DashboardPageState 的分享卡渲染区：分享卡/指标格/榜单/站点卡/身份信息。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {
  Widget _shareCard() {
    final sites = _visibleSites();
    final totalUploaded = _sumInt(sites, (s) => s.uploaded);
    final totalDownloaded = _sumInt(sites, (s) => s.downloaded);
    final totalSeeding = _sumInt(sites, (s) => s.seeding);
    final totalSeedingSize = _sumInt(sites, (s) => s.seedingSize);
    final totalUploads = _sumInt(sites, (s) => s.uploads);
    final totalBonus = _sumDouble(sites, (s) => s.bonus);
    final totalBonusPerHour = _sumDouble(sites, (s) => s.bonusPerHour);
    // 优先真实分享率（totals.true_ratio），避免魔力膨胀虚高。
    final trueRatio = _overview?.trueRatio ?? 0;
    final ratio = trueRatio > 0
        ? trueRatio
        : (totalDownloaded > 0 ? totalUploaded / totalDownloaded : 0.0);
    final displayUser = _shareDisplayUser(sites);
    final shownSites = _config.showSiteList
        ? sites.take(8).toList()
        : <SiteStatItem>[];
    final summaryMetrics = <ShareMetric>[
      if (_config.showTotalSites)
        ShareMetric('总站点数', '${sites.length}', const Color(0xFF45C4F0)),
      if (_config.showUploadCount)
        ShareMetric('总发布数', '$totalUploads', const Color(0xFFFFC857)),
      if (_config.showUpload)
        ShareMetric('总上传量', _fmtBytes(totalUploaded), const Color(0xFF62D48E)),
      if (_config.showDownload)
        ShareMetric(
          '总下载量',
          _fmtBytes(totalDownloaded),
          const Color(0xFFFF6B6B),
        ),
      if (_config.showSeedingCount)
        ShareMetric('总做种数', '$totalSeeding', const Color(0xFF45C4F0)),
      if (_config.showSeedingVolume)
        ShareMetric(
          '总做种量',
          _fmtBytes(totalSeedingSize),
          const Color(0xFF62D48E),
        ),
      if (_config.showBonus)
        ShareMetric('总积分值', _fmtCompact(totalBonus), const Color(0xFFFFC857)),
      if (_config.showBonusPerHour)
        ShareMetric(
          '每小时积分',
          _fmtCompact(totalBonusPerHour),
          const Color(0xFFFF9F43),
        ),
      if (_config.showRatio)
        ShareMetric('总分享率', _fmtRatio(ratio), const Color(0xFF45C4F0)),
    ];

    return Container(
      width: 360,
      decoration: const BoxDecoration(color: Color(0xFF121212)),
      padding: const EdgeInsets.fromLTRB(18, 18, 18, 16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 34,
                height: 34,
                alignment: Alignment.center,
                decoration: const BoxDecoration(
                  color: Color(0xFF45C4F0),
                  shape: BoxShape.circle,
                ),
                child: Text(
                  _shareUserInitial(displayUser),
                  style: const TextStyle(
                    color: Color(0xFF121212),
                    fontSize: 17,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      displayUser,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: Color(0xFFF8FAFC),
                        fontSize: 18,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 2),
                    const Text(
                      'PT Patronus 总仪表盘',
                      style: TextStyle(color: Color(0xFFB6C2CF), fontSize: 11),
                    ),
                  ],
                ),
              ),
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(
                    _nowStamp(),
                    style: const TextStyle(
                      color: Color(0xFFB6C2CF),
                      fontSize: 10,
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    '详细数据导出',
                    style: TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
                  ),
                ],
              ),
            ],
          ),
          const SizedBox(height: 16),
          const Divider(height: 1, color: Color(0x33FFFFFF)),
          const SizedBox(height: 16),
          _sharePanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  '站点数据统计',
                  style: TextStyle(
                    color: Color(0xFFF8FAFC),
                    fontSize: 16,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 4),
                const Text(
                  '聚合当前总仪表盘筛选范围内的最新数据',
                  style: TextStyle(color: Color(0xFFB6C2CF), fontSize: 10),
                ),
                const SizedBox(height: 14),
                _shareMetricGrid(summaryMetrics, columns: 3),
              ],
            ),
          ),
          if (_config.showRankPanel && sites.isNotEmpty) ...[
            const SizedBox(height: 12),
            _shareRankPanel(),
          ],
          if (shownSites.isNotEmpty) ...[
            const SizedBox(height: 16),
            const Text(
              '分站点用户档案',
              style: TextStyle(
                color: Color(0xFFF8FAFC),
                fontSize: 16,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: 3),
            const Text(
              '用户名、UID、等级等来自 user_profiles 最新真实快照',
              style: TextStyle(color: Color(0xFF7F8B98), fontSize: 10),
            ),
            const SizedBox(height: 10),
            for (var i = 0; i < shownSites.length; i++) ...[
              _shareSiteCard(shownSites[i], i),
              if (i != shownSites.length - 1) const SizedBox(height: 8),
            ],
            if (sites.length > shownSites.length)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  '及其它 ${sites.length - shownSites.length} 站',
                  style: const TextStyle(
                    color: Color(0xFF7F8B98),
                    fontSize: 10,
                  ),
                ),
              ),
          ],
          const SizedBox(height: 16),
          const Divider(height: 1, color: Color(0x33FFFFFF)),
          const SizedBox(height: 10),
          Row(
            children: [
              const Text(
                'Generated by PT Patronus',
                style: TextStyle(color: Color(0xFFB6C2CF), fontSize: 10),
              ),
              const Spacer(),
              Text(
                '${sites.length} sites · ${_nowStamp()}',
                style: const TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _sharePanel({required Widget child, Color? color}) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: color ?? const Color(0xFF1E293B),
        borderRadius: BorderRadius.circular(8),
      ),
      child: child,
    );
  }

  Widget _shareMetricGrid(List<ShareMetric> metrics, {required int columns}) {
    if (metrics.isEmpty) {
      return const Text(
        '暂无可导出字段',
        style: TextStyle(color: Color(0xFF7F8B98), fontSize: 11),
      );
    }
    final rows = metrics.chunked(columns);
    return Column(
      children: [
        for (var rowIndex = 0; rowIndex < rows.length; rowIndex++)
          Padding(
            padding: EdgeInsets.only(
              bottom: rowIndex == rows.length - 1 ? 0 : 10,
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final metric in rows[rowIndex])
                  Expanded(child: _shareMetric(metric)),
                for (var i = rows[rowIndex].length; i < columns; i++)
                  const Expanded(child: SizedBox()),
              ],
            ),
          ),
      ],
    );
  }

  Widget _shareMetric(ShareMetric metric) {
    return Padding(
      padding: const EdgeInsets.only(right: 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            metric.label,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
          ),
          const SizedBox(height: 3),
          Text(
            metric.value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              color: metric.color,
              fontSize: 13,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }

}
