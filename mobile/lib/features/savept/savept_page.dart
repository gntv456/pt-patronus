import 'dart:math' as math;
import 'package:confetti/confetti.dart';
import 'package:flutter/material.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/api/savept.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:url_launcher/url_launcher.dart';

/// PT监护室页：聚合 savept.icu 站点健康数据（ICU 仪表盘风格）。
/// 迁移自 MoviePilot savept 插件 Page.vue，对齐 Web Savept.vue：品牌头部脉冲 +
/// 紧凑生命体征 + 筛选 + 结构化公告（站点名高亮）+ 年份分组折叠 + 状态色卡 +
/// 今天站庆彩带 + 回到顶部。
class SaveptPage extends StatefulWidget {
  const SaveptPage({super.key});
  @override
  State<SaveptPage> createState() => _SaveptPageState();
}

class _SaveptPageState extends State<SaveptPage>
    with SingleTickerProviderStateMixin {
  final _api = SaveptApi();
  final _searchCtrl = TextEditingController();
  final ScrollController _scroll = ScrollController();
  late final AnimationController _pulse;
  SaveptResult? _data;
  bool _loading = true;
  String? _error;
  bool _showBackToTop = false;

  // 筛选
  String _keyword = '';
  String _year = '全部';
  String _mpStatus = '全部'; // 全部/owned/available/unsupported
  String _status = '全部'; // 全部/健康/病危/死亡
  String _type = '全部'; // 全部/内站/外站
  bool _annivOnly = false;

  @override
  void initState() {
    super.initState();
    _load();
    _pulse = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 900),
    )..repeat(reverse: true);
    _scroll.addListener(() {
      final show = _scroll.hasClients && _scroll.offset > 240;
      if (show != _showBackToTop) setState(() => _showBackToTop = show);
    });
  }

  @override
  void dispose() {
    _pulse.dispose();
    _searchCtrl.dispose();
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _data = await _api.get(refresh: refresh);
      if (refresh && mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('数据已刷新')));
      }
    } catch (e) {
      _error = e.toString();
      if (refresh && mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('刷新失败：$e')));
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _clearFilters() {
    _searchCtrl.clear();
    setState(() {
      _keyword = '';
      _year = '全部';
      _mpStatus = '全部';
      _status = '全部';
      _type = '全部';
      _annivOnly = false;
    });
  }

  void _scrollToTop() => _scroll.animateTo(
    0,
    duration: const Duration(milliseconds: 300),
    curve: Curves.easeOut,
  );

  // ── 派生数据 ──
  List<String> get _yearOptions {
    final years = <String>{};
    for (final c in _data?.cards ?? const <SaveptSiteCard>[]) {
      if (c.year.isNotEmpty && c.year != '未知') years.add(c.year);
    }
    final list = ['全部', ...years.toList()..sort()];
    if ((_data?.cards ?? const []).any(
          (c) => c.year.isEmpty || c.year == '未知',
        ) &&
        !list.contains('未知')) {
      list.add('未知');
    }
    return list;
  }

  // 公告站点名高亮用：已知站点名集合，长名优先匹配（避免短名误命中）。
  List<String> get _siteNames =>
      (_data?.cards ?? const <SaveptSiteCard>[])
          .map((c) => c.name.trim())
          .where((s) => s.isNotEmpty)
          .toSet()
          .toList()
        ..sort((a, b) => b.length.compareTo(a.length));

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

  // 公告 4 分区
  List<_NoticeSection> get _noticeSections {
    final g = <String, List<String>>{
      'critical': [],
      'success': [],
      'info': [],
      'error': [],
    };
    for (final a in _data?.alerts ?? const <SaveptAlert>[]) {
      final level = switch (a.type) {
        AlertType.danger => 'critical',
        AlertType.obituary => 'error',
        AlertType.warning => 'success',
        AlertType.info || AlertType.celebration => 'info',
      };
      final t = a.content.trim();
      if (t.isNotEmpty) g[level]!.add(t);
    }
    return [
      ('critical', '⚠️ 病危通知', g['critical']!),
      ('success', '✅ 恢复公告', g['success']!),
      ('info', '🎉 站庆预告', g['info']!),
      ('error', '🕯️ 死亡讣告', g['error']!),
    ].map((e) => _NoticeSection(e.$1, e.$2, e.$3)).toList();
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

  List<SaveptSiteCard> get _filtered =>
      (_data?.cards ?? const []).where(_matches).toList();

  // 按年份分组
  Map<String, List<SaveptSiteCard>> get _grouped {
    final m = <String, List<SaveptSiteCard>>{};
    for (final c in _filtered) {
      final y = (c.year.isEmpty || c.year == '未知') ? '未知' : c.year;
      m.putIfAbsent(y, () => []).add(c);
    }
    return m;
  }

  @override
  Widget build(BuildContext context) {
    final s = _data?.summary;
    return PTPageScaffold(
      title: _headerTitle(),
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新数据',
          loading: _loading,
          onPressed: _loading ? null : () => _load(refresh: true),
        ),
      ],
      search: PTSearchConfig(
        controller: _searchCtrl,
        onChanged: (v) => setState(() => _keyword = v),
        hintText: '搜索站点名称...',
      ),
      filterDimensions: _filterDimensions(),
      filterOnClearAll: _clearFilters,
      floatingActionButton: _showBackToTop
          ? FloatingActionButton(
              mini: true,
              onPressed: _scrollToTop,
              child: const Icon(Icons.arrow_upward),
            )
          : null,
      body: RefreshIndicator(
        onRefresh: () => _load(refresh: true),
        child: _loading
            ? const PTLoadingView()
            : _error != null
            ? _Empty(text: _error!, onRetry: _load)
            : _data == null
            ? const _Empty(text: '无数据')
            : ListView(
                controller: _scroll,
                padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
                children: [
                  if (_data!.stale && _data!.notice.isNotEmpty)
                    _banner(_data!.notice, const Color(0xFFF39C12)),
                  if (s != null) ...[_vitals(s), const SizedBox(height: 10)],
                  _filterMeta(),
                  const SizedBox(height: 12),
                  _noticesGrid(),
                  const SizedBox(height: 12),
                  ..._groupedEntries(),
                  if (_filtered.isEmpty)
                    Padding(
                      padding: const EdgeInsets.symmetric(vertical: 48),
                      child: Column(
                        children: [
                          Center(
                            child: Text(
                              '没有匹配的站点',
                              style: TextStyle(color: context.dimText),
                            ),
                          ),
                          const SizedBox(height: 12),
                          Center(
                            child: TextButton(
                              onPressed: _clearFilters,
                              child: const Text('清除筛选'),
                            ),
                          ),
                        ],
                      ),
                    ),
                ],
              ),
      ),
    );
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

  // 体征点按 = 快捷筛选：点某项设对应维度，重点同一项或点「监护中」清空。
  void _toggleStatus(String v) =>
      setState(() => _status = (_status == v) ? '全部' : v);
  void _toggleType(String v) => setState(() => _type = (_type == v) ? '全部' : v);
  void _toggleMp(String v) =>
      setState(() => _mpStatus = (_mpStatus == v) ? '全部' : v);

  Widget _vitals(SaveptSummary s) {
    // 「监护中」高亮 ⇔ 当前无任何筛选（年份/关键词不计，因其另有入口）。
    final noFilter =
        _status == '全部' &&
        _type == '全部' &&
        _mpStatus == '全部' &&
        _year == '全部' &&
        !_annivOnly;
    final items = <PTStat>[
      PTStat(
        value: '${s.total}',
        label: '监护中',
        color: PTColors.primary,
        selected: noFilter,
        onTap: () => setState(() {
          _status = '全部';
          _type = '全部';
          _mpStatus = '全部';
          _year = '全部';
          _annivOnly = false;
        }),
      ),
      PTStat(
        value: '${s.healthy}',
        label: '健康',
        color: const Color(0xFF22C55E),
        selected: _status == '健康',
        onTap: () => _toggleStatus('健康'),
      ),
      PTStat(
        value: '${s.mpOwned}',
        label: '已拥有',
        color: const Color(0xFF16A34A),
        selected: _mpStatus == 'owned',
        onTap: () => _toggleMp('owned'),
      ),
      PTStat(
        value: '${s.critical}',
        label: '病危',
        color: const Color(0xFFEF4444),
        selected: _status == '病危',
        onTap: () => _toggleStatus('病危'),
      ),
      PTStat(
        value: '${s.closed}',
        label: '已关站',
        color: const Color(0xFF94A3B8),
        selected: _status == '死亡',
        onTap: () => _toggleStatus('死亡'),
      ),
      PTStat(
        value: '${s.todayAnniv}',
        label: '站庆',
        color: const Color(0xFFF59E0B),
        selected: _annivOnly,
        onTap: () => setState(() => _annivOnly = !_annivOnly),
      ),
      PTStat(
        value: '${s.internal}',
        label: '内站',
        color: const Color(0xFF3B82F6),
        selected: _type == '内站',
        onTap: () => _toggleType('内站'),
      ),
      PTStat(
        value: '${s.external}',
        label: '外站',
        color: const Color(0xFF8B5CF6),
        selected: _type == '外站',
        onTap: () => _toggleType('外站'),
      ),
    ];
    // 单行指标条：宽屏 Row 均匀分布 + 竖分隔（监护仪风格）/ 窄屏横向滚动，永不折行。
    return GlassCard(
      padding: const EdgeInsets.symmetric(vertical: 6, horizontal: 4),
      child: PTStatStrip(
        items: items,
        dividers: isWideScreen(context),
        height: 44,
      ),
    );
  }

  // 5 维筛选（收进顶栏筛选按钮 → showMultiFilterSheet）：状态/类型/年份/MP 单选 +
  // 近期站庆开关。每次构建传新 List（廉价值对象）；isDefault 驱动顶栏激活徽标。
  List<PTFilterDimension> _filterDimensions() {
    String mpLabel(String v) => switch (v) {
      'owned' => '已拥有',
      'available' => '可添加',
      'unsupported' => '未收录',
      _ => '全部',
    };
    return [
      PTFilterDimension.choice(
        label: '健康状态',
        selected: _status,
        isDefault: _status == '全部',
        onChanged: (v) => setState(() => _status = (v as String?) ?? '全部'),
        options: const [
          PTFilterOption<Object?>(value: '全部', label: '全部'),
          PTFilterOption<Object?>(value: '健康', label: '健康'),
          PTFilterOption<Object?>(value: '病危', label: '病危'),
          PTFilterOption<Object?>(value: '死亡', label: '死亡'),
        ],
      ),
      PTFilterDimension.choice(
        label: '站点类型',
        selected: _type,
        isDefault: _type == '全部',
        onChanged: (v) => setState(() => _type = (v as String?) ?? '全部'),
        options: const [
          PTFilterOption<Object?>(value: '全部', label: '全部'),
          PTFilterOption<Object?>(value: '内站', label: '内站'),
          PTFilterOption<Object?>(value: '外站', label: '外站'),
        ],
      ),
      PTFilterDimension.toggle(
        label: '仅看近期站庆',
        value: _annivOnly,
        onChanged: (v) => setState(() => _annivOnly = v),
      ),
      PTFilterDimension.choice(
        label: '建站年份',
        selected: _year,
        isDefault: _year == '全部',
        onChanged: (v) => setState(() => _year = (v as String?) ?? '全部'),
        options: _yearOptions
            .map((y) => PTFilterOption<Object?>(value: y, label: y))
            .toList(),
      ),
      PTFilterDimension.choice(
        label: 'MP 状态',
        selected: _mpStatus,
        isDefault: _mpStatus == '全部',
        onChanged: (v) => setState(() => _mpStatus = (v as String?) ?? '全部'),
        options: [
          const PTFilterOption<Object?>(value: '全部', label: '全部'),
          PTFilterOption<Object?>(value: 'owned', label: mpLabel('owned')),
          PTFilterOption<Object?>(
            value: 'available',
            label: mpLabel('available'),
          ),
          PTFilterOption<Object?>(
            value: 'unsupported',
            label: mpLabel('unsupported'),
          ),
        ],
      ),
    ];
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

class _NoticeSection {
  final String key;
  final String title;
  final List<String> items;
  _NoticeSection(this.key, this.title, this.items);
}

/// 单站点卡片：今天站庆时叠加 confetti 彩带；死亡站灰度图标 + 状态色卡底。
class _SiteCard extends StatefulWidget {
  final SaveptSiteCard card;
  final String Function(String) siteTypeLabel;
  final String Function(SiteHealthStatus) statusLabel;
  final String Function(MatchType) matchLabel;
  final Color Function(SiteHealthStatus) healthColor;
  final bool Function(String) isTodayAnniv;
  final String Function(String) annivDisplay;
  const _SiteCard(
    this.card,
    this.siteTypeLabel,
    this.statusLabel,
    this.matchLabel,
    this.healthColor,
    this.isTodayAnniv,
    this.annivDisplay,
  );

  @override
  State<_SiteCard> createState() => _SiteCardState();
}

class _SiteCardState extends State<_SiteCard> {
  late ConfettiController _confetti;
  bool get _isToday => widget.isTodayAnniv(widget.card.anniversaryText);

  @override
  void initState() {
    super.initState();
    _confetti = ConfettiController(duration: const Duration(seconds: 4));
    if (_isToday) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _confetti.play();
      });
    }
  }

  @override
  void dispose() {
    _confetti.dispose();
    super.dispose();
  }

  Future<void> _launchUrl(String url) async {
    try {
      await launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('打开站点失败：$e')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.card;
    final color = widget.healthColor(c.status);
    final matchColor = switch (c.matchType) {
      MatchType.urlExact || MatchType.domainExact => const Color(0xFF16A34A),
      MatchType.nameExact || MatchType.nameFuzzy => const Color(0xFF2563EB),
      MatchType.none => const Color(0xFF64748B),
    };
    final iconWidget = Image.network(
      c.icon!,
      width: 34,
      height: 34,
      fit: BoxFit.contain,
      errorBuilder: (_, _, _) => const SizedBox(width: 34, height: 34),
    );
    return Container(
      margin: EdgeInsets.zero,
      child: Stack(
        children: [
          GlassCard(
            onTap: c.url.isNotEmpty ? () => _launchUrl(c.url) : null,
            tint: color, // 健康状态淡色底（对齐 Web 卡片 healthy/critical/closed 着色）
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // 标签行：纯彩色文字 + 「·」分隔（无底色框，最省地方；一行排不下才自然换行）。
                Builder(builder: (_) {
                  final spans = <TextSpan>[];
                  void add(String text, Color color) {
                    if (spans.isNotEmpty) spans.add(const TextSpan(text: ' · '));
                    spans.add(TextSpan(
                      text: text,
                      style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w500),
                    ));
                  }
                  add(widget.siteTypeLabel(c.siteType), context.dimText);
                  if (c.durationText.isNotEmpty) add(c.durationText, PTColors.primary);
                  if (c.anniversaryText.isNotEmpty) {
                    add(widget.annivDisplay(c.anniversaryText), const Color(0xFFF59E0B));
                  }
                  if (c.faultDuration.isNotEmpty) {
                    add('⏱ 抢救 ${c.faultDuration}', const Color(0xFFEF4444));
                  }
                  return Text.rich(
                    TextSpan(children: spans, style: const TextStyle(fontSize: 11, height: 1.3)),
                  );
                }),
                const SizedBox(height: 6),
                Row(
                  children: [
                    if (c.icon != null && c.icon!.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(right: 10),
                        child: ClipRRect(
                          borderRadius: BorderRadius.circular(8),
                          // 死亡站图标灰度（对齐 Web closed 卡 filter: grayscale(1)）
                          child: c.status == SiteHealthStatus.closed
                              ? ColorFiltered(
                                  colorFilter:
                                      const ColorFilter.matrix(<double>[
                                        0.2126,
                                        0.7152,
                                        0.0722,
                                        0,
                                        0,
                                        0.2126,
                                        0.7152,
                                        0.0722,
                                        0,
                                        0,
                                        0.2126,
                                        0.7152,
                                        0.0722,
                                        0,
                                        0,
                                        0,
                                        0,
                                        0,
                                        1,
                                        0,
                                      ]),
                                  child: iconWidget,
                                )
                              : iconWidget,
                        ),
                      ),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              CircleAvatar(radius: 4, backgroundColor: color),
                              const SizedBox(width: 6),
                              Expanded(
                                child: Text(
                                  c.name,
                                  style: const TextStyle(
                                    fontWeight: FontWeight.w600,
                                  ),
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                              Icon(
                                _statusIcon(c.status),
                                color: color,
                                size: 16,
                              ),
                            ],
                          ),
                          const SizedBox(height: 4),
                          Text(
                            '开站 ${c.openTime.isNotEmpty ? c.openTime : '未知'}${c.closeTime.isNotEmpty && c.closeTime != '未知' ? ' · 关站 ${c.closeTime}' : ''}',
                            style: TextStyle(
                              fontSize: 11,
                              color: context.dimText,
                            ),
                          ),
                          if (c.localSiteName.isNotEmpty)
                            Padding(
                              padding: const EdgeInsets.only(top: 2),
                              child: Text(
                                c.localSiteName,
                                style: const TextStyle(
                                  fontSize: 11,
                                  color: PTColors.primary,
                                ),
                                overflow: TextOverflow.ellipsis,
                              ),
                            ),
                        ],
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Row(
                  children: [
                    // MP 状态徽章带前缀图标（已拥有=check / 可添加=plus / 未收录=help）
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: matchColor.withValues(alpha: 0.18),
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(
                            _mpIcon(c.matchType),
                            size: 12,
                            color: matchColor,
                          ),
                          const SizedBox(width: 3),
                          Text(
                            widget.matchLabel(c.matchType),
                            style: TextStyle(color: matchColor, fontSize: 11),
                          ),
                        ],
                      ),
                    ),
                    if (c.domain.isNotEmpty) ...[
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          c.domain,
                          style: TextStyle(
                            color: context.dimText,
                            fontSize: 11,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ],
                  ],
                ),
              ],
            ),
          ),
          if (_isToday)
            Positioned(
              top: 0,
              left: 0,
              right: 0,
              child: ConfettiWidget(
                confettiController: _confetti,
                blastDirectionality: BlastDirectionality.explosive,
                shouldLoop: true,
                colors: const [
                  Color(0xFFF59E0B),
                  Color(0xFFFACC15),
                  Color(0xFFFB7185),
                  Color(0xFF22C55E),
                  Color(0xFF38BDF8),
                ],
                maxBlastForce: 6,
                minBlastForce: 3,
                emissionFrequency: 0.4,
                numberOfParticles: 6,
                blastDirection: math.pi / 2,
                gravity: 0.25,
              ),
            ),
        ],
      ),
    );
  }

  IconData _statusIcon(SiteHealthStatus s) => switch (s) {
    SiteHealthStatus.healthy => Icons.favorite,
    SiteHealthStatus.critical => Icons.error_outline,
    SiteHealthStatus.closed => Icons.sentiment_very_dissatisfied,
    SiteHealthStatus.unknown => Icons.help_outline,
  };

  IconData _mpIcon(MatchType m) => switch (m) {
    MatchType.urlExact || MatchType.domainExact => Icons.check_circle_outline,
    MatchType.nameExact || MatchType.nameFuzzy => Icons.add_circle_outline,
    MatchType.none => Icons.help_outline,
  };
}

class _Empty extends StatelessWidget {
  final String text;
  final VoidCallback? onRetry;
  const _Empty({required this.text, this.onRetry});
  @override
  Widget build(BuildContext context) {
    return ListView(
      children: [
        const SizedBox(height: 120),
        Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              text,
              textAlign: TextAlign.center,
              style: TextStyle(color: context.dimText),
            ),
          ),
        ),
        if (onRetry != null)
          Center(
            child: ElevatedButton(onPressed: onRetry, child: const Text('重试')),
          ),
      ],
    );
  }
}

// ── 公告正文站点名高亮分词（移植自 Web frontend/src/utils/savept.ts 的 tokenizeNoticeText）──
// 按 `|` 切段，段内用「已知站点名集合正则前缀匹配」+「已抢救/已/还有/抢救/于 动词前缀启发式」
// 找出站点名，便于在 RichText 里把站点名段着色成 inline tag。

class _NoticeToken {
  final bool isSite;
  String text;
  _NoticeToken.text(this.text) : isSite = false;
  _NoticeToken.site(String name) : isSite = true, text = name;
}

void _appendNoticeText(List<_NoticeToken> tokens, String text) {
  if (text.isEmpty) return;
  if (tokens.isNotEmpty && !tokens.last.isSite) {
    tokens.last.text += text;
    return;
  }
  tokens.add(_NoticeToken.text(text));
}

String _escapeRegExp(String s) =>
    s.replaceAll(RegExp(r'[.*+?^${}()|[\]\\]'), r'\$0');

// 检测一段公告里是否以站点名开头（含可能的「前缀：」）。
({String prefix, String siteName, String suffix})? _detectNoticeSiteName(
  String segmentText,
  List<String> siteNames,
) {
  final raw = segmentText;
  final trimmed = raw.trim();
  if (trimmed.isEmpty) return null;

  final colonMatch = RegExp(r'^(.*?[：:]\s*)(.+)$').firstMatch(raw);
  final prefix = colonMatch != null ? colonMatch.group(1)! : '';
  final body = colonMatch != null ? colonMatch.group(2)! : raw;
  final trimmedBody = body.trimLeft();
  final leadingWs = body.substring(0, body.length - trimmedBody.length);

  for (final name in siteNames) {
    if (RegExp('^${_escapeRegExp(name)}(?=\\s|\$)').hasMatch(trimmedBody)) {
      return (
        prefix: '$prefix$leadingWs',
        siteName: name,
        suffix: trimmedBody.substring(name.length),
      );
    }
  }
  // 启发式：站点名通常在「已抢救/已/还有/抢救/于」等动词前。
  final heur = RegExp(r'^(.+?)(?=\s(?:已抢救|已|还有|抢救|于))').firstMatch(trimmedBody);
  if (heur != null && heur.group(1) != null) {
    final name = heur.group(1)!.trim();
    if (name.isNotEmpty) {
      return (
        prefix: '$prefix$leadingWs',
        siteName: name,
        suffix: trimmedBody.substring(heur.group(1)!.length),
      );
    }
  }
  return null;
}

void _tokenizeNoticeSegment(
  String seg,
  List<String> siteNames,
  List<_NoticeToken> tokens,
) {
  if (seg.isEmpty) return;
  final detected = _detectNoticeSiteName(seg, siteNames);
  if (detected == null) {
    _appendNoticeText(tokens, seg);
    return;
  }
  _appendNoticeText(tokens, detected.prefix);
  tokens.add(_NoticeToken.site(detected.siteName));
  _appendNoticeText(tokens, detected.suffix);
}

// 把一条公告正文按 `|` 切段、段内探站点名，产出 token 序列（text/site 交替）。
List<_NoticeToken> _tokenizeNoticeText(String text, List<String> siteNames) {
  final content = text.trim();
  if (content.isEmpty) return const [];
  final tokens = <_NoticeToken>[];
  final parts = content.split('|');
  for (var i = 0; i < parts.length; i++) {
    if (i > 0) _appendNoticeText(tokens, ' | ');
    _tokenizeNoticeSegment(parts[i], siteNames, tokens);
  }
  return tokens;
}
