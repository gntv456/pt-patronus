// 从 media_server_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/media_server/media_server_page.dart';

/// 观影报告 sheet 主体：加载 + 展示用户/热门/最近播放。
class _PlaybackReportBody extends StatefulWidget {
  final int serverId;
  final MediaServerApi api;
  const _PlaybackReportBody({required this.serverId, required this.api});
  @override
  State<_PlaybackReportBody> createState() => _PlaybackReportBodyState();
}

class _PlaybackReportBodyState extends State<_PlaybackReportBody> {
  static const _days = 30;
  PlaybackReport? _report;
  bool _loading = true;
  String? _error;

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
      _report = await widget.api.playbackReport(widget.serverId, days: _days);
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  String _fmtDur(int sec) {
    if (sec <= 0) return '-';
    if (sec < 3600) return '${(sec / 60).round()} 分钟';
    return '${(sec / 3600).toStringAsFixed(1)} 小时';
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const PTLoadingView();
    if (_error != null) {
      return PTEmptyState(text: '加载失败：$_error', onRetry: _load);
    }
    final r = _report;
    if (r == null) return const PTEmptyState(text: '无数据');
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(padding: const EdgeInsets.all(12), children: [
        Text('近 $_days 天 · 共 ${r.totalPlays} 次播放',
            style: TextStyle(fontSize: 13, color: context.dimText)),
        const SizedBox(height: 8),
        const Text('用户', style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700)),
        const SizedBox(height: 4),
        if (r.users.isEmpty)
          Text('无用户数据', style: TextStyle(fontSize: 12, color: context.dimText))
        else
          Wrap(
            spacing: 6,
            children: r.users
                .map((u) => Chip(
                      label: Text('${u.name} · ${u.playCount} 次', style: const TextStyle(fontSize: 12)),
                    ))
                .toList(),
          ),
        const SizedBox(height: 12),
        const Text('热门条目', style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700)),
        ...r.topItems.map((p) => ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              title: Text(p.name, style: const TextStyle(fontSize: 13)),
              trailing: Text('${p.playCount} 次 · ${_fmtDur(p.duration)}',
                  style: TextStyle(fontSize: 12, color: context.dimText)),
            )),
        const SizedBox(height: 8),
        const Text('最近播放', style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700)),
        ...r.recentPlays.map((p) => ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              title: Text(p.name, style: const TextStyle(fontSize: 13)),
              subtitle: Text(
                p.lastPlayed.isEmpty
                    ? p.userName
                    : '${p.userName} · ${DateTime.tryParse(p.lastPlayed)?.toLocal() ?? p.lastPlayed}',
                style: TextStyle(fontSize: 11, color: context.dimText),
              ),
              trailing: Text('${p.playCount} 次', style: TextStyle(fontSize: 12, color: context.dimText)),
            )),
      ]),
    );
  }
}
