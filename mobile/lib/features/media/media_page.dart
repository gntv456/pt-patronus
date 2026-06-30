import 'package:flutter/material.dart';
import 'package:ptpatronus/api/media.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'media_detail_page.dart';
import 'media_settings_page.dart';

// 媒体/海报页：搜影视/音乐 → 海报网格（来源/标题/类型/年份/评分）。
class MediaPage extends StatefulWidget {
  const MediaPage({super.key});
  @override
  State<MediaPage> createState() => _MediaPageState();
}

class _MediaPageState extends State<MediaPage> {
  final _ctrl = TextEditingController();
  final _yearCtrl = TextEditingController();
  List<MediaInfo> _results = [];
  bool _loading = false;
  bool _searched = false;

  Future<void> _search() async {
    final kw = _ctrl.text.trim();
    if (kw.isEmpty) return;
    final year = int.tryParse(_yearCtrl.text.trim()) ?? 0;
    setState(() => _loading = true);
    try {
      _results = await MediaApi().search(kw, year);
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

  @override
  void dispose() {
    _ctrl.dispose();
    _yearCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.of(context).size.width > 600;
    return GlassBackground(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 8, 8),
              child: Row(children: [
                const Text('媒体', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700)),
                const Spacer(),
                IconButton(
                  icon: const Icon(Icons.settings_outlined),
                  tooltip: '媒体源设置',
                  onPressed: () => Navigator.push(
                    context,
                    MaterialPageRoute(builder: (_) => const MediaSettingsPage()),
                  ),
                ),
              ]),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: Row(children: [
                Expanded(
                  flex: 3,
                  child: TextField(
                    controller: _ctrl,
                    textInputAction: TextInputAction.search,
                    onSubmitted: (_) => _search(),
                    decoration: InputDecoration(
                      hintText: '搜索影视 / 音乐',
                      prefixIcon: const Icon(Icons.search),
                      isDense: true,
                      border: OutlineInputBorder(borderRadius: BorderRadius.circular(14)),
                    ),
                  ),
                ),
                const SizedBox(width: 8),
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
            Expanded(
              child: _results.isEmpty
                  ? _empty
                  : GridView.builder(
                      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
                      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                        crossAxisCount: wide ? 3 : 2,
                        mainAxisSpacing: 12,
                        crossAxisSpacing: 12,
                        childAspectRatio: 0.54,
                      ),
                      itemCount: _results.length,
                      itemBuilder: (_, i) => _card(_results[i]),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Widget get _empty => Center(
        child: Text(_searched ? '无结果' : '输入关键字搜索影视/音乐元数据与海报',
            style: const TextStyle(color: Colors.grey)),
      );

  Widget _card(MediaInfo m) {
    return GestureDetector(
      onTap: () => _openDetail(m),
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
                    _poster(m),
                    Positioned(
                      top: 6,
                      right: 6,
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                        decoration: BoxDecoration(
                            color: Colors.black54, borderRadius: BorderRadius.circular(6)),
                        child: Text(_src(m.source),
                            style: const TextStyle(
                                color: Colors.white, fontSize: 10, fontWeight: FontWeight.w600)),
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

  // 点击卡片 → 进入应用内影视详情页（对标 MoviePilot），不再跳外站。
  void _openDetail(MediaInfo m) {
    Navigator.push(
      context,
      MaterialPageRoute(builder: (_) => MediaDetailPage(source: m.source, id: m.sourceId)),
    );
  }

  Widget _noPoster() => Container(
        color: Colors.black12,
        child: const Center(child: Icon(Icons.movie, size: 36, color: Colors.grey)),
      );

  // 豆瓣 doubanio.com 防盗链：海报图改走后端 /media/image 反代（带 JWT），
  // 其它公网 CDN（TMDB / Cover Art）直连。失败/无图回退占位图标。
  Widget _poster(MediaInfo m) {
    if (m.posterUrl.isEmpty) return _noPoster();
    final proxy = ApiClient().mediaImageUri(m.posterUrl);
    if (proxy == null) {
      return Image.network(m.posterUrl,
          fit: BoxFit.cover, errorBuilder: (_, _, _) => _noPoster());
    }
    return Image.network(proxy.toString(),
        fit: BoxFit.cover,
        headers: ApiClient().authHeaders,
        errorBuilder: (_, _, _) => _noPoster());
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
