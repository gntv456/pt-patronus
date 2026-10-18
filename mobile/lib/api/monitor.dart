import 'dart:convert';

import '../core/api_client.dart';
import '../core/config.dart';

part 'monitor_agent_api.dart';
part 'monitor_models.dart';

class DiskMetric {
  final String device;
  final int total;
  final int used;
  final double usage;
  DiskMetric({
    required this.device,
    required this.total,
    required this.used,
    required this.usage,
  });
  factory DiskMetric.fromJson(Map<String, dynamic> j) => DiskMetric(
    device: j['device'] as String? ?? '',
    total: (j['total'] as num?)?.toInt() ?? 0,
    used: (j['used'] as num?)?.toInt() ?? 0,
    usage: (j['usage'] as num?)?.toDouble() ?? 0,
  );
}

class TempMetric {
  final String name;
  final double temp;
  TempMetric({required this.name, required this.temp});
  factory TempMetric.fromJson(Map<String, dynamic> j) => TempMetric(
    name: j['name'] as String? ?? '',
    temp: (j['temp'] as num?)?.toDouble() ?? 0,
  );
}

class DeviceInfo {
  final String manufacturer;
  final String model;
  final String firmware;
  final String serial;
  DeviceInfo({
    required this.manufacturer,
    required this.model,
    required this.firmware,
    required this.serial,
  });
  factory DeviceInfo.fromJson(Map<String, dynamic> j) => DeviceInfo(
    manufacturer: j['manufacturer'] as String? ?? '',
    model: j['model'] as String? ?? '',
    firmware: j['firmware'] as String? ?? '',
    serial: j['serial'] as String? ?? '',
  );
}

/// 采集指标（host/nas 共用；camera 主要填 device_info）。
class Metrics {
  final bool online;
  final double? cpuUsage;
  final int? cpuCores;
  final int? memTotal;
  final int? memUsed;
  final double? memUsage;
  final List<DiskMetric> disks;
  final double? netRxRate;
  final double? netTxRate;
  final double? load1;
  final int? uptime;
  final List<TempMetric> temps;
  final DeviceInfo? deviceInfo;

  Metrics({
    required this.online,
    this.cpuUsage,
    this.cpuCores,
    this.memTotal,
    this.memUsed,
    this.memUsage,
    required this.disks,
    this.netRxRate,
    this.netTxRate,
    this.load1,
    this.uptime,
    required this.temps,
    this.deviceInfo,
  });

  factory Metrics.fromJson(Map<String, dynamic> j) => Metrics(
    online: j['online'] as bool? ?? false,
    cpuUsage: (j['cpu_usage'] as num?)?.toDouble(),
    cpuCores: (j['cpu_cores'] as num?)?.toInt(),
    memTotal: (j['mem_total'] as num?)?.toInt(),
    memUsed: (j['mem_used'] as num?)?.toInt(),
    memUsage: (j['mem_usage'] as num?)?.toDouble(),
    disks: ((j['disks'] as List?) ?? const [])
        .map((e) => DiskMetric.fromJson(e as Map<String, dynamic>))
        .toList(),
    netRxRate: (j['net_rx_rate'] as num?)?.toDouble(),
    netTxRate: (j['net_tx_rate'] as num?)?.toDouble(),
    load1: (j['load1'] as num?)?.toDouble(),
    uptime: (j['uptime'] as num?)?.toInt(),
    temps: ((j['temps'] as List?) ?? const [])
        .map((e) => TempMetric.fromJson(e as Map<String, dynamic>))
        .toList(),
    deviceInfo: j['device_info'] is Map<String, dynamic>
        ? DeviceInfo.fromJson(j['device_info'] as Map<String, dynamic>)
        : null,
  );
}

/// 历史快照（metrics 是 JSON 字符串，需二次解析）。
class MetricSnapshot {
  final String snapAt;
  final bool online;
  final Metrics? metrics;
  MetricSnapshot({
    required this.snapAt,
    required this.online,
    required this.metrics,
  });
  factory MetricSnapshot.fromJson(Map<String, dynamic> j) {
    Metrics? m;
    final raw = j['metrics'];
    if (raw is String && raw.isNotEmpty) {
      try {
        m = Metrics.fromJson(jsonDecode(raw) as Map<String, dynamic>);
      } catch (_) {}
    } else if (raw is Map<String, dynamic>) {
      m = Metrics.fromJson(raw);
    }
    return MetricSnapshot(
      snapAt: j['snap_at'] as String? ?? '',
      online: j['online'] as bool? ?? false,
      metrics: m,
    );
  }
}

/// 一页历史快照 + 取数账面：列表有单页上限，total/truncated 不同行给出就会被当成全部历史。
class MetricsPage {
  final List<MetricSnapshot> snapshots;

  /// 该设备的采集总数；后端计数读不出时为 null（界面说「可能还有更多」，不写 0）。
  final int? total;

  /// 服务端生效的 limit（请求值被归一后的真值）。
  final int limit;
  final bool truncated;
  const MetricsPage({
    required this.snapshots,
    this.total,
    this.limit = 0,
    this.truncated = false,
  });
}

class CollectResult {
  final bool ok;
  final Metrics? metrics;
  final String? error;
  CollectResult({required this.ok, this.metrics, this.error});
  factory CollectResult.fromJson(Map<String, dynamic> j) => CollectResult(
    ok: j['ok'] as bool? ?? false,
    metrics: j['metrics'] is Map<String, dynamic>
        ? Metrics.fromJson(j['metrics'] as Map<String, dynamic>)
        : null,
    error: j['error'] as String?,
  );
}

class ProbeResult {
  final bool ok;
  final bool? online;
  final String? error;
  ProbeResult({required this.ok, this.online, this.error});
  factory ProbeResult.fromJson(Map<String, dynamic> j) => ProbeResult(
    ok: j['ok'] as bool? ?? false,
    online: j['online'] as bool?,
    error: j['error'] as String?,
  );
}

class PowerResult {
  final bool ok;
  final String? error;
  PowerResult({required this.ok, this.error});
  factory PowerResult.fromJson(Map<String, dynamic> j) =>
      PowerResult(ok: j['ok'] as bool? ?? false, error: j['error'] as String?);
}

class MonitorAlert {
  final int id;
  final int targetId;
  final String level; // info | warning | danger
  final String metric;
  final String value;
  final String message;
  final bool read;
  final String createdAt;
  final String? resolvedAt; // 非空=已恢复
  MonitorAlert({
    required this.id,
    required this.targetId,
    required this.level,
    required this.metric,
    required this.value,
    required this.message,
    required this.read,
    required this.createdAt,
    required this.resolvedAt,
  });
  factory MonitorAlert.fromJson(Map<String, dynamic> j) => MonitorAlert(
    id: (j['id'] as num?)?.toInt() ?? 0,
    targetId: (j['target_id'] as num?)?.toInt() ?? 0,
    level: j['level'] as String? ?? 'info',
    metric: j['metric'] as String? ?? '',
    value: j['value'] as String? ?? '',
    message: j['message'] as String? ?? '',
    read: j['read'] as bool? ?? false,
    createdAt: j['created_at'] as String? ?? '',
    resolvedAt: j['resolved_at'] as String?,
  );
}

/// 监控 API：设备 CRUD / 探活 / 采集 / 历史 / 告警。对齐后端 /monitors、/monitor/alerts。
class MonitorApi {
  final _api = ApiClient();

  Future<List<MonitorTarget>> list() async {
    final r = await _api.get('/monitors');
    final arr = (r['targets'] as List?) ?? const [];
    return arr
        .map((e) => MonitorTarget.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<MonitorTarget> create(TargetInput in_) async {
    final r = await _api.post('/monitors', in_.toJson());
    return MonitorTarget.fromJson(r);
  }

  Future<MonitorTarget> update(int id, TargetInput in_) async {
    final r = await _api.put('/monitors/$id', in_.toJson());
    return MonitorTarget.fromJson(r);
  }

  Future<void> remove(int id) async => _api.delete('/monitors/$id');

  Future<ProbeResult> probe(int id) async {
    final r = await _api.post('/monitors/$id/probe', {});
    return ProbeResult.fromJson(r);
  }

  Future<CollectResult> collect(int id) async {
    final r = await _api.post('/monitors/$id/collect', {});
    return CollectResult.fromJson(r);
  }

  Future<PowerResult> power(int id, String action) async {
    final r = await _api.post('/monitors/$id/power', {'action': action});
    return PowerResult.fromJson(r);
  }

  Future<MetricsPage> metrics(int id, {int limit = 120}) async {
    final r = await _api.get('/monitors/$id/metrics?limit=$limit');
    final arr = (r['snapshots'] as List?) ?? const [];
    return MetricsPage(
      snapshots: arr
          .map((e) => MetricSnapshot.fromJson(e as Map<String, dynamic>))
          .toList(),
      // 后端没给 truncated 时按「可能被截断」处理：宁可多说一句，也不把窗口当全量。
      truncated: r['truncated'] as bool? ?? true,
      total: (r['total'] as num?)?.toInt(),
      limit: (r['limit'] as num?)?.toInt() ?? limit,
    );
  }

  Future<List<MonitorAlert>> alerts({
    int? targetId,
    bool unread = false,
  }) async {
    final q = StringBuffer(
      '/monitor/alerts?unread=${unread ? 'true' : 'false'}',
    );
    if (targetId != null) q.write('&target_id=$targetId');
    final r = await _api.get(q.toString());
    final arr = (r['alerts'] as List?) ?? const [];
    return arr
        .map((e) => MonitorAlert.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> readAlert(int id) async =>
      _api.post('/monitor/alerts/$id/read', {});

  /// 摄像头 HLS 播放地址（标准协议，media_kit 可播；token 走 query，与 stream 同构）。
  String streamURL(int id) {
    final token = _api.token ?? '';
    return '${Config.baseUrl}/api/v1/monitor/cameras/$id/hls/playlist.m3u8?token=${Uri.encodeComponent(token)}';
  }
}
