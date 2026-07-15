import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/recommend.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/media_poster.dart';
import 'package:ptpatronus/widgets/find_on_sites_sheet.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import '../media/media_detail_page.dart';

// 推荐页：多源热门信息流（豆瓣热映/TMDB trending/Bangumi 热门/星光集短剧）。
// 关键词搜索统一到「搜索」页（影视 Tab），本页只保留源+分类筛选的信息流，不再单独占一排放搜索框。
class RecommendPage extends StatefulWidget {
  const RecommendPage({super.key});
  @override
  State<RecommendPage> createState() => _RecommendPageState();
}

class _RecommendPageState extends State<RecommendPage> {
  final _api = RecommendApi();
  final _searchApi = SearchApi(); // 供「在站点查找」用；关键词搜索已统一到搜索页。

  StreamSubscription<List<MediaInfo>>? _sub; // 流式首屏订阅（切换/刷新时 cancel）
  List<MediaInfo> _items = []; // 信息流
  bool _loading = true;
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
    _sub?.cancel();
    super.dispose();
  }

  Future<void> _load({bool refresh = false}) {
    _sub?.cancel();
    setState(() {
      _items = [];
      _loading = true;
      _error = null;
      _page = 1;
      _hasMore = true;
    });
    _sub = _api
        .feedStream(source: _source, category: _category, page: 1, refresh: refresh)
        .listen(
      (batch) {
        if (!mounted) return;
        setState(() => _items = [..._items, ...batch]);
      },
      onDone: () {
        if (!mounted) return;
        setState(() {
          _loading = false;
          if (_items.length < _pageSize) _hasMore = false;
        });
      },
      onError: (e) {
        if (!mounted) return;
        setState(() {
          _loading = false;
          _error = e.toString();
        });
      },
    );
    return Future.value();
  }

  // 加载更多：追加下一页；返回不足一页视为到底。
  Future<void> _loadMore() async {
    if (_loadingMore || !_hasMore) return;
    setState(() => _loadingMore = true);
    try {
      final next = await _api.feed(
        source: _source,
        category: _category,
        page: _page + 1,
      );
      if (!mounted) return;
      setState(() {
        _items = [..._items, ...next];
        _page += 1;
        if (next.length < _pageSize) _hasMore = false;
      });
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('$e')));
      }
    } finally {
      if (mounted) setState(() => _loadingMore = false);
    }
  }

  // 顶部刷新 / 下拉刷新。
  Future<void> _refresh() => _load(refresh: true);

  String _sourceLabel(String key) =>
      _sources.firstWhere((e) => e.$1 == key, orElse: () => ('', key)).$2;

  void _findOnSites(MediaInfo m) async {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) =>
          FindOnSitesSheet(api: _searchApi, title: m.title, mediaType: m.type),
    );
  }

  void _openDetail(MediaInfo m) {
    Navigator.push(
      context,
      MaterialPageRoute(
        builder: (_) => MediaDetailPage(source: m.source, id: m.sourceId),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '推荐',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _refresh,
        ),
      ],
      filterDimensions: _filterDimensions(),
      filterOnClearAll: () => setState(() {
        _source = '';
        _category = 'movie';
        _load();
      }),
      body: RefreshIndicator(onRefresh: _refresh, child: _buildList()),
    );
  }

  Widget _buildList() {
    if (_error != null) {
      return PTEmptyState(text: _error!, onRetry: _refresh);
    }
    final list = _items;
    if (list.isEmpty) {
      // 首批未到：轻量转圈占位（非全屏遮罩，首批一到即切列表渐进显示）
      if (_loading) {
        return const Center(child: CircularProgressIndicator());
      }
      return RefreshIndicator(
        onRefresh: _refresh,
        child: const PTEmptyState(
          icon: Icons.recommend_outlined,
          text: '暂无推荐',
          description: '下拉刷新或切换源/分类',
        ),
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
              if (_loading)
                const SliverToBoxAdapter(
                  child: LinearProgressIndicator(minHeight: 2),
                ),
              SliverList(
                delegate: SliverChildBuilderDelegate((ctx, rowIdx) {
                  final start = rowIdx * cols;
                  final end = start + cols > list.length
                      ? list.length
                      : start + cols;
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        for (int i = start; i < end; i++)
                          Expanded(
                            child: Padding(
                              padding: const EdgeInsets.symmetric(
                                horizontal: 6,
                              ),
                              child: _mediaCard(list[i]),
                            ),
                          ),
                        for (int i = end - start; i < cols; i++)
                          const Expanded(child: SizedBox()), // 末行不足补空占位，保持等宽
                      ],
                    ),
                  );
                }, childCount: rowCount),
              ),
              SliverToBoxAdapter(
                child: Padding(
                  padding: const EdgeInsets.only(bottom: 24),
                  child: _loadingMore
                      ? const Center(
                          child: Padding(
                            padding: EdgeInsets.all(12),
                            child: SizedBox(
                              width: 18,
                              height: 18,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            ),
                          ),
                        )
                      : _hasMore
                      ? Center(
                          child: FilledButton.tonal(
                            onPressed: _loadMore,
                            child: const Text('加载更多'),
                          ),
                        )
                      : const Center(
                          child: Padding(
                            padding: EdgeInsets.all(8),
                            child: Text(
                              '没有更多了',
                              style: TextStyle(
                                color: Colors.grey,
                                fontSize: 12,
                              ),
                            ),
                          ),
                        ),
                ),
              ),
            ],
          );
        },
      ),
    );
  }

  // 源 + 分类两维度（收进顶栏筛选按钮）。保留星光集↔短剧互斥联动（星光集只产短剧、
  // 短剧只在星光集）——在各自 onChanged 里校正另一维，选完 _load() 回信息流。
  List<PTFilterDimension> _filterDimensions() {
    return [
      PTFilterDimension.choice(
        label: '来源',
        selected: _source,
        isDefault: _source.isEmpty,
        onChanged: (v) => setState(() {
          _source = (v as String?) ?? '';
          if (_source == 'xingguangji') {
            _category = 'duanju';
          } else if (_category == 'duanju') {
            _category = 'movie';
          }
          _load();
        }),
        options: _sources
            .map((e) => PTFilterOption<Object?>(value: e.$1, label: e.$2))
            .toList(),
      ),
      PTFilterDimension.choice(
        label: '分类',
        selected: _category,
        isDefault: _category == _categories.first.$1,
        onChanged: (v) => setState(() {
          _category = (v as String?) ?? 'movie';
          if (_category == 'duanju') {
            _source = '';
          } else if (_source == 'xingguangji') {
            _source = '';
          }
          _load();
        }),
        options: _categories
            .map((e) => PTFilterOption<Object?>(value: e.$1, label: e.$2))
            .toList(),
      ),
    ];
  }

  Widget _mediaCard(MediaInfo m) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      onTap: () => _openDetail(m),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 72,
            height: 104,
            child: ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: MediaPoster(
                url: m.posterUrl,
                kind: m.type == 'music' ? 'music' : 'movie',
              ),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  m.title,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 15,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  '${_sourceLabel(m.source)}${m.year > 0 ? " · ${m.year}" : ""}${m.rating > 0 ? " · ⭐ ${m.rating.toStringAsFixed(1)}" : ""}',
                  style: const TextStyle(color: Colors.grey, fontSize: 12),
                ),
                if (m.overview.isNotEmpty) ...[
                  const SizedBox(height: 4),
                  Text(
                    m.overview,
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(color: Colors.grey, fontSize: 12),
                  ),
                ],
                const SizedBox(height: 6),
                Align(
                  alignment: Alignment.centerLeft,
                  child: FilledButton.tonalIcon(
                    onPressed: () => _findOnSites(m),
                    icon: const Icon(
                      Icons.download_for_offline_outlined,
                      size: 16,
                    ),
                    label: const Text('在站点查找', style: TextStyle(fontSize: 12)),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
