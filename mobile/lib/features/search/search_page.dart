import 'package:flutter/material.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/theme/glass.dart';

// 搜索页：跨站聚合搜索种子 → 点种子推送下载器。闭合移动端「搜索→下载」工作流。
class SearchPage extends StatefulWidget {
  // 跨页跳转预填关键词（如影视详情页「搜索资源」），非空时进入即自动搜。
  final String? initialKeyword;
  const SearchPage({super.key, this.initialKeyword});
  @override
  State<SearchPage> createState() => _SearchPageState();
}

class _SearchPageState extends State<SearchPage> {
  late final _ctrl = TextEditingController(text: widget.initialKeyword ?? '');
  List<SiteSearchResult> _results = [];
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
      _results = await SearchApi().search(kw, category: _category);
      _searched = true;
    } catch (_) {
      _toast('搜索失败');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
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
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // 展平为「段头 + 种子」混排列表
    final items = <_Item>[];
    for (final s in _results) {
      items.add(_Item.header(s));
      for (final t in s.torrents) {
        items.add(_Item.torrent(t));
      }
    }

    return GlassBackground(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
              child: Row(children: [
                Expanded(
                  child: TextField(
                    controller: _ctrl,
                    textInputAction: TextInputAction.search,
                    onSubmitted: (_) => _search(),
                    decoration: InputDecoration(
                      hintText: '搜索种子…',
                      prefixIcon: const Icon(Icons.search),
                      isDense: true,
                      border: OutlineInputBorder(borderRadius: BorderRadius.circular(14)),
                    ),
                  ),
                ),
                const SizedBox(width: 8),
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
                ),
                const SizedBox(width: 8),
                FilledButton(
                  onPressed: _loading ? null : _search,
                  child: const Text('搜索'),
                ),
              ]),
            ),
            if (_loading) const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: items.isEmpty
                  ? _empty
                  : ListView.builder(
                      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
                      itemCount: items.length,
                      itemBuilder: (_, i) {
                        final it = items[i];
                        if (it.isHeader) return _sectionHeader(it.site!);
                        return _torrentRow(it.torrent!);
                      },
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Widget get _empty {
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

  Widget _torrentRow(SearchTorrent t) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: GlassCard(
        padding: const EdgeInsets.fromLTRB(14, 12, 10, 12),
        onTap: () => _download(t),
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
            Icon(
              t.downloadUrl.isEmpty ? Icons.download_outlined : Icons.download,
              color: PTColors.primary,
            ),
          ],
        ),
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
}

// 列表项：站点段头或种子行（展平分组，便于单 ListView 渲染）。
class _Item {
  final SiteSearchResult? site;
  final SearchTorrent? torrent;
  final bool isHeader;
  _Item.header(this.site)
      : torrent = null,
        isHeader = true;
  _Item.torrent(this.torrent)
      : site = null,
        isHeader = false;
}
