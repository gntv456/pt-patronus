import 'package:flutter/material.dart';
import 'package:ptpatronus/api/system.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

/// 诊断面板：跑健康检查（对标 MP doctor）。SAFE_MODE 自救时主入口。
class DoctorPage extends StatefulWidget {
  const DoctorPage({super.key, this.run});

  /// 取数注入缝（对齐 #195 流式 API 的做法）：默认走后端，
  /// 测试可塞一个假的，用来证「跑失败必须在界面上说清楚」这条分支真的会红。
  final Future<DoctorReport> Function(bool fix)? run;

  @override
  State<DoctorPage> createState() => _DoctorPageState();
}

class _DoctorPageState extends State<DoctorPage> {
  DoctorReport? _report;
  bool _loading = false;
  bool _fixing = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    // GET/POST /system/doctor 是 admin-only：非管理员不发必然 403 的请求。
    if (ApiClient().isAdmin) _run(false);
  }

  Future<void> _run(bool fix) async {
    setState(() {
      _fixing = fix;
      _loading = !fix;
      _error = null;
    });
    try {
      final injected = widget.run;
      _report = injected != null
          ? await injected(fix)
          : (fix ? await SystemApi.fixDoctor() : await SystemApi.runDoctor());
    } catch (e) {
      // 静默吞掉会让「跑挂了」和「没有发现」长得一样，用户只能猜。
      _report = null;
      _error = '$e';
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
    // 整页拦截（后端仍是权威鉴权，这里只挡 UI 入口；对齐系统设置页的既有惯例）。
    // 拦在 build 顶部顺带拿掉了「重新诊断 / 尝试修复」两个必然 403 的动作按钮。
    if (!ApiClient().isAdmin) {
      return Scaffold(
        appBar: AppBar(title: const Text('诊断')),
        body: const Center(child: Text('仅管理员可访问')),
      );
    }
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
          ? const PTLoadingView()
          : r == null
              ? Center(child: Text(_error == null ? '无数据' : '诊断失败：$_error'))
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
