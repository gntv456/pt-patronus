// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// SaveptPage 的展示层扩展（无 setState 段；从 savept_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/savept/savept_page.dart';

extension on _SaveptPageState {
  String _siteTypeLabel(String t) => switch (t) {
    'internal' => '内站',
    'external' => '外站',
    _ => '未知',
  };

  String _statusLabel(SiteHealthStatus s) => switch (s) {
    SiteHealthStatus.healthy => '健康',
    SiteHealthStatus.critical => '病危',
    SiteHealthStatus.closed => '死亡',
    SiteHealthStatus.unknown => '未知',
  };

  String _matchLabel(MatchType m) => switch (m) {
    MatchType.urlExact || MatchType.domainExact => '已拥有',
    MatchType.nameExact || MatchType.nameFuzzy => '可添加',
    MatchType.none => '未收录',
  };

  Color _healthColor(SiteHealthStatus s) => switch (s) {
    SiteHealthStatus.healthy => const Color(0xFF22C55E),
    SiteHealthStatus.critical => const Color(0xFFEF4444),
    SiteHealthStatus.closed => const Color(0xFF94A3B8),
    SiteHealthStatus.unknown => const Color(0xFF94A3B8),
  };

  bool _isTodayAnniv(String text) => text.contains('今天') || text.contains('0天');

  String _annivDisplay(String text) {
    if (text.contains('今天站庆') || text == '0天') return '🎉 今天站庆！';
    return text;
  }

  bool _matches(SaveptSiteCard c) {
    if (_keyword.isNotEmpty) {
      final kw = [
        c.name,
        c.search,
        c.year,
        c.statusText,
        _siteTypeLabel(c.siteType),
      ].join(' ').toLowerCase();
      if (!kw.contains(_keyword.toLowerCase())) return false;
    }
    if (_year != '全部' && c.year != _year) return false;
    if (_mpStatus != '全部') {
      final chip = switch (c.matchType) {
        MatchType.urlExact || MatchType.domainExact => 'owned',
        MatchType.nameExact || MatchType.nameFuzzy => 'available',
        MatchType.none => 'unsupported',
      };
      if (chip != _mpStatus) return false;
    }
    if (_status != '全部' && _statusLabel(c.status) != _status) return false;
    if (_type != '全部' && _siteTypeLabel(c.siteType) != _type) return false;
    if (_annivOnly &&
        (c.anniversaryText.isEmpty || c.status == SiteHealthStatus.closed)) {
      return false;
    }
    return true;
  }

  // 品牌头部：心跳脉冲图标 + 标题 + 副标题（对齐 Web Savept.vue 的 spt-pulse 头部）。
  Widget _headerTitle() {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        ScaleTransition(
          scale: Tween(
            begin: 0.82,
            end: 1.18,
          ).animate(CurvedAnimation(parent: _pulse, curve: Curves.easeInOut)),
          child: const Icon(Icons.favorite, color: Color(0xFFEF4444), size: 18),
        ),
        const SizedBox(width: 8),
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'PT监护室',
              style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
            ),
            Text(
              '站点运行状态实时监护',
              style: TextStyle(
                fontSize: 11,
                color: context.dimText,
                height: 1.1,
              ),
            ),
          ],
        ),
      ],
    );
  }

  // 筛选元信息：显示 X / Y 个站点 + 抓取时间（对齐 Web 的 spt-filters__meta）。
  Widget _filterMeta() {
    final d = _data;
    if (d == null) return const SizedBox.shrink();
    final fetched = _fmtFetched(d.fetchedAt);
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Text(
        '显示 ${_filtered.length} / ${d.cards.length} 个站点${fetched.isNotEmpty ? ' · $fetched 更新' : ''}',
        style: TextStyle(fontSize: 11, color: context.dimText),
      ),
    );
  }

  String _fmtFetched(String s) {
    final d = DateTime.tryParse(s);
    if (d == null) return '';
    String p(int n) => n.toString().padLeft(2, '0');
    return '${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  List<Widget> _groupedEntries() {
    final entries = _grouped.entries.toList()
      ..sort((a, b) {
        if (a.key == '未知') return 1;
        if (b.key == '未知') return -1;
        return b.key.compareTo(a.key);
      });
    return entries
        .map(
          (e) => Container(
            margin: const EdgeInsets.only(bottom: 8),
            child: GlassCard(
              padding: EdgeInsets.zero,
              child: ExpansionTile(
                shape: const Border(),
                collapsedBackgroundColor: Colors.transparent,
                backgroundColor: Colors.transparent,
                title: Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 10,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: PTColors.primary.withValues(alpha: 0.1),
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: Text(
                        e.key,
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                          color: PTColors.primary,
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Text(
                      '${e.key == '未知' ? '' : '年'}建站 · ${e.value.length} 个',
                      style: TextStyle(fontSize: 12, color: context.dimText),
                    ),
                  ],
                ),
                initiallyExpanded: true,
                childrenPadding: const EdgeInsets.fromLTRB(8, 0, 8, 8),
                children: [_siteGrid(e.value)],
              ),
            ),
          ),
        )
        .toList();
  }

  // 站点卡片响应式网格：宽屏 4 列→900 3 列→600 2 列→窄屏 1 列（对齐 Web .spt-grid 断点）。
  // 用 LayoutBuilder+Wrap（非滚动）而非 GridView，避免在主 ListView 内嵌套纵向滚动报错。
  Widget _siteGrid(List<SaveptSiteCard> items) {
    return LayoutBuilder(
      builder: (ctx, c) {
        // 单卡最小宽 340、最多 3 列：卡片更宽，顶部标签行（类型·存活·站庆·抢救）一行排得下，
        // 不再被窄卡挤成多行。窄屏仍 1 列全宽。
        final cols = ptAutoColumns(c.maxWidth, 340, minColumns: 1).clamp(1, 3);
        const gap = 8.0;
        final w = cols <= 1
            ? c.maxWidth
            : (c.maxWidth - gap * (cols - 1)) / cols;
        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: items
              .map(
                (card) => SizedBox(
                  width: w,
                  child: _SiteCard(
                    card,
                    _siteTypeLabel,
                    _statusLabel,
                    _matchLabel,
                    _healthColor,
                    _isTodayAnniv,
                    _annivDisplay,
                  ),
                ),
              )
              .toList(),
        );
      },
    );
  }

  // 公告区：宽屏 2 列、窄屏 1 列（对齐 Web .spt-notices repeat(2,1fr)→768px 1fr）。
  Widget _noticesGrid() {
    final sections = _noticeSections.where((n) => n.items.isNotEmpty).toList();
    if (sections.isEmpty) return const SizedBox.shrink();
    return LayoutBuilder(
      builder: (ctx, c) {
        final cols = isWideScreen(context) && c.maxWidth >= 560 ? 2 : 1;
        const gap = 8.0;
        final w = cols <= 1 ? c.maxWidth : (c.maxWidth - gap) / 2;
        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: sections
              .map((n) => SizedBox(width: w, child: _noticeCard(n)))
              .toList(),
        );
      },
    );
  }

  Widget _noticeCard(_NoticeSection n) {
    final color = switch (n.key) {
      'critical' => const Color(0xFFEF4444),
      'success' => const Color(0xFF22C55E),
      'info' => const Color(0xFFF59E0B),
      _ => const Color(0xFF94A3B8),
    };
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            n.title,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: color,
            ),
          ),
          const SizedBox(height: 6),
          ...n.items.map(
            (t) => Padding(
              padding: const EdgeInsets.only(top: 4),
              child: _noticeRichText(t),
            ),
          ),
        ],
      ),
    );
  }

  // 公告正文：按站点名切片，命中站点名着色成 inline tag（移植自 Web utils/savept.ts 的 tokenizeNoticeText）。
  Widget _noticeRichText(String text) {
    final tokens = _tokenizeNoticeText(text, _siteNames);
    if (tokens.isEmpty) {
      return Text(text, style: const TextStyle(fontSize: 12, height: 1.5));
    }
    return Text.rich(
      TextSpan(
        style: const TextStyle(fontSize: 12, height: 1.5),
        children: tokens
            .map(
              (t) => t.isSite
                  ? TextSpan(
                      text: t.text,
                      style: const TextStyle(
                        color: Color(0xFF6C5CE7),
                        fontWeight: FontWeight.w600,
                      ),
                    )
                  : TextSpan(text: t.text),
            )
            .toList(),
      ),
    );
  }

  Widget _banner(String text, Color color) {
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: color.withValues(alpha: 0.3)),
      ),
      child: Row(
        children: [
          Icon(Icons.warning_amber_rounded, color: color, size: 18),
          const SizedBox(width: 8),
          Expanded(
            child: Text(text, style: TextStyle(fontSize: 12, color: color)),
          ),
        ],
      ),
    );
  }
}
