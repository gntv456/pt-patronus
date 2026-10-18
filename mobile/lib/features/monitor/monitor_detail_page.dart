import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter_inappwebview/flutter_inappwebview.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import 'package:ptpatronus/api/monitor.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'device_form_sheet.dart';
import 'trend_window.dart';

part 'inapp_webview_page.dart';

part 'monitordetailopsmixin.dart';
part 'monitor_detail_widgets.dart';

/// 设备详情：探活/采集/编辑/删除；host/nas 指标卡+趋势；摄像头 media_kit 实时预览（鸿蒙降级）；告警。
class MonitorDetailPage extends StatefulWidget {
  final MonitorTarget target;
  const MonitorDetailPage({super.key, required this.target});
  @override
  State<MonitorDetailPage> createState() => _MonitorDetailPageState();
}

class _MonitorDetailPageState extends State<MonitorDetailPage>
    with MonitorDetailOpsMixin {
  @override
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  void initState() {
    super.initState();
    _load();
    if (_isCamera && widget.target.rtspUrl.isNotEmpty) _initPlayer();
  }

  @override
  void dispose() {
    _player?.dispose();
    super.dispose();
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

  Widget _info(String k, String v) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 3),
    child: Row(
      children: [
        SizedBox(
          width: 80,
          child: Text(
            k,
            style: TextStyle(color: context.dimText, fontSize: 13),
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
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: '刷新',
            onPressed: _load,
          ),
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
                    style: TextStyle(color: context.dimText, fontSize: 13),
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
                        for (final s in _history) s.metrics?.cpuUsage,
                      ]),
                      _trend('内存 %', [
                        for (final s in _history) s.metrics?.memUsage,
                      ], color: PTColors.primary2),
                      _trendWindowNote(),
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
                const PTLoadingView()
              else
                GlassCard(
                  padding: const EdgeInsets.all(16),
                  child: Center(
                    child: Text(
                      t.lastError.isNotEmpty ? t.lastError : '点击右上角刷新采集数据',
                      style: TextStyle(color: context.dimText),
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
