// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _DashboardPageState 的分享榜单区：榜单面板/网格/单元格。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {
  Widget _shareRankPanel() {
    final ranks = <ShareRank?>[
      if (_config.showUpload)
        _shareRank(
          '上传',
          const Color(0xFF62D48E),
          (s) => s.uploaded,
          (s) => _fmtBytes(s.uploaded),
        ),
      if (_config.showDownload)
        _shareRank(
          '下载',
          const Color(0xFFFF6B6B),
          (s) => s.downloaded,
          (s) => _fmtBytes(s.downloaded),
        ),
      if (_config.showUploadCount)
        _shareRank(
          '发布',
          const Color(0xFFFFC857),
          (s) => s.uploads,
          (s) => '${s.uploads}',
        ),
      if (_config.showSeedingCount)
        _shareRank(
          '做种',
          const Color(0xFF45C4F0),
          (s) => s.seeding,
          (s) => '${s.seeding}',
        ),
      if (_config.showRatio)
        _shareRank(
          '分享率',
          const Color(0xFF45C4F0),
          (s) => s.ratio,
          (s) => _fmtRatio(s.ratio),
        ),
      if (_config.showBonus)
        _shareRank(
          '积分',
          const Color(0xFFB98CFF),
          (s) => s.bonus,
          (s) => _fmtCompact(s.bonus),
        ),
      if (_config.showBonusPerHour)
        _shareRank(
          '每小时积分',
          const Color(0xFFFF9F43),
          (s) => s.bonusPerHour,
          (s) => _fmtCompact(s.bonusPerHour),
        ),
    ].whereType<ShareRank>().take(6).toList();
    if (ranks.isEmpty) return const SizedBox.shrink();
    return _sharePanel(
      color: const Color(0xFF263238),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            '冠军排行',
            style: TextStyle(
              color: Color(0xFFF8FAFC),
              fontSize: 16,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 12),
          _shareRankGrid(ranks),
        ],
      ),
    );
  }

  Widget _shareRankGrid(List<ShareRank> ranks) {
    final rows = ranks.chunked(3);
    return Column(
      children: [
        for (var rowIndex = 0; rowIndex < rows.length; rowIndex++)
          Padding(
            padding: EdgeInsets.only(
              bottom: rowIndex == rows.length - 1 ? 0 : 12,
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final rank in rows[rowIndex])
                  Expanded(child: _shareRankCell(rank)),
                for (var i = rows[rowIndex].length; i < 3; i++)
                  const Expanded(child: SizedBox()),
              ],
            ),
          ),
      ],
    );
  }

  ShareRank? _shareRank(
    String label,
    Color color,
    num Function(SiteStatItem) score,
    String Function(SiteStatItem) value,
  ) {
    final site = _topSiteBy(score);
    if (site == null) return null;
    return ShareRank(label, value(site), _shareDisplaySiteName(site), color);
  }

  Widget _shareRankCell(ShareRank rank) {
    return Padding(
      padding: const EdgeInsets.only(right: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                  color: rank.color,
                  shape: BoxShape.circle,
                ),
              ),
              const SizedBox(width: 5),
              Expanded(
                child: Text(
                  rank.label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(color: Color(0xFF7F8B98), fontSize: 9),
                ),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            rank.value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              color: rank.color,
              fontSize: 13,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            rank.siteName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Color(0xFFB6C2CF), fontSize: 9),
          ),
        ],
      ),
    );
  }
}
