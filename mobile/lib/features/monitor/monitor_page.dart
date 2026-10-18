import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/api/monitor.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'device_form_sheet.dart';
import 'monitor_detail_page.dart';

part 'agent_manager_sheet.dart';

part 'monitoropsmixin.dart';
part 'monitor_widgets.dart';

/// 设备监控页：服务器/NAS/摄像头 三标签 + 未读告警条 + 卡片内联指标（CPU/内存/磁盘）
/// + Agent 管理器（admin）+ 15s 轮询。对齐 Web NasServer.vue。
class MonitorPage extends StatefulWidget {
  const MonitorPage({super.key});
  @override
  State<MonitorPage> createState() => _MonitorPageState();
}

class _MonitorPageState extends State<MonitorPage>
    with
        SingleTickerProviderStateMixin,
        WidgetsBindingObserver,
        MonitorOpsMixin {
  late final TabController _tab = TabController(length: 3, vsync: this);

  Future<void> _load() async {
    if (_loading) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([
        _api.list(),
        _api.alerts(unread: true),
      ]);
      _targets = results[0] as List<MonitorTarget>;
      _unread = results[1] as List<MonitorAlert>;
      // 并发取每个非摄像头设备的最新快照（best-effort，任一失败不阻断整体加载）。
      final mFutures = _targets.where((t) => t.type != 'camera').map((t) async {
        try {
          final page = await _api.metrics(t.id, limit: 1);
          _latest[t.id] = page.snapshots.isEmpty
              ? null
              : page.snapshots.last.metrics;
        } catch (_) {
          _latest[t.id] = null;
        }
      }).toList();
      await Future.wait(mFutures);
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

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(
      this,
    ); // 前后台：后台停轮询省电 + 避免 token 过期触发 401 弹回登录
    _load();
    _startTimer();
  }

  void _startTimer() {
    _timer?.cancel();
    // 30s 轮询（原 15s 偏频）；后台暂停、回前台立即刷新 + 重启。
    _timer = Timer.periodic(const Duration(seconds: 30), (_) => _load());
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      _load();
      _startTimer();
    } else if (state == AppLifecycleState.paused ||
        state == AppLifecycleState.hidden) {
      _timer?.cancel();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _timer?.cancel();
    _tab.dispose();
    super.dispose();
  }

  Future<void> _add() async {
    final ok = await DeviceFormSheet.show(context);
    if (ok == true) _load();
  }

  Future<void> _open(MonitorTarget t) async {
    await Navigator.of(
      context,
    ).push(MaterialPageRoute(builder: (_) => MonitorDetailPage(target: t)));
    _load(); // 返回后刷新（可能已读告警/采集/删除）
  }

  Future<void> _openAgents() async {
    await showAdaptiveSheet(
      context,
      title: 'Agent 管理',
      wideWidth: 640,
      builder: (_) => const _AgentManagerSheet(),
    );
    // agent 关联 host/nas|agent 的 MonitorTarget，关闭后刷新列表。
    if (mounted) _load();
  }

  int _count(String type) => _targets.where((t) => t.type == type).length;

  @override
  Widget build(BuildContext context) {
    final fullError = _targets.isEmpty && _error != null && !_loading;
    return PTPageScaffold(
      title: '设备监控',
      actions: [
        PTBarAction(icon: Icons.refresh, label: '刷新', onPressed: _load),
        PTBarAction(icon: Icons.add, label: '添加设备', onPressed: _add),
        if (ApiClient().isAdmin)
          PTBarAction(
            icon: Icons.cloud_sync_outlined,
            label: 'Agent 管理',
            onPressed: _openAgents,
            overflow: true,
          ),
      ],
      filterDimensions: [
        // 设备类型（原 TabBar 三标签折叠而来；structural 不计入筛选激活徽标）。
        // 保留 _tab 作数据源 + TabBarView 保活各类型列表状态；点筛选切 _tab.index。
        PTFilterDimension.choice(
          label: '设备类型',
          selected: _tab.index,
          isDefault: false,
          isStructural: true,
          onChanged: (v) {
            _tab.index = (v as int?) ?? 0;
            setState(() {});
          },
          options: [
            PTFilterOption<Object?>(
              value: 0,
              label: '服务器',
              count: _count('host'),
            ),
            PTFilterOption<Object?>(
              value: 1,
              label: 'NAS',
              count: _count('nas'),
            ),
            PTFilterOption<Object?>(
              value: 2,
              label: '摄像头',
              count: _count('camera'),
            ),
          ],
        ),
      ],
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_unread.isNotEmpty)
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 0, 12, 4),
              child: GlassCard(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 7,
                ),
                onTap: () => _showAlerts(),
                child: Row(
                  children: [
                    const Icon(
                      Icons.warning_amber_rounded,
                      color: PTColors.danger,
                      size: 20,
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        '${_unread.length} 条未读告警',
                        style: const TextStyle(
                          color: PTColors.danger,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                    Text(
                      '查看',
                      style: TextStyle(color: context.dimText, fontSize: 13),
                    ),
                  ],
                ),
              ),
            ),
          if (_loading) const LinearProgressIndicator(minHeight: 2),
          Expanded(
            child: fullError
                ? _errorView()
                : TabBarView(
                    controller: _tab,
                    children: [
                      _listFor('host', '暂无服务器，点右上角 + 添加'),
                      _listFor('nas', '暂无 NAS 设备'),
                      _listFor('camera', '暂无摄像头'),
                    ],
                  ),
          ),
        ],
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
                leading: Icon(
                  Icons.circle,
                  size: 10,
                  color: a.level == 'danger'
                      ? PTColors.danger
                      : a.level == 'warning'
                      ? PTColors.warn
                      : PTColors.primary2,
                ),
                title: Text(a.message, style: const TextStyle(fontSize: 14)),
                subtitle: Text(
                  '${a.metric} · ${a.value}',
                  style: const TextStyle(fontSize: 12),
                ),
              );
            },
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('关闭'),
          ),
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
