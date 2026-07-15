import 'package:ptpatronus/core/api_client.dart';

/// 在线更新检查结果（对应后端 GET /system/update/check 响应）。
class UpdateCheckResult {
  final bool hasUpdate;
  final String currentVersion;
  final String latestVersion;
  final String releaseName;
  final String releaseNotes;
  final String downloadUrl; // APK 直链走应用内 OTA，否则外链兜底
  final String sha256; // APK/包 sha256，应用内下载后校验（防篡改）
  final String instructions; // 更新命令（docker 形态，客户端一般用不到）
  final bool isForced; // 强制更新（对话框不可忽略）
  final bool isBelowMin; // 当前版本低于 minVersion（真·强制）

  const UpdateCheckResult({
    this.hasUpdate = false,
    this.currentVersion = '',
    this.latestVersion = '',
    this.releaseName = '',
    this.releaseNotes = '',
    this.downloadUrl = '',
    this.sha256 = '',
    this.instructions = '',
    this.isForced = false,
    this.isBelowMin = false,
  });

  factory UpdateCheckResult.fromJson(Map<String, dynamic> j) => UpdateCheckResult(
        hasUpdate: j['hasUpdate'] ?? false,
        currentVersion: (j['currentVersion'] ?? '').toString(),
        latestVersion: (j['latestVersion'] ?? '').toString(),
        releaseName: (j['releaseName'] ?? '').toString(),
        releaseNotes: (j['releaseNotes'] ?? '').toString(),
        downloadUrl: (j['downloadUrl'] ?? '').toString(),
        sha256: (j['sha256'] ?? '').toString(),
        instructions: (j['instructions'] ?? '').toString(),
        isForced: j['isForced'] ?? false,
        isBelowMin: j['isBelowMin'] ?? false,
      );
}

/// 系统级 API：在线更新检查等。客户端经所连后端中转（/system/update/check），
/// 不直连官方源（避墙、不泄露官方清单地址）。
class SystemApi {
  static Future<UpdateCheckResult> checkUpdate({
    String platform = 'android',
    String? version, // 客户端版本；空则后端默认取服务端版本（服务端自检）
  }) async {
    var path =
        '/system/update/check?platform=${Uri.encodeQueryComponent(platform)}';
    if (version != null && version.isNotEmpty) {
      path += '&version=${Uri.encodeQueryComponent(version)}';
    }
    final m = await ApiClient().get(path);
    return UpdateCheckResult.fromJson(m);
  }

  /// 诊断面板：跑健康检查。
  static Future<DoctorReport> runDoctor() async {
    final m = await ApiClient().get('/system/doctor');
    return DoctorReport.fromJson(m);
  }

  /// 尝试修复（MVP 原样跑一遍，Fixable 检查未来扩展）。
  static Future<DoctorReport> fixDoctor() async {
    final m = await ApiClient().post('/system/doctor/fix', {});
    return DoctorReport.fromJson(m);
  }

  /// 本机系统资源快照（管理员）：CPU/内存/磁盘/负载。仪表盘「系统资源」卡用。
  static Future<SystemMetrics> metrics() async {
    final m = await ApiClient().get('/system/metrics');
    return SystemMetrics.fromJson(m);
  }
}

/// 单项诊断发现。
class DoctorFinding {
  final String id;
  final String severity; // info | warn | error
  final String status; // ok | skipped | degraded | failed | fixed
  final String title;
  final String detail;
  final String recommendation;

  const DoctorFinding({
    this.id = '',
    this.severity = 'info',
    this.status = 'ok',
    this.title = '',
    this.detail = '',
    this.recommendation = '',
  });

  factory DoctorFinding.fromJson(Map<String, dynamic> j) => DoctorFinding(
        id: (j['id'] ?? '').toString(),
        severity: (j['severity'] ?? 'info').toString(),
        status: (j['status'] ?? 'ok').toString(),
        title: (j['title'] ?? '').toString(),
        detail: (j['detail'] ?? '').toString(),
        recommendation: (j['recommendation'] ?? '').toString(),
      );
}

/// 诊断报告（对标后端 doctor.Report）。
class DoctorReport {
  final String status; // healthy | degraded | failed
  final List<DoctorFinding> findings;
  final Map<String, dynamic> environment;
  final int total;
  final int info;
  final int warn;
  final int error;

  const DoctorReport({
    this.status = 'healthy',
    this.findings = const [],
    this.environment = const {},
    this.total = 0,
    this.info = 0,
    this.warn = 0,
    this.error = 0,
  });

  factory DoctorReport.fromJson(Map<String, dynamic> j) {
    final s = (j['summary'] ?? const {}) as Map<String, dynamic>;
    final list = (j['findings'] ?? const []) as List;
    return DoctorReport(
      status: (j['status'] ?? 'healthy').toString(),
      findings: list.map((e) => DoctorFinding.fromJson(e as Map<String, dynamic>)).toList(),
      environment: (j['environment'] ?? const {}) as Map<String, dynamic>,
      total: (s['total'] ?? 0) as int,
      info: (s['info'] ?? 0) as int,
      warn: (s['warn'] ?? 0) as int,
      error: (s['error'] ?? 0) as int,
    );
  }
}

/// 单个磁盘分区用量（对齐后端 sysinfo.DiskMetric）。
class DiskMetric {
  final String device;
  final int total;
  final int used;
  final double usage; // %
  final String type; // partition | pool

  const DiskMetric({
    this.device = '',
    this.total = 0,
    this.used = 0,
    this.usage = 0,
    this.type = '',
  });

  factory DiskMetric.fromJson(Map<String, dynamic> j) => DiskMetric(
        device: (j['device'] ?? '').toString(),
        total: (j['total'] as num?)?.toInt() ?? 0,
        used: (j['used'] as num?)?.toInt() ?? 0,
        usage: (j['usage'] as num?)?.toDouble() ?? 0,
        type: (j['type'] ?? '').toString(),
      );
}

/// 本机系统资源快照（对齐后端 sysinfo.Metrics）。集合字段带 omitempty，无数据时为空。
class SystemMetrics {
  final bool online;
  final double cpuUsage; // %
  final int cpuCores;
  final int memTotal; // bytes
  final int memUsed;
  final double memUsage; // %
  final List<DiskMetric> disks;
  final double load1;
  final double load5;
  final double load15;
  final String hostname;
  final String os;

  const SystemMetrics({
    this.online = true,
    this.cpuUsage = 0,
    this.cpuCores = 0,
    this.memTotal = 0,
    this.memUsed = 0,
    this.memUsage = 0,
    this.disks = const [],
    this.load1 = 0,
    this.load5 = 0,
    this.load15 = 0,
    this.hostname = '',
    this.os = '',
  });

  factory SystemMetrics.fromJson(Map<String, dynamic> j) => SystemMetrics(
        online: j['online'] ?? true,
        cpuUsage: (j['cpu_usage'] as num?)?.toDouble() ?? 0,
        cpuCores: (j['cpu_cores'] as num?)?.toInt() ?? 0,
        memTotal: (j['mem_total'] as num?)?.toInt() ?? 0,
        memUsed: (j['mem_used'] as num?)?.toInt() ?? 0,
        memUsage: (j['mem_usage'] as num?)?.toDouble() ?? 0,
        disks: ((j['disks'] as List?) ?? const [])
            .map((e) => DiskMetric.fromJson(e as Map<String, dynamic>))
            .toList(),
        load1: (j['load1'] as num?)?.toDouble() ?? 0,
        load5: (j['load5'] as num?)?.toDouble() ?? 0,
        load15: (j['load15'] as num?)?.toDouble() ?? 0,
        hostname: (j['hostname'] ?? '').toString(),
        os: (j['os'] ?? '').toString(),
      );
}
