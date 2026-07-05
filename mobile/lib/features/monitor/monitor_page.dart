import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/api/monitor.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'device_form_sheet.dart';
import 'monitor_detail_page.dart';

/// 设备监控页：服务器/NAS/摄像头 三标签 + 未读告警条 + 卡片内联指标（CPU/内存/磁盘）
/// + Agent 管理器（admin）+ 15s 轮询。对齐 Web NasServer.vue。
class MonitorPage extends StatefulWidget {
  const MonitorPage({super.key});
  @override
  State<MonitorPage> createState() => _MonitorPageState();
}

class _MonitorPageState extends State<MonitorPage>
    with SingleTickerProviderStateMixin, WidgetsBindingObserver {
  final _api = MonitorApi();
  List<MonitorTarget> _targets = [];
  // 每设备最新指标快照（卡片内联 CPU/内存/磁盘 用）。无批量端点，按 target 取 metrics?limit=1（同 Web loadAll）。
  final Map<int, Metrics?> _latest = {};
  List<MonitorAlert> _unread = [];
  bool _loading = false;
  String? _error;

  late final TabController _tab = TabController(length: 3, vsync: this);
  Timer? _timer;

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

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  // 单飞：15s 轮询与手动刷新互不叠加（慢请求期间跳过新轮询，避免堆积）。
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
          final snaps = await _api.metrics(t.id, limit: 1);
          _latest[t.id] = snaps.isEmpty ? null : snaps.last.metrics;
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
    // agent 也是 host|agent 的 MonitorTarget，关闭后刷新列表。
    if (mounted) _load();
  }

  Widget _errorView() => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Icon(Icons.error_outline, size: 40, color: Colors.redAccent),
        const SizedBox(height: 8),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 32),
          child: Text(
            _error ?? '加载失败',
            textAlign: TextAlign.center,
            style: const TextStyle(color: Colors.redAccent, fontSize: 14),
          ),
        ),
        const SizedBox(height: 12),
        FilledButton.tonalIcon(
          icon: const Icon(Icons.refresh, size: 18),
          label: const Text('重试'),
          onPressed: _load,
        ),
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
      child: Text(
        type.toUpperCase(),
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  int _count(String type) => _targets.where((t) => t.type == type).length;

  Widget _row(MonitorTarget t) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 12, 12, 12),
      onTap: () => _open(t),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              _typeBadge(t.type),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      t.name,
                      style: const TextStyle(
                        fontWeight: FontWeight.w600,
                        fontSize: 16,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      t.host.isEmpty ? '(本机)' : t.host,
                      style: const TextStyle(color: Colors.grey, fontSize: 12),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (t.lastError.isNotEmpty)
                      Text(
                        t.lastError,
                        style: const TextStyle(
                          color: Colors.redAccent,
                          fontSize: 11,
                        ),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
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
          // 内联指标条（CPU/内存/磁盘）——对齐 Web DeviceCard 的 MetricBar。摄像头不显示。
          if (t.type != 'camera')
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: _metricBars(_latest[t.id]),
            ),
        ],
      ),
    ),
  );

  // 内联指标：离线显「离线」；在线显 CPU/内存/磁盘 三条迷你进度条。
  Widget _metricBars(Metrics? m) {
    if (m == null) return const SizedBox.shrink();
    if (!m.online) {
      return const Text(
        '离线',
        style: TextStyle(color: Colors.grey, fontSize: 11),
      );
    }
    final cpu = m.cpuUsage ?? 0;
    final mem = m.memUsage ?? 0;
    final disk = m.disks.fold<double>(0, (a, d) => d.usage > a ? d.usage : a);
    return Wrap(
      spacing: 14,
      runSpacing: 4,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [_gauge('CPU', cpu), _gauge('内存', mem), _gauge('磁盘', disk)],
    );
  }

  Widget _gauge(String label, double pct) {
    final p = pct.clamp(0, 100).toDouble();
    final color = p > 90
        ? PTColors.danger
        : (p > 70 ? PTColors.warn : PTColors.success);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          label,
          style: TextStyle(color: Colors.grey.shade600, fontSize: 11),
        ),
        const SizedBox(width: 4),
        SizedBox(
          width: 44,
          height: 4,
          child: ClipRRect(
            borderRadius: BorderRadius.circular(2),
            child: LinearProgressIndicator(
              value: p / 100,
              backgroundColor: Colors.grey.withValues(alpha: 0.2),
              valueColor: AlwaysStoppedAnimation<Color>(color),
            ),
          ),
        ),
        const SizedBox(width: 4),
        Text(
          '${p.toStringAsFixed(0)}%',
          style: TextStyle(
            fontSize: 11,
            color: color,
            fontWeight: FontWeight.w600,
          ),
        ),
      ],
    );
  }

  Widget _listFor(String type, String emptyText) {
    if (_loading && _targets.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    final items = _targets.where((t) => t.type == type).toList();
    if (items.isEmpty) {
      return Center(
        child: Text(emptyText, style: const TextStyle(color: Colors.grey)),
      );
    }
    return ListView(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      children: items.map(_row).toList(),
    );
  }

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
                    const Text(
                      '查看',
                      style: TextStyle(color: Colors.grey, fontSize: 13),
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

// ── Agent 管理器（admin）：列表 + 创建/重置 secret（明文仅一次）/删除。对齐 Web AgentManager.vue ──
class _AgentManagerSheet extends StatefulWidget {
  const _AgentManagerSheet();
  @override
  State<_AgentManagerSheet> createState() => _AgentManagerSheetState();
}

class _AgentManagerSheetState extends State<_AgentManagerSheet> {
  final _api = AgentApi();
  List<Agent> _agents = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _load() async {
    try {
      _agents = await _api.list();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _create() async {
    final name = await _promptName();
    if (name == null || name.isEmpty) return;
    try {
      final res = await _api.create(name);
      _toast('已创建');
      await _showSecret(res.plainSecret);
      _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _rotate(Agent a) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('重置 Secret'),
        content: const Text('重置后旧 secret 立即失效，agent 需用新 secret 重连。确认？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('重置'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      final secret = await _api.rotate(a.id);
      await _showSecret(secret);
      _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _delete(Agent a) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除 Agent'),
        content: Text('删除 Agent「${a.name}」？将级联删除关联的监控目标（含快照/告警）。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            style: FilledButton.styleFrom(backgroundColor: Colors.redAccent),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(a.id);
      _toast('已删除');
      _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<String?> _promptName() async {
    final ctrl = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('新建 Agent'),
        content: TextField(
          controller: ctrl,
          autofocus: true,
          decoration: const InputDecoration(hintText: 'Agent 名称（如 飞牛NAS）'),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, ctrl.text.trim()),
            child: const Text('创建'),
          ),
        ],
      ),
    );
  }

  Future<void> _showSecret(String secret) async {
    if (secret.isEmpty) return;
    await showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Agent Secret（仅显示一次）'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              '请立即复制保存，用于 agent 上报鉴权。关闭后无法再查看（可重置）。',
              style: TextStyle(color: Colors.grey, fontSize: 12),
            ),
            const SizedBox(height: 10),
            Container(
              width: double.maxFinite,
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color: Colors.grey.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(8),
              ),
              child: SelectableText(
                secret,
                style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () async {
              await Clipboard.setData(ClipboardData(text: secret));
              if (!mounted) return;
              ScaffoldMessenger.of(
                context,
              ).showSnackBar(const SnackBar(content: Text('Secret 已复制')));
            },
            child: const Text('复制'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('关闭'),
          ),
        ],
      ),
    );
  }

  String _fmt(String iso) {
    final d = DateTime.tryParse(iso);
    if (d == null) return iso;
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.month}-${d.day} ${p(d.hour)}:${p(d.minute)}';
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Text(
              '${_agents.length} 个 Agent',
              style: const TextStyle(fontSize: 12, color: Colors.grey),
            ),
            const Spacer(),
            OutlinedButton.icon(
              onPressed: _create,
              icon: const Icon(Icons.add, size: 16),
              label: const Text('新建'),
            ),
          ],
        ),
        const SizedBox(height: 10),
        if (_loading)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(child: CircularProgressIndicator()),
          )
        else if (_agents.isEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text(
                '暂无 Agent，点「新建」创建',
                style: TextStyle(color: Colors.grey),
              ),
            ),
          )
        else
          ..._agents.map(_card),
      ],
    );
  }

  Widget _card(Agent a) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: GlassCard(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.circle,
                  size: 10,
                  color: a.online ? PTColors.success : Colors.grey,
                ),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    a.name,
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 15,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                if (a.version.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(right: 6),
                    child: Text(
                      'v${a.version}',
                      style: const TextStyle(color: Colors.grey, fontSize: 11),
                    ),
                  ),
                PopupMenuButton<String>(
                  icon: const Icon(Icons.more_vert, size: 18),
                  onSelected: (v) {
                    if (v == 'rotate') {
                      _rotate(a);
                    } else if (v == 'delete') {
                      _delete(a);
                    }
                  },
                  itemBuilder: (_) => const [
                    PopupMenuItem(value: 'rotate', child: Text('重置 Secret')),
                    PopupMenuItem(value: 'delete', child: Text('删除')),
                  ],
                ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              '${a.hostname.isEmpty ? '—' : a.hostname}${a.ip.isNotEmpty ? ' · ${a.ip}' : ''}',
              style: const TextStyle(color: Colors.grey, fontSize: 12),
            ),
            const SizedBox(height: 2),
            Text(
              a.online
                  ? '在线 · 上报 ${_fmt(a.lastSeenAt)}'
                  : '离线 · 上次 ${a.lastSeenAt.isEmpty ? '—' : _fmt(a.lastSeenAt)}',
              style: TextStyle(
                color: a.online ? PTColors.success : Colors.grey,
                fontSize: 11,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
