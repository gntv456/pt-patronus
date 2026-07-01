import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/monitor.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'device_form_sheet.dart';
import 'monitor_detail_page.dart';

/// 设备监控页：按 服务器/NAS/摄像头 分段列表 + 未读告警条 + 增删改 + 探活/采集入口。
class MonitorPage extends StatefulWidget {
  const MonitorPage({super.key});
  @override
  State<MonitorPage> createState() => _MonitorPageState();
}

class _MonitorPageState extends State<MonitorPage> {
  final _api = MonitorApi();
  List<MonitorTarget> _targets = [];
  List<MonitorAlert> _unread = [];
  bool _loading = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([_api.list(), _api.alerts(unread: true)]);
      _targets = results[0] as List<MonitorTarget>;
      _unread = results[1] as List<MonitorAlert>;
    } catch (e) {
      if (_targets.isEmpty) {
        _error = e.toString().replaceFirst('Exception: ', '');
      } else {
        _toast(e.toString().replaceFirst('Exception: ', ''));
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _add() async {
    final ok = await DeviceFormSheet.show(context);
    if (ok == true) _load();
  }

  Future<void> _open(MonitorTarget t) async {
    await Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => MonitorDetailPage(target: t)),
    );
    _load(); // 返回后刷新（可能已读告警/采集/删除）
  }

  Widget _errorView() => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, size: 40, color: Colors.redAccent),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Text(_error ?? '加载失败',
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: Colors.redAccent, fontSize: 14)),
            ),
            const SizedBox(height: 12),
            FilledButton.tonalIcon(
                icon: const Icon(Icons.refresh, size: 18), label: const Text('重试'), onPressed: _load),
          ],
        ),
      );

  Widget _typeBadge(String type) {
    final color = switch (type) {
      'host' => const Color(0xFF4285F4),
      'nas' => PTColors.primary,
      'camera' => const Color(0xFFFF5F6D),
      _ => Colors.grey,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(6),
        color: color.withValues(alpha: 0.18),
      ),
      child: Text(type.toUpperCase(),
          style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }

  Widget _row(MonitorTarget t) => Padding(
        padding: const EdgeInsets.only(bottom: 10),
        child: GlassCard(
          padding: const EdgeInsets.fromLTRB(14, 12, 12, 12),
          onTap: () => _open(t),
          child: Row(
            children: [
              _typeBadge(t.type),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(t.name,
                        style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 16),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis),
                    const SizedBox(height: 2),
                    Text(
                      t.host.isEmpty ? '(本机)' : t.host,
                      style: const TextStyle(color: Colors.grey, fontSize: 12),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (t.lastError.isNotEmpty)
                      Text(t.lastError,
                          style: const TextStyle(color: Colors.redAccent, fontSize: 11),
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis),
                  ],
                ),
              ),
              Icon(
                t.lastOnline == true
                    ? Icons.circle
                    : t.lastOnline == false
                        ? Icons.circle_outlined
                        : Icons.help_outline,
                size: 14,
                color: t.lastOnline == true
                    ? PTColors.success
                    : t.lastOnline == false
                        ? PTColors.danger
                        : Colors.grey,
              ),
            ],
          ),
        ),
      );

  Widget _section(String title, IconData icon, String type) {
    final items = _targets.where((t) => t.type == type).toList();
    if (items.isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(4, 12, 4, 6),
          child: Row(children: [
            Icon(icon, size: 18, color: Colors.grey),
            const SizedBox(width: 6),
            Text(title, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
            const SizedBox(width: 6),
            Text('${items.length}', style: const TextStyle(color: Colors.grey, fontSize: 13)),
          ]),
        ),
        ...items.map(_row),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 12, 4),
              child: Row(children: [
                const HomeMenuButton(),
                const Text('设备监控', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700)),
                const Spacer(),
                IconButton(icon: const Icon(Icons.refresh), onPressed: _load),
                IconButton(icon: const Icon(Icons.add), onPressed: _add),
              ]),
            ),
            if (_unread.isNotEmpty)
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 0, 12, 4),
                child: GlassCard(
                  padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                  onTap: () => _showAlerts(),
                  child: Row(children: [
                    const Icon(Icons.warning_amber_rounded, color: PTColors.danger, size: 20),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text('${_unread.length} 条未读告警',
                          style: const TextStyle(color: PTColors.danger, fontWeight: FontWeight.w600)),
                    ),
                    const Text('查看', style: TextStyle(color: Colors.grey, fontSize: 13)),
                  ]),
                ),
              ),
            if (_loading) const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: _targets.isEmpty && !_loading
                  ? (_error != null
                      ? _errorView()
                      : const Center(child: Text('暂无设备，点右上角 + 添加', style: TextStyle(color: Colors.grey))))
                  : ListView(
                      padding: const EdgeInsets.fromLTRB(12, 0, 12, 24),
                      children: [
                        _section('服务器', Icons.dns, 'host'),
                        _section('NAS', Icons.storage, 'nas'),
                        _section('摄像头', Icons.videocam_outlined, 'camera'),
                      ],
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _showAlerts() async {
    await showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('未读告警'),
        content: SizedBox(
          width: double.maxFinite,
          child: ListView.builder(
            shrinkWrap: true,
            itemCount: _unread.length,
            itemBuilder: (_, i) {
              final a = _unread[i];
              return ListTile(
                dense: true,
                leading: Icon(Icons.circle,
                    size: 10,
                    color: a.level == 'danger'
                        ? PTColors.danger
                        : a.level == 'warning'
                            ? PTColors.warn
                            : PTColors.primary2),
                title: Text(a.message, style: const TextStyle(fontSize: 14)),
                subtitle: Text('${a.metric} · ${a.value}', style: const TextStyle(fontSize: 12)),
              );
            },
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('关闭')),
          TextButton(
            onPressed: () async {
              for (final a in _unread) {
                try {
                  await _api.readAlert(a.id);
                } catch (_) {}
              }
              if (mounted) Navigator.pop(context);
              _load();
            },
            child: const Text('全部已读'),
          ),
        ],
      ),
    );
  }
}
