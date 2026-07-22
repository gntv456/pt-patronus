import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter_inappwebview/flutter_inappwebview.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import 'package:ptpatronus/api/monitor.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'device_form_sheet.dart';

/// 设备详情：探活/采集/编辑/删除；host/nas 指标卡+趋势；摄像头 media_kit 实时预览（鸿蒙降级）；告警。
class MonitorDetailPage extends StatefulWidget {
  final MonitorTarget target;
  const MonitorDetailPage({super.key, required this.target});
  @override
  State<MonitorDetailPage> createState() => _MonitorDetailPageState();
}

class _MonitorDetailPageState extends State<MonitorDetailPage> {
  final _api = MonitorApi();
  Metrics? _cur;
  List<MetricSnapshot> _history = const [];
  List<MonitorAlert> _alerts = const [];
  bool _loading = false;
  bool _probing = false;

  // 摄像头预览
  Player? _player;
  VideoController? _controller;
  String? _playerErr; // 鸿蒙等无 media_kit 原生时的降级文案

  bool get _isCamera => widget.target.type == 'camera';

  String get _resolvedProtocol {
    final t = widget.target;
    if (t.protocol.isNotEmpty) return t.protocol;
    if (t.type == 'nas') return t.host.isEmpty ? 'local' : 'api';
    if (t.type == 'camera') return 'onvif';
    return t.host.isEmpty ? 'local' : 'ssh';
  }

  bool get _canPower {
    final t = widget.target;
    if (t.type == 'camera') return false;
    if (t.type == 'host') {
      return _resolvedProtocol == 'local' || _resolvedProtocol == 'ssh';
    }
    if (t.type == 'nas') {
      return _resolvedProtocol == 'local' || _resolvedProtocol == 'api';
    }
    return false;
  }

  @override
  void initState() {
    super.initState();
    _load();
    if (_isCamera && widget.target.rtspUrl.isNotEmpty) _initPlayer();
  }

  Future<void> _initPlayer() async {
    try {
      final p = Player();
      final c = VideoController(p);
      await p.open(Media(_api.streamURL(widget.target.id)));
      if (!mounted) {
        p.dispose(); // 页面已退出，回收播放器避免泄漏
        return;
      }
      setState(() {
        _player = p;
        _controller = c;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() => _playerErr = '当前平台不支持实时画面（约 5-10s 延迟），请在 Web 端查看');
    }
  }

  @override
  void dispose() {
    _player?.dispose();
    super.dispose();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  String _tempName(String raw) {
    final name = raw.trim();
    final key = name.toLowerCase();
    if (key.isEmpty) return '温度传感器';
    if (key == 'acpitz') return 'ACPI 温区';
    if (key == 'nvme_composite') return 'NVMe 综合温度';
    final nvmeSensor = RegExp(r'^nvme_sensor_(\d+)$').firstMatch(key);
    if (nvmeSensor != null) return 'NVMe 传感器 ${nvmeSensor.group(1)}';
    final nvme = RegExp(r'^nvme(?:_temp)?_?(\d+)$').firstMatch(key);
    if (nvme != null) return 'NVMe 温度 ${nvme.group(1)}';
    final pkg = RegExp(r'^coretemp_package_id_(\d+)$').firstMatch(key);
    if (pkg != null) {
      final id = pkg.group(1);
      return id == '0' ? 'CPU 封装' : 'CPU 封装 $id';
    }
    final core = RegExp(r'^coretemp_core_(\d+)$').firstMatch(key);
    if (core != null) return 'CPU 核心 ${core.group(1)}';
    if (key == 'k10temp_tctl') return 'AMD CPU 控制温度';
    if (key == 'k10temp_tccd1') return 'AMD CCD1 温度';
    if (key.startsWith('k10temp_tccd')) {
      return 'AMD CCD${key.replaceFirst('k10temp_tccd', '')} 温度';
    }
    return name.replaceAll('_', ' ');
  }

  String _protocolName(String raw) => switch (raw) {
    'local' => '本机',
    'ssh' => 'SSH 远程',
    'agent' => 'Agent 上报',
    'api' => '群晖 / TrueNAS API',
    'snmp' => '通用 SNMP',
    'onvif' => 'ONVIF',
    'rtsp' => 'RTSP 直连',
    _ => raw,
  };

  String _vendorName(String raw) => switch (raw) {
    'synology' => '群晖',
    'truenas' || 'freenas' => 'TrueNAS',
    'snmp' => 'SNMP',
    _ => raw,
  };

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final results = await Future.wait([
        _api.collect(widget.target.id),
        _api.metrics(widget.target.id, limit: 60),
        _api.alerts(targetId: widget.target.id),
      ]);
      if (!mounted) return;
      setState(() {
        _cur = (results[0] as CollectResult).metrics;
        _history = results[1] as List<MetricSnapshot>;
        _alerts = results[2] as List<MonitorAlert>;
        _loading = false;
      });
    } catch (e) {
      if (mounted) {
        _toast(e.toString().replaceFirst('Exception: ', ''));
        setState(() => _loading = false);
      }
    }
  }

  Future<void> _probe() async {
    setState(() => _probing = true);
    try {
      final r = await _api.probe(widget.target.id);
      _toast(r.ok && r.online != false ? '在线' : (r.error ?? '离线/失败'));
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _probing = false);
    }
  }

  Future<void> _edit() async {
    final ok = await DeviceFormSheet.show(context, target: widget.target);
    if (ok == true) _load();
  }

  Future<void> _delete() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除设备'),
        content: Text('确认删除「${widget.target.name}」？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(widget.target.id);
      if (mounted) Navigator.pop(context);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  Future<void> _power(String action) async {
    final isShutdown = action == 'shutdown';
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(isShutdown ? '确认关机' : '确认重启'),
        content: Text(
          '确认${isShutdown ? '关闭' : '重启'}「${widget.target.name}」？'
          '${isShutdown ? '关机后可能需要手动开机。' : '设备会短暂离线。'}',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            style: isShutdown
                ? FilledButton.styleFrom(backgroundColor: Colors.redAccent)
                : null,
            onPressed: () => Navigator.pop(context, true),
            child: Text(isShutdown ? '关机' : '重启'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      final r = await _api.power(widget.target.id, action);
      if (r.ok) {
        _toast(isShutdown ? '关机命令已发送' : '重启命令已发送');
      } else {
        _toast(r.error ?? '电源操作失败');
      }
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  Widget _card(String label, String value, {Color? color}) => GlassCard(
    padding: const EdgeInsets.all(14),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: const TextStyle(color: Colors.grey, fontSize: 12)),
        const SizedBox(height: 4),
        Text(
          value,
          style: TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w700,
            color: color,
          ),
        ),
      ],
    ),
  );

  Widget _cameraPreview() {
    if (_playerErr != null) {
      return GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: [
            const Icon(Icons.videocam_off, size: 36, color: Colors.grey),
            const SizedBox(height: 8),
            Text(
              _playerErr!,
              textAlign: TextAlign.center,
              style: const TextStyle(color: Colors.grey),
            ),
            const SizedBox(height: 12),
            OutlinedButton.icon(
              icon: const Icon(Icons.open_in_browser),
              label: const Text('在 WebView 中打开 Web 版'),
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => InAppWebViewPage(initialUrl: _webStreamUrl()),
                ),
              ),
            ),
          ],
        ),
      );
    }
    if (_controller == null) {
      return const AspectRatio(
        aspectRatio: 16 / 9,
        child: ColoredBox(
          color: Colors.black54,
          child: Center(
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: Colors.white70,
            ),
          ),
        ),
      );
    }
    return ClipRRect(
      borderRadius: BorderRadius.circular(16),
      child: AspectRatio(
        aspectRatio: 16 / 9,
        child: Video(controller: _controller!),
      ),
    );
  }

  String _webStreamUrl() {
    // 降级：用 WebView 打开后端内嵌 Web 的摄像头流（需已登录态；粗略指向 Web 端）。
    final u = Uri.parse(_api.streamURL(widget.target.id));
    return '${u.scheme}://${u.host}/';
  }

  Widget _trend(
    String label,
    List<double> data, {
    Color color = PTColors.primary,
  }) {
    if (data.length < 2) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 12),
        child: Text(
          '$label 趋势：数据不足',
          style: const TextStyle(color: Colors.grey, fontSize: 13),
        ),
      );
    }
    final maxY =
        data.fold<double>(1, (a, b) => a > b ? a : b).clamp(1, 100) * 1.2;
    return Padding(
      padding: const EdgeInsets.only(top: 8, bottom: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
          ),
          const SizedBox(height: 6),
          SizedBox(
            height: 90,
            child: LineChart(
              LineChartData(
                gridData: FlGridData(show: true, drawVerticalLine: false),
                borderData: FlBorderData(show: false),
                titlesData: const FlTitlesData(show: false),
                minY: 0,
                maxY: maxY,
                lineBarsData: [
                  LineChartBarData(
                    isCurved: true,
                    spots: [
                      for (var i = 0; i < data.length; i++)
                        FlSpot(i.toDouble(), data[i]),
                    ],
                    color: color,
                    barWidth: 2,
                    dotData: const FlDotData(show: false),
                    belowBarData: BarAreaData(
                      show: true,
                      color: color.withValues(alpha: 0.12),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _info(String k, String v) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 3),
    child: Row(
      children: [
        SizedBox(
          width: 80,
          child: Text(
            k,
            style: const TextStyle(color: Colors.grey, fontSize: 13),
          ),
        ),
        Expanded(child: Text(v, style: const TextStyle(fontSize: 13))),
      ],
    ),
  );

  @override
  Widget build(BuildContext context) {
    final t = widget.target;
    final m = _cur;
    return Scaffold(
      backgroundColor: Colors.transparent,
      appBar: AppBar(
        title: Text(t.name),
        backgroundColor: Colors.transparent,
        elevation: 0,
        actions: [
          IconButton(
            icon: _probing
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.network_check),
            onPressed: _probing ? null : _probe,
            tooltip: '探活',
          ),
          IconButton(icon: const Icon(Icons.refresh), onPressed: _load),
          PopupMenuButton<String>(
            onSelected: (v) {
              if (v == 'edit') _edit();
              if (v == 'delete') _delete();
              if (v == 'reboot') _power('reboot');
              if (v == 'shutdown') _power('shutdown');
            },
            itemBuilder: (_) => [
              if (_canPower)
                const PopupMenuItem(
                  value: 'reboot',
                  child: Row(
                    children: [
                      Icon(Icons.restart_alt, size: 18),
                      SizedBox(width: 10),
                      Text('重启'),
                    ],
                  ),
                ),
              if (_canPower)
                const PopupMenuItem(
                  value: 'shutdown',
                  child: Row(
                    children: [
                      Icon(
                        Icons.power_settings_new,
                        size: 18,
                        color: Colors.redAccent,
                      ),
                      SizedBox(width: 10),
                      Text('关机'),
                    ],
                  ),
                ),
              if (_canPower) const PopupMenuDivider(),
              const PopupMenuItem(value: 'edit', child: Text('编辑')),
              const PopupMenuItem(value: 'delete', child: Text('删除')),
            ],
          ),
        ],
      ),
      body: GlassBackground(
        child: SafeArea(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
            children: [
              Row(
                children: [
                  Icon(
                    t.lastOnline == true
                        ? Icons.check_circle
                        : Icons.error_outline,
                    size: 18,
                    color: t.lastOnline == true
                        ? PTColors.success
                        : PTColors.danger,
                  ),
                  const SizedBox(width: 6),
                  Text(
                    t.lastOnline == true
                        ? '在线'
                        : (t.lastOnline == false ? '离线' : '未知'),
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(width: 12),
                  Text(
                    '${_protocolName(_resolvedProtocol)}${t.vendor.isNotEmpty ? ' · ${_vendorName(t.vendor)}' : ''}',
                    style: const TextStyle(color: Colors.grey, fontSize: 13),
                  ),
                ],
              ),
              if (t.lastError.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Text(
                    t.lastError,
                    style: const TextStyle(
                      color: Colors.redAccent,
                      fontSize: 12,
                    ),
                  ),
                ),
              const SizedBox(height: 12),
              if (_isCamera) ...[
                _cameraPreview(),
                const SizedBox(height: 12),
                if (m?.deviceInfo != null) ...[
                  GlassCard(
                    padding: const EdgeInsets.all(14),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          '设备信息',
                          style: TextStyle(
                            fontWeight: FontWeight.w600,
                            fontSize: 14,
                          ),
                        ),
                        const SizedBox(height: 6),
                        _info('厂商', m!.deviceInfo!.manufacturer),
                        _info('型号', m.deviceInfo!.model),
                        _info('固件', m.deviceInfo!.firmware),
                        _info('序列号', m.deviceInfo!.serial),
                      ],
                    ),
                  ),
                  const SizedBox(height: 12),
                ],
              ] else if (m != null) ...[
                // 指标卡
                GridView.count(
                  crossAxisCount: 2,
                  shrinkWrap: true,
                  physics: const NeverScrollableScrollPhysics(),
                  mainAxisSpacing: 10,
                  crossAxisSpacing: 10,
                  childAspectRatio: 2.4,
                  children: [
                    _card('CPU', _pct(m.cpuUsage)),
                    _card('内存', _pct(m.memUsage)),
                    _card(
                      '最满磁盘',
                      _pct(
                        m.disks.isEmpty
                            ? null
                            : m.disks
                                  .map((d) => d.usage)
                                  .reduce((a, b) => a > b ? a : b),
                      ),
                    ),
                    _card(
                      '最高温度',
                      m.temps.isEmpty
                          ? '-'
                          : '${m.temps.map((t) => t.temp).reduce((a, b) => a > b ? a : b).toStringAsFixed(0)}°C',
                    ),
                  ],
                ),
                const SizedBox(height: 12),
                GlassCard(
                  padding: const EdgeInsets.all(14),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _trend('CPU %', [
                        for (final s in _history) s.metrics?.cpuUsage ?? 0,
                      ]),
                      _trend('内存 %', [
                        for (final s in _history) s.metrics?.memUsage ?? 0,
                      ], color: PTColors.primary2),
                      const Divider(),
                      _info('网络 ↓', _fmtRate(m.netRxRate)),
                      _info('网络 ↑', _fmtRate(m.netTxRate)),
                      if (m.load1 != null)
                        _info('负载', m.load1!.toStringAsFixed(2)),
                      if (m.uptime != null)
                        _info('运行时长', _fmtUptime(m.uptime!)),
                      if (m.disks.isNotEmpty) ...[
                        const SizedBox(height: 6),
                        const Text(
                          '磁盘',
                          style: TextStyle(
                            fontWeight: FontWeight.w600,
                            fontSize: 14,
                          ),
                        ),
                        for (final d in m.disks)
                          _info(
                            d.device,
                            '${_fmtBytes(d.used)} / ${_fmtBytes(d.total)} (${(d.usage).toStringAsFixed(0)}%)',
                          ),
                      ],
                      if (m.temps.isNotEmpty) ...[
                        const SizedBox(height: 6),
                        const Text(
                          '温度',
                          style: TextStyle(
                            fontWeight: FontWeight.w600,
                            fontSize: 14,
                          ),
                        ),
                        for (final tp in m.temps)
                          _info(
                            _tempName(tp.name),
                            '${tp.temp.toStringAsFixed(1)}°C',
                          ),
                      ],
                    ],
                  ),
                ),
              ] else if (_loading)
                const Padding(
                  padding: EdgeInsets.all(24),
                  child: Center(child: CircularProgressIndicator()),
                )
              else
                GlassCard(
                  padding: const EdgeInsets.all(16),
                  child: Center(
                    child: Text(
                      t.lastError.isNotEmpty ? t.lastError : '点击右上角刷新采集数据',
                      style: const TextStyle(color: Colors.grey),
                    ),
                  ),
                ),
              if (_alerts.isNotEmpty) ...[
                const SizedBox(height: 12),
                GlassCard(
                  padding: const EdgeInsets.all(14),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text(
                        '告警',
                        style: TextStyle(
                          fontWeight: FontWeight.w600,
                          fontSize: 14,
                        ),
                      ),
                      const SizedBox(height: 6),
                      for (final a in _alerts.take(10))
                        ListTile(
                          dense: true,
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(
                            Icons.circle,
                            size: 10,
                            color: a.level == 'danger'
                                ? PTColors.danger
                                : a.level == 'warning'
                                ? PTColors.warn
                                : PTColors.primary2,
                          ),
                          title: Text(
                            a.message,
                            style: const TextStyle(fontSize: 13),
                          ),
                          subtitle: Text(
                            '${a.metric} · ${a.value}${a.resolvedAt != null ? ' · 已恢复' : ''}',
                            style: const TextStyle(fontSize: 11),
                          ),
                          trailing: a.read
                              ? null
                              : TextButton(
                                  child: const Text('已读'),
                                  onPressed: () async {
                                    try {
                                      await _api.readAlert(a.id);
                                      _load();
                                    } catch (e) {
                                      _toast(
                                        e.toString().replaceFirst(
                                          'Exception: ',
                                          '',
                                        ),
                                      );
                                    }
                                  },
                                ),
                        ),
                    ],
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

String _pct(num? v) =>
    v == null ? '-' : '${v.toStringAsFixed(v < 10 ? 1 : 0)}%';

String _fmtBytes(int n) {
  if (n <= 0) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  double s = n.toDouble();
  var i = 0;
  while (s >= 1024 && i < u.length - 1) {
    s /= 1024;
    i++;
  }
  return i == 0 ? '${s.toInt()} B' : '${s.toStringAsFixed(1)} ${u[i]}';
}

String _fmtRate(num? bps) => bps == null ? '-' : '${_fmtBytes(bps.toInt())}/s';

String _fmtUptime(int sec) {
  final d = sec ~/ 86400, h = (sec % 86400) ~/ 3600, mi = (sec % 3600) ~/ 60;
  if (d > 0) return '$d天$h时';
  if (h > 0) return '$h时$mi分';
  return '$mi分';
}

/// WebView 降级容器（鸿蒙无 media_kit 原生时，跳后端 Web 版看流）。
class InAppWebViewPage extends StatelessWidget {
  final String initialUrl;
  const InAppWebViewPage({super.key, required this.initialUrl});
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('实时画面（Web 版）')),
      body: InAppWebView(
        initialUrlRequest: URLRequest(url: WebUri(initialUrl)),
      ),
    );
  }
}
