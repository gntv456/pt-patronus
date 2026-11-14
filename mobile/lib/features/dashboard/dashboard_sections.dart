// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _DashboardPageState 的站点区/榜单/引导/指标渲染扩展。
// （无 setState 段；从 dashboard_page.dart 拆出，行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {

  Widget? _rankRow(
    String label,
    IconData icon,
    SiteStatItem? site,
    String Function(SiteStatItem) value,
    bool compact,
  ) {
    if (site == null) return null;
    return Row(
      children: [
        Icon(icon, size: compact ? 13 : 15, color: PTColors.primary),
        const SizedBox(width: 4),
        SizedBox(
          width: compact ? 42 : 52,
          child: Text(
            label,
            style: TextStyle(color: context.dimText, fontSize: compact ? 11 : 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        Expanded(
          child: Text(
            value(site),
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: compact ? 11 : 12,
            ),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        if (_config.showSiteName) ...[
          const SizedBox(width: 4),
          Flexible(
            child: Text(
              _displaySiteName(site),
              style: TextStyle(color: context.dimText, fontSize: compact ? 10 : 11),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ],
    );
  }

  Widget _rankPanel(bool compact) {
    final rows = <Widget?>[
      if (_config.showUpload)
        _rankRow(
          '上传',
          Icons.upload,
          _topSiteBy((s) => s.uploaded),
          (s) => _fmtBytes(s.uploaded),
          compact,
        ),
      if (_config.showDownload)
        _rankRow(
          '下载',
          Icons.download,
          _topSiteBy((s) => s.downloaded),
          (s) => _fmtBytes(s.downloaded),
          compact,
        ),
      if (_config.showUploadCount)
        _rankRow(
          '发布',
          Icons.publish,
          _topSiteBy((s) => s.uploads),
          (s) => '${s.uploads}',
          compact,
        ),
      if (_config.showSeedingCount)
        _rankRow(
          '做种',
          Icons.park,
          _topSiteBy((s) => s.seeding),
          (s) => '${s.seeding}',
          compact,
        ),
      if (_config.showRatio)
        _rankRow(
          '分享率',
          Icons.equalizer,
          _topSiteBy((s) => s.ratio),
          (s) => _fmtRatio(s.ratio),
          compact,
        ),
      if (_config.showBonus)
        _rankRow(
          '积分',
          Icons.star,
          _topSiteBy((s) => s.bonus),
          (s) => _fmtCompact(s.bonus),
          compact,
        ),
      if (_config.showBonusPerHour)
        _rankRow(
          '小时积分',
          Icons.bolt,
          _topSiteBy((s) => s.bonusPerHour),
          (s) => _fmtCompact(s.bonusPerHour),
          compact,
        ),
    ].whereType<Widget>().toList();
    if (rows.isEmpty) {
      return Text(
        '暂无排行',
        style: TextStyle(color: context.dimText, fontSize: 12),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children:
          rows.expand((w) => [w, SizedBox(height: compact ? 4 : 7)]).toList()
            ..removeLast(),
    );
  }

  Widget _statLabelValue(String label, String value) {
    return Row(
      children: [
        Expanded(
          child: Text(
            '$label：',
            style: TextStyle(color: context.dimText, fontSize: 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        Flexible(
          child: Text(
            value,
            textAlign: TextAlign.right,
            style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }

  Widget _totalStatsOverviewCard(BuildContext context) {
    final sites = _visibleSites();
    final compact = _config.compactMode || !isWideScreen(context);
    final gap = compact ? 4.0 : 8.0;
    final totalUploaded = _sumInt(sites, (s) => s.uploaded);
    final totalDownloaded = _sumInt(sites, (s) => s.downloaded);
    final totalSeeding = _sumInt(sites, (s) => s.seeding);
    final totalSeedingSize = _sumInt(sites, (s) => s.seedingSize);
    final totalUploads = _sumInt(sites, (s) => s.uploads);
    final totalBonus = _sumDouble(sites, (s) => s.bonus);
    final totalBonusPerHour = _sumDouble(sites, (s) => s.bonusPerHour);
    final ratio = _totalRatio(sites);

    return GlassCard(
      padding: EdgeInsets.all(compact ? 12 : 16),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              children:
                  [
                      if (_config.showTotalSites)
                        _statLabelValue('总站点数', '${sites.length}'),
                      if (_config.showUploadCount)
                        _statLabelValue('总发布数', '$totalUploads'),
                      if (_config.showUpload)
                        _statLabelValue('总上传量', _fmtBytes(totalUploaded)),
                      if (_config.showDownload)
                        _statLabelValue('总下载量', _fmtBytes(totalDownloaded)),
                      if (_config.showSeedingCount)
                        _statLabelValue('总做种数', '$totalSeeding'),
                      if (_config.showSeedingVolume)
                        _statLabelValue('总做种量', _fmtBytes(totalSeedingSize)),
                      if (_config.showBonus)
                        _statLabelValue('总积分值', _fmtCompact(totalBonus)),
                      if (_config.showBonusPerHour)
                        _statLabelValue('小时积分', _fmtCompact(totalBonusPerHour)),
                      if (_config.showRatio)
                        _statLabelValue('总分享率', _fmtRatio(ratio)),
                    ].expand((w) => [w, SizedBox(height: gap)]).toList()
                    ..removeLast(),
            ),
          ),
          if (_config.showRankPanel) ...[
            Container(
              width: 1,
              height: compact ? 150 : 190,
              margin: EdgeInsets.symmetric(horizontal: compact ? 8 : 12),
              color: Theme.of(
                context,
              ).colorScheme.outlineVariant.withValues(alpha: 0.35),
            ),
            Expanded(child: _rankPanel(compact)),
          ],
        ],
      ),
    );
  }

  // 5 张指标卡（上传/下载/做种/做种体积/魔力），对齐 Web metric-grid（auto-fit minmax 210）。
  // 每卡内嵌近 14 天 sparkline；上传/下载额外显日环比徽标。
  // 窄屏 2 列紧凑（minItemWidth 150）/ 宽屏对齐 Web（210）自然撑高——弃固定 aspect-ratio
  // （曾与 minItemWidth:200 合用致窄屏单列每卡高 ~370px、Spacer 撑出大块留白）。
  Widget _metricGrid(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: EdgeInsets.only(
            left: 2,
            bottom: isWideScreen(context) ? 8 : 6,
          ),
          child: Text(
            '站点数据统计',
            style: TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: isWideScreen(context) ? 15 : 14,
            ),
          ),
        ),
        _totalStatsOverviewCard(context),
      ],
    );
  }

  // 首登向导：无站点且无下载器时置顶，3 步引导（加站点→配下载器→同步），每步直达对应页。
  Widget _onboardingCard(BuildContext context) {
    final c = HomeShellScope.of(context);
    int indexOf(String glyph) => navItems.indexWhere((n) => n.glyph == glyph);
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                Icons.rocket_launch,
                color: PTColors.primary,
                size: 20,
              ),
              const SizedBox(width: 8),
              const Text(
                '欢迎使用 PTPatronus',
                style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '跟着 3 步，开启账号保号自动化：',
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
          const SizedBox(height: 10),
          _onboardStep(
            '1',
            '添加 PT 站点',
            '粘贴 cookie 或 WebView 登录',
            Icons.add_link,
            () => c.select(indexOf('sites')),
          ),
          _onboardStep(
            '2',
            '配置下载器',
            'qBittorrent / Transmission',
            Icons.download_outlined,
            () => c.select(indexOf('downloaders')),
          ),
          _onboardStep('3', '同步统计', '一键拉取，签到 / 辅种自动运行', Icons.sync, _refresh),
        ],
      ),
    );
  }

  Widget _onboardStep(
    String no,
    String title,
    String desc,
    IconData icon,
    VoidCallback onTap,
  ) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: InkWell(
        borderRadius: BorderRadius.circular(10),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
          child: Row(
            children: [
              Container(
                width: 26,
                height: 26,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: PTColors.primary.withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(
                  no,
                  style: TextStyle(
                    color: PTColors.primary,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Icon(icon, size: 18, color: context.dimText),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: TextStyle(fontWeight: FontWeight.w600),
                    ),
                    Text(
                      desc,
                      style: TextStyle(fontSize: 11, color: context.dimText),
                    ),
                  ],
                ),
              ),
              Icon(Icons.chevron_right, color: context.dimText, size: 18),
            ],
          ),
        ),
      ),
    );
  }

  Widget _kvCell(String label, String value, Color color) => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: context.dimText.withValues(alpha: 0.08),
          borderRadius: BorderRadius.circular(8),
        ),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(label, style: TextStyle(fontSize: 11, color: context.dimText)),
          const SizedBox(height: 3),
          Text(value,
              style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700, color: color)),
        ]),
      );

  Widget _resBar(String label, double pct, String detail) {
    final p = pct.clamp(0.0, 100.0);
    final color = p >= 90
        ? const Color(0xffe74c3c)
        : p >= 80
            ? const Color(0xfff39c12)
            : const Color(0xff16a34a);
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
        Text(label, style: TextStyle(fontSize: 12, color: context.dimText)),
        Text('${p.toStringAsFixed(0)}%',
            style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: color)),
      ]),
      const SizedBox(height: 4),
      ClipRRect(
        borderRadius: BorderRadius.circular(999),
        child: LinearProgressIndicator(
          value: p / 100,
          minHeight: 7,
          backgroundColor: context.dimText.withValues(alpha: 0.15),
          valueColor: AlwaysStoppedAnimation<Color>(color),
        ),
      ),
      Text(detail, style: TextStyle(fontSize: 10, color: context.dimText)),
    ]);
  }
}
