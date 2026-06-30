import 'package:flutter/material.dart';
import 'package:ptpatronus/api/announcement.dart';
import 'package:ptpatronus/theme/glass.dart';

/// 公告页：按站点分组展示公告条目，每条可展开正文。
class AnnouncementsPage extends StatefulWidget {
  const AnnouncementsPage({super.key});
  @override
  State<AnnouncementsPage> createState() => _AnnouncementsPageState();
}

class _AnnouncementsPageState extends State<AnnouncementsPage> {
  final _api = AnnouncementApi();
  List<AnnouncementResult> _results = [];
  bool _loading = true;
  String? _error;
  final _open = <String, bool>{};

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _results = await _api.all(refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          title: const Text('公告'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : () => _load(refresh: true)),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => _load(refresh: true),
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error != null
                  ? _Empty(text: _error!, onRetry: _load)
                  : _results.isEmpty
                      ? const _Empty(text: '暂无数据，请先添加并登录站点')
                      : ListView.builder(
                          padding: const EdgeInsets.all(12),
                          itemCount: _results.length,
                          itemBuilder: (_, i) => _group(_results[i]),
                        ),
        ),
      ),
    );
  }

  Widget _group(AnnouncementResult r) {
    final hasItems = r.items.isNotEmpty;
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Text(r.siteName, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
            const SizedBox(width: 8),
            if (hasItems)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(color: const Color(0xFF6C5CE7).withValues(alpha: 0.16), borderRadius: BorderRadius.circular(999)),
                child: Text('${r.items.length} 条', style: const TextStyle(color: Color(0xFF6C5CE7), fontSize: 12)),
              )
            else if (r.error.isNotEmpty)
              Text(r.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 12))
            else
              const Text('暂无公告', style: TextStyle(color: Colors.grey, fontSize: 12)),
          ]),
          if (hasItems)
            ...r.items.asMap().entries.map((e) {
              final key = '${r.siteId}-${e.key}';
              final open = _open[key] ?? false;
              final it = e.value;
              return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                InkWell(
                  onTap: () => setState(() => _open[key] = !open),
                  child: Padding(
                    padding: const EdgeInsets.only(top: 10),
                    child: Row(children: [
                      Expanded(child: Text(it.title.isNotEmpty ? it.title : '（无标题）', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14))),
                      if (it.date.isNotEmpty) Padding(padding: const EdgeInsets.only(right: 8), child: Text(it.date, style: const TextStyle(color: Colors.grey, fontSize: 12))),
                      Text(open ? '收起' : '展开', style: const TextStyle(color: Color(0xFF6C5CE7), fontSize: 12)),
                    ]),
                  ),
                ),
                if (open)
                  Padding(padding: const EdgeInsets.only(top: 6, left: 4), child: Text(it.content, style: const TextStyle(fontSize: 13, height: 1.5))),
              ]);
            })
          else if (r.rawText.isNotEmpty)
            Padding(padding: const EdgeInsets.only(top: 8), child: Text(r.rawText.length > 200 ? '${r.rawText.substring(0, 200)}…' : r.rawText, style: const TextStyle(color: Colors.grey, fontSize: 13))),
        ],
      ),
    );
  }
}

class _Empty extends StatelessWidget {
  final String text;
  final VoidCallback? onRetry;
  const _Empty({required this.text, this.onRetry});
  @override
  Widget build(BuildContext context) {
    return ListView(
      children: [
        const SizedBox(height: 120),
        Center(child: Padding(padding: const EdgeInsets.all(24), child: Text(text, textAlign: TextAlign.center, style: const TextStyle(color: Colors.grey)))),
        if (onRetry != null) Center(child: ElevatedButton(onPressed: onRetry, child: const Text('重试'))),
      ],
    );
  }
}
