import 'dart:math' as math;
import 'package:confetti/confetti.dart';
import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/savept.dart';
import 'package:ptpatronus/theme/glass.dart';

/// PT监护室页：聚合 savept.icu 站点健康数据（ICU 仪表盘风格）。
/// 迁移自 MoviePilot savept 插件 Page.vue，针对小屏适配：紧凑生命体征 + 筛选 +
/// 结构化公告（病危/恢复/站庆/讣告）+ 年份分组折叠 + 丰富卡片 + 今天站庆彩带。
class SaveptPage extends StatefulWidget {
  const SaveptPage({super.key});
  @override
  State<SaveptPage> createState() => _SaveptPageState();
}

class _SaveptPageState extends State<SaveptPage> {
  final _api = SaveptApi();
  SaveptResult? _data;
  bool _loading = true;
  String? _error;

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
  }

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _data = await _api.get(refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  // ── 派生数据 ──
  List<String> get _yearOptions {
    final years = <String>{};
    for (final c in _data?.cards ?? const <SaveptSiteCard>[]) {
      if (c.year.isNotEmpty && c.year != '未知') years.add(c.year);
    }
    final list = ['全部', ...years.toList()..sort()];
    if ((_data?.cards ?? const []).any((c) => c.year.isEmpty || c.year == '未知') && !list.contains('未知')) {
      list.add('未知');
    }
    return list;
  }

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
      final kw = [c.name, c.year, c.statusText, _siteTypeLabel(c.siteType)].join(' ').toLowerCase();
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
    if (_annivOnly && (c.anniversaryText.isEmpty || c.status == SiteHealthStatus.closed)) return false;
    return true;
  }

  List<SaveptSiteCard> get _filtered => (_data?.cards ?? const []).where(_matches).toList();

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
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('PT监护室'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(
              icon: _loading
                  ? const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.refresh),
              onPressed: _loading ? null : () => _load(refresh: true),
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => _load(refresh: true),
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error != null
                  ? _Empty(text: _error!, onRetry: _load)
                  : _data == null
                      ? const _Empty(text: '无数据')
                      : ListView(
                          padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
                          children: [
                            if (_data!.stale && _data!.notice.isNotEmpty)
                              _banner(_data!.notice, const Color(0xFFF39C12)),
                            if (s != null) ...[
                              _vitals(s),
                              const SizedBox(height: 10),
                            ],
                            _searchField(),
                            const SizedBox(height: 8),
                            _filterChips(),
                            const SizedBox(height: 12),
                            ..._noticeSections.where((n) => n.items.isNotEmpty).map(_noticeCard),
                            const SizedBox(height: 12),
                            ..._groupedEntries(),
                            if (_filtered.isEmpty)
                              const Padding(
                                padding: EdgeInsets.symmetric(vertical: 48),
                                child: Center(child: Text('没有匹配的站点', style: TextStyle(color: Colors.grey))),
                              ),
                          ],
                        ),
        ),
      ),
    );
  }

  List<Widget> _groupedEntries() {
    final entries = _grouped.entries.toList()
      ..sort((a, b) {
        if (a.key == '未知') return 1;
        if (b.key == '未知') return -1;
        return b.key.compareTo(a.key);
      });
    return entries
        .map((e) => Container(
              margin: const EdgeInsets.only(bottom: 8),
              child: GlassCard(
                padding: EdgeInsets.zero,
                child: ExpansionTile(
                  shape: const Border(),
                  collapsedBackgroundColor: Colors.transparent,
                  backgroundColor: Colors.transparent,
                  title: Row(children: [
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 2),
                      decoration: BoxDecoration(color: PTColors.primary.withValues(alpha: 0.1), borderRadius: BorderRadius.circular(999)),
                      child: Text(e.key, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: PTColors.primary)),
                    ),
                    const SizedBox(width: 8),
                    Text('${e.key == '未知' ? '' : '年'}建站 · ${e.value.length} 个',
                        style: const TextStyle(fontSize: 12, color: Colors.grey)),
                  ]),
                  initiallyExpanded: true,
                  childrenPadding: const EdgeInsets.fromLTRB(8, 0, 8, 8),
                  children: e.value.map((c) => _SiteCard(c, _siteTypeLabel, _statusLabel, _matchLabel, _healthColor, _isTodayAnniv, _annivDisplay)).toList(),
                ),
              ),
            ))
        .toList();
  }

  Widget _vitals(SaveptSummary s) {
    final items = <(int, String, Color)>[
      (s.total, '监护中', PTColors.primary),
      (s.healthy, '健康', const Color(0xFF22C55E)),
      (s.mpOwned, '已拥有', const Color(0xFF16A34A)),
      (s.critical, '病危', const Color(0xFFEF4444)),
      (s.closed, '已关站', const Color(0xFF94A3B8)),
      (s.todayAnniv, '站庆', const Color(0xFFF59E0B)),
      (s.internal, '内站', const Color(0xFF3B82F6)),
      (s.external, '外站', const Color(0xFF8B5CF6)),
    ];
    return GlassCard(
      padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 10),
      child: Wrap(
        alignment: WrapAlignment.spaceAround,
        runSpacing: 12,
        children: items
            .map((it) => SizedBox(
                  width: 68,
                  child: Column(children: [
                    Text('${it.$1}', style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700, color: it.$3)),
                    const SizedBox(height: 2),
                    Text(it.$2, style: const TextStyle(fontSize: 11, color: Colors.grey), textAlign: TextAlign.center),
                  ]),
                ))
            .toList(),
      ),
    );
  }

  Widget _searchField() {
    return GlassCard(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
      child: TextField(
        decoration: const InputDecoration(
          icon: Icon(Icons.search, color: Colors.grey),
          hintText: '搜索站点名称...',
          border: InputBorder.none,
          isDense: true,
          contentPadding: EdgeInsets.symmetric(vertical: 12),
        ),
        onChanged: (v) => setState(() => _keyword = v),
      ),
    );
  }

  Widget _filterChips() {
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      // 状态
      Wrap(spacing: 6, runSpacing: 4, children: [
        _chip('全部', _status == '全部', () => setState(() => _status = '全部')),
        _chip('健康', _status == '健康', () => setState(() => _status = '健康'), const Color(0xFF22C55E)),
        _chip('病危', _status == '病危', () => setState(() => _status = '病危'), const Color(0xFFEF4444)),
        _chip('死亡', _status == '死亡', () => setState(() => _status = '死亡'), const Color(0xFF94A3B8)),
        _chip('内站', _type == '内站', () => setState(() => _type = _type == '内站' ? '全部' : '内站')),
        _chip('外站', _type == '外站', () => setState(() => _type = _type == '外站' ? '全部' : '外站')),
        _chip('近期站庆', _annivOnly, () => setState(() => _annivOnly = !_annivOnly), const Color(0xFFF59E0B)),
      ]),
      const SizedBox(height: 6),
      // 年份 + MP 下拉
      Row(children: [
        Expanded(child: _dropdown('年份', _year, _yearOptions, (v) => setState(() => _year = v))),
        const SizedBox(width: 8),
        Expanded(
          child: _dropdown(
            'MP状态',
            _mpStatus,
            const ['全部', 'owned', 'available', 'unsupported'],
            (v) => setState(() => _mpStatus = v),
            labelOf: (v) => switch (v) {
              'owned' => '已拥有',
              'available' => '可添加',
              'unsupported' => '未收录',
              _ => '全部',
            },
          ),
        ),
      ]),
    ]);
  }

  Widget _chip(String label, bool active, VoidCallback onTap, [Color? color]) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        decoration: BoxDecoration(
          color: active ? (color ?? PTColors.primary).withValues(alpha: 0.14) : Colors.transparent,
          border: Border.all(color: active ? (color ?? PTColors.primary).withValues(alpha: 0.4) : Colors.grey.withValues(alpha: 0.3)),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w500, color: active ? (color ?? PTColors.primary) : Colors.grey)),
      ),
    );
  }

  Widget _dropdown(String hint, String value, List<String> options, ValueChanged<String> onChanged, {String Function(String)? labelOf}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10),
      decoration: BoxDecoration(
        border: Border.all(color: Colors.grey.withValues(alpha: 0.3)),
        borderRadius: BorderRadius.circular(999),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          isExpanded: true,
          value: value,
          style: const TextStyle(fontSize: 13, color: Colors.black87),
          items: options
              .map((o) => DropdownMenuItem(value: o, child: Text(labelOf?.call(o) ?? (o == '全部' ? '$hint：全部' : o))))
              .toList(),
          onChanged: (v) => v == null ? null : onChanged(v),
        ),
      ),
    );
  }

  Widget _noticeCard(_NoticeSection n) {
    final color = switch (n.key) {
      'critical' => const Color(0xFFEF4444),
      'success' => const Color(0xFF22C55E),
      'info' => const Color(0xFFF59E0B),
      _ => const Color(0xFF94A3B8),
    };
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      child: GlassCard(
        padding: const EdgeInsets.all(12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(n.title, style: TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: color)),
          const SizedBox(height: 6),
          ...n.items.map((t) => Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text(t, style: const TextStyle(fontSize: 12, height: 1.5)),
              )),
        ]),
      ),
    );
  }

  Widget _banner(String text, Color color) {
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(color: color.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(12), border: Border.all(color: color.withValues(alpha: 0.3))),
      child: Row(children: [Icon(Icons.warning_amber_rounded, color: color, size: 18), const SizedBox(width: 8), Expanded(child: Text(text, style: TextStyle(fontSize: 12, color: color)))]),
    );
  }
}

class _NoticeSection {
  final String key;
  final String title;
  final List<String> items;
  _NoticeSection(this.key, this.title, this.items);
}

/// 单站点卡片：今天站庆时叠加 confetti 彩带。
class _SiteCard extends StatefulWidget {
  final SaveptSiteCard card;
  final String Function(String) siteTypeLabel;
  final String Function(SiteHealthStatus) statusLabel;
  final String Function(MatchType) matchLabel;
  final Color Function(SiteHealthStatus) healthColor;
  final bool Function(String) isTodayAnniv;
  final String Function(String) annivDisplay;
  const _SiteCard(this.card, this.siteTypeLabel, this.statusLabel, this.matchLabel, this.healthColor, this.isTodayAnniv, this.annivDisplay);

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

  @override
  Widget build(BuildContext context) {
    final c = widget.card;
    final color = widget.healthColor(c.status);
    final matchColor = switch (c.matchType) {
      MatchType.urlExact || MatchType.domainExact => const Color(0xFF16A34A),
      MatchType.nameExact || MatchType.nameFuzzy => const Color(0xFF2563EB),
      MatchType.none => const Color(0xFF64748B),
    };
    return Container(
      margin: const EdgeInsets.only(top: 6),
      child: Stack(
        children: [
          GlassCard(
            onTap: c.url.isNotEmpty ? () {} : null,
            padding: const EdgeInsets.all(12),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              // 标签行
              Wrap(spacing: 6, runSpacing: 4, children: [
                _tag(widget.siteTypeLabel(c.siteType), Colors.grey),
                if (c.durationText.isNotEmpty) _tag(c.durationText, PTColors.primary),
                if (c.anniversaryText.isNotEmpty)
                  _tag(widget.annivDisplay(c.anniversaryText), const Color(0xFFF59E0B)),
                if (c.faultDuration.isNotEmpty) _tag('⏱ 抢救 ${c.faultDuration}', const Color(0xFFEF4444)),
              ]),
              const SizedBox(height: 8),
              Row(children: [
                if (c.icon != null && c.icon!.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(right: 10),
                    child: ClipRRect(
                      borderRadius: BorderRadius.circular(8),
                      child: Image.network(c.icon!, width: 34, height: 34, fit: BoxFit.contain, errorBuilder: (_, _, _) => const SizedBox(width: 34, height: 34)),
                    ),
                  ),
                Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Row(children: [
                    CircleAvatar(radius: 4, backgroundColor: color),
                    const SizedBox(width: 6),
                    Expanded(child: Text(c.name, style: const TextStyle(fontWeight: FontWeight.w600), overflow: TextOverflow.ellipsis)),
                    Icon(_statusIcon(c.status), color: color, size: 16),
                  ]),
                  const SizedBox(height: 4),
                  Text('开站 ${c.openTime.isNotEmpty ? c.openTime : '未知'}${c.closeTime.isNotEmpty && c.closeTime != '未知' ? ' · 关站 ${c.closeTime}' : ''}',
                      style: const TextStyle(fontSize: 11, color: Colors.grey)),
                  if (c.localSiteName.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(top: 2),
                      child: Text(c.localSiteName, style: const TextStyle(fontSize: 11, color: PTColors.primary), overflow: TextOverflow.ellipsis),
                    ),
                ])),
              ]),
              const SizedBox(height: 8),
              Row(children: [
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                  decoration: BoxDecoration(color: matchColor.withValues(alpha: 0.18), borderRadius: BorderRadius.circular(999)),
                  child: Text(widget.matchLabel(c.matchType), style: TextStyle(color: matchColor, fontSize: 11)),
                ),
                if (c.domain.isNotEmpty) ...[
                  const SizedBox(width: 8),
                  Expanded(child: Text(c.domain, style: const TextStyle(color: Colors.grey, fontSize: 11), overflow: TextOverflow.ellipsis)),
                ],
              ]),
            ]),
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
                colors: const [Color(0xFFF59E0B), Color(0xFFFACC15), Color(0xFFFB7185), Color(0xFF22C55E), Color(0xFF38BDF8)],
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

  Widget _tag(String text, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 1),
      decoration: BoxDecoration(color: color.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(999)),
      child: Text(text, style: TextStyle(fontSize: 10, color: color, fontWeight: FontWeight.w500)),
    );
  }

  IconData _statusIcon(SiteHealthStatus s) => switch (s) {
        SiteHealthStatus.healthy => Icons.favorite,
        SiteHealthStatus.critical => Icons.error_outline,
        SiteHealthStatus.closed => Icons.sentiment_very_dissatisfied,
        SiteHealthStatus.unknown => Icons.help_outline,
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
        Center(child: Padding(padding: const EdgeInsets.all(24), child: Text(text, textAlign: TextAlign.center, style: const TextStyle(color: Colors.grey)))),
        if (onRetry != null) Center(child: ElevatedButton(onPressed: onRetry, child: const Text('重试'))),
      ],
    );
  }
}
