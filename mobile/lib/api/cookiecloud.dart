import '../core/api_client.dart';

/// CookieCloud 配置（脱敏回显，对齐后端 /cookiecloud/config）。
class CookieCloudConfig {
  final bool enabled;
  final String server;
  final String uuid;
  final bool passwordConfigured;
  final int intervalMin;
  final int lastSyncUnix;
  final String lastError;

  CookieCloudConfig({
    required this.enabled,
    required this.server,
    required this.uuid,
    required this.passwordConfigured,
    required this.intervalMin,
    required this.lastSyncUnix,
    required this.lastError,
  });

  factory CookieCloudConfig.fromJson(Map<String, dynamic> j) => CookieCloudConfig(
        enabled: j['enabled'] as bool? ?? false,
        server: j['server'] as String? ?? '',
        uuid: j['uuid'] as String? ?? '',
        passwordConfigured: j['password_configured'] as bool? ?? false,
        intervalMin: j['interval_min'] as int? ?? 120,
        lastSyncUnix: j['last_sync_unix'] as int? ?? 0,
        lastError: j['last_error'] as String? ?? '',
      );
}

/// CookieCloud 一次同步结果汇总。
class CookieCloudSyncReport {
  final int updated;
  final int added;
  final int skipped;

  CookieCloudSyncReport({
    required this.updated,
    required this.added,
    required this.skipped,
  });

  factory CookieCloudSyncReport.fromJson(Map<String, dynamic> j) => CookieCloudSyncReport(
        updated: j['updated'] as int? ?? 0,
        added: j['added'] as int? ?? 0,
        skipped: j['skipped'] as int? ?? 0,
      );
}

/// CookieCloud API（对齐后端 /cookiecloud/*）。
class CookieCloudApi {
  final _api = ApiClient();

  Future<CookieCloudConfig> get() async {
    final r = await _api.get('/cookiecloud/config');
    return CookieCloudConfig.fromJson(r);
  }

  /// password 留空（不带 key）= 保留旧密码。
  Future<void> update(Map<String, dynamic> body) => _api.put('/cookiecloud/config', body);

  /// 立即强制同步一次，返回结果汇总。
  Future<CookieCloudSyncReport> sync() async {
    final r = await _api.post('/cookiecloud/sync', {});
    return CookieCloudSyncReport.fromJson(
        (r['report'] as Map<String, dynamic>?) ?? const {});
  }
}
