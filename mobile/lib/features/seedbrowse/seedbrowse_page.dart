import 'package:flutter/material.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/api/seedbrowse.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/media_poster.dart';

/// 种子浏览：跨站搜索 → 点击种子查看详情页（简介/截图/MediaInfo，复用详情页解析能力）。
class SeedBrowsePage extends StatefulWidget {
  const SeedBrowsePage({super.key});
  @override
  State<SeedBrowsePage> createState() => _SeedBrowsePageState();
}

class _SeedBrowsePageState extends State<SeedBrowsePage> {
  final _searchApi = SearchApi();
  final _detailApi = SeedBrowseApi();
  final _ctrl = TextEditingController();
  List<SiteSearchResult> _results = [];
  bool _loading = false;

  Future<void> _search() async {
    final kw = _ctrl.text.trim();
    if (kw.isEmpty) return;
    setState(() => _loading = true);
    try {
      _results = await _searchApi.search(kw);
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  List<({int siteId, String siteName, SearchTorrent t})> get _flat {
    final out = <({int siteId, String siteName, SearchTorrent t})>[];
    for (final s in _results) {
      if (s.error.isNotEmpty) continue;
      for (final t in s.torrents) {
        out.add((siteId: s.siteId, siteName: s.siteName, t: t));
      }
    }
    return out;
  }

  void _openDetail(int siteId, SearchTorrent t) async {
    if (t.detailUrl.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('该种子无详情页链接')));
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('加载详情…'), duration: Duration(seconds: 1)));
    try {
      final d = await _detailApi.detail(siteId, t.detailUrl);
      if (!mounted) return;
      if (d == null) return;
      showModalBottomSheet(
        context: context,
        isScrollControlled: true,
        builder: (_) => _DetailSheet(detail: d),
      );
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    }
  }

  @override
  Widget build(BuildContext context) {
    final flat = _flat;
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(title: const Text('种子浏览'), backgroundColor: Colors.transparent, elevation: 0),
        body: Column(
          children: [
            Padding(
              padding: const EdgeInsets.all(12),
              child: GlassCard(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
                child: Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _ctrl,
                        decoration: const InputDecoration(hintText: '跨站搜索，点击查看详情页', border: InputBorder.none),
                        onSubmitted: (_) => _search(),
                      ),
                    ),
                    IconButton(icon: const Icon(Icons.search), onPressed: _loading ? null : _search),
                  ],
                ),
              ),
            ),
            Expanded(
              child: _loading && flat.isEmpty
                  ? const Center(child: CircularProgressIndicator())
                  : flat.isEmpty
                      ? const _Empty(text: '输入关键词搜索，点击种子查看详情')
                      : RefreshIndicator(
                          onRefresh: _search,
                          child: ListView.builder(
                            padding: const EdgeInsets.symmetric(horizontal: 12),
                            itemCount: flat.length,
                            itemBuilder: (_, i) {
                              final r = flat[i];
                              return GlassCard(
                                onTap: () => _openDetail(r.siteId, r.t),
                                padding: const EdgeInsets.all(12),
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Row(children: [
                                      _siteTag(r.siteName),
                                      const Spacer(),
                                      Text(r.t.size, style: const TextStyle(color: Colors.grey, fontSize: 12)),
                                    ]),
                                    const SizedBox(height: 4),
                                    Text(r.t.title, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontWeight: FontWeight.w500)),
                                    const SizedBox(height: 4),
                                    Row(children: [
                                      Text('${r.t.seeders} 种', style: const TextStyle(color: Color(0xFF2ECC71), fontSize: 12)),
                                      const SizedBox(width: 8),
                                      Text('${r.t.leechers} 下载', style: const TextStyle(color: Color(0xFFF39C12), fontSize: 12)),
                                      if (r.t.free) ...[
                                        const SizedBox(width: 8),
                                        const Text('FREE', style: TextStyle(color: Color(0xFF2ECC71), fontSize: 12, fontWeight: FontWeight.w600)),
                                      ],
                                    ]),
                                  ],
                                ),
                              );
                            },
                          ),
                        ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _siteTag(String name) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
        decoration: BoxDecoration(color: const Color(0xFF6C5CE7).withValues(alpha: 0.16), borderRadius: BorderRadius.circular(999)),
        child: Text(name, style: const TextStyle(color: Color(0xFF6C5CE7), fontSize: 11)),
      );
}

class _DetailSheet extends StatelessWidget {
  final TorrentDetail detail;
  const _DetailSheet({required this.detail});
  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.85,
      child: Column(
        children: [
          AppBar(
            title: Text(detail.title, overflow: TextOverflow.ellipsis),
            automaticallyImplyLeading: false,
            actions: [IconButton(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.close))],
          ),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.all(12),
              children: [
                Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  if (detail.posterUrl.isNotEmpty)
                    SizedBox(width: 90, height: 130, child: ClipRRect(borderRadius: BorderRadius.circular(8), child: MediaPoster(url: detail.posterUrl)))
                  else
                    const SizedBox.shrink(),
                  const SizedBox(width: 12),
                  Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text(detail.title, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                    if (detail.subtitle.isNotEmpty) ...[const SizedBox(height: 2), Text(detail.subtitle, style: const TextStyle(color: Colors.grey, fontSize: 12))],
                    const SizedBox(height: 8),
                    Wrap(spacing: 6, runSpacing: 4, children: [
                      if (detail.category.isNotEmpty) _chip(detail.category),
                      if (detail.size.isNotEmpty) _chip(detail.size),
                      if (detail.resolution.isNotEmpty) _chip(detail.resolution),
                      if (detail.codec.isNotEmpty) _chip(detail.codec),
                      if (detail.source.isNotEmpty) _chip(detail.source),
                      if (detail.imdbId.isNotEmpty) _chip(detail.imdbId),
                    ]),
                  ])),
                ]),
                if (detail.screenshots.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  const Text('截图', style: TextStyle(fontWeight: FontWeight.w600)),
                  const SizedBox(height: 6),
                  SizedBox(
                    height: 100,
                    child: ListView.separated(
                      scrollDirection: Axis.horizontal,
                      itemCount: detail.screenshots.length,
                      separatorBuilder: (_, _) => const SizedBox(width: 8),
                      itemBuilder: (_, i) => SizedBox(width: 160, child: ClipRRect(borderRadius: BorderRadius.circular(8), child: MediaPoster(url: detail.screenshots[i]))),
                    ),
                  ),
                ],
                if (detail.description.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  const Text('简介', style: TextStyle(fontWeight: FontWeight.w600)),
                  const SizedBox(height: 4),
                  Text(detail.description.replaceAll(RegExp(r'<[^>]*>'), ' ').trim(), style: const TextStyle(fontSize: 13, height: 1.5)),
                ],
                if (detail.mediainfo.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  const Text('MediaInfo', style: TextStyle(fontWeight: FontWeight.w600)),
                  const SizedBox(height: 4),
                  Container(
                    padding: const EdgeInsets.all(8),
                    decoration: BoxDecoration(color: const Color(0x0D000000), borderRadius: BorderRadius.circular(8)),
                    child: Text(detail.mediainfo, style: const TextStyle(fontFamily: 'monospace', fontSize: 11)),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _chip(String t) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
        decoration: BoxDecoration(color: const Color(0x0F000000), borderRadius: BorderRadius.circular(999)),
        child: Text(t, style: const TextStyle(fontSize: 10)),
      );
}

class _Empty extends StatelessWidget {
  final String text;
  const _Empty({required this.text});
  @override
  Widget build(BuildContext context) => ListView(children: [
        const SizedBox(height: 120),
        Center(child: Padding(padding: const EdgeInsets.all(24), child: Text(text, textAlign: TextAlign.center, style: const TextStyle(color: Colors.grey)))),
      ]);
}
