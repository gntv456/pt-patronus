// SearchPage 的 SearchOpsMixin mixin（含 setState 段；从 search_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/search/search_page.dart';

mixin SearchOpsMixin on State<SearchPage> {
  void _toast(String m);

  // 影视年份候选：当年倒推到 1980 + 全部（pill 候选列表，避免再来一个输入框）。
  List<PTFilterOption<String>> get _yearOptions {
    final now = DateTime.now().year;
    return [
      const PTFilterOption(value: '', label: '全部'),
      for (var y = now; y >= 1980; y--)
        PTFilterOption(value: '$y', label: '$y'),
    ];
  }

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
                    color: context.dimText,
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
              style: TextStyle(fontSize: 12, color: context.dimText),
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
}
