import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/recommend.dart';
import 'package:ptpatronus/api/discover.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/media_poster.dart';
import 'package:ptpatronus/widgets/find_on_sites_sheet.dart';
import '../media/media_detail_page.dart';

// 推荐页：多源热门信息流（豆瓣热映/TMDB trending/Bangumi 热门）。
// 原「探索」并入：顶部分类筛选项右侧的关键词搜索框 → 跨源搜索，与信息流共用本页与卡片。
class RecommendPage extends StatefulWidget {
  const RecommendPage({super.key});
  @override
  State<RecommendPage> createState() => _RecommendPageState();
}

class _RecommendPageState extends State<RecommendPage> {
  final _api = RecommendApi();
  final _searchApi = SearchApi();
  final _discoverApi = DiscoverApi();
  final _kwCtrl = TextEditingController();

  // 信息流（默认）与搜索结果两态：搜索态接管列表，不参与下拉刷新/分页语义。
  List<MediaInfo> _items = []; // 信息流
  List<MediaInfo> _searchItems = []; // 关键词搜索结果
  bool _searchMode = false;
  bool _loading = true;
  bool _searchLoading = false;
  String? _error;
  String _source = '';
  String _category = 'movie';
  // 信息流分页（搜索态不分页，与 Web Recommend loadMore 一致）。
  int _page = 1;
  bool _loadingMore = false;
  bool _hasMore = true;
  static const _pageSize = 20;

  static const _sources = [
    ('', '全部'),
    ('douban', '豆瓣'),
    ('tmdb', 'TMDB'),
    ('bangumi', 'Bangumi'),
    ('xingguangji', '星光集'),
  ];
  static const _categories = [
    ('movie', '热映'),
    ('coming', '即将上映'),
    ('tv', '剧集'),
    ('anime', '动漫'),
    ('duanju', '短剧'),
  ];

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _kwCtrl.dispose();
    super.dispose();
  }

  List<MediaInfo> get _current => _searchMode ? _searchItems : _items;

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
      _searchMode = false; // 回到信息流
      _page = 1;
      _hasMore = true;
    });
    try {
      _items = await _api.feed(source: _source, category: _category, page: 1, refresh: refresh);
      if (_items.length < _pageSize) _hasMore = false;
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  // 加载更多：追加下一页；返回不足一页视为到底。
  Future<void> _loadMore() async {
    if (_searchMode || _loadingMore || !_hasMore) return;
    setState(() => _loadingMore = true);
    try {
      final next = await _api.feed(source: _source, category: _category, page: _page + 1);
      if (!mounted) return;
      setState(() {
        _items = [..._items, ...next];
        _page += 1;
        if (next.length < _pageSize) _hasMore = false;
      });
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    } finally {
      if (mounted) setState(() => _loadingMore = false);
    }
  }

  // 关键词跨源搜索（原「探索」）：沿用当前选中的源筛选。
  Future<void> _doSearch() async {
    final kw = _kwCtrl.text.trim();
    if (kw.isEmpty) return;
    setState(() {
      _searchLoading = true;
      _searchMode = true;
    });
    try {
      _searchItems = await _discoverApi.search(kw, source: _source);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
        setState(() => _searchMode = false);
      }
    } finally {
      if (mounted) setState(() => _searchLoading = false);
    }
  }

  // 顶部刷新 / 下拉刷新：搜索态重搜，否则刷信息流。
  Future<void> _refresh() async {
    if (_searchMode) {
      await _doSearch();
    } else {
      await _load(refresh: true);
    }
  }

  String _sourceLabel(String key) =>
      _sources.firstWhere((e) => e.$1 == key, orElse: () => ('', key)).$2;

  void _findOnSites(MediaInfo m) async {
    showModalBottomSheet(
        context: context,
        isScrollControlled: true,
        builder: (_) => FindOnSitesSheet(api: _searchApi, title: m.title, mediaType: m.type));
  }

  void _openDetail(MediaInfo m) {
    Navigator.push(
      context,
      MaterialPageRoute(builder: (_) => MediaDetailPage(source: m.source, id: m.sourceId)),
    );
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('推荐'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(
              icon: const Icon(Icons.refresh),
              onPressed: (_loading || _searchLoading) ? null : () => _refresh(),
            ),
          ],
        ),
        body: Column(
          children: [
            _filterPills(),
            _searchRow(),
            const SizedBox(height: 4),
            Expanded(
              child: RefreshIndicator(
                onRefresh: _refresh,
                child: _buildList(),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildList() {
    if (_searchLoading && _searchItems.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_loading && _items.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return PTEmptyState(text: _error!, onRetry: _refresh);
    }
    final list = _current;
    if (list.isEmpty) {
      return RefreshIndicator(
        onRefresh: _refresh,
        child: PTEmptyState(icon: Icons.recommend_outlined, text: _searchMode ? '无搜索结果' : '暂无推荐', description: _searchMode ? null : '下拉刷新或切换源/分类'),
      );
    }
    // 多列网格 + 懒加载：SliverList 按行（每行 cols 个 Expanded 卡片）懒构建，
    // 避免 Wrap 把累积多页内容一次性 eager 渲染（100+ 卡片 + Image.network 卡顿/内存膨胀——审计 P1）。
    // 卡片高度随内容变（overview 有无），故不用固定宽高比的 SliverGrid（会裁剪），改按行 Row 自适应高度。
    return RefreshIndicator(
      onRefresh: _refresh,
      child: LayoutBuilder(
        builder: (context, c) {
          final cols = ptAutoColumns(c.maxWidth, 300, minColumns: 1);
          final rowCount = (list.length / cols).ceil();
          return CustomScrollView(
            slivers: [
              SliverList(
                delegate: SliverChildBuilderDelegate(
                  (ctx, rowIdx) {
                    final start = rowIdx * cols;
                    final end = start + cols > list.length ? list.length : start + cols;
                    return Padding(
                      padding: const EdgeInsets.only(bottom: 12),
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          for (int i = start; i < end; i++)
                            Expanded(child: Padding(padding: const EdgeInsets.symmetric(horizontal: 6), child: _mediaCard(list[i]))),
                          for (int i = end - start; i < cols; i++)
                            const Expanded(child: SizedBox()), // 末行不足补空占位，保持等宽
                        ],
                      ),
                    );
                  },
                  childCount: rowCount,
                ),
              ),
              if (!_searchMode)
                SliverToBoxAdapter(
                  child: Padding(
                    padding: const EdgeInsets.only(bottom: 24),
                    child: _loadingMore
                        ? const Center(child: Padding(padding: EdgeInsets.all(12), child: SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))))
                        : _hasMore
                            ? Center(child: FilledButton.tonal(onPressed: _loadMore, child: const Text('加载更多')))
                            : const Center(child: Padding(padding: EdgeInsets.all(8), child: Text('没有更多了', style: TextStyle(color: Colors.grey, fontSize: 12)))),
                  ),
                ),
            ],
          );
        },
      ),
    );
  }

  // 源 + 分类两 pill（各弹候选列表，保留星光集↔短剧联动）；搜索框常驻在下一行。
  Widget _filterPills() {
    String sourceLabel(String key) => _sources.firstWhere((e) => e.$1 == key, orElse: () => ('', key)).$2;
    String categoryLabel(String key) => _categories.firstWhere((e) => e.$1 == key, orElse: () => ('', key)).$2;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 6),
      child: Wrap(
        spacing: 8,
        runSpacing: 6,
        children: [
          PTFilterPill(
            label: '源',
            value: sourceLabel(_source),
            active: _source.isNotEmpty,
            onTap: () => showFilterSheet<String>(
              context,
              title: '筛选来源',
              selected: _source,
              onSelected: (v) => setState(() {
                // 星光集↔短剧互斥：星光集只产短剧、短剧只在星光集，联动校正避免空态。
                _source = v;
                if (v == 'xingguangji') {
                  _category = 'duanju';
                } else if (_category == 'duanju') {
                  _category = 'movie';
                }
                _load(); // 选筛选 → 回信息流
              }),
              options: _sources.map((e) => PTFilterOption(value: e.$1, label: e.$2)).toList(),
            ),
          ),
          PTFilterPill(
            label: '分类',
            value: categoryLabel(_category),
            active: _category != _categories.first.$1,
            onTap: () => showFilterSheet<String>(
              context,
              title: '筛选分类',
              selected: _category,
              onSelected: (v) => setState(() {
                _category = v;
                if (v == 'duanju') {
                  _source = '';
                } else if (_source == 'xingguangji') {
                  _source = '';
                }
                _load(); // 选筛选 → 回信息流
              }),
              options: _categories.map((e) => PTFilterOption(value: e.$1, label: e.$2)).toList(),
            ),
          ),
        ],
      ),
    );
  }

  // 搜索框行：回车或点「搜索」触发跨源搜索；与 Web 推荐页一致并入此处。
  Widget _searchRow() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 0, 12, 4),
      child: Row(children: [
        Expanded(
          child: TextField(
            controller: _kwCtrl,
            textInputAction: TextInputAction.search,
            onSubmitted: (_) => _doSearch(),
            decoration: InputDecoration(
              hintText: '搜索影视/动漫（多源）',
              prefixIcon: const Icon(Icons.search),
              isDense: true,
              border: OutlineInputBorder(borderRadius: BorderRadius.circular(14)),
            ),
          ),
        ),
        const SizedBox(width: 8),
        FilledButton(
          onPressed: _searchLoading ? null : _doSearch,
          child: const Text('搜索'),
        ),
      ]),
    );
  }

  Widget _mediaCard(MediaInfo m) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      onTap: () => _openDetail(m),
      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        SizedBox(width: 72, height: 104, child: ClipRRect(borderRadius: BorderRadius.circular(8), child: MediaPoster(url: m.posterUrl, kind: m.type == 'music' ? 'music' : 'movie'))),
        const SizedBox(width: 12),
        Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(m.title, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
          const SizedBox(height: 2),
          Text('${_sourceLabel(m.source)}${m.year > 0 ? " · ${m.year}" : ""}${m.rating > 0 ? " · ⭐ ${m.rating.toStringAsFixed(1)}" : ""}',
              style: const TextStyle(color: Colors.grey, fontSize: 12)),
          if (m.overview.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(m.overview, maxLines: 3, overflow: TextOverflow.ellipsis, style: const TextStyle(color: Colors.grey, fontSize: 12)),
          ],
          const SizedBox(height: 6),
          Align(
            alignment: Alignment.centerLeft,
            child: FilledButton.tonalIcon(
              onPressed: () => _findOnSites(m),
              icon: const Icon(Icons.download_for_offline_outlined, size: 16),
              label: const Text('在站点查找', style: TextStyle(fontSize: 12)),
            ),
          ),
        ])),
      ]),
    );
  }
}
