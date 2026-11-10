// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_widget_from_html/flutter_widget_from_html.dart';
import 'package:ptpatronus/api/media.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/seedbrowse.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/media_poster.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import '../media/media_detail_page.dart';
import '../media/media_settings_page.dart';

part 'searchopsmixin.dart';
part 'search_widgets.dart';

part 'torrent_detail_content.dart';

// 统一搜索页（对齐 Web frontend/src/views/Search.vue）：[种子 | 影视] 分段切换，共享关键词。
//  - 种子：跨启用站点聚合搜索。宽屏=统一可排序表格（站点/标题/大小/做种/吸血/Free/操作），
//    窄屏=带站点名的卡片列表；含失败站点折叠栏。推送下载器 + 详情浮层。
//  - 影视：TMDB/豆瓣/Bangumi 元数据 + 海报网格（PTAutoGrid 流式重排）。
enum SearchMode { torrent, media }

enum _Sort { none, seeders, leechers }

class SearchPage extends StatefulWidget {
  final String? initialKeyword;
  const SearchPage({super.key, this.initialKeyword});

  /// 命令面板等在 IndexedStack 保活页上注入的待搜关键词。
  static final ValueNotifier<String?> pendingKeyword = ValueNotifier<String?>(null);

  @override
  State<SearchPage> createState() => _SearchPageState();
}

class _SearchPageState extends State<SearchPage> with SearchOpsMixin {

  @override
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  void initState() {
    super.initState();
    SearchPage.pendingKeyword.addListener(_onPendingKeyword);
    _loadSites();
    if ((widget.initialKeyword ?? '').isNotEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _search());
    } else {
      WidgetsBinding.instance.addPostFrameCallback((_) => _onPendingKeyword());
    }
  }

  // 展平所有站点的种子为统一行（带站点名），按当前排序键排序。
  List<_TR> get _rows {
    final r = <_TR>[];
    for (final s in _results) {
      for (final t in s.torrents) {
        r.add(_TR(t, s.siteId, s.siteName));
      }
    }
    switch (_sort) {
      case _Sort.seeders:
        r.sort((a, b) => b.t.seeders.compareTo(a.t.seeders));
      case _Sort.leechers:
        r.sort((a, b) => b.t.leechers.compareTo(a.t.leechers));
      case _Sort.none:
        break;
    }
    return r;
  }

  List<SiteSearchResult> get _failed =>
      _results.where((s) => s.error.isNotEmpty).toList();

  @override
  void dispose() {
    SearchPage.pendingKeyword.removeListener(_onPendingKeyword);
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '',
      trailing: [
        SegmentedButton<SearchMode>(
          segments: const [
            ButtonSegment(value: SearchMode.torrent, label: Text('种子')),
            ButtonSegment(value: SearchMode.media, label: Text('影视')),
          ],
          selected: {_mode},
          onSelectionChanged: (s) => _switchMode(s.first),
        ),
      ],
      actions: [
        if (_mode == SearchMode.media)
          PTBarAction(
            icon: Icons.settings_outlined,
            label: '媒体源设置',
            onPressed: () => Navigator.push(
              context,
              MaterialPageRoute(builder: (_) => const MediaSettingsPage()),
            ),
          ),
      ],
      filterDimensions: _filterDimensions(),
      siteFilter: _mode == SearchMode.torrent && _sites.isNotEmpty
          ? PTSiteFilterConfig(
              selected: _selectedSiteIds,
              onChanged: (v) {
                setState(() => _selectedSiteIds = v);
                if (_searched) _search(); // 改筛选后自动重搜
              },
              options: _sites
                  .map((s) => PTFilterOption<Object?>(value: s.id, label: s.name))
                  .toList(),
            )
          : null,
      filterOnClearAll: () => setState(() {
        if (_mode == SearchMode.torrent) {
          _category = '';
          _selectedSiteIds = [];
          if (_searched) _search();
        } else {
          _year = '';
        }
      }),
      // 顶栏可展开搜索（唯一搜索入口）：窄屏点搜索图标在标题位展开输入框，宽屏 inline。
      // 正文不再放搜索框（避免与顶栏重复）。
      search: PTSearchConfig(
        controller: _ctrl,
        onSubmitted: (_) => _search(),
        onChanged: (_) {},
        hintText: _mode == SearchMode.torrent ? '搜索种子…' : '搜索影视（电影 / 剧集）',
      ),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_loading) const LinearProgressIndicator(minHeight: 2),
          Expanded(
            child: _mode == SearchMode.torrent ? _torrentArea : _mediaArea,
          ),
        ],
      ),
    );
  }

  // —— 种子结果区 ——
  Widget get _torrentArea {
    final rows = _rows;
    final failed = _failed;
    if (rows.isEmpty && failed.isEmpty) {
      if (_error != null) {
        return PTEmptyState(
          icon: Icons.error_outline,
          text: _error!,
          onRetry: _search,
        );
      }
      return _searched
          ? PTEmptyState(
              icon: Icons.search_off,
              text: '无结果',
              description: '请先在「站点」添加并登录站点',
            )
          : PTEmptyState(icon: Icons.search, text: '输入关键词跨站搜索');
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _scopeBar(rows.length, _results.length, failed.length),
        if (failed.isNotEmpty) _failBar(failed),
        if (isWideScreen(context) && rows.isNotEmpty) _sortControl(),
        Expanded(
          child: PTAdaptiveTable<_TR>(
            items: rows,
            columns: [
              PTColumn<_TR>(
                label: '站点',
                flex: 1,
                cell: (_, tr) =>
                    Text(tr.site, style: const TextStyle(fontSize: 12)),
              ),
              PTColumn<_TR>(
                label: '标题',
                flex: 4,
                cell: (_, tr) => InkWell(
                  onTap: () => _openDetail(tr),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        tr.t.title,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w600,
                          fontSize: 13,
                        ),
                      ),
                      if (tr.t.subtitle.isNotEmpty)
                        Text(
                          tr.t.subtitle,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            color: context.dimText,
                            fontSize: 11,
                          ),
                        ),
                    ],
                  ),
                ),
              ),
              PTColumn<_TR>(
                label: '大小',
                flex: 1,
                numeric: true,
                cell: (_, tr) =>
                    Text(tr.t.size, style: const TextStyle(fontSize: 12)),
              ),
              PTColumn<_TR>(
                label: '做种',
                flex: 1,
                numeric: true,
                cell: (_, tr) => Text(
                  '${tr.t.seeders}',
                  style: TextStyle(
                    fontSize: 12,
                    color: tr.t.seeders > 0 ? PTColors.success : context.dimText,
                  ),
                ),
              ),
              PTColumn<_TR>(
                label: '吸血',
                flex: 1,
                numeric: true,
                cell: (_, tr) => Text(
                  '${tr.t.leechers}',
                  style: TextStyle(
                    fontSize: 12,
                    color: tr.t.leechers > 0 ? PTColors.warn : context.dimText,
                  ),
                ),
              ),
              PTColumn<_TR>(
                label: 'Free',
                flex: 1,
                cell: (_, tr) => tr.t.free
                    ? Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 7,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: PTColors.success.withValues(alpha: 0.16),
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: Text(
                          'FREE',
                          style: TextStyle(
                            color: PTColors.success,
                            fontSize: 11,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      )
                    : Text(
                        '—',
                        style: TextStyle(color: context.dimText, fontSize: 12),
                      ),
              ),
              PTColumn<_TR>(
                label: '操作',
                flex: 1,
                cell: (_, tr) => IconButton(
                  visualDensity: VisualDensity.compact,
                  icon: Icon(
                    tr.t.downloadUrl.isEmpty
                        ? Icons.download_outlined
                        : Icons.download,
                    color: PTColors.primary,
                    size: 20,
                  ),
                  tooltip: '推送下载',
                  onPressed: () => _download(tr.t),
                ),
              ),
            ],
            narrowCard: (_, tr) => GlassCard(
              onTap: () => _openDetail(tr),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          tr.t.title,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            fontWeight: FontWeight.w600,
                            fontSize: 15,
                          ),
                        ),
                        if (tr.t.subtitle.isNotEmpty) ...[
                          const SizedBox(height: 2),
                          Text(
                            tr.t.subtitle,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              color: context.dimText,
                              fontSize: 12,
                            ),
                          ),
                        ],
                        const SizedBox(height: 6),
                        Wrap(
                          spacing: 6,
                          runSpacing: 4,
                          crossAxisAlignment: WrapCrossAlignment.center,
                          children: [
                            _chip(tr.site, PTColors.primary),
                            if (tr.t.size.isNotEmpty)
                              _chip(tr.t.size, Colors.blueGrey),
                            if (tr.t.seeders > 0)
                              _chip('↑${tr.t.seeders}', Colors.green),
                            if (tr.t.leechers > 0)
                              _chip('↓${tr.t.leechers}', Colors.orange),
                            if (tr.t.free) _chip('FREE', PTColors.success),
                          ],
                        ),
                      ],
                    ),
                  ),
                  IconButton(
                    icon: Icon(
                      tr.t.downloadUrl.isEmpty
                          ? Icons.download_outlined
                          : Icons.download,
                      color: PTColors.primary,
                    ),
                    tooltip: '推送下载',
                    onPressed: () => _download(tr.t),
                  ),
                ],
              ),
            ),
            emptyText: '无结果',
          ),
        ),
      ],
    );
  }

  // —— 影视结果区 ——
  Widget get _mediaArea {
    if (_mediaResults.isEmpty) {
      return PTEmptyState(
        icon: Icons.movie_outlined,
        text: _searched ? '无结果' : '输入关键字搜索影视元数据与海报',
      );
    }
    return PTAutoGrid(
      minItemWidth: 170,
      childAspectRatio: 0.54,
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      itemCount: _mediaResults.length,
      itemBuilder: (_, i) => _mediaCard(_mediaResults[i]),
    );
  }

  String _src(String s) =>
      const {'tmdb': 'TMDB', 'douban': '豆瓣', 'musicbrainz': 'MB'}[s] ??
      s.toUpperCase();
}

/// 统一种子行（带站点名），供宽屏表格 / 窄屏卡片共用。
class _TR {
  final SearchTorrent t;
  final int siteId;
  final String site;
  _TR(this.t, this.siteId, this.site);
}
