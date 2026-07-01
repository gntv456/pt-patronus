import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/system_log.dart';
import 'package:ptpatronus/theme/glass.dart';

/// 运行日志页（对齐 Web Settings 日志 tab）：展示后端环形缓冲日志，
/// 支持级别过滤、刷新与清空。后端 API：GET/DELETE /system/logs。
class LogsPage extends StatefulWidget {
  const LogsPage({super.key});
  @override
  State<LogsPage> createState() => _LogsPageState();
}

class _LogsPageState extends State<LogsPage> {
  final _api = SystemLogApi();
  List<LogEntry> _entries = [];
  bool _loading = true;
  String? _error;
  String _level = ''; // ''=全部

  // 级别过滤项（值, 标签）。
  static const _levels = [
    ('', '全部'),
    ('error', 'ERROR'),
    ('warn', 'WARN'),
    ('info', 'INFO'),
    ('debug', 'DEBUG'),
  ];

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
      _entries = await _api.list(level: _level);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _clear() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('清空日志'),
        content: const Text('确认清空后端环形缓冲中的全部日志？此操作不可撤销。'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('清空')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.clear();
      if (mounted) setState(() => _entries = []);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('清空失败：$e')));
      }
    }
  }

  Color _levelColor(String lv) => switch (lv) {
        'ERROR' => const Color(0xFFF56C6C),
        'WARN' => const Color(0xFFE6A23C),
        'DEBUG' => Colors.grey,
        _ => const Color(0xFF6C5CE7),
      };

  String _fmtTime(DateTime t) {
    String two(int n) => n.toString().padLeft(2, '0');
    return '${two(t.hour)}:${two(t.minute)}:${two(t.second)}';
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('运行日志'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : _load),
            IconButton(icon: const Icon(Icons.delete_sweep), onPressed: _loading ? null : _clear),
          ],
        ),
        body: Column(
          children: [
            // 级别过滤条
            SizedBox(
              height: 46,
              child: ListView(
                scrollDirection: Axis.horizontal,
                padding: const EdgeInsets.symmetric(horizontal: 12),
                children: _levels.map((e) {
                  final on = _level == e.$1;
                  return Padding(
                    padding: const EdgeInsets.only(right: 8, top: 8),
                    child: ChoiceChip(
                      label: Text(e.$2),
                      selected: on,
                      onSelected: (_) {
                        setState(() => _level = e.$1);
                        _load();
                      },
                    ),
                  );
                }).toList(),
              ),
            ),
            Expanded(
              child: RefreshIndicator(
                onRefresh: _load,
                child: _loading
                    ? const Center(child: CircularProgressIndicator())
                    : _error != null
                        ? _Empty(text: _error!, onRetry: _load)
                        : _entries.isEmpty
                            ? const _Empty(text: '暂无日志')
                            : ListView.builder(
                                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
                                itemCount: _entries.length,
                                itemBuilder: (_, i) => _row(_entries[i]),
                              ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _row(LogEntry e) {
    final hasAttrs = e.attrs.isNotEmpty;
    final lc = _levelColor(e.level);
    return GlassCard(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(_fmtTime(e.time), style: const TextStyle(color: Colors.grey, fontSize: 12)),
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
                decoration: BoxDecoration(color: lc.withValues(alpha: 0.16), borderRadius: BorderRadius.circular(4)),
                child: Text(e.level, style: TextStyle(color: lc, fontSize: 11, fontWeight: FontWeight.w700)),
              ),
              const SizedBox(width: 8),
              Expanded(child: Text(e.message, style: const TextStyle(fontSize: 13, height: 1.4))),
            ],
          ),
          if (hasAttrs)
            Padding(
              padding: const EdgeInsets.only(top: 4, left: 60),
              child: Wrap(
                spacing: 8,
                runSpacing: 2,
                children: e.attrs
                    .map((a) => Text('${a.key}=${a.value}', style: const TextStyle(color: Colors.grey, fontSize: 11)))
                    .toList(),
              ),
            ),
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
