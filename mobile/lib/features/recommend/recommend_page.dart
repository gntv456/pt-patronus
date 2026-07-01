import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/recommend.dart';
import 'package:ptpatronus/api/discover.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/theme/glass.dart';
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
    });
    try {
      _items = await _api.feed(source: _source, category: _category, refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
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
            _chipRow(_sources, _source, (v) => setState(() {
                  // 星光集↔短剧互斥：星光集只产短剧、短剧只在星光集，联动校正避免空态。
                  _source = v;
                  if (v == 'xingguangji') {
                    _category = 'duanju';
                  } else if (_category == 'duanju') {
                    _category = 'movie';
                  }
                })),
            _chipRow(_categories, _category, (v) => setState(() {
                  _category = v;
                  if (v == 'duanju') {
                    _source = '';
                  } else if (_source == 'xingguangji') {
                    _source = '';
                  }
                })),
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
      return ListView(children: [
        const SizedBox(height: 120),
        Center(child: Text(_error!, style: const TextStyle(color: Colors.redAccent))),
      ]);
    }
    final list = _current;
    if (list.isEmpty) {
      return ListView(
        children: [
          const SizedBox(height: 120),
          Center(
            child: Text(
              _searchMode ? '无搜索结果' : '暂无推荐，下拉刷新或切换源/分类',
              style: const TextStyle(color: Colors.grey),
            ),
          ),
        ],
      );
    }
    return ListView.builder(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      itemCount: list.length,
      itemBuilder: (_, i) => _mediaCard(list[i]),
    );
  }

  Widget _chipRow(List<(String, String)> opts, String current, ValueChanged<String> onTap) {
    return SizedBox(
      height: 38,
      child: ListView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        children: opts
            .map((e) => Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: GestureDetector(
                    onTap: () {
                      onTap(e.$1);
                      _load(); // 选筛选 → 回信息流
                    },
                    child: Container(
                      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
                      decoration: BoxDecoration(
                        color: current == e.$1 ? const Color(0xFF6C5CE7).withValues(alpha: 0.18) : Colors.transparent,
                        border: Border.all(color: current == e.$1 ? const Color(0xFF6C5CE7) : Colors.transparent),
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: Text(e.$2, style: const TextStyle(fontSize: 13)),
                    ),
                  ),
                ))
            .toList(),
      ),
    );
  }

  // 搜索框行：回车或点「搜索」触发跨源搜索；与 Web 推荐页一致并入此处。
  Widget _searchRow() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 2, 12, 6),
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
