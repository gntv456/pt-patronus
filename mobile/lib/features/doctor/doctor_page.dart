import 'package:flutter/material.dart';
import 'package:ptpatronus/api/system.dart';

/// 诊断面板：跑健康检查（对标 MP doctor）。SAFE_MODE 自救时主入口。
class DoctorPage extends StatefulWidget {
  const DoctorPage({super.key});
  @override
  State<DoctorPage> createState() => _DoctorPageState();
}

class _DoctorPageState extends State<DoctorPage> {
  DoctorReport? _report;
  bool _loading = false;
  bool _fixing = false;

  @override
  void initState() {
    super.initState();
    _run(false);
  }

  Future<void> _run(bool fix) async {
    setState(() {
      _fixing = fix;
      _loading = !fix;
    });
    try {
      _report = fix ? await SystemApi.fixDoctor() : await SystemApi.runDoctor();
    } catch (_) {
      // 静默：列表区显示空态
    } finally {
      if (mounted) {
        setState(() {
          _loading = false;
          _fixing = false;
        });
      }
    }
  }

  Color _sevColor(String s) => switch (s) {
        'error' => const Color(0xFFFF4D4F),
        'warn' => const Color(0xFFFAAD14),
        _ => const Color(0xFF52C41A),
      };

  @override
  Widget build(BuildContext context) {
    final r = _report;
    return Scaffold(
      appBar: AppBar(
        title: const Text('诊断'),
        actions: [
          IconButton(
            onPressed: (_loading || _fixing) ? null : () => _run(false),
            icon: const Icon(Icons.refresh),
            tooltip: '重新诊断',
          ),
          IconButton(
            onPressed: (_loading || _fixing) ? null : () => _run(true),
            icon: const Icon(Icons.build_outlined),
            tooltip: '尝试修复',
          ),
        ],
      ),
      body: _loading && r == null
          ? const Center(child: CircularProgressIndicator())
          : r == null
              ? const Center(child: Text('无数据'))
              : ListView(children: [
                  if (r.environment['safe_mode'] == true)
                    const ListTile(
                      leading: Icon(Icons.warning_amber_rounded, color: Colors.orange),
                      title: Text('SAFE_MODE 已启用'),
                      subtitle: Text('插件 / 调度 / 监控未激活，仅核心 HTTP + 诊断可达'),
                    ),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                    child: Text(
                      '状态：${r.status} · 共 ${r.total} 项（info ${r.info} / warn ${r.warn} / error ${r.error}）',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ),
                  for (final f in r.findings)
                    Card(
                      child: ListTile(
                        leading: CircleAvatar(backgroundColor: _sevColor(f.severity), radius: 6),
                        title: Text(f.title),
                        subtitle: Text([
                          if (f.detail.isNotEmpty) f.detail,
                          if (f.recommendation.isNotEmpty) '建议：${f.recommendation}',
                        ].join('\n')),
                        trailing: Text(f.status, style: const TextStyle(fontSize: 12)),
                      ),
                    ),
                ]),
    );
  }
}
