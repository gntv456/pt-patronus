import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_widget_from_html/flutter_widget_from_html.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/media.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/api/seedbrowse.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/media_poster.dart';
import '../media/media_detail_page.dart';
import '../media/media_settings_page.dart';

// 统一搜索页（对齐 Web frontend/src/views/Search.vue）：[种子 | 影视] 分段切换，共享关键词。
//  - 种子：跨启用站点聚合搜索 + 推送下载器（原独立 Search）。
//  - 影视：TMDB/豆瓣/Bangumi 元数据 + 海报卡片（原独立 Media，已从导航折叠进此 Tab）。
enum SearchMode { torrent, media }

class SearchPage extends StatefulWidget {
  // 跨页跳转预填关键词（如影视详情页「搜索资源」），非空时进入即自动搜。
  final String? initialKeyword;
  const SearchPage({super.key, this.initialKeyword});
  @override
  State<SearchPage> createState() => _SearchPageState();
}

class _SearchPageState extends State<SearchPage> {
  SearchMode _mode = SearchMode.torrent;
  late final _ctrl = TextEditingController(text: widget.initialKeyword ?? '');
  final _yearCtrl = TextEditingController();

  // 种子结果
  List<SiteSearchResult> _results = [];
  // 影视结果
  List<MediaInfo> _mediaResults = [];

  bool _loading = false;
  bool _searched = false;
  String _category = ''; // ''综合 / movie / tv / music

  @override
  void initState() {
    super.initState();
    // 预填关键词 → 首帧后自动搜索（详情页「搜索资源」回跳）。
    if ((widget.initialKeyword ?? '').isNotEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _search());
    }
  }

  Future<void> _search() async {
    final kw = _ctrl.text.trim();
    if (kw.isEmpty) return;
    setState(() => _loading = true);
    try {
      if (_mode == SearchMode.torrent) {
        _results = await SearchApi().search(kw, category: _category);
      } else {
        final year = int.tryParse(_yearCtrl.text.trim()) ?? 0;
        _mediaResults = await MediaApi().search(kw, year);
      }
      _searched = true;
    } catch (_) {
      _toast(_mode == SearchMode.torrent ? '搜索失败' : '影视搜索失败（请先在「媒体源设置」配置 TMDB/豆瓣）');
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
    });
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  // 下载流程：downloadUrl 空 → 提示；否则弹下载器选择 → 推送。
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
    final picked = await showModalBottomSheet<Downloader>(
      context: context,
      showDragHandle: true,
      builder: (_) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Padding(
              padding: EdgeInsets.only(bottom: 4),
              child: Text('选择下载器', style: TextStyle(fontWeight: FontWeight.w600, fontSize: 16)),
            ),
            ...ds.map((d) => ListTile(
                  leading: const Icon(Icons.download),
                  title: Text(d.name),
                  subtitle: Text(d.url, style: const TextStyle(fontSize: 12)),
                  onTap: () => Navigator.pop(context, d),
                )),
          ],
        ),
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
    _yearCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // 顶栏：☰ + 种子/影视 分段 + （影视模式）媒体源设置
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 12, 12, 4),
              child: Row(children: [
                const HomeMenuButton(),
                const Spacer(),
                SegmentedButton<SearchMode>(
                  segments: const [
                    ButtonSegment(value: SearchMode.torrent, label: Text('种子')),
                    ButtonSegment(value: SearchMode.media, label: Text('影视')),
                  ],
                  selected: {_mode},
                  onSelectionChanged: (s) => _switchMode(s.first),
                ),
                if (_mode == SearchMode.media) ...[
                  const SizedBox(width: 4),
                  IconButton(
                    icon: const Icon(Icons.settings_outlined),
                    tooltip: '媒体源设置',
                    onPressed: () => Navigator.push(
                      context,
                      MaterialPageRoute(builder: (_) => const MediaSettingsPage()),
                    ),
                  ),
                ],
              ]),
            ),
            // 搜索栏
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
              child: Row(children: [
                Expanded(
                  child: TextField(
                    controller: _ctrl,
                    textInputAction: TextInputAction.search,
                    onSubmitted: (_) => _search(),
                    decoration: InputDecoration(
                      hintText: _mode == SearchMode.torrent ? '搜索种子…' : '搜索影视（电影 / 剧集）',
                      prefixIcon: const Icon(Icons.search),
                      isDense: true,
                      border: OutlineInputBorder(borderRadius: BorderRadius.circular(14)),
                    ),
                  ),
                ),
                const SizedBox(width: 8),
                if (_mode == SearchMode.torrent)
                  DropdownButton<String>(
                    value: _category,
                    underline: const SizedBox(),
                    style: const TextStyle(fontSize: 14, color: Colors.black87),
                    items: const [
                      DropdownMenuItem(value: '', child: Text('全部')),
                      DropdownMenuItem(value: 'movie', child: Text('电影')),
                      DropdownMenuItem(value: 'tv', child: Text('剧集')),
                      DropdownMenuItem(value: 'music', child: Text('音乐')),
                    ],
                    onChanged: (v) => setState(() => _category = v ?? ''),
                  )
                else
                  SizedBox(
                    width: 76,
                    child: TextField(
                      controller: _yearCtrl,
                      keyboardType: TextInputType.number,
                      decoration: InputDecoration(
                        hintText: '年份',
                        isDense: true,
                        border: OutlineInputBorder(borderRadius: BorderRadius.circular(14)),
                      ),
                    ),
                  ),
                const SizedBox(width: 8),
                FilledButton(
                  onPressed: _loading ? null : _search,
                  child: const Text('搜索'),
                ),
              ]),
            ),
            if (_loading) const LinearProgressIndicator(minHeight: 2),
            Expanded(child: _mode == SearchMode.torrent ? _torrentArea : _mediaArea),
          ],
        ),
      ),
    );
  }

  // —— 种子结果区 ——
  Widget get _torrentArea {
    // 展平为「段头 + 种子」混排列表（种子带所属站点，供详情抽屉取 detail）
    final items = <_Item>[];
    for (final s in _results) {
      items.add(_Item.header(s));
      for (final t in s.torrents) {
        items.add(_Item.torrent(t, s.siteId, s.siteName));
      }
    }
    if (items.isEmpty) return _emptyTorrent;
    return RefreshIndicator(
      onRefresh: _search,
      child: ListView.builder(
        padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
        itemCount: items.length,
        itemBuilder: (_, i) {
          final it = items[i];
          if (it.isHeader) return _sectionHeader(it.site!);
          return _torrentRow(it.torrent!, it.siteId, it.siteName);
        },
      ),
    );
  }

  Widget get _emptyTorrent {
    final msg = _searched ? '无结果，请先在「站点」添加并登录站点' : '输入关键词跨站搜索';
    return Center(child: Text(msg, style: const TextStyle(color: Colors.grey)));
  }

  Widget _sectionHeader(SiteSearchResult s) {
    final hasErr = s.error.isNotEmpty;
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 14, 4, 6),
      child: Row(children: [
        Text(s.siteName, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
        const SizedBox(width: 8),
        if (hasErr)
          Expanded(child: Text(s.error, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(color: Colors.redAccent, fontSize: 12)))
        else
          Text('${s.torrents.length} 条', style: const TextStyle(color: Colors.grey, fontSize: 12)),
      ]),
    );
  }

  Widget _torrentRow(SearchTorrent t, int siteId, String siteName) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: GlassCard(
        // 点卡片 → 打开详情抽屉（对齐 Web Search.vue「详情」；主操作先看再下）
        onTap: () => _openDetail(t, siteId, siteName),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(t.title, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
                  if (t.subtitle.isNotEmpty) ...[
                    const SizedBox(height: 2),
                    Text(t.subtitle, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(color: Colors.grey, fontSize: 12)),
                  ],
                  const SizedBox(height: 6),
                  Wrap(
                    spacing: 6,
                    runSpacing: 4,
                    crossAxisAlignment: WrapCrossAlignment.center,
                    children: [
                      if (t.size.isNotEmpty) _chip(t.size, Colors.blueGrey),
                      if (t.seeders > 0) _chip('↑${t.seeders}', Colors.green),
                      if (t.leechers > 0) _chip('↓${t.leechers}', Colors.orange),
                      if (t.free) _chip('FREE', PTColors.success),
                    ],
                  ),
                ],
              ),
            ),
            // 快速推送下载（详情抽屉里也有下载按钮）
            IconButton(
              icon: Icon(
                t.downloadUrl.isEmpty ? Icons.download_outlined : Icons.download,
                color: PTColors.primary,
              ),
              tooltip: '推送下载',
              onPressed: () => _download(t),
            ),
          ],
        ),
      ),
    );
  }

  // 打开种子详情抽屉（对齐 Web Search.vue openTorrentDetail）：抓 detail_url 解析简介/截图/MediaInfo/文件。
  void _openDetail(SearchTorrent t, int siteId, String siteName) {
    if (t.detailUrl.isEmpty) {
      _toast('该种子无详情页');
      return;
    }
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      backgroundColor: Colors.transparent,
      builder: (_) => _TorrentDetailSheet(
        siteId: siteId,
        siteName: siteName,
        detailUrl: t.detailUrl,
        torrent: t,
        onDownload: () => _download(t),
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
      child: Text(label, style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }

  // —— 影视结果区 ——
  Widget get _mediaArea {
    if (_mediaResults.isEmpty) {
      return Center(
        child: Text(_searched ? '无结果' : '输入关键字搜索影视元数据与海报',
            style: const TextStyle(color: Colors.grey)),
      );
    }
    final wide = MediaQuery.of(context).size.width > 600;
    return GridView.builder(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: wide ? 3 : 2,
        mainAxisSpacing: 12,
        crossAxisSpacing: 12,
        childAspectRatio: 0.54,
      ),
      itemCount: _mediaResults.length,
      itemBuilder: (_, i) => _mediaCard(_mediaResults[i]),
    );
  }

  Widget _mediaCard(MediaInfo m) {
    return GestureDetector(
      onTap: () => Navigator.push(
        context,
        MaterialPageRoute(builder: (_) => MediaDetailPage(source: m.source, id: m.sourceId)),
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
                        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                        decoration: BoxDecoration(color: Colors.black54, borderRadius: BorderRadius.circular(6)),
                        child: Text(_src(m.source),
                            style: const TextStyle(color: Colors.white, fontSize: 10, fontWeight: FontWeight.w600)),
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
                    Text(m.title,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13)),
                    const SizedBox(height: 4),
                    Row(children: [
                      _typeBadge(m.type),
                      if (m.year > 0) ...[
                        const SizedBox(width: 5),
                        Text('${m.year}', style: const TextStyle(color: Colors.grey, fontSize: 11)),
                      ],
                      if (m.rating > 0) ...[
                        const SizedBox(width: 5),
                        Text('⭐${m.rating.toStringAsFixed(1)}',
                            style: const TextStyle(color: Colors.orange, fontSize: 11)),
                      ],
                    ]),
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
        gradient: const LinearGradient(colors: [PTColors.primary, PTColors.primary2]),
      ),
      child: Text(label,
          style: const TextStyle(color: Colors.white, fontSize: 10, fontWeight: FontWeight.w600)),
    );
  }

  String _src(String s) =>
      const {'tmdb': 'TMDB', 'douban': '豆瓣', 'musicbrainz': 'MB'}[s] ?? s.toUpperCase();
}

// 列表项：站点段头或种子行（展平分组，便于单 ListView 渲染）。
class _Item {
  final SiteSearchResult? site;
  final SearchTorrent? torrent;
  final int siteId; // 种子项所属站点（详情抽屉取 detail 用）
  final String siteName;
  final bool isHeader;
  _Item.header(this.site)
      : torrent = null,
        siteId = 0,
        siteName = '',
        isHeader = true;
  _Item.torrent(this.torrent, this.siteId, this.siteName)
      : site = null,
        isHeader = false;
}

/// 种子详情抽屉（对齐 Web Search.vue 的 el-drawer）：抓取 detail_url 解析后展示
/// 海报/规格/截图/简介/MediaInfo/文件列表 + 下载/复制链接。简介是站点原始 HTML，展示前剥标签为纯文本。
class _TorrentDetailSheet extends StatefulWidget {
  final int siteId;
  final String siteName;
  final String detailUrl;
  final SearchTorrent torrent; // fetch 完成前先展示已知字段
  final VoidCallback onDownload;

  const _TorrentDetailSheet({
    required this.siteId,
    required this.siteName,
    required this.detailUrl,
    required this.torrent,
    required this.onDownload,
  });

  @override
  State<_TorrentDetailSheet> createState() => _TorrentDetailSheetState();
}

class _TorrentDetailSheetState extends State<_TorrentDetailSheet> {
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
    if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已复制下载链接')));
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final bg = isDark ? const Color(0xFF1C1B29) : Colors.white;
    final sub = isDark ? Colors.white70 : Colors.black54;
    return Container(
      height: MediaQuery.of(context).size.height * 0.90,
      decoration: BoxDecoration(
        color: bg,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Center(
            child: Container(
              width: 40,
              height: 4,
              margin: const EdgeInsets.symmetric(vertical: 10),
              decoration: BoxDecoration(color: sub.withValues(alpha: 0.4), borderRadius: BorderRadius.circular(2)),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 2, 8, 8),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(widget.siteName, style: TextStyle(color: sub, fontSize: 12)),
                      Text(_detail?.title ?? widget.torrent.title,
                          maxLines: 2, overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 17)),
                    ],
                  ),
                ),
                IconButton(
                  icon: const Icon(Icons.close),
                  tooltip: '关闭',
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ],
            ),
          ),
          const Divider(height: 1),
          Expanded(child: _body),
        ],
      ),
    );
  }

  Widget get _body {
    if (_loading) return const Center(child: CircularProgressIndicator());
    if (_err != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Text(_err!, style: const TextStyle(color: Colors.redAccent), textAlign: TextAlign.center),
        ),
      );
    }
    final d = _detail;
    if (d == null) return const Center(child: Text('无详情'));
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
      children: [
        _header(d),
        if (d.screenshots.isNotEmpty) ...[_section('截图（${d.screenshots.length}）'), _screens(d.screenshots)],
        if (d.description.trim().isNotEmpty) ...[
          _section('简介'),
          HtmlWidget(d.description, textStyle: const TextStyle(height: 1.6)),
        ],
        if (d.mediainfo.isNotEmpty) ...[_section('MediaInfo'), _mediainfo(d.mediainfo)],
        if (d.fileList.isNotEmpty) ...[_section('文件列表（${d.fileList.length}）'), _fileList(d.fileList)],
      ],
    );
  }

  Widget _header(TorrentDetail d) {
    final dlUrl = d.downloadUrl.isEmpty ? widget.torrent.downloadUrl : d.downloadUrl;
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
                  child: Text(d.subtitle, style: TextStyle(color: Colors.grey, fontSize: 12)),
                ),
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [
                  if (d.category.isNotEmpty) _spec(d.category, PTColors.primary),
                  if (d.resolution.isNotEmpty) _spec(d.resolution, const Color(0xFF4285F4)),
                  if (d.codec.isNotEmpty) _spec(d.codec, const Color(0xFF9C27B0)),
                  if (d.source.isNotEmpty) _spec(d.source, const Color(0xFF009688)),
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
        child: Image.network(urls[i], fit: BoxFit.cover, errorBuilder: (_, _, _) => Container(color: Colors.black12)),
      ),
    );
  }

  Widget _mediainfo(String text) => Container(
        width: double.infinity,
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(color: const Color(0xEE000000), borderRadius: BorderRadius.circular(8)),
        child: SelectableText(text,
            style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 12, height: 1.5, fontFamily: 'monospace')),
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
                    Expanded(child: SelectableText(files[i], style: const TextStyle(fontSize: 12, fontFamily: 'monospace'))),
                  ],
                ),
              ),
          ],
        ),
      );

  Widget _section(String title) => Padding(
        padding: const EdgeInsets.fromLTRB(0, 18, 0, 8),
        child: Text(title, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14)),
      );

  Widget _spec(String label, Color c) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
        decoration: BoxDecoration(color: c.withValues(alpha: 0.16), borderRadius: BorderRadius.circular(6)),
        child: Text(label, style: TextStyle(color: c, fontSize: 11, fontWeight: FontWeight.w600)),
      );
}
