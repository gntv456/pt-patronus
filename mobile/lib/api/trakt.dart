import '../core/api_client.dart';

/// Trakt 配置（脱敏回显，对齐后端 /trakt/config）。
class TraktConfig {
  final bool enabled;
  final String clientId;
  final bool clientSecretConfigured;
  final String redirectUri;
  final bool syncMovies;
  final bool syncShows;
  final String resolution;
  final int downloaderId;
  final bool authorized;
  final int lastSyncAt;
  final int lastSyncCount;
  final String lastError;
  final String pendingUserCode;
  final String pendingVerificationUrl;
  final int pendingExpiresAt;

  TraktConfig({
    required this.enabled,
    required this.clientId,
    required this.clientSecretConfigured,
    required this.redirectUri,
    required this.syncMovies,
    required this.syncShows,
    required this.resolution,
    required this.downloaderId,
    required this.authorized,
    required this.lastSyncAt,
    required this.lastSyncCount,
    required this.lastError,
    required this.pendingUserCode,
    required this.pendingVerificationUrl,
    required this.pendingExpiresAt,
  });

  factory TraktConfig.fromJson(Map<String, dynamic> j) => TraktConfig(
        enabled: j['enabled'] as bool? ?? false,
        clientId: j['client_id'] as String? ?? '',
        clientSecretConfigured: j['client_secret_configured'] as bool? ?? false,
        redirectUri: j['redirect_uri'] as String? ?? 'urn:ietf:wg:oauth:2.0:oob',
        syncMovies: j['sync_movies'] as bool? ?? true,
        syncShows: j['sync_shows'] as bool? ?? true,
        resolution: j['resolution'] as String? ?? '',
        downloaderId: j['downloader_id'] as int? ?? 0,
        authorized: j['authorized'] as bool? ?? false,
        lastSyncAt: j['last_sync_at'] as int? ?? 0,
        lastSyncCount: j['last_sync_count'] as int? ?? 0,
        lastError: j['last_error'] as String? ?? '',
        pendingUserCode: j['pending_user_code'] as String? ?? '',
        pendingVerificationUrl: j['pending_verification_url'] as String? ?? '',
        pendingExpiresAt: j['pending_expires_at'] as int? ?? 0,
      );
}

/// Trakt 设备授权码（POST /trakt/device-code）。
class TraktDeviceCode {
  final String userCode;
  final String verificationUrl;
  final int expiresIn;
  final int interval;
  final String deviceUrl;

  TraktDeviceCode({
    required this.userCode,
    required this.verificationUrl,
    required this.expiresIn,
    required this.interval,
    required this.deviceUrl,
  });

  factory TraktDeviceCode.fromJson(Map<String, dynamic> j) => TraktDeviceCode(
        userCode: j['user_code'] as String? ?? '',
        verificationUrl: j['verification_url'] as String? ?? '',
        expiresIn: j['expires_in'] as int? ?? 0,
        interval: j['interval'] as int? ?? 5,
        deviceUrl: j['device_url'] as String? ?? '/api/v1/trakt/device',
      );
}

/// Trakt API（对齐后端 /trakt/*）。
class TraktApi {
  final _api = ApiClient();

  Future<TraktConfig> getConfig() async {
    final r = await _api.get('/trakt/config');
    return TraktConfig.fromJson(r);
  }

  Future<void> setConfig(Map<String, dynamic> body) => _api.put('/trakt/config', body);

  Future<TraktDeviceCode> deviceCode() async {
    final r = await _api.post('/trakt/device-code', {});
    return TraktDeviceCode.fromJson(r);
  }

  /// 轮询设备授权状态。返 authorized +（未完成时）pending。
  Future<bool> deviceStatus() async {
    final r = await _api.get('/trakt/device-status');
    return r['authorized'] as bool? ?? false;
  }

  /// 立即同步，返新增订阅数。
  Future<int> sync() async {
    final r = await _api.post('/trakt/sync', {});
    return r['created'] as int? ?? 0;
  }
}
