import 'package:flutter/material.dart';
import 'package:ptpatronus/api/search.dart';
import 'package:ptpatronus/theme/glass.dart';

/// 「在站点查找」结果抽屉：用媒体标题做 keyword 跨站搜索（复用既有 /api/v1/search），
/// 展示每个命中站点的种子。按需联动（on-demand join）——不做可用性预过滤。
/// mediaType（movie/tv/music）透传给后端按分类过滤（馒头 mode / NexusPHP cat[]+后置）。
class FindOnSitesSheet extends StatefulWidget {
  final SearchApi api;
  final String title;
  final String mediaType;
  const FindOnSitesSheet({super.key, required this.api, required this.title, this.mediaType = ''});
  @override
  State<FindOnSitesSheet> createState() => _FindOnSitesSheetState();
}

class _FindOnSitesSheetState extends State<FindOnSitesSheet> {
  List<SiteSearchResult> _results = [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _run();
  }

  Future<void> _run() async {
    try {
      _results = await widget.api.search(widget.title, category: widget.mediaType);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    int sites = 0, total = 0;
    for (final s in _results) {
      if (s.error.isEmpty && s.torrents.isNotEmpty) {
        sites++;
        total += s.torrents.length;
      }
    }
    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.8,
      child: Column(children: [
        AppBar(
          title: Text('在站点查找：${widget.title}', overflow: TextOverflow.ellipsis),
          automaticallyImplyLeading: false,
          actions: [IconButton(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.close))],
        ),
        if (_error != null)
          Padding(padding: const EdgeInsets.all(12), child: Text('查找失败：$_error', style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 13)))
        else if (!_loading)
          Padding(padding: const EdgeInsets.all(12), child: Text('命中 $sites 个站点，共 $total 条种子', style: const TextStyle(fontSize: 13))),
        Expanded(
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error != null
                  ? const SizedBox.shrink()
                  : ListView.builder(
                  padding: const EdgeInsets.all(12),
                  itemCount: _results.length,
                  itemBuilder: (_, i) => _siteBlock(_results[i]),
                ),
        ),
      ]),
    );
  }

  Widget _siteBlock(SiteSearchResult s) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
            decoration: BoxDecoration(color: const Color(0xFF6C5CE7).withValues(alpha: 0.16), borderRadius: BorderRadius.circular(999)),
            child: Text(s.siteName, style: const TextStyle(color: Color(0xFF6C5CE7), fontSize: 11)),
          ),
          const SizedBox(width: 8),
          if (s.error.isNotEmpty)
            Text(s.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 12))
          else if (s.torrents.isEmpty)
            const Text('未命中', style: TextStyle(color: Colors.grey, fontSize: 12))
          else
            Text('${s.torrents.length} 条', style: const TextStyle(color: Colors.grey, fontSize: 12)),
        ]),
        ...s.torrents.take(5).map((t) => Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Row(children: [
                Expanded(child: Text(t.title, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 12))),
                Text('${t.size} · ${t.seeders}种', style: const TextStyle(color: Colors.grey, fontSize: 11)),
              ]),
            )),
      ]),
    );
  }
}
