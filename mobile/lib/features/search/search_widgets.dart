// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// SearchPage 的展示层扩展（无 setState 段；从 search_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/search/search_page.dart';

extension on _SearchPageState {
  Widget _chip(String label, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(6),
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

  Widget _scopeBar(int rows, int sites, int failed) {
    if (!_searched || sites == 0) return const SizedBox.shrink();
    final okSites = sites - failed;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 4),
      child: Row(
        children: [
          Icon(Icons.travel_explore, size: 16, color: context.dimText),
          const SizedBox(width: 6),
          Expanded(
            child: Text(
              '已搜索 $sites 个站点，$okSites 个成功，命中 $rows 条',
              style: TextStyle(fontSize: 12, color: context.dimText),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }

  Widget _mediaCard(MediaInfo m) {
    return GestureDetector(
      onTap: () => Navigator.push(
        context,
        MaterialPageRoute(
          builder: (_) => MediaDetailPage(source: m.source, id: m.sourceId),
        ),
      ),
      child: GlassCard(
        padding: EdgeInsets.zero,
        child: ClipRRect(
          borderRadius: BorderRadius.circular(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Expanded(
                child: Stack(
                  fit: StackFit.expand,
                  children: [
                    MediaPoster(url: m.posterUrl, kind: m.type),
                    Positioned(
                      top: 6,
                      right: 6,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 7,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: context.mainText,
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: Text(
                          _src(m.source),
                          style: const TextStyle(
                            color: Colors.white,
                            fontSize: 11,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(8, 6, 8, 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      m.title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w600,
                        fontSize: 13,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        _typeBadge(m.type),
                        if (m.year > 0) ...[
                          const SizedBox(width: 5),
                          Text(
                            '${m.year}',
                            style: TextStyle(
                              color: context.dimText,
                              fontSize: 11,
                            ),
                          ),
                        ],
                        if (m.rating > 0) ...[
                          const SizedBox(width: 5),
                          Text(
                            '⭐${m.rating.toStringAsFixed(1)}',
                            style: const TextStyle(
                              color: Colors.orange,
                              fontSize: 11,
                            ),
                          ),
                        ],
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _typeBadge(String t) {
    final label = const {'movie': '电影', 'tv': '剧集', 'music': '音乐'}[t] ?? t;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(5),
        gradient: const LinearGradient(
          colors: [PTColors.primary, PTColors.primary2],
        ),
      ),
      child: Text(
        label,
        style: const TextStyle(
          color: Colors.white,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }


  void _onPendingKeyword() {
    final q = SearchPage.pendingKeyword.value;
    if (q == null || q.trim().isEmpty) return;
    SearchPage.pendingKeyword.value = null;
    if (!mounted) return;
    _ctrl.text = q.trim();
    _search();
  }

  void _openDetail(_TR tr) {
    if (tr.t.detailUrl.isEmpty) {
      _toast('该种子无详情页');
      return;
    }
    showAdaptiveSheet(
      context,
      title: tr.t.title,
      wideWidth: 820,
      builder: (_) => _TorrentDetailContent(
        siteId: tr.siteId,
        siteName: tr.site,
        detailUrl: tr.t.detailUrl,
        torrent: tr.t,
        onDownload: () => _download(tr.t),
      ),
    );
  }
}
