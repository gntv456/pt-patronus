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
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import '../media/media_detail_page.dart';
import '../media/media_settings_page.dart';

// 统一搜索页（对齐 Web frontend/src/views/Search.vue）：[种子 | 影视] 分段切换，共享关键词。
//  - 种子：跨启用站点聚合搜索。宽屏=统一可排序表格（站点/标题/大小/做种/吸血/Free/操作），
//    窄屏=带站点名的卡片列表；含失败站点折叠栏。推送下载器 + 详情浮层。
//  - 影视：TMDB/豆瓣/Bangumi 元数据 + 海报网格（PTAutoGrid 流式重排）。
enum SearchMode { torrent, media }

enum _Sort { none, seeders, leechers }

class SearchPage extends StatefulWidget {
  final String? initialKeyword;
  const SearchPage({super.key, this.initialKeyword});
  @override
  State<SearchPage> createState() => _SearchPageState();
}

class _SearchPageState extends State<SearchPage> {
  SearchMode _mode = SearchMode.torrent;
  late final _ctrl = TextEditingController(text: widget.initialKeyword ?? '');
  String _year = ''; // 影视搜索年份（'' = 全部），由 pill 选择而非输入框，统一只剩一个输入框

  List<SiteSearchResult> _results = [];
  List<MediaInfo> _mediaResults = [];
  List<Site> _sites = []; // 启用站点（种子搜索站点筛选用）
  List<int> _selectedSiteIds = []; // 种子搜索站点筛选，空=全部启用站
  bool _loading = false;
  bool _searched = false;
  String? _error; // 搜索失败文案（非空时列表区显示可重试错误态——审计 P1）
  String _category = '';

  _Sort _sort = _Sort.none;
  bool _showFails = false;

  @override
  void initState() {
    super.initState();
    _loadSites();
    if ((widget.initialKeyword ?? '').isNotEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _search());
    }
  }

  Future<void> _loadSites() async {
    try {
      final list = await SiteApi().list();
      if (mounted) setState(() => _sites = list.where((s) => s.isActive).toList());
    } catch (_) {}
  }

  Future<void> _search() async {
    final kw = _ctrl.text.trim();
    if (kw.isEmpty) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      if (_mode == SearchMode.torrent) {
        _results = await SearchApi().search(kw, category: _category, siteIds: _selectedSiteIds);
      } else {
        final year = int.tryParse(_year) ?? 0;
        _mediaResults = await MediaApi().search(kw, year);
      }
      _searched = true;
    } catch (e) {
      // 失败也置 _searched + _error：让列表区显示可重试错误态，而非停在初始引导（审计 P1）
      _error = _mode == SearchMode.torrent
          ? '搜索失败：$e'
          : '影视搜索失败（请先在「媒体源设置」配置 TMDB/豆瓣）';
      setState(() => _searched = true);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _switchMode(SearchMode m) {
    if (_mode == m) return;
    setState(() {
      _mode = m;
      _results = [];
      _mediaResults = [];
      _searched = false;
      _error = null;
    });
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
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

  Future<void> _download(SearchTorrent t) async {
    if (t.downloadUrl.isEmpty) {
      _toast('该站点暂不支持直接下载（如馒头，需后续 genDlToken）');
      return;
    }
    List<Downloader> ds;
    try {
      ds = await DownloaderApi().list();
    } catch (_) {
      _toast('加载下载器失败');
      return;
    }
    if (ds.isEmpty) {
      _toast('请先在「下载器」添加一个');
      return;
    }
    if (!mounted) return;
    final picked = await showAdaptiveSheet<Downloader>(
      context,
      title: '选择下载器',
      wideWidth: 480,
      builder: (ctx) => Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(
              '${t.title}${t.size.isNotEmpty ? " · ${t.size}" : ""}',
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(fontSize: 12, color: Colors.grey),
            ),
          ),
          ...ds.map(
            (d) => ListTile(
              leading: const Icon(Icons.download),
              title: Text(d.name),
              subtitle: Text(d.url, style: const TextStyle(fontSize: 12)),
              onTap: () => Navigator.pop(ctx, d),
            ),
          ),
        ],
      ),
    );
    if (picked == null) return;
    try {
      await DownloaderApi().addTorrent(picked.id, t.downloadUrl);
      _toast('已推送到 ${picked.name}');
    } catch (e) {
      _toast('失败：$e');
    }
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  // 影视年份候选：当年倒推到 1980 + 全部（pill 候选列表，避免再来一个输入框）。
  List<PTFilterOption<String>> get _yearOptions {
    final now = DateTime.now().year;
    return [
      const PTFilterOption(value: '', label: '全部'),
      for (var y = now; y >= 1980; y--)
        PTFilterOption(value: '$y', label: '$y'),
    ];
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

  // 分类（种子模式）/ 年份（影视模式）筛选维度，收进顶栏筛选按钮。随当前模式切换。
  List<PTFilterDimension> _filterDimensions() {
    if (_mode == SearchMode.torrent) {
      return [
        PTFilterDimension.choice(
          label: '分类',
          selected: _category,
          isDefault: _category.isEmpty,
          onChanged: (v) => setState(() => _category = (v as String?) ?? ''),
          options: const [
            PTFilterOption<Object?>(value: '', label: '全部'),
            PTFilterOption<Object?>(value: 'movie', label: '电影'),
            PTFilterOption<Object?>(value: 'tv', label: '剧集'),
            PTFilterOption<Object?>(value: 'music', label: '音乐'),
          ],
        ),
      ];
    }
    return [
      PTFilterDimension.choice(
        label: '年份',
        selected: _year,
        isDefault: _year.isEmpty,
        onChanged: (v) => setState(() => _year = (v as String?) ?? ''),
        options: _yearOptions,
      ),
    ];
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
                          style: const TextStyle(
                            color: Colors.grey,
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
                    color: tr.t.seeders > 0 ? PTColors.success : Colors.grey,
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
                    color: tr.t.leechers > 0 ? PTColors.warn : Colors.grey,
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
                    : const Text(
                        '—',
                        style: TextStyle(color: Colors.grey, fontSize: 12),
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
                            style: const TextStyle(
                              color: Colors.grey,
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

  Widget _failBar(List<SiteSearchResult> failed) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: GlassCard(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            InkWell(
              onTap: () => setState(() => _showFails = !_showFails),
              child: Row(
                children: [
                  Icon(
                    Icons.warning_amber_rounded,
                    color: PTColors.warn,
                    size: 18,
                  ),
                  const SizedBox(width: 6),
                  Text(
                    '${failed.length} 个站点搜索失败',
                    style: TextStyle(
                      color: PTColors.warn,
                      fontWeight: FontWeight.w600,
                      fontSize: 13,
                    ),
                  ),
                  const Spacer(),
                  Icon(
                    _showFails ? Icons.expand_less : Icons.expand_more,
                    color: Colors.grey,
                  ),
                ],
              ),
            ),
            if (_showFails)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Column(
                  children: failed
                      .map(
                        (s) => Padding(
                          padding: const EdgeInsets.symmetric(vertical: 3),
                          child: Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                '${s.siteName}：',
                                style: const TextStyle(
                                  fontWeight: FontWeight.w600,
                                  fontSize: 12,
                                ),
                              ),
                              Expanded(
                                child: Text(
                                  s.error,
                                  style: const TextStyle(
                                    color: Colors.redAccent,
                                    fontSize: 12,
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ),
                      )
                      .toList(),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _sortControl() {
    String labelOf(_Sort k) => switch (k) {
      _Sort.none => '默认',
      _Sort.seeders => '做种↓',
      _Sort.leechers => '吸血↓',
    };
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 4),
      child: Align(
        alignment: Alignment.centerLeft,
        child: PTFilterPill(
          label: '排序',
          value: labelOf(_sort),
          active: _sort != _Sort.none,
          onTap: () => showFilterSheet<_Sort>(
            context,
            title: '排序方式',
            selected: _sort,
            onSelected: (v) => setState(() => _sort = v),
            options: const [
              PTFilterOption(value: _Sort.none, label: '默认'),
              PTFilterOption(value: _Sort.seeders, label: '做种↓'),
              PTFilterOption(value: _Sort.leechers, label: '吸血↓'),
            ],
          ),
        ),
      ),
    );
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
          Icon(Icons.travel_explore, size: 16, color: Colors.grey.shade600),
          const SizedBox(width: 6),
          Expanded(
            child: Text(
              '已搜索 $sites 个站点，$okSites 个成功，命中 $rows 条',
              style: const TextStyle(fontSize: 12, color: Colors.grey),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
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
                          color: Colors.black54,
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
                            style: const TextStyle(
                              color: Colors.grey,
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

/// 种子详情浮层内容（对齐 Web Search.vue 的 el-drawer，宽屏对话框 / 窄屏底栏由 showAdaptiveSheet 套壳）。
/// 抓取 detail_url 解析海报/规格/截图/简介/MediaInfo/文件列表 + 下载/复制链接。内容为纵向 Column，由外层滚动。
class _TorrentDetailContent extends StatefulWidget {
  final int siteId;
  final String siteName;
  final String detailUrl;
  final SearchTorrent torrent;
  final VoidCallback onDownload;

  const _TorrentDetailContent({
    required this.siteId,
    required this.siteName,
    required this.detailUrl,
    required this.torrent,
    required this.onDownload,
  });

  @override
  State<_TorrentDetailContent> createState() => _TorrentDetailContentState();
}

class _TorrentDetailContentState extends State<_TorrentDetailContent> {
  TorrentDetail? _detail;
  bool _loading = true;
  String? _err;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      _detail = await SeedBrowseApi().detail(widget.siteId, widget.detailUrl);
    } catch (e) {
      _err = '加载详情失败：$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _copyLink() async {
    final url = _detail?.downloadUrl ?? widget.torrent.downloadUrl;
    if (url.isEmpty) return;
    await Clipboard.setData(ClipboardData(text: url));
    if (mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('已复制下载链接')));
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.all(40),
        child: Center(child: CircularProgressIndicator()),
      );
    }
    if (_err != null) {
      return Padding(
        padding: const EdgeInsets.all(24),
        child: Text(
          _err!,
          style: const TextStyle(color: Colors.redAccent),
          textAlign: TextAlign.center,
        ),
      );
    }
    final d = _detail;
    if (d == null) {
      return const Padding(padding: EdgeInsets.all(24), child: Text('无详情'));
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          widget.siteName,
          style: const TextStyle(color: Colors.grey, fontSize: 12),
        ),
        const SizedBox(height: 8),
        _header(d),
        if (d.screenshots.isNotEmpty) ...[
          _section('截图（${d.screenshots.length}）'),
          _screens(d.screenshots),
        ],
        if (d.description.trim().isNotEmpty) ...[
          _section('简介'),
          HtmlWidget(d.description, textStyle: const TextStyle(height: 1.6)),
        ],
        if (d.mediainfo.isNotEmpty) ...[
          _section('MediaInfo'),
          _mediainfo(d.mediainfo),
        ],
        if (d.fileList.isNotEmpty) ...[
          _section('文件列表（${d.fileList.length}）'),
          _fileList(d.fileList),
        ],
        const SizedBox(height: 24),
      ],
    );
  }

  Widget _header(TorrentDetail d) {
    final dlUrl = d.downloadUrl.isEmpty
        ? widget.torrent.downloadUrl
        : d.downloadUrl;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (d.posterUrl.isNotEmpty) ...[
          ClipRRect(
            borderRadius: BorderRadius.circular(8),
            child: SizedBox(
              width: 96,
              height: 138,
              child: MediaPoster(url: d.posterUrl, kind: 'movie'),
            ),
          ),
          const SizedBox(width: 14),
        ],
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (d.subtitle.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(bottom: 6),
                  child: Text(
                    d.subtitle,
                    style: const TextStyle(color: Colors.grey, fontSize: 12),
                  ),
                ),
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [
                  if (d.category.isNotEmpty)
                    _spec(d.category, PTColors.primary),
                  if (d.resolution.isNotEmpty)
                    _spec(d.resolution, const Color(0xFF4285F4)),
                  if (d.codec.isNotEmpty)
                    _spec(d.codec, const Color(0xFF9C27B0)),
                  if (d.source.isNotEmpty)
                    _spec(d.source, const Color(0xFF009688)),
                  if (d.size.isNotEmpty) _spec(d.size, Colors.blueGrey),
                  if (d.seeders > 0) _spec('↑ ${d.seeders}', PTColors.success),
                ],
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  FilledButton.icon(
                    onPressed: dlUrl.isEmpty ? null : widget.onDownload,
                    icon: const Icon(Icons.download, size: 18),
                    label: const Text('下载'),
                  ),
                  const SizedBox(width: 8),
                  OutlinedButton.icon(
                    onPressed: dlUrl.isEmpty ? null : _copyLink,
                    icon: const Icon(Icons.copy, size: 16),
                    label: const Text('复制链接'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _screens(List<String> urls) {
    return GridView.builder(
      shrinkWrap: true,
      physics: const NeverScrollableScrollPhysics(),
      itemCount: urls.length,
      gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
        maxCrossAxisExtent: 160,
        mainAxisSpacing: 8,
        crossAxisSpacing: 8,
        childAspectRatio: 16 / 9,
      ),
      itemBuilder: (_, i) => ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: Image.network(
          urls[i],
          fit: BoxFit.cover,
          errorBuilder: (_, _, _) => Container(color: Colors.black12),
        ),
      ),
    );
  }

  Widget _mediainfo(String text) => Container(
    width: double.infinity,
    padding: const EdgeInsets.all(10),
    decoration: BoxDecoration(
      color: const Color(0xEE000000),
      borderRadius: BorderRadius.circular(8),
    ),
    child: SelectableText(
      text,
      style: const TextStyle(
        color: Color(0xFFD4D4D4),
        fontSize: 12,
        height: 1.5,
        fontFamily: 'monospace',
      ),
    ),
  );

  Widget _fileList(List<String> files) => Container(
    decoration: BoxDecoration(
      color: Colors.black.withValues(alpha: 0.04),
      borderRadius: BorderRadius.circular(8),
    ),
    child: Column(
      children: [
        for (int i = 0; i < files.length; i++)
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('📄 ', style: TextStyle(fontSize: 12)),
                Expanded(
                  child: SelectableText(
                    files[i],
                    style: const TextStyle(
                      fontSize: 12,
                      fontFamily: 'monospace',
                    ),
                  ),
                ),
              ],
            ),
          ),
      ],
    ),
  );

  Widget _section(String title) => Padding(
    padding: const EdgeInsets.fromLTRB(0, 18, 0, 8),
    child: Text(
      title,
      style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14),
    ),
  );

  Widget _spec(String label, Color c) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
    decoration: BoxDecoration(
      color: c.withValues(alpha: 0.16),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Text(
      label,
      style: TextStyle(color: c, fontSize: 11, fontWeight: FontWeight.w600),
    ),
  );
}
