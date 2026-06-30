import 'package:flutter/material.dart';
import 'package:ptpatronus/api/subscribe.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/media_poster.dart';

// 订阅列表（对标 Web Subscriptions）：展示订阅状态，支持手动检查 / 暂停-恢复 /
// 改分辨率 / 删除。订阅的创建在影视详情页「订阅」按钮（同源同 id 幂等）。
class SubscriptionsPage extends StatefulWidget {
  const SubscriptionsPage({super.key});
  @override
  State<SubscriptionsPage> createState() => _SubscriptionsPageState();
}

class _SubscriptionsPageState extends State<SubscriptionsPage> {
  final _api = SubscribeApi();
  List<Subscription> _subs = [];
  bool _loading = true;
  String? _error;
  final _checking = <int>{};

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _subs = await _api.list();
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  // 立即检查：返回本轮命中推送条数。
  Future<void> _check(Subscription s) async {
    setState(() => _checking.add(s.id));
    try {
      final r = await _api.check(s.id);
      final ok = r['ok'] == true;
      final pushed = (r['pushed'] as num?)?.toInt() ?? 0;
      final err = r['error'] as String?;
      _toast(ok
          ? (pushed > 0 ? '命中并推送 $pushed 条' : '本轮暂无新资源')
          : (err?.isNotEmpty == true ? err! : '检查失败'));
      await _load();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _checking.remove(s.id));
    }
  }

  Future<void> _toggle(Subscription s) async {
    final next = s.status == 'paused' ? 'subscribed' : 'paused';
    try {
      await _api.update(s.id, {'status': next});
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  // 分辨率过滤：弹窗编辑（留空=不过滤；后端空=不改，故已设值无法在此清空，与 Web 一致）。
  Future<void> _editResolution(Subscription s) async {
    final ctrl = TextEditingController(text: s.resolution);
    final v = await showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('分辨率过滤'),
        content: TextField(
          controller: ctrl,
          autofocus: true,
          decoration: const InputDecoration(hintText: '如 1080p / 2160p，留空=不限'),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('取消')),
          TextButton(
              onPressed: () => Navigator.pop(context, ctrl.text.trim()),
              child: const Text('确定')),
        ],
      ),
    );
    if (v == null) return;
    try {
      await _api.update(s.id, {'resolution': v});
      _toast('分辨率已更新');
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _remove(Subscription s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除订阅'),
        content: Text('确认删除「${s.title}」的订阅？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(s.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  String _statusText(String s) =>
      const {'subscribed': '订阅中', 'completed': '已完成', 'paused': '已暂停'}[s] ?? s;

  Color _statusColor(String s) => switch (s) {
        'subscribed' => PTColors.success,
        'completed' => PTColors.primary,
        _ => PTColors.warn,
      };

  String _fmtTime(String? t) => (t == null || t.isEmpty)
      ? '—'
      : t.replaceAll('T', ' ').replaceAll(RegExp(r'\.\d+Z?$'), '');

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          title: const Text('订阅'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : _load),
          ],
        ),
        body: SafeArea(
          child: RefreshIndicator(
            onRefresh: _load,
            child: _buildBody(),
          ),
        ),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading && _subs.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return ListView(children: [
        const SizedBox(height: 120),
        Center(child: Text(_error!, style: const TextStyle(color: Colors.redAccent))),
      ]);
    }
    if (_subs.isEmpty) {
      return ListView(children: [
        const SizedBox(height: 120),
        const Center(
          child: Padding(
            padding: EdgeInsets.all(24),
            child: Text('暂无订阅。去推荐/媒体搜到影视 → 进详情页 → 订阅',
                textAlign: TextAlign.center, style: TextStyle(color: Colors.grey)),
          ),
        ),
      ]);
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      itemCount: _subs.length,
      separatorBuilder: (_, _) => const SizedBox(height: 12),
      itemBuilder: (_, i) => _card(_subs[i]),
    );
  }

  Widget _card(Subscription s) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 72,
            height: 104,
            child: ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: MediaPoster(url: s.posterUrl, kind: s.type == 'music' ? 'music' : 'movie'),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Flexible(
                      child: Text(s.title,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
                    ),
                    if (s.year > 0)
                      Padding(
                        padding: const EdgeInsets.only(left: 4),
                        child: Text('(${s.year})',
                            style: const TextStyle(color: Colors.grey, fontSize: 13)),
                      ),
                  ],
                ),
                const SizedBox(height: 4),
                Wrap(
                  spacing: 6,
                  runSpacing: 4,
                  children: [
                    _chip(_statusText(s.status), _statusColor(s.status)),
                    if (s.resolution.isNotEmpty) _chip(s.resolution, PTColors.primary2),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  '上次检查：${_fmtTime(s.lastCheckedAt)} · 类型 ${s.type == 'tv' ? '剧集' : '电影'} · 源 ${s.source}',
                  style: const TextStyle(color: Colors.grey, fontSize: 12),
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    FilledButton.tonalIcon(
                      onPressed: _checking.contains(s.id) ? null : () => _check(s),
                      icon: _checking.contains(s.id)
                          ? const SizedBox(
                              width: 14,
                              height: 14,
                              child: CircularProgressIndicator(strokeWidth: 2))
                          : const Icon(Icons.refresh, size: 16),
                      label: const Text('立即检查', style: TextStyle(fontSize: 12)),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _editResolution(s),
                      icon: const Icon(Icons.high_quality_outlined, size: 16),
                      label: Text(
                        s.resolution.isEmpty ? '设分辨率' : '改分辨率',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _toggle(s),
                      icon: Icon(
                        s.status == 'paused' ? Icons.play_arrow : Icons.pause,
                        size: 16,
                      ),
                      label: Text(s.status == 'paused' ? '恢复' : '暂停',
                          style: const TextStyle(fontSize: 12)),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _remove(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: PTColors.danger,
                      ),
                      icon: const Icon(Icons.delete_outline, size: 16),
                      label: const Text('删除', style: TextStyle(fontSize: 12)),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _chip(String label, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(label,
          style: TextStyle(
              color: color, fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }
}
