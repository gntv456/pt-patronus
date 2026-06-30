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
  static Future<UpdateCheckResult> checkUpdate({String platform = 'android'}) async {
    final m = await ApiClient().get('/system/update/check?platform=$platform');
    return UpdateCheckResult.fromJson(m);
  }
}
