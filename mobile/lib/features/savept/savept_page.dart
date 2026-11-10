// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

part 'saveptopsmixin.dart';
part 'savept_widgets.dart';

part 'site_card.dart';
part 'notice_models.dart';

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
    with SingleTickerProviderStateMixin, SaveptOpsMixin {

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

  void _scrollToTop() => _scroll.animateTo(
    0,
    duration: const Duration(milliseconds: 300),
    curve: Curves.easeOut,
  );


  // 公告站点名高亮用：已知站点名集合，长名优先匹配（避免短名误命中）。
  List<String> get _siteNames =>
      (_data?.cards ?? const <SaveptSiteCard>[])
          .map((c) => c.name.trim())
          .where((s) => s.isNotEmpty)
          .toSet()
          .toList()
        ..sort((a, b) => b.length.compareTo(a.length));

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
              tooltip: '回到顶部',
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

}

// ── 公告正文站点名高亮分词（移植自 Web frontend/src/utils/savept.ts 的 tokenizeNoticeText）──
// 按 `|` 切段，段内用「已知站点名集合正则前缀匹配」+「已抢救/已/还有/抢救/于 动词前缀启发式」
// 找出站点名，便于在 RichText 里把站点名段着色成 inline tag。

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
