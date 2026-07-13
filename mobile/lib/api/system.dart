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
